// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

var (
	errCSVTypeMismatch     = errors.New("csv type error")
	errCSVStringifyNoCol   = errors.New("csv stringify: columns required")
	errCSVStringifyBadType = errors.New("csv stringify: unsupported input type")
)

// csvFileState is persisted in flowPers per (nodeID, msg.filename) pair. It
// is engaged by the streaming path so a chunk that ends mid-row (because the
// upstream file-in read mid-write) does not get parsed as a truncated
// record. The trailing bytes after the last '\n' are buffered in Residue
// and prepended to the next chunk. Offset records the last clean byte
// position in the source file. Columns persists a header parsed from the
// first chunk so subsequent chunks do not re-consume the first data row.
type csvFileState struct {
	Offset  int64    `json:"offset"`
	Residue string   `json:"residue"`
	Columns []string `json:"columns,omitempty"`
}

// csvStateKey hashes (nodeID, absFilename) so the resulting key is short and
// free of characters that NATS KV rejects. Same scheme as file-in's cursor.
func csvStateKey(nodeID, absFilename string) string {
	h := sha256.Sum256([]byte(nodeID + ":" + absFilename))
	return "_csvstate." + hex.EncodeToString(h[:8])
}

// loadCSVState reconstitutes a csvFileState from the store via a JSON round
// trip. NATS returns map[string]any after json.Unmarshal; in-memory test
// stores return the original struct. Marshalling and unmarshalling handles
// both shapes uniformly.
func loadCSVState(store flow.ContextStore, key string) (csvFileState, error) {
	var s csvFileState
	if store == nil {
		return s, nil
	}
	raw, err := store.Get(key)
	if err != nil || raw == nil {
		return s, err
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return s, fmt.Errorf("marshal state: %w", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("unmarshal state: %w", err)
	}
	return s, nil
}

func saveCSVState(store flow.ContextStore, key string, s csvFileState) error {
	if store == nil {
		return nil
	}
	return store.Set(key, s)
}

// CSVParserNode converts a message property between a CSV string/buffer
// representation and a structured Go value. See specifications/issues/
// PARSER_CSV_NODE.md for the full contract.
type CSVParserNode struct {
	config flow.NodeConfig
	nodes.BaseNode

	property       string
	action         string
	header         bool
	headerOnce     bool
	columns        []string
	delimiter      rune
	quoteChar      rune
	comment        rune // 0 = disabled
	trimSpaces     bool
	skipEmptyLines bool
	forceQuote     bool
	newline        string // "\n" or "\r\n"
	output         string // "rows" or "array"
	cast           bool

	flowPers     flow.ContextStore
	inErrorState bool
	// headerEmitted tracks whether stringify has already written the header
	// in this node's lifetime. With headerOnce=true the header is suppressed
	// on every call after the first. Lifetime-scoped (no NATS persistence);
	// a redeploy starts fresh — matching the typical file-out append pattern
	// where a redeploy usually targets a fresh / rotated file.
	headerEmitted bool
}

// NewCSVParserNode is the NodeFactory for the csv node type.
func NewCSVParserNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &CSVParserNode{config: config}, nil
}

// SetContext implements flow.ContextProvider. Only flowPers is needed —
// per-file streaming state is flow-scoped and survives redeploys.
func (n *CSVParserNode) SetContext(_, _, _, flowPers flow.ContextStore) {
	n.flowPers = flowPers
}

