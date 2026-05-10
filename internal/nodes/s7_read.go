// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// S7ReadNode reads one or more variables from a SIEMENS S7 PLC. Two modes
// are supported in PR-4 (block mode arrives in PR-6):
//
//   - "static" (default): polls the configured variables list at pollInterval; 0 inputs.
//   - "dynamic": triggered by an incoming message; 1 input. The variables list
//     can be overridden per message via `msg.variables` (full override) or via
//     the convenience pair `msg.address`/`msg.dataType` (single read).
//
// Implements flow.ConfigProvider and flow.ErrorProvider.
type S7ReadNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc
	errFn        flow.ErrorFunc

	mode          string
	plcID         string
	plc           *S7PLC
	variables     []s7Variable
	outputShape   string // "single" | "array" | "object"
	topicTemplate string

	// Block-mode config (used when mode=="block"). Otherwise zero-valued.
	block s7BlockConfig

	pollInterval time.Duration
	emitOnChange bool
	emitOnError  bool

	mu          sync.Mutex
	lastPayload any
	done        chan struct{}
	wg          sync.WaitGroup
}

// s7Variable is one entry in the read node's variables list, both at config
// time (Init) and for the per-message override path (HandleMessage). The
// parser-derived S7Item carries the wire-level address descriptor; the
// human-facing fields (Name, Address, DataType, Scale, Offset) are kept for
// the output metadata and for re-parsing on dynamic overrides.
type s7Variable struct {
	Name     string
	Address  string
	DataType string
	Scale    float64
	Offset   float64
	Item     S7Item
}

// NewS7ReadNode is the NodeFactory for the s7-read node type.
func NewS7ReadNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &S7ReadNode{
		config: config,
		done:   make(chan struct{}),
	}, nil
}

