// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// s7ParserField describes one entry of the S7 parser's byte-block layout.
// Offsets are 0-based on the input byte block (NOT the absolute PLC address)
// so a layout is portable between different DBs / start positions.
//
// For BOOL fields, the dotted-offset notation `"12.3"` lands as `Byte=12,
// Bit=3`. For non-BOOL types, `Bit` stays at 0 (it's only meaningful for the
// bit-extract / bit-pack path).
//
// Length is consumed for `string` (= maxLen, on-wire span = length+2),
// `raw` (= byte count), and char arrays. Other types ignore it.
type s7ParserField struct {
	Byte        int
	Bit         int
	Name        string
	Type        string
	Length      int
	Signed      bool
	Scale       float64 // 0 → treated as 1
	OffsetValue float64
	Unit        string
}

// S7ParserNode parses raw byte blocks into structured objects (parse) or
// builds raw byte blocks from objects (encode). The layout is configured
// declaratively as a list of fields and is shared by both directions.
//
// Action modes mirror modbus-parser:
//   - auto:   array/buffer input → parse, map input → encode
//   - parse:  forces parse, errors on map input
//   - encode: forces encode, errors on array/buffer input
type S7ParserNode struct {
	config flow.NodeConfig
	nodes.BaseNode
	errFn  flow.ErrorFunc

	action        string // "auto" | "parse" | "encode"
	parseFrom     string // msg path for parse input (default "payload")
	encodeFrom    string // msg path for encode input (default "payload")
	blockLength   int    // 0 = derive from layout
	preserveBytes bool   // parse: also forward the raw bytes as msg.bytes
	layout        []s7ParserField

	inErrorState bool
}

// NewS7ParserNode is the NodeFactory for the s7-parser node type.
func NewS7ParserNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &S7ParserNode{config: config}, nil
}

// Init parses and validates the layout. Returns an error if the configuration
// is structurally invalid; overlapping fields produce a slog.Warn but are
// not rejected — bit-mixing on the same byte is a legitimate Siemens pattern.
func (n *S7ParserNode) Init() error {
	props := n.config.Properties

	n.action = nodes.StringVal(props, "action", "auto")
	switch n.action {
	case "auto", "parse", "encode":
	default:
		slog.Warn("s7-parser: unknown action, falling back to auto",
			"node_id", n.config.ID, "action", n.action)
		n.action = "auto"
	}

	n.parseFrom = nodes.StringVal(props, "parseFrom", "payload")
	n.encodeFrom = nodes.StringVal(props, "encodeFrom", "payload")
	n.blockLength = nodes.IntVal(props, "blockLength", 0)
	if v, ok := props["preserveBytes"].(bool); ok {
		n.preserveBytes = v
	}

	rawLayout, ok := props["layout"].([]any)
	if !ok {
		if _, present := props["layout"]; present {
			return fmt.Errorf("s7-parser %s: layout must be an array", n.config.ID)
		}
		return fmt.Errorf("s7-parser %s: layout is empty (define at least one field)", n.config.ID)
	}
	if len(rawLayout) == 0 {
		return fmt.Errorf("s7-parser %s: layout is empty (define at least one field)", n.config.ID)
	}

	seen := make(map[string]struct{}, len(rawLayout))
	n.layout = make([]s7ParserField, 0, len(rawLayout))
	for i, raw := range rawLayout {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("s7-parser %s: layout[%d] is not an object", n.config.ID, i)
		}
		f, err := parseS7LayoutField(entry)
		if err != nil {
			return fmt.Errorf("s7-parser %s: layout[%d]: %w", n.config.ID, i, err)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("s7-parser %s: duplicate field name %q", n.config.ID, f.Name)
		}
		seen[f.Name] = struct{}{}
		n.layout = append(n.layout, f)
	}

	// Validate blockLength against the layout's maximum extent. 0 is a
	// signal to derive the size at parse/encode time from the layout itself.
	if n.blockLength > 0 {
		maxEnd := s7LayoutMaxEnd(n.layout)
		if maxEnd > n.blockLength {
			return fmt.Errorf("s7-parser %s: blockLength %d is smaller than the layout's max extent %d",
				n.config.ID, n.blockLength, maxEnd)
		}
	}

	checkS7LayoutOverlap(n.layout, n.config.ID)

	return nil
}