func (n *CSVParserNode) Init() error {
	props := n.config.Properties

	n.property = nodes.StringVal(props, "property", "payload")
	n.action = nodes.StringVal(props, "action", "auto")
	switch n.action {
	case "auto", "parse", "stringify":
	default:
		slog.Warn("csv node: unknown action, falling back to auto",
			"node_id", n.config.ID, "action", n.action)
		n.action = "auto"
	}

	n.header = nodes.BoolVal(props, "header", true)
	n.headerOnce = nodes.BoolVal(props, "headerOnce", false)
	n.columns = parseCSVColumnsProp(props["columns"])

	n.delimiter = firstRune(props["delimiter"], ',')
	n.quoteChar = firstRune(props["quoteChar"], '"')
	// "" is the valid "no comment handling" value — handled as rune 0.
	n.comment = firstRune(props["comment"], 0)

	n.trimSpaces = nodes.BoolVal(props, "trimSpaces", false)
	n.skipEmptyLines = nodes.BoolVal(props, "skipEmptyLines", true)
	n.forceQuote = nodes.BoolVal(props, "forceQuote", false)

	n.newline = nodes.StringVal(props, "newline", "\n")
	if n.newline != "\n" && n.newline != "\r\n" {
		slog.Warn("csv node: invalid newline, falling back to \\n",
			"node_id", n.config.ID, "newline", n.newline)
		n.newline = "\n"
	}

	n.output = nodes.StringVal(props, "output", "rows")
	switch n.output {
	case "rows", "array":
	default:
		slog.Warn("csv node: unknown output mode, falling back to rows",
			"node_id", n.config.ID, "output", n.output)
		n.output = "rows"
	}

	n.cast = nodes.BoolVal(props, "cast", false)

	// Phase 1 only supports '"' for quoting on stringify. encoding/csv has no
	// per-quote-char setting, and a hand-rolled writer for custom quote chars
	// is out of scope. Warn rather than reject so a stale config keeps working.
	if n.quoteChar != '"' {
		slog.Warn("csv node: custom quoteChar not supported in phase 1; using \"",
			"node_id", n.config.ID, "configured", string(n.quoteChar))
		n.quoteChar = '"'
	}

	return nil
}

func (n *CSVParserNode) Start() error {
	// Clear lifetime-scoped headerOnce state so a redeploy emits the header
	// again on the first stringify of the new run.
	n.headerEmitted = false

	slog.Info("csv parser node started",
		"node_id", n.config.ID,
		"property", n.property,
		"action", n.action,
		"header", n.header,
		"headerOnce", n.headerOnce,
		"output", n.output,
		"delimiter", string(n.delimiter),
	)
	return nil
}