// Init validates the config and parses the variables list once so per-poll
// dispatch is cheap.
func (n *S7ReadNode) Init() error {
	props := n.config.Properties

	n.plcID, _ = props["plc"].(string)
	if n.plcID == "" {
		return fmt.Errorf("s7-read %s: no plc configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	if mode == "" {
		mode = "static"
	}
	if mode != "static" && mode != "dynamic" && mode != "block" {
		return fmt.Errorf("s7-read %s: invalid mode %q (expected static|dynamic|block)", n.config.ID, mode)
	}
	n.mode = mode

	if n.mode == "block" {
		blockProps, _ := props["block"].(map[string]any)
		blk, err := parseS7BlockConfigRead(blockProps, n.config.ID)
		if err != nil {
			return err
		}
		n.block = blk
	} else {
		rawVars, _ := props["variables"].([]any)
		vars, err := parseS7Variables(rawVars, n.config.ID)
		if err != nil {
			return err
		}
		n.variables = vars

		// Static mode requires at least one variable so the poll loop has
		// something to do. Dynamic mode is allowed to start empty — the user
		// supplies variables per-message.
		if n.mode == "static" && len(n.variables) == 0 {
			return fmt.Errorf("s7-read %s: static mode requires at least one variable", n.config.ID)
		}
	}

	// outputShape only applies to variables modes — block mode always emits
	// raw `[]byte` in `msg.payload`. A stale value from a previous mode is
	// silently ignored so users can switch modes without manually clearing
	// the config field (the UI hides the dropdown in block mode).
	if n.mode == "block" {
		// no-op: outputShape carries no meaning here.
	} else {
		shape, _ := props["outputShape"].(string)
		if shape == "" {
			if len(n.variables) <= 1 {
				shape = "single"
			} else {
				shape = "object"
			}
		}
		switch shape {
		case "single":
			if len(n.variables) > 1 {
				return fmt.Errorf("s7-read %s: outputShape=single requires exactly 1 variable, got %d", n.config.ID, len(n.variables))
			}
		case "array", "object":
			// nothing to check — these accept any non-zero count
		default:
			return fmt.Errorf("s7-read %s: invalid outputShape %q (expected single|array|object)", n.config.ID, shape)
		}
		n.outputShape = shape
	}

	if t, ok := props["topicTemplate"].(string); ok {
		n.topicTemplate = t
	}

	if v, ok := props["pollInterval"].(float64); ok && v > 0 {
		n.pollInterval = time.Duration(v) * time.Millisecond
	} else {
		n.pollInterval = time.Second
	}

	if v, ok := props["emitOnChange"].(bool); ok {
		n.emitOnChange = v
	}
	if v, ok := props["emitOnError"].(bool); ok {
		n.emitOnError = v
	}

	return nil
}

func (n *S7ReadNode) SetSend(fn flow.SendFunc)                { n.send = fn }
func (n *S7ReadNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *S7ReadNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *S7ReadNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }
func (n *S7ReadNode) SetError(fn flow.ErrorFunc)              { n.errFn = fn }

// Start resolves the PLC config instance, hooks up the status propagator, and
// kicks off the polling goroutine in static mode. Dynamic mode just sits idle
// waiting for input messages.
func (n *S7ReadNode) Start() error {
	plc, err := resolveConfigInstance[S7PLC](n.configLookup, n.plcID, n.status, resolveConfigParams{
		NodeKind:   "s7-read",
		NodeID:     n.config.ID,
		ConfigKind: "plc",
		TypeLabel:  "an S7 PLC",
	})
	if err != nil {
		return err
	}
	n.plc = plc
	n.plc.RegisterStatusFunc(n.handlePLCStatus)

	switch n.mode {
	case "static":
		n.wg.Add(1)
		go n.pollLoop()
		slog.Info("s7-read started (static)",
			"node_id", n.config.ID, "vars", len(n.variables), "interval", n.pollInterval)
	case "block":
		// Block mode polls cyclically just like static. The optional input
		// port (when triggerOnInput=true) lets users force an extra read on
		// demand without waiting for the next tick.
		n.wg.Add(1)
		go n.pollLoop()
		slog.Info("s7-read started (block)",
			"node_id", n.config.ID,
			"area", n.block.AreaName, "db", n.block.DB,
			"start", n.block.Start, "length", n.block.Length,
			"interval", n.pollInterval, "triggerOnInput", n.block.TriggerOnInput)
	default: // dynamic
		slog.Info("s7-read started (dynamic, idle)", "node_id", n.config.ID, "vars", len(n.variables))
	}
	return nil
}

// Stop closes the done channel and waits for the polling goroutine.
func (n *S7ReadNode) Stop() error {
	select {
	case <-n.done:
	default:
		close(n.done)
	}
	n.wg.Wait()
	slog.Info("s7-read stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage handles input messages for dynamic mode and for block mode
// when `triggerOnInput=true`. In dynamic mode the variables list (and its
// values) come from the message; in block mode the input merely triggers a
// fresh read, optionally overriding the area/db/start/length via `msg.s7.*`.
//
// Static mode and block mode without `triggerOnInput` ignore inputs entirely
// (those branches return immediately without producing output).
func (n *S7ReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if n.mode == "block" {
		if !n.block.TriggerOnInput {
			return nil, nil
		}
		out, err := n.doBlockRead(n.effectiveBlock(msg))
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, nil
		}
		return [][]*flow.Message{{out}}, nil
	}
	if n.mode != "dynamic" {
		return nil, nil
	}
	vars, err := n.effectiveVariables(msg)
	if err != nil {
		return nil, err
	}
	if len(vars) == 0 {
		// Nothing to read — silently swallow, matching the spec ("if dynamic
		// mode without msg.nodeIds and without default NodeIDs in the config,
		// the read is skipped with a status warning").
		if n.status != nil {
			n.status("yellow", "no variables to read")
		}
		return nil, nil
	}

	out, err := n.doRead(vars, msg)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return [][]*flow.Message{{out}}, nil
}

// effectiveVariables overlays per-message overrides onto the configured
// variables list. The override rules mirror the spec:
//
//   - msg.variables (object[]) — full override of the variable list
//   - msg.address + msg.dataType (+ optional msg.name) — convenience for a
//     single-variable read
//   - otherwise: the config defaults are used
func (n *S7ReadNode) effectiveVariables(msg *flow.Message) ([]s7Variable, error) {
	if msg == nil {
		return n.variables, nil
	}
	if raw, ok := msg.Get("variables").([]any); ok {
		return parseS7Variables(raw, n.config.ID)
	}
	addr, hasAddr := msg.Get("address").(string)
	dt, hasDT := msg.Get("dataType").(string)
	if hasAddr && hasDT {
		v, err := buildS7Variable(map[string]any{
			"name":     msg.Get("name"),
			"address":  addr,
			"dataType": dt,
		}, 0, n.config.ID)
		if err != nil {
			return nil, err
		}
		return []s7Variable{v}, nil
	}
	return n.variables, nil
}

// pollLoop is the static-mode polling goroutine.
func (n *S7ReadNode) pollLoop() {
	defer n.wg.Done()
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	// First read fires immediately so the user sees data without waiting a tick.
	n.tick()

	for {
		select {
		case <-n.done:
			return
		case <-ticker.C:
			n.tick()
		}
	}
}

// tick performs one read of the configured variables list (or the configured
// block in block mode) and dispatches the result. Errors are routed through
// the catch-node mechanism (errFn) and optionally emitted as an error
// message on port 0.
func (n *S7ReadNode) tick() {
	var (
		out *flow.Message
		err error
	)
	if n.mode == "block" {
		out, err = n.doBlockRead(n.block)
	} else {
		out, err = n.doRead(n.variables, nil)
	}
	if err != nil {
		if n.errFn != nil {
			n.errFn(err, nil)
		}
		if n.emitOnError {
			errMsg := flow.NewMessage()
			errMsg.Set("error", err.Error())
			n.send(0, errMsg)
		}
		return
	}
	if out != nil {
		n.send(0, out)
	}
}

// doRead executes one multi-read, decodes per-variable values, and assembles
// the outgoing message according to outputShape. Returns (nil, nil) when
// emitOnChange suppresses an unchanged read.
//
// The triggerMsg parameter (nil for static-mode polls, non-nil for dynamic)
// is currently unused on the output path — incoming messages are not
// forwarded — but reserved for future use (e.g. correlation IDs, op tracing).
func (n *S7ReadNode) doRead(vars []s7Variable, _ *flow.Message) (*flow.Message, error) {
	items := make([]S7Item, len(vars))
	for i, v := range vars {
		items[i] = v.Item
	}

	results, err := n.plc.ReadItems(items)
	if err != nil {
		return nil, fmt.Errorf("s7-read %s: %w", n.config.ID, err)
	}
	if len(results) != len(vars) {
		return nil, fmt.Errorf("s7-read %s: expected %d results, got %d", n.config.ID, len(vars), len(results))
	}

	// Decode each variable and accumulate the per-item entries. Per-variable
	// errors land in the metadata but don't fail the whole transaction.
	out := make([]s7VarOut, len(vars))
	for i, v := range vars {
		entry := s7VarOut{Name: v.Name, Address: v.Address, DataType: v.DataType}
		if results[i].Err != "" {
			entry.Err = results[i].Err
		} else {
			value, decErr := decodeS7ItemResult(v.Item, v.DataType, results[i].Data)
			if decErr != nil {
				entry.Err = decErr.Error()
			} else {
				entry.Value = ApplyScale(value, v.Scale, v.Offset)
			}
		}
		out[i] = entry
	}

	payload := buildS7ReadPayload(out, n.outputShape)

	if n.emitOnChange {
		n.mu.Lock()
		if reflect.DeepEqual(payload, n.lastPayload) {
			n.mu.Unlock()
			return nil, nil
		}
		n.lastPayload = payload
		n.mu.Unlock()
	}

	msg := flow.NewMessage()
	msg.SetPayload(payload)
	msg.SetTopic(n.renderTopic(vars))

	// Metadata block — always present, mirrors the modbus-read s7-equivalent.
	varsMeta := make([]map[string]any, len(out))
	for i, e := range out {
		entry := map[string]any{
			"name":     e.Name,
			"address":  e.Address,
			"dataType": e.DataType,
		}
		if e.Err != "" {
			entry["error"] = e.Err
		} else {
			entry["value"] = e.Value
		}
		varsMeta[i] = entry
	}
	msg.Set("s7", map[string]any{
		"plc":       plcTopicSegment(n.plc),
		"variables": varsMeta,
	})

	return msg, nil
}

// decodeS7ItemResult maps an S7 wire-byte slice plus its declared dataType to
// a Go value, dispatching on WordLen. Bit results from a multi-read arrive
// already extracted by the lib (Data[0] is 0 or 1), so we surface them as a
// straight bool. STRING goes through the codec's S7-aware decoder; the rest
// flows through DecodeS7Scalar.
func decodeS7ItemResult(item S7Item, dataType string, data []byte) (any, error) {
	switch {
	case item.WordLen == S7WLBit:
		// AGReadMulti returns the bit value as 0x00 / 0x01 in Data[0] —
		// gos7 has already shifted by the configured Bit position on the
		// wire side.
		if len(data) < 1 {
			return nil, fmt.Errorf("decode bool %q: empty buffer", item.DataType)
		}
		return data[0] != 0, nil
	case dataType == "string":
		return DecodeS7String(data)
	case dataType == "wstring":
		return DecodeS7WString(data)
	case dataType == "raw":
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	default:
		// `signed=false` is the default; signed-byte/word/dword shapes are
		// expressed via the explicit `int`/`dint` types.
		return DecodeS7Scalar(dataType, false, data)
	}
}

// s7VarOut is the per-variable decoded record produced by doRead and
// consumed by buildS7ReadPayload. Kept package-private and minimal — it's
// not part of any wire contract, just an intermediate representation.
type s7VarOut struct {
	Name     string
	Address  string
	DataType string
	Value    any
	Err      string
}

// buildS7ReadPayload shapes the per-variable results into the user-selected
// payload form. `single` returns the bare value for the lone variable;
// `array` returns the per-entry list; `object` returns a name-keyed map.
//
// Per-variable errors in `single` and `object` shapes leave the value out
// (the metadata block carries the error string regardless of shape).
func buildS7ReadPayload(out []s7VarOut, shape string) any {
	switch shape {
	case "single":
		if len(out) == 0 {
			return nil
		}
		return out[0].Value
	case "array":
		arr := make([]map[string]any, len(out))
		for i, e := range out {
			entry := map[string]any{
				"name":     e.Name,
				"address":  e.Address,
				"dataType": e.DataType,
			}
			if e.Err != "" {
				entry["error"] = e.Err
			} else {
				entry["value"] = e.Value
			}
			arr[i] = entry
		}
		return arr
	default: // "object"
		obj := make(map[string]any, len(out))
		for _, e := range out {
			if e.Err != "" {
				continue
			}
			obj[e.Name] = e.Value
		}
		return obj
	}
}

// renderTopic substitutes the supported template variables. When no template
// is configured we fall back to a sane default that shows the PLC name and
// — for single-variable reads — the address. Multi-variable reads default
// to just the PLC name (a single topic per message can't carry multiple
// addresses meaningfully).
func (n *S7ReadNode) renderTopic(vars []s7Variable) string {
	template := n.topicTemplate
	if template == "" {
		if len(vars) == 1 {
			template = "s7/<plc-name>/<address>"
		} else {
			template = "s7/<plc-name>"
		}
	}
	r := strings.NewReplacer(
		"<plc-name>", plcTopicSegment(n.plc),
	)
	out := r.Replace(template)
	if len(vars) == 1 {
		out = strings.NewReplacer(
			"<address>", vars[0].Address,
			"<name>", vars[0].Name,
		).Replace(out)
	}
	return out
}

// handlePLCStatus relays the underlying connection state up to the node's
// status pill, with a node-specific text on green that describes the polling
// configuration.
func (n *S7ReadNode) handlePLCStatus(fill, text string) {
	if n.status == nil {
		return
	}
	if fill == "green" {
		n.status("green", n.connectedStatus())
		return
	}
	n.status(fill, text)
}

func (n *S7ReadNode) connectedStatus() string {
	if n.mode == "static" {
		return fmt.Sprintf("connected · %s", n.pollInterval)
	}
	return "connected · idle"
}

// s7BlockConfig captures the parameters of a block-mode read or write. Used
// by both s7-read and s7-write — kept package-level so the write node can
// reuse the same parser.
type s7BlockConfig struct {
	Area           int    // S7Area* constant
	AreaName       string // user-facing string ("DB" / "M" / "I" / "Q") — kept for output metadata
	DB             int    // 0 unless Area == S7AreaDB
	Start          int    // byte offset within the area
	Length         int    // byte count to read; 0 means "use incoming data length" (write only)
	TriggerOnInput bool   // read-only: adds an input port that fires extra reads
	InputProperty  string // write-only: msg field that carries the byte slice
}

// parseS7BlockConfigRead parses the read node's `block` config object.
// Returns an error if any required field is missing or malformed.
func parseS7BlockConfigRead(props map[string]any, nodeID string) (s7BlockConfig, error) {
	if props == nil {
		return s7BlockConfig{}, fmt.Errorf("s7-read %s: block mode requires a `block` config object", nodeID)
	}
	areaName, _ := props["area"].(string)
	if areaName == "" {
		areaName = "DB"
	}
	area, err := parseS7AreaName(areaName)
	if err != nil {
		return s7BlockConfig{}, fmt.Errorf("s7-read %s: %w", nodeID, err)
	}
	db := readIntProp(props, "db", 0)
	if area == S7AreaDB && db <= 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-read %s: block.db must be > 0 for area=DB", nodeID)
	}
	start := readIntProp(props, "start", 0)
	if start < 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-read %s: block.start must be >= 0, got %d", nodeID, start)
	}
	length := readIntProp(props, "length", 0)
	if length <= 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-read %s: block.length must be > 0, got %d", nodeID, length)
	}
	trigger := false
	if v, ok := props["triggerOnInput"].(bool); ok {
		trigger = v
	}
	return s7BlockConfig{
		Area: area, AreaName: areaName, DB: db,
		Start: start, Length: length, TriggerOnInput: trigger,
	}, nil
}