// parseS7LayoutField turns one wire-format layout entry into an s7ParserField,
// applying defaults and per-field validation. The offset accepts both an
// integer (`12`) for byte-aligned types and a dotted string (`"12.3"`) for
// BOOL bit positions.
func parseS7LayoutField(m map[string]any) (s7ParserField, error) {
	f := s7ParserField{
		Name:   nodes.StringVal(m, "name", ""),
		Type:   strings.ToLower(nodes.StringVal(m, "type", "")),
		Length: nodes.IntVal(m, "length", 0),
		Unit:   nodes.StringVal(m, "unit", ""),
	}
	if v, ok := m["signed"].(bool); ok {
		f.Signed = v
	}
	if v, ok := m["scale"]; ok {
		switch x := v.(type) {
		case float64:
			f.Scale = x
		case int:
			f.Scale = float64(x)
		}
	}
	if v, ok := m["valueOffset"]; ok {
		switch x := v.(type) {
		case float64:
			f.OffsetValue = x
		case int:
			f.OffsetValue = float64(x)
		}
	}
	if f.Scale == 0 {
		f.Scale = 1
	}

	// Offset: int or dotted-string. Required.
	rawOffset, present := m["offset"]
	if !present {
		return f, fmt.Errorf("offset is required")
	}
	byteOff, bitOff, err := parseS7Offset(rawOffset)
	if err != nil {
		return f, err
	}
	f.Byte = byteOff
	f.Bit = bitOff

	// ── Required-field validation ──────────────────────────────────────
	if f.Name == "" {
		return f, fmt.Errorf("name is required")
	}
	switch f.Type {
	case "bool",
		"byte", "char", "sint", "usint",
		"word", "int", "uint", "wchar", "date",
		"dword", "dint", "udint", "real", "time", "tod",
		"lreal", "lint", "ulint", "lword", "ltime", "ltod", "ldt", "dt",
		"dtl",
		"string", "wstring", "raw", "counter", "timer":
	case "":
		return f, fmt.Errorf("field %q: type is required", f.Name)
	default:
		return f, fmt.Errorf("field %q: unknown type %q", f.Name, f.Type)
	}

	// ── Per-type constraints ────────────────────────────────────────────
	if f.Type == "string" {
		if f.Length < 1 || f.Length > 254 {
			return f, fmt.Errorf("field %q: string requires length in [1, 254], got %d", f.Name, f.Length)
		}
	}
	if f.Type == "wstring" {
		if f.Length < 1 || f.Length > 16382 {
			return f, fmt.Errorf("field %q: wstring requires length in [1, 16382], got %d", f.Name, f.Length)
		}
	}
	if f.Type == "raw" && f.Length < 1 {
		return f, fmt.Errorf("field %q: raw requires length ≥ 1", f.Name)
	}
	if f.Bit != 0 && f.Type != "bool" {
		return f, fmt.Errorf("field %q: dotted offset (bit %d) only valid with type=bool, got %s",
			f.Name, f.Bit, f.Type)
	}
	if f.Signed && f.Type != "byte" && f.Type != "word" && f.Type != "dword" {
		return f, fmt.Errorf("field %q: signed flag only meaningful for byte/word/dword (use int/dint for inherently-signed types), got %s",
			f.Name, f.Type)
	}

	return f, nil
}

// parseS7Offset accepts an int (`12`) or a dotted string (`"12.3"`) and
// returns the byte / bit components. The bit position must be in [0, 7] —
// Siemens BOOLs are byte-addressed at the bit level.
func parseS7Offset(v any) (int, int, error) {
	switch x := v.(type) {
	case float64:
		return int(x), 0, nil
	case int:
		return x, 0, nil
	case int64:
		return int(x), 0, nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, 0, fmt.Errorf("offset string is empty")
		}
		dot := strings.IndexByte(s, '.')
		if dot < 0 {
			n, err := strconv.Atoi(s)
			if err != nil {
				return 0, 0, fmt.Errorf("offset %q: %w", s, err)
			}
			return n, 0, nil
		}
		byteStr, bitStr := s[:dot], s[dot+1:]
		byteOff, err := strconv.Atoi(byteStr)
		if err != nil {
			return 0, 0, fmt.Errorf("offset %q: byte part: %w", s, err)
		}
		bitOff, err := strconv.Atoi(bitStr)
		if err != nil {
			return 0, 0, fmt.Errorf("offset %q: bit part: %w", s, err)
		}
		if bitOff < 0 || bitOff > 7 {
			return 0, 0, fmt.Errorf("offset %q: bit %d outside [0, 7]", s, bitOff)
		}
		return byteOff, bitOff, nil
	default:
		return 0, 0, fmt.Errorf("offset must be a number or dotted string, got %T", v)
	}
}