func (n *CSVParserNode) Stop() error {
	slog.Info("csv parser node stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage runs the configured action against msg.<property>.
//
// The streaming path is engaged only when BOTH msg.filename AND
// msg.position are set — that pair is set exclusively by file-read in
// incremental mode. A whole-file read (no msg.position) or any other
// node that happens to carry msg.filename (HTTP upload metadata, watch
// events, manual injection) takes the one-shot path so the header is
// consumed normally and no stale per-file state is loaded.
func (n *CSVParserNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	filename, _ := msg.Get("filename").(string)
	value := msg.Get(n.property)

	stream := filename != "" && n.action != "stringify" &&
		hasNumericField(msg, "position") &&
		(n.action == "parse" || isStringish(value))

	if stream {
		return n.handleStreaming(msg, filename, value)
	}
	return n.handleOneShot(msg, value)
}

// hasNumericField reports whether msg carries a numeric value under the
// given key — used to distinguish incremental file-read output (which sets
// position) from one-shot reads / unrelated nodes that happen to carry the
// same metadata field. Returns false for nil and for non-numeric types.
func hasNumericField(msg *flow.Message, key string) bool {
	switch msg.Get(key).(type) {
	case int64, int, int32, float64, float32:
		return true
	}
	return false
}

// handleOneShot is the non-streaming path: one input message, one output
// (or N fan-out messages in output=rows mode). Used when no msg.filename is
// present or when the action is stringify.
func (n *CSVParserNode) handleOneShot(msg *flow.Message, value any) ([][]*flow.Message, error) {
	var (
		stringified string
		rows        []map[string]any
		arrRows     [][]any
		cols        []string
		isStringify bool
		err         error
	)

	switch n.action {
	case "parse":
		rows, arrRows, cols, err = n.parseCSV(value, nil)
	case "stringify":
		stringified, err = n.stringifyCSV(value)
		isStringify = true
	case "auto":
		if isStringish(value) {
			rows, arrRows, cols, err = n.parseCSV(value, nil)
		} else {
			stringified, err = n.stringifyCSV(value)
			isStringify = true
		}
	}
	if err != nil {
		return n.fail(err)
	}

	n.clearError()

	if isStringify {
		msg.Set(n.property, stringified)
		return [][]*flow.Message{{msg}}, nil
	}

	if n.output == "rows" {
		return n.buildFanOut(msg, rows, arrRows, cols, -1, "")
	}

	// output=array — always emit one message, even when the result is empty.
	if rows != nil {
		msg.Set(n.property, anySliceFromMaps(rows))
	} else if arrRows != nil {
		msg.Set(n.property, anySliceFromSlices(arrRows))
	} else {
		msg.Set(n.property, []any{})
	}
	if len(cols) > 0 {
		msg.Set("columns", cols)
	}
	return [][]*flow.Message{{msg}}, nil
}

// handleStreaming carries out the partial-row-safe ingestion described in
// section 11 of PARSER_CSV_NODE.md.
//
//  1. Load per-file state.
//  2. Prepend stored residue to the incoming bytes.
//  3. Locate the last '\n'; bytes after it are a (possibly incomplete) row
//     and get buffered as the new residue.
//  4. Parse the complete prefix. When a header has been seen before, the
//     persisted column list short-circuits header consumption.
//  5. Persist the new state.
//  6. Emit rows or a single array message; both carry csvPosition (the
//     adjusted clean offset) and msg.filename.
func (n *CSVParserNode) handleStreaming(msg *flow.Message, filename string, value any) ([][]*flow.Message, error) {
	stateKey := csvStateKey(n.config.ID, filename)

	state, err := loadCSVState(n.flowPers, stateKey)
	if err != nil {
		return n.fail(fmt.Errorf("load state: %w", err))
	}

	// file-in signals truncation/rotation via msg.reset; discard stale
	// residue and any cached header before processing the new chunk.
	if reset, _ := msg.Get("reset").(bool); reset {
		state = csvFileState{}
	}

	incoming := toBytes(value)
	combined := append([]byte(state.Residue), incoming...)

	lastNL := bytes.LastIndexByte(combined, '\n')
	if lastNL < 0 {
		// Whole chunk is partial — buffer it and emit nothing.
		state.Residue = string(combined)
		if err := saveCSVState(n.flowPers, stateKey, state); err != nil {
			return n.fail(fmt.Errorf("save state: %w", err))
		}
		n.clearError()
		return nil, nil
	}

	complete := combined[:lastNL+1]
	newResidue := combined[lastNL+1:]

	msgPos := readInt64(msg.Get("position"))
	adjustedOffset := msgPos - int64(len(newResidue))

	var knownCols []string
	if n.header && len(state.Columns) > 0 {
		knownCols = state.Columns
	}

	rows, arrRows, cols, err := n.parseCSV(complete, knownCols)
	if err != nil {
		return n.fail(err)
	}

	newState := csvFileState{
		Offset:  adjustedOffset,
		Residue: string(newResidue),
	}
	if n.header {
		switch {
		case len(state.Columns) > 0:
			newState.Columns = state.Columns
		case len(cols) > 0:
			newState.Columns = cols
		}
	}
	if err := saveCSVState(n.flowPers, stateKey, newState); err != nil {
		return n.fail(fmt.Errorf("save state: %w", err))
	}

	n.clearError()

	effectiveCols := cols
	if len(effectiveCols) == 0 {
		effectiveCols = newState.Columns
	}

	if n.output == "rows" {
		return n.buildFanOut(msg, rows, arrRows, effectiveCols, adjustedOffset, filename)
	}

	// output=array — single message; still set csvPosition/filename so
	// downstream observers can track progress.
	if rows != nil {
		msg.Set(n.property, anySliceFromMaps(rows))
	} else if arrRows != nil {
		msg.Set(n.property, anySliceFromSlices(arrRows))
	} else {
		msg.Set(n.property, []any{})
	}
	if len(effectiveCols) > 0 {
		msg.Set("columns", effectiveCols)
	}
	msg.Set("csvPosition", adjustedOffset)
	msg.Set("filename", filename)
	return [][]*flow.Message{{msg}}, nil
}

// buildFanOut converts a parsed slice of rows into N outgoing messages — one
// per row — preserving the upstream msg fields and tagging each clone with
// isFirst/isLast/index/total. When csvPos >= 0 or filename != "" (streaming
// path), those fields are added too.
//
// An empty rows slice yields no output messages, mirroring the file-in
// "no new bytes" convention and the Split node's behavior on an empty source.
func (n *CSVParserNode) buildFanOut(
	msg *flow.Message,
	rows []map[string]any,
	arrRows [][]any,
	cols []string,
	csvPos int64,
	filename string,
) ([][]*flow.Message, error) {
	total := len(rows)
	if total == 0 {
		total = len(arrRows)
	}
	if total == 0 {
		return nil, nil
	}

	out := make([]*flow.Message, total)
	for i := 0; i < total; i++ {
		c := msg.Clone()
		if rows != nil {
			c.Set(n.property, rows[i])
		} else {
			c.Set(n.property, arrRows[i])
		}
		c.Set("isFirst", i == 0)
		c.Set("isLast", i == total-1)
		c.Set("index", i)
		c.Set("total", total)
		if len(cols) > 0 {
			c.Set("columns", cols)
		}
		if csvPos >= 0 {
			c.Set("csvPosition", csvPos)
		}
		if filename != "" {
			c.Set("filename", filename)
		}
		out[i] = c
	}
	return [][]*flow.Message{out}, nil
}

// parseCSV decodes a CSV value to structured rows. knownCols supplies
// pre-resolved column names (typically from streaming state) and suppresses
// header consumption; pass nil for a fresh parse.
//
// When knownCols is set and n.header is true, the returned cols is nil so
// the caller knows to keep using its persisted copy (no need to re-emit the
// header on every chunk).
func (n *CSVParserNode) parseCSV(v any, knownCols []string) (
	rows []map[string]any, arrRows [][]any, cols []string, err error,
) {
	data := toBytes(v)
	if data == nil {
		if v == nil {
			return nil, nil, nil, fmt.Errorf("parse: value is nil: %w", errCSVTypeMismatch)
		}
		return nil, nil, nil, fmt.Errorf("parse: expected string or []byte, got %T: %w", v, errCSVTypeMismatch)
	}
	// UTF-8 BOM strip — irrelevant for ASCII delimiters but harmless to do
	// unconditionally and matches the spec.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = n.delimiter
	r.LazyQuotes = false
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = n.trimSpaces
	if n.comment != 0 {
		r.Comment = n.comment
	}

	// Resolve effective columns and whether the first record of this read
	// should be consumed as a header.
	//
	// - knownCols set (streaming, header already seen): use them, don't
	//   re-consume any header row.
	// - n.columns set + n.header=true: use config columns as keys but still
	//   skip the file's header row (typical case: file carries a header but
	//   the user wants to rename the columns or pin the order).
	// - n.columns set + n.header=false: use config columns; emit every row.
	// - n.columns empty + n.header=true: derive columns from the first row.
	// - n.columns empty + n.header=false: positional rows ([]any).
	var (
		effectiveCols  []string
		headerConsumed bool
		useConfigCols  bool
	)
	switch {
	case len(knownCols) > 0:
		effectiveCols = knownCols
		headerConsumed = true
	case len(n.columns) > 0:
		effectiveCols = n.columns
		useConfigCols = true
		// headerConsumed stays false when n.header=true so the loop still
		// drops the file's header row.
		if !n.header {
			headerConsumed = true
		}
	}

	for {
		rec, rerr := r.Read()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return nil, nil, nil, fmt.Errorf("parse: %w", rerr)
		}

		if n.skipEmptyLines && isEmptyRecord(rec) {
			continue
		}

		if n.header && !headerConsumed {
			if !useConfigCols {
				effectiveCols = make([]string, len(rec))
				for i, s := range rec {
					if n.trimSpaces {
						s = strings.TrimRight(s, " \t")
					}
					effectiveCols[i] = s
				}
			}
			headerConsumed = true
			continue
		}

		// encoding/csv strips leading whitespace only — apply a trailing trim
		// here so trimSpaces honors both ends.
		if n.trimSpaces {
			for i, s := range rec {
				rec[i] = strings.TrimRight(s, " \t")
			}
		}

		if len(effectiveCols) > 0 {
			row := make(map[string]any, len(effectiveCols))
			for i, col := range effectiveCols {
				if i < len(rec) {
					if n.cast {
						row[col] = castCell(rec[i])
					} else {
						row[col] = rec[i]
					}
				} else if n.cast {
					row[col] = nil
				} else {
					row[col] = ""
				}
			}
			rows = append(rows, row)
		} else {
			row := make([]any, len(rec))
			for i, s := range rec {
				if n.cast {
					row[i] = castCell(s)
				} else {
					row[i] = s
				}
			}
			arrRows = append(arrRows, row)
		}
	}

	if len(knownCols) > 0 {
		// Caller already knows the columns; signal that by returning nil
		// so they keep their persisted copy untouched.
		return rows, arrRows, nil, nil
	}
	return rows, arrRows, effectiveCols, nil
}

// csvWriteOpts bundles the CSV serialization options that are independent
// of which node owns the call. Used by both CSVParserNode.stringifyCSV and
// the csv-out node, so the wire-format logic has a single implementation.
type csvWriteOpts struct {
	Columns    []string // configured column order; empty = derive
	Header     bool     // whether a header row may be emitted (caller still gates with WithHeader)
	Delimiter  rune
	QuoteChar  rune
	ForceQuote bool
	Newline    string
}

// stringifyCSVRaw is the pure serializer. It writes the header iff
// withHeader is true AND the resolved column list is non-empty. The caller
// owns the high-level "should I emit the header now?" decision (lifetime
// flag, on-disk state, etc.); this function only does the wire-format work.
//
// Returns the serialized bytes, the resolved column list, and a flag
// indicating whether the header row was actually written so the caller
// can latch a once-only flag correctly.
func stringifyCSVRaw(v any, opts csvWriteOpts, withHeader bool) ([]byte, []string, bool, error) {
	switch v.(type) {
	case string:
		return nil, nil, false, fmt.Errorf("stringify: value already a string: %w", errCSVTypeMismatch)
	case []byte:
		return nil, nil, false, fmt.Errorf("stringify: value is a byte buffer: %w", errCSVTypeMismatch)
	}

	objRows, arrRows, isObj, err := normalizeStringifyInput(v)
	if err != nil {
		return nil, nil, false, err
	}

	var (
		cols      []string
		writeHead bool
	)
	if isObj {
		switch {
		case len(opts.Columns) > 0:
			cols = opts.Columns
			writeHead = opts.Header
		case opts.Header:
			cols = deriveColumns(objRows)
			writeHead = true
		default:
			return nil, nil, false, fmt.Errorf("stringify: %w", errCSVStringifyNoCol)
		}
	} else {
		// Positional rows: header is only emitted when columns are configured.
		if opts.Header && len(opts.Columns) > 0 {
			cols = opts.Columns
			writeHead = true
		}
	}

	// Caller gate (lifetime flag / file-state check).
	if writeHead && !withHeader {
		writeHead = false
	}

	var buf strings.Builder
	emittedHeader := false
	if writeHead && len(cols) > 0 {
		writeCSVRow(&buf, cols, opts.Delimiter, opts.QuoteChar, opts.ForceQuote, opts.Newline)
		emittedHeader = true
	}
	if isObj {
		for _, r := range objRows {
			cells := make([]string, len(cols))
			for i, c := range cols {
				cells[i] = formatCell(r[c])
			}
			writeCSVRow(&buf, cells, opts.Delimiter, opts.QuoteChar, opts.ForceQuote, opts.Newline)
		}
	} else {
		for _, r := range arrRows {
			cells := make([]string, len(r))
			for i, v := range r {
				cells[i] = formatCell(v)
			}
			writeCSVRow(&buf, cells, opts.Delimiter, opts.QuoteChar, opts.ForceQuote, opts.Newline)
		}
	}
	return []byte(buf.String()), cols, emittedHeader, nil
}

// stringifyCSV serialises a value to a CSV string using the parser node's
// configured options. Honors headerOnce (lifetime-scoped header gate).
func (n *CSVParserNode) stringifyCSV(v any) (string, error) {
	withHeader := !(n.headerOnce && n.headerEmitted)

	opts := csvWriteOpts{
		Columns:    n.columns,
		Header:     n.header,
		Delimiter:  n.delimiter,
		QuoteChar:  n.quoteChar,
		ForceQuote: n.forceQuote,
		Newline:    n.newline,
	}
	data, _, emitted, err := stringifyCSVRaw(v, opts, withHeader)
	if err != nil {
		return "", err
	}
	if emitted {
		n.headerEmitted = true
	}
	return string(data), nil
}

// normalizeStringifyInput inspects v and returns it as either a []map[string]any
// or a [][]any depending on the shape. The third return distinguishes the two
// branches; the fourth carries an error for unsupported shapes.
//
// A single map[string]any / map[string]string is accepted as a single-row
// object input — this is the typical shape after a JSON-parsed MQTT or HTTP
// payload that carries one record per message.
func normalizeStringifyInput(v any) ([]map[string]any, [][]any, bool, error) {
	switch x := v.(type) {
	case map[string]any:
		return []map[string]any{x}, nil, true, nil

	case map[string]string:
		obj := make(map[string]any, len(x))
		for k, v := range x {
			obj[k] = v
		}
		return []map[string]any{obj}, nil, true, nil

	case []map[string]any:
		return x, nil, true, nil

	case []map[string]string:
		out := make([]map[string]any, len(x))
		for i, m := range x {
			obj := make(map[string]any, len(m))
			for k, v := range m {
				obj[k] = v
			}
			out[i] = obj
		}
		return out, nil, true, nil

	case [][]any:
		return nil, x, false, nil

	case [][]string:
		out := make([][]any, len(x))
		for i, r := range x {
			row := make([]any, len(r))
			for j, s := range r {
				row[j] = s
			}
			out[i] = row
		}
		return nil, out, false, nil

	case []any:
		if len(x) == 0 {
			// Empty slice — pick the object branch by default; an empty CSV
			// output will be emitted (just the header if header=true).
			return []map[string]any{}, nil, true, nil
		}
		// Inspect the first element to choose the branch.
		switch x[0].(type) {
		case map[string]any, map[string]string:
			out := make([]map[string]any, len(x))
			for i, e := range x {
				switch m := e.(type) {
				case map[string]any:
					out[i] = m
				case map[string]string:
					obj := make(map[string]any, len(m))
					for k, v := range m {
						obj[k] = v
					}
					out[i] = obj
				default:
					return nil, nil, false, fmt.Errorf("stringify: heterogeneous []any: %w", errCSVStringifyBadType)
				}
			}
			return out, nil, true, nil
		case []any, []string:
			out := make([][]any, len(x))
			for i, e := range x {
				switch r := e.(type) {
				case []any:
					out[i] = r
				case []string:
					row := make([]any, len(r))
					for j, s := range r {
						row[j] = s
					}
					out[i] = row
				default:
					return nil, nil, false, fmt.Errorf("stringify: heterogeneous []any: %w", errCSVStringifyBadType)
				}
			}
			return nil, out, false, nil
		default:
			return nil, nil, false, fmt.Errorf("stringify: []any of unsupported element type %T: %w", x[0], errCSVStringifyBadType)
		}
	}
	return nil, nil, false, fmt.Errorf("stringify: unsupported input type %T: %w", v, errCSVStringifyBadType)
}

// deriveColumns picks a deterministic column order from the input rows when
// the user did not configure one explicitly. Go map iteration is randomised,
// so honouring "first object's insertion order" is impossible after a JSON
// round-trip — we sort alphabetically instead. Users who need a specific
// column order should set the columns config.
func deriveColumns(rows []map[string]any) []string {
	seen := make(map[string]struct{})
	for _, r := range rows {
		for k := range r {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// writeCSVRow appends one CSV record + line terminator to buf. Uses
// RFC-4180 quoting: a cell is quoted when it contains the delimiter, the
// quote char, CR or LF, or when forceQuote is true. An embedded quote char
// inside a quoted cell is doubled.
func writeCSVRow(buf *strings.Builder, cells []string, delim, quote rune, forceQuote bool, newline string) {
	for i, c := range cells {
		if i > 0 {
			buf.WriteRune(delim)
		}
		needsQuote := forceQuote ||
			strings.ContainsRune(c, delim) ||
			strings.ContainsRune(c, quote) ||
			strings.ContainsAny(c, "\r\n")
		if !needsQuote {
			buf.WriteString(c)
			continue
		}
		buf.WriteRune(quote)
		buf.WriteString(strings.ReplaceAll(c, string(quote), string(quote)+string(quote)))
		buf.WriteRune(quote)
	}
	buf.WriteString(newline)
}

// formatCell renders a cell value as a string for CSV output. nil becomes
// empty; strings pass through; everything else goes through fmt.Sprint.
func formatCell(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// castCell applies the conservative type coercion described in section 7 of
// the spec. The order matters: bool before int, int before float, and an
// empty cell maps to nil rather than to an empty string.
func castCell(s string) any {
	if s == "" {
		return nil
	}
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func isEmptyRecord(rec []string) bool {
	if len(rec) == 0 {
		return true
	}
	return len(rec) == 1 && rec[0] == ""
}

func anySliceFromMaps(rows []map[string]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

func anySliceFromSlices(rows [][]any) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

// toBytes converts a parseable CSV input to bytes. Other types return nil;
// the caller has already gated this with isStringish() in the streaming path.
func toBytes(v any) []byte {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []byte(x)
	case []byte:
		return x
	}
	return nil
}

// readInt64 normalises a message field that may have been stored as int,
// int64, or float64 (post-JSON round-trip in NATS).
func readInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	}
	return 0
}

// firstRune extracts the first rune from a string-typed prop, returning dflt
// on empty or non-string values. Multi-rune strings keep only the first rune
// (multi-character delimiters are out of scope in Phase 1).
func firstRune(v any, dflt rune) rune {
	s, ok := v.(string)
	if !ok || s == "" {
		return dflt
	}
	for _, r := range s {
		return r
	}
	return dflt
}

// parseCSVColumnsProp accepts the columns config field in the three shapes a
// frontend or workspace.json might produce: a comma-separated string, a
// []string, or a []any of strings.
func parseCSVColumnsProp(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		parts := strings.Split(x, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

// fail reports an error to the catch pipeline and updates the status pill.
func (n *CSVParserNode) fail(err error) ([][]*flow.Message, error) {
	if n.Status != nil {
		n.Status("red", n.errorLabel(err))
	}
	n.inErrorState = true
	return nil, fmt.Errorf("csv: %w", err)
}

// clearError resets the status pill when a successful run follows an error.
func (n *CSVParserNode) clearError() {
	if n.inErrorState && n.Status != nil {
		n.Status("", "")
	}
	n.inErrorState = false
}

// errorLabel maps an internal error to the status text shown on the node.
func (n *CSVParserNode) errorLabel(err error) string {
	switch {
	case errors.Is(err, errCSVTypeMismatch):
		return "csv type error"
	case errors.Is(err, errCSVStringifyNoCol):
		return "csv stringify: columns required"
	case errors.Is(err, errCSVStringifyBadType):
		return "csv stringify: unsupported input type"
	default:
		return "csv parse error"
	}
}

// CSVParserTypeInfo returns the NodeTypeInfo for the csv parser node.
func CSVParserTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "csv",
		Category:    "parser",
		Label:       "CSV",
		Description: "Convert CSV strings to objects/arrays and back",
		Icon:        "csv",
		Defaults: map[string]any{
			"property":       "payload",
			"action":         "auto",
			"header":         true,
			"headerOnce":     false,
			"columns":        "",
			"delimiter":      ",",
			"quoteChar":      "\"",
			"comment":        "",
			"trimSpaces":     false,
			"skipEmptyLines": true,
			"forceQuote":     false,
			"newline":        "\n",
			"output":         "rows",
			"cast":           false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