// effectiveBlock overlays the `msg.s7.{area,db,start,length}` per-message
// overrides on top of the configured block. Only used when the node is in
// block mode with `triggerOnInput=true`.
func (n *S7ReadNode) effectiveBlock(msg *flow.Message) s7BlockConfig {
	blk := n.block
	if msg == nil {
		return blk
	}
	overrides, _ := msg.Get("s7").(map[string]any)
	if overrides == nil {
		return blk
	}
	if name, ok := overrides["area"].(string); ok && name != "" {
		if a, err := parseS7AreaName(name); err == nil {
			blk.Area = a
			blk.AreaName = name
		}
	}
	if v, ok := readPositiveInt(overrides["db"]); ok {
		blk.DB = v
	}
	if v, ok := readNonNegInt(overrides["start"]); ok {
		blk.Start = v
	}
	if v, ok := readPositiveInt(overrides["length"]); ok {
		blk.Length = v
	}
	return blk
}

// readNonNegInt is like readPositiveInt but accepts zero as a valid value.
// Used for fields like `start` where 0 is the natural default.
func readNonNegInt(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		if x < 0 {
			return 0, false
		}
		return int(x), true
	case int:
		if x < 0 {
			return 0, false
		}
		return x, true
	case int64:
		if x < 0 {
			return 0, false
		}
		return int(x), true
	}
	return 0, false
}