// s7LayoutMaxEnd returns the highest byte index (exclusive) covered by any
// field in the layout. Used by Init for blockLength validation and by
// parse/encode for sparse output sizing.
func s7LayoutMaxEnd(layout []s7ParserField) int {
	maxEnd := 0
	for _, f := range layout {
		end := f.Byte + S7TypeByteSize(f.Type, f.Length)
		// BOOL fields consume only their host byte; if the type-size table
		// returned 1 that's already correct. Sub-byte BOOLs at byte N still
		// occupy byte N + 1 in the output.
		if end > maxEnd {
			maxEnd = end
		}
	}
	return maxEnd
}

// checkS7LayoutOverlap warns about fields that occupy the same byte without
// a bit-mask reason to coexist. BOOLs sharing a host byte are explicitly
// allowed — that's the Siemens status-word pattern.
func checkS7LayoutOverlap(layout []s7ParserField, nodeID string) {
	type slot struct {
		end int // inclusive
		idx int
	}
	occupied := map[int]slot{}
	for i, f := range layout {
		if f.Type == "bool" {
			// BOOLs share host bytes by design; skip overlap reporting.
			continue
		}
		size := S7TypeByteSize(f.Type, f.Length)
		end := f.Byte + size - 1
		for offset := f.Byte; offset <= end; offset++ {
			if existing, ok := occupied[offset]; ok && layout[existing.idx].Type != "bool" {
				slog.Warn("s7-parser: overlapping fields",
					"node_id", nodeID,
					"field_a", layout[existing.idx].Name,
					"field_b", f.Name,
					"byte", offset)
				break
			}
			occupied[offset] = slot{end: end, idx: i}
		}
	}
}

func (n *S7ParserNode) SetError(fn flow.ErrorFunc)     { n.errFn = fn }

// Start clears any leftover status from a previous deploy (matches modbus-parser).
func (n *S7ParserNode) Start() error {
	if n.Status != nil {
		n.Status("", "")
	}
	slog.Info("s7-parser started",
		"node_id", n.config.ID,
		"action", n.action,
		"fields", len(n.layout))
	return nil
}