// doBlockRead executes one block fetch and assembles the outgoing message.
// emitOnChange suppression compares the raw byte slice — in block mode that
// is the whole "value", so a mid-block change still emits.
func (n *S7ReadNode) doBlockRead(blk s7BlockConfig) (*flow.Message, error) {
	data, err := n.plc.ReadArea(blk.Area, blk.DB, blk.Start, blk.Length)
	if err != nil {
		return nil, fmt.Errorf("s7-read %s: %w", n.config.ID, err)
	}

	if n.emitOnChange {
		n.mu.Lock()
		if last, ok := n.lastPayload.([]byte); ok && bytesEqual(last, data) {
			n.mu.Unlock()
			return nil, nil
		}
		// Store a copy so a downstream mutation of the message doesn't
		// affect our suppression state.
		stored := make([]byte, len(data))
		copy(stored, data)
		n.lastPayload = stored
		n.mu.Unlock()
	}

	// Convert []byte → []int before SetPayload so the JSON marshaller
	// (used by the debug node and the workspace API) renders the bytes as
	// `[222, 173, 190, 239, …]` instead of base64-encoding them into an
	// opaque string. The s7-parser's normaliseS7BytesInput accepts both
	// shapes, so downstream wiring is unaffected. Mirrors the modbus-read
	// raw-mode convention (msg.bytes = []int).
	bytesArr := make([]int, len(data))
	for i, b := range data {
		bytesArr[i] = int(b)
	}

	msg := flow.NewMessage()
	msg.SetPayload(bytesArr)
	msg.SetTopic(n.renderBlockTopic(blk))
	msg.Set("s7", map[string]any{
		"plc":    plcTopicSegment(n.plc),
		"area":   blk.AreaName,
		"db":     blk.DB,
		"start":  blk.Start,
		"length": len(data),
	})
	return msg, nil
}

func (n *S7ReadNode) renderBlockTopic(blk s7BlockConfig) string {
	if n.topicTemplate == "" {
		if blk.Area == S7AreaDB {
			return fmt.Sprintf("s7/%s/db%d", plcTopicSegment(n.plc), blk.DB)
		}
		return fmt.Sprintf("s7/%s/%s", plcTopicSegment(n.plc), strings.ToLower(blk.AreaName))
	}
	return strings.NewReplacer(
		"<plc-name>", plcTopicSegment(n.plc),
		"<area>", blk.AreaName,
		"<db>", fmt.Sprintf("%d", blk.DB),
		"<start>", fmt.Sprintf("%d", blk.Start),
		"<length>", fmt.Sprintf("%d", blk.Length),
	).Replace(n.topicTemplate)
}

// parseS7AreaName maps the user-facing area name to its protocol constant.
// Used by both read and write block configs.
func parseS7AreaName(name string) (int, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DB":
		return S7AreaDB, nil
	case "M", "MK":
		return S7AreaMK, nil
	case "I", "PE":
		return S7AreaPE, nil
	case "Q", "PA":
		return S7AreaPA, nil
	default:
		return 0, fmt.Errorf("unknown S7 area %q (expected DB|M|I|Q)", name)
	}
}

// bytesEqual is a small helper so we don't pull in bytes.Equal alone.
// (Worth its own function because the slices may be of different lengths
// across reads when the configured block length changed via msg.s7.length.)
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parseS7Variables turns the JSON-decoded `variables` array into validated
// s7Variable records. Used by Init (config) and by HandleMessage (dynamic
// overrides via `msg.variables`).
func parseS7Variables(raw []any, nodeID string) ([]s7Variable, error) {
	vars := make([]s7Variable, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("s7-read %s: variables[%d] is not an object", nodeID, i)
		}
		v, err := buildS7Variable(obj, i, nodeID)
		if err != nil {
			return nil, err
		}
		vars = append(vars, v)
	}
	return vars, nil
}