// Stop is a no-op (no goroutines or open resources).
func (n *S7ParserNode) Stop() error {
	slog.Info("s7-parser stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage dispatches to parse or encode based on the configured action
// and the input value's type. Errors are returned so the engine routes them
// to Catch nodes; the status pill is updated red on failure and cleared on
// the next success.
func (n *S7ParserNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	out, err := n.dispatch(msg)
	if err != nil {
		if n.Status != nil {
			n.Status("red", "parse error")
		}
		n.inErrorState = true
		return nil, fmt.Errorf("s7-parser %s: %w", n.config.ID, err)
	}
	if n.inErrorState && n.Status != nil {
		n.Status("", "")
	}
	n.inErrorState = false
	return [][]*flow.Message{{out}}, nil
}

func (n *S7ParserNode) dispatch(msg *flow.Message) (*flow.Message, error) {
	switch n.action {
	case "parse":
		return n.dispatchParse(msg)
	case "encode":
		return n.dispatchEncode(msg)
	case "auto":
		// In auto mode we look at the encode-side input (typically `payload`).
		// If it's a map, encode. Otherwise treat it as a byte buffer and
		// parse. The two paths share a default property name (`payload`)
		// which is exactly what s7-read block mode emits and what the user
		// typically populates for write-side encoding too.
		encodeInput := msg.Get(n.encodeFrom)
		if nodes.IsMapInput(encodeInput) {
			return n.dispatchEncode(msg)
		}
		return n.dispatchParse(msg)
	default:
		return nil, fmt.Errorf("unknown action %q", n.action)
	}
}

// dispatchParse reads parseFrom, normalises to []byte, parses, and writes
// the resulting object to msg.payload.
func (n *S7ParserNode) dispatchParse(msg *flow.Message) (*flow.Message, error) {
	value := msg.Get(n.parseFrom)
	buf, err := normaliseS7BytesInput(value)
	if err != nil {
		return nil, fmt.Errorf("parse input %q: %w", n.parseFrom, err)
	}
	out, err := n.parse(buf)
	if err != nil {
		return nil, err
	}
	msg.SetPayload(out)
	if n.preserveBytes {
		// Convert to []int so JSON-decoded downstream consumers see the same
		// shape they would after a s7-read block fetch (which uses []byte
		// natively but JSON-marshals identically).
		bytesArr := make([]int, len(buf))
		for i, b := range buf {
			bytesArr[i] = int(b)
		}
		msg.Set("bytes", bytesArr)
	}
	return msg, nil
}

// dispatchEncode reads encodeFrom (must be a map), encodes via the layout,
// and writes the result `[]int` (JSON-friendly view of the wire bytes) to
// msg.payload + mirror to msg.bytes.
//
// Note: layout offsets are buffer-relative — `field.offset = 12` means
// "buffer index 12 holds the field". The caller is responsible for setting
// `start` on the downstream s7-write block to position the buffer at the
// right PLC byte. We deliberately do NOT auto-write `msg.s7.start`: an
// earlier "convenience" that set it to the layout's minOffset broke the
// invariant that encode and parse share offset semantics, and led to the
// buffer being placed at PLC byte `minOffset + field.offset` instead of
// `field.offset`. If you want a non-destructive write of just the field
// range, design your layout with the first field at offset 0 and configure
// `start` on the s7-write to where that range lives in the PLC.
func (n *S7ParserNode) dispatchEncode(msg *flow.Message) (*flow.Message, error) {
	value := msg.Get(n.encodeFrom)
	input, ok := nodes.NormaliseMapInput(value)
	if !ok {
		return nil, fmt.Errorf("encode input %q: expected object/map, got %T", n.encodeFrom, value)
	}
	buf, _, err := n.encode(input)
	if err != nil {
		return nil, err
	}
	// Surface as []int (not []byte) so the debug-panel JSON marshaller renders
	// the bytes as `[222, 173, …]` instead of base64-encoding them. The
	// downstream s7-write block-mode `toByteSlice` accepts []int, []byte, and
	// []any, so wiring is unaffected. Mirrors the s7-read block-mode payload
	// shape so the round-trip is symmetric.
	bytesArr := make([]int, len(buf))
	for i, b := range buf {
		bytesArr[i] = int(b)
	}
	msg.SetPayload(bytesArr)
	msg.Set("bytes", bytesArr)
	return msg, nil
}

// normaliseS7BytesInput accepts the various wire shapes a byte block may
// arrive in and returns a []byte ready for parse(). Native []byte from
// s7-read block mode passes through directly; JSON-decoded numeric arrays
// are coerced.
func normaliseS7BytesInput(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("value is nil")
	case []byte:
		return x, nil
	case []int:
		out := make([]byte, len(x))
		for i, n := range x {
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("element %d (%d) out of byte range [0, 255]", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	case []any:
		out := make([]byte, len(x))
		for i, e := range x {
			n, err := nodes.ToInt64(e)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("element %d (%d) out of byte range [0, 255]", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected byte array, got %T", v)
	}
}

// parse decodes a byte block into a flat object keyed by field name. Errors
// mention the failing field by name so users can correct their layout
// without guessing.
func (n *S7ParserNode) parse(buf []byte) (map[string]any, error) {
	out := make(map[string]any, len(n.layout))
	for _, f := range n.layout {
		size := S7TypeByteSize(f.Type, f.Length)
		// BOOLs need at least byte+1 bytes (the host byte itself).
		if f.Type == "bool" {
			if f.Byte >= len(buf) {
				return nil, fmt.Errorf("field %q: BOOL byte %d > input length %d", f.Name, f.Byte, len(buf))
			}
			b, err := DecodeS7Bit(buf[f.Byte], f.Bit)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", f.Name, err)
			}
			out[f.Name] = b
			continue
		}
		if f.Byte+size > len(buf) {
			return nil, fmt.Errorf("field %q: needs %d byte(s) at offset %d but input only has %d",
				f.Name, size, f.Byte, len(buf))
		}
		slice := buf[f.Byte : f.Byte+size]

		var (
			value any
			err   error
		)
		switch f.Type {
		case "string":
			value, err = DecodeS7String(slice)
		case "wstring":
			value, err = DecodeS7WString(slice)
		case "raw":
			out := make([]byte, len(slice))
			copy(out, slice)
			value = out
		default:
			value, err = DecodeS7Scalar(f.Type, f.Signed, slice)
		}
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}

		// Scale only applies to numeric scalars. String- and time-typed fields
		// (string/wstring/raw/date/dt/ldt/dtl/wchar) pass through unchanged
		// since their wire→Go form is text, not a number. Counter/Timer are
		// integer ticks/ms; users that want unit conversion can chain a
		// Function node.
		if !s7IsNonScalable(f.Type) {
			value = nodes.ApplyScale(value, f.Scale, f.OffsetValue)
		}
		out[f.Name] = value
	}
	return out, nil
}

// encode builds a byte block from a structured input. Fields not present in
// the input are left at zero (sparse-zero semantics, see PARSER_S7_NODE.md).
//
// BOOL fields with the same Byte are OR-aggregated in a pre-pass so multiple
// status bits can share one host byte without overwriting each other.
//
// The returned slice is sized to `blockLength` if set, else to the highest
// field's end. The minimum offset is reported so the caller can attach
// msg.s7.start for direct wiring into s7-write block mode.
func (n *S7ParserNode) encode(input map[string]any) ([]byte, int, error) {
	if len(input) == 0 {
		return nil, 0, fmt.Errorf("encode: input object is empty")
	}

	// Pass 1: find block size and minimum offset.
	maxEnd := s7LayoutMaxEnd(n.layout)
	minOffset := -1
	for _, f := range n.layout {
		if minOffset < 0 || f.Byte < minOffset {
			minOffset = f.Byte
		}
	}
	size := maxEnd
	if n.blockLength > 0 {
		size = n.blockLength
	}
	buf := make([]byte, size)

	// Pass 2: BOOL bits — OR-aggregate per host byte. Encoding-encoding
	// errors here propagate immediately; we don't half-write.
	for _, f := range n.layout {
		if f.Type != "bool" {
			continue
		}
		v, present := input[f.Name]
		if !present {
			continue // sparse-zero
		}
		b, err := nodes.ToBool(v)
		if err != nil {
			return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
		}
		if f.Byte >= len(buf) {
			return nil, 0, fmt.Errorf("field %q: BOOL byte %d outside output length %d", f.Name, f.Byte, len(buf))
		}
		updated, err := EncodeS7Bit(buf[f.Byte], f.Bit, b)
		if err != nil {
			return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
		}
		buf[f.Byte] = updated
	}

	// Pass 3: non-BOOL fields. Inverse-scale numeric values, then encode and
	// copy into the output at the field's byte offset.
	for _, f := range n.layout {
		if f.Type == "bool" {
			continue
		}
		v, present := input[f.Name]
		if !present {
			continue
		}

		// Inverse scaling on numeric scalars only — same skip set as parse().
		if !s7IsNonScalable(f.Type) {
			if f.Scale != 1 || f.OffsetValue != 0 {
				if scaled, err := nodes.UnapplyScale(v, f.Scale, f.OffsetValue); err == nil {
					v = scaled
				}
			}
		}

		var (
			encoded []byte
			err     error
		)
		switch f.Type {
		case "string":
			s, sErr := toStringValue(v)
			if sErr != nil {
				return nil, 0, fmt.Errorf("field %q: %w", f.Name, sErr)
			}
			encoded, err = EncodeS7String(f.Length, s)
		case "wstring":
			s, sErr := toStringValue(v)
			if sErr != nil {
				return nil, 0, fmt.Errorf("field %q: %w", f.Name, sErr)
			}
			encoded, err = EncodeS7WString(f.Length, s)
		case "raw":
			bs, bErr := toByteSlice(v)
			if bErr != nil {
				return nil, 0, fmt.Errorf("field %q: %w", f.Name, bErr)
			}
			if len(bs) != f.Length {
				return nil, 0, fmt.Errorf("field %q: raw expects %d bytes, got %d", f.Name, f.Length, len(bs))
			}
			encoded = bs
		default:
			encoded, err = EncodeS7Scalar(f.Type, f.Signed, v)
		}
		if err != nil {
			return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
		}
		fieldSize := S7TypeByteSize(f.Type, f.Length)
		if f.Byte+fieldSize > len(buf) {
			return nil, 0, fmt.Errorf("field %q: writes past output length (offset %d + size %d > %d)",
				f.Name, f.Byte, fieldSize, len(buf))
		}
		copy(buf[f.Byte:], encoded)
	}

	if minOffset < 0 {
		minOffset = 0
	}
	return buf, minOffset, nil
}

// s7IsNonScalable reports whether a parser field type emits/accepts a
// non-numeric Go value (string or byte slice). Scaling is meaningless on
// these — the parser skips ApplyScale/UnapplyScale for them.
func s7IsNonScalable(typ string) bool {
	switch typ {
	case "string", "wstring", "raw",
		"date", "dt", "ldt", "dtl", "wchar",
		"counter", "timer":
		return true
	}
	return false
}

// S7ParserTypeInfo returns the node type metadata for the palette.
func S7ParserTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "s7-parser",
		Category:    "industrial",
		Label:       "S7 Parser",
		Description: "Parse S7 byte blocks into objects and back",
		Icon:        "memory",
		Defaults: map[string]any{
			"action":          "auto",
			"parseFrom":       "payload",
			"encodeFrom":      "payload",
			"blockLength":   0,
			"preserveBytes": false,
			"layout":        []any{},
		},
		Inputs:  1,
		Outputs: 1,
	}
}