// buildS7Variable validates one variable record and parses the address.
// The `i` index and `nodeID` are only for error message attribution.
func buildS7Variable(obj map[string]any, i int, nodeID string) (s7Variable, error) {
	addr, _ := obj["address"].(string)
	if addr == "" {
		return s7Variable{}, fmt.Errorf("s7-read %s: variables[%d].address is required", nodeID, i)
	}
	dt, _ := obj["dataType"].(string)
	if dt == "" {
		return s7Variable{}, fmt.Errorf("s7-read %s: variables[%d].dataType is required", nodeID, i)
	}
	name, _ := obj["name"].(string)
	if name == "" {
		// Default the name to the address — keeps `outputShape=object` from
		// collapsing to an empty key when the user skips the label.
		name = addr
	}
	scale := readFloatProp(obj, "scale", 1)
	if scale == 0 {
		scale = 1
	}
	offset := readFloatProp(obj, "offset", 0)

	item, err := ParseS7Address(addr, dt)
	if err != nil {
		return s7Variable{}, fmt.Errorf("s7-read %s: variables[%d] (%q): %w", nodeID, i, addr, err)
	}
	return s7Variable{
		Name:     name,
		Address:  addr,
		DataType: dt,
		Scale:    scale,
		Offset:   offset,
		Item:     item,
	}, nil
}

// plcTopicSegment returns a topic-safe segment for the PLC name — either the
// configured display name or its config ID when unnamed.
func plcTopicSegment(p *S7PLC) string {
	if p == nil {
		return "unknown"
	}
	if name := p.Name(); name != "" {
		return name
	}
	return p.id
}

// S7ReadTypeInfo returns the node type metadata for the palette.
func S7ReadTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "s7-read",
		Category:    "industrial",
		Label:       "S7 Read",
		Description: "Reads variables from a SIEMENS S7 PLC",
		Icon:        "memory",
		Defaults: map[string]any{
			"plc":           "",
			"mode":          "static",
			"variables":     []any{},
			"outputShape":   "",
			"topicTemplate": "",
			"pollInterval":  1000,
			"emitOnChange":  false,
			"emitOnError":   false,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
