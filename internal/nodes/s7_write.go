// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// S7WriteNode publishes incoming flow messages onto a SIEMENS S7 PLC. PR-5
// supports static + dynamic modes (block mode arrives in PR-6).
//
// Output behaviour is configurable via two mutually-exclusive flags:
//
//   - emitAck=true  — emit a new ACK message on port 0 after every write
//   - passthrough=true — forward the input message with `msg.s7Write` enriched
//
// With both flags off (the default), the node is a pure sink — 0 outputs.
//
// Per-variable encoding errors (out-of-range, type mismatch) and per-variable
// PLC errors are collected into `msg.s7Write.results[i].error` so a single
// bad variable doesn't prevent the rest from writing. Whole-transaction
// failures (transport, lost connection) escalate to the function's error
// return and route through the catch-node mechanism.
//
// Implements flow.ConfigProvider and flow.ErrorProvider.
type S7WriteNode struct {
	config       flow.NodeConfig
	BaseNode
	configLookup flow.ConfigLookupFunc
	errFn        flow.ErrorFunc

	mode        string
	plcID       string
	plc         *S7PLC
	variables   []s7WriteVariable
	block       s7BlockConfig // used only when mode=="block"
	emitAck     bool
	passthrough bool
}

// s7WriteVariable is one entry in the write node's variables list. Static
// mode parses these once at Init from props["variables"]; dynamic mode
// re-parses them per message from msg.variables (with value provided inline).
type s7WriteVariable struct {
	Address  string
	DataType string
	Scale    float64
	Offset   float64
	Item     S7Item

	// Value source — exactly one of these is set:
	//   - StaticValue + HasStatic: value baked into the config (valueSource=static)
	//   - ValuePath: pick value from msg.<path>           (valueSource=msg)
	//   - DirectValue + HasDirect: value supplied inline  (dynamic-mode msg.variables[].value)
	StaticValue any
	HasStatic   bool
	ValuePath   string
	DirectValue any
	HasDirect   bool
}

// NewS7WriteNode is the NodeFactory for the s7-write node type.
func NewS7WriteNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &S7WriteNode{config: config}, nil
}

// Init parses configuration. In static mode the `variables` list is required;
// in dynamic mode it is optional (the variables come per-message instead).
func (n *S7WriteNode) Init() error {
	props := n.config.Properties

	n.plcID, _ = props["plc"].(string)
	if n.plcID == "" {
		return fmt.Errorf("s7-write %s: no plc configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	if mode == "" {
		mode = "static"
	}
	if mode != "static" && mode != "dynamic" && mode != "block" {
		return fmt.Errorf("s7-write %s: invalid mode %q (expected static|dynamic|block)", n.config.ID, mode)
	}
	n.mode = mode

	if n.mode == "block" {
		blockProps, _ := props["block"].(map[string]any)
		blk, err := parseS7BlockConfigWrite(blockProps, n.config.ID)
		if err != nil {
			return err
		}
		n.block = blk
	} else {
		rawVars, _ := props["variables"].([]any)
		vars, err := parseS7WriteVariablesStatic(rawVars, n.config.ID)
		if err != nil {
			return err
		}
		n.variables = vars

		if n.mode == "static" && len(n.variables) == 0 {
			return fmt.Errorf("s7-write %s: static mode requires at least one variable", n.config.ID)
		}
	}

	if v, ok := props["emitAck"].(bool); ok {
		n.emitAck = v
	}
	if v, ok := props["passthrough"].(bool); ok {
		n.passthrough = v
	}
	if n.emitAck && n.passthrough {
		return fmt.Errorf("s7-write %s: emitAck and passthrough are mutually exclusive", n.config.ID)
	}

	return nil
}

func (n *S7WriteNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }
func (n *S7WriteNode) SetError(fn flow.ErrorFunc)              { n.errFn = fn }

// Start resolves the PLC config instance and registers for status changes.
func (n *S7WriteNode) Start() error {
	plc, err := resolveConfigInstance[S7PLC](n.configLookup, n.plcID, n.Status, resolveConfigParams{
		NodeKind:   "s7-write",
		NodeID:     n.config.ID,
		ConfigKind: "plc",
		TypeLabel:  "an S7 PLC",
	})
	if err != nil {
		return err
	}
	n.plc = plc
	n.plc.RegisterStatusFunc(n.handlePLCStatus)
	slog.Info("s7-write started",
		"node_id", n.config.ID, "mode", n.mode, "vars", len(n.variables),
		"emitAck", n.emitAck, "passthrough", n.passthrough)
	return nil
}

// Stop is a no-op for the write node (no goroutines to clean up).
func (n *S7WriteNode) Stop() error {
	slog.Info("s7-write stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage encodes per-variable values, dispatches a multi-write, and —
// depending on emitAck/passthrough — emits a result message on port 0.
//
// Block mode bypasses the per-variable encoding pipeline entirely: the input
// `[]byte` from `msg.<inputProperty>` is sent in one `AGWriteArea` call.
func (n *S7WriteNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if n.mode == "block" {
		return n.handleBlockWrite(msg)
	}

	vars, err := n.effectiveVariables(msg)
	if err != nil {
		return nil, err
	}
	if len(vars) == 0 {
		// Nothing to write — silent (matching the spec's description for
		// dynamic-mode empty input).
		return nil, nil
	}

	// Encode every variable upfront so per-variable encoding errors don't
	// block the rest from writing. Encoding failures (out-of-range, type
	// mismatch) are recorded as per-item errors and the items themselves are
	// excluded from the wire request.
	results := make([]s7WriteResultRow, len(vars))
	writeReqs := make([]S7WriteItem, 0, len(vars))
	writeIdx := make([]int, 0, len(vars)) // results-index for each writeReq
	for i, v := range vars {
		results[i] = s7WriteResultRow{
			Address:  v.Address,
			DataType: v.DataType,
		}
		raw, err := n.resolveAndEncode(v, msg)
		if err != nil {
			results[i].Err = err.Error()
			continue
		}
		writeReqs = append(writeReqs, S7WriteItem{Item: v.Item, Data: raw})
		writeIdx = append(writeIdx, i)
		results[i].Ok = true
	}

	// Issue the wire write only if at least one item survived encoding.
	if len(writeReqs) > 0 {
		wres, err := n.plc.WriteItems(writeReqs)
		if err != nil {
			// Whole-transaction failure (transport / lost connection) — the
			// client-side encoding errors we already collected stay in
			// `results` for the catch-node to inspect.
			return nil, fmt.Errorf("s7-write %s: %w", n.config.ID, err)
		}
		// Map per-item PLC results back to the original variable indices.
		for j, w := range wres {
			i := writeIdx[j]
			if w.Err != "" {
				results[i].Ok = false
				results[i].Err = w.Err
			}
		}
	}

	allOk := true
	for _, r := range results {
		if !r.Ok {
			allOk = false
			break
		}
	}

	out := n.buildResultMessage(msg, results, allOk)
	if out == nil {
		return nil, nil
	}
	return [][]*flow.Message{{out}}, nil
}

// s7WriteResultRow is one entry in the per-variable result array surfaced
// through msg.s7Write.results.
type s7WriteResultRow struct {
	Address  string
	DataType string
	Ok       bool
	Err      string
}

// effectiveVariables resolves the variables list depending on mode:
//   - static: returns the configured list as-is
//   - dynamic: parses msg.variables (full form) or msg.address+dataType+payload
//     (convenience form for a single write)
func (n *S7WriteNode) effectiveVariables(msg *flow.Message) ([]s7WriteVariable, error) {
	if n.mode == "static" {
		return n.variables, nil
	}
	if raw, ok := msg.Get("variables").([]any); ok {
		return parseS7WriteVariablesDynamic(raw, n.config.ID)
	}
	addr, hasAddr := msg.Get("address").(string)
	dt, hasDT := msg.Get("dataType").(string)
	if hasAddr && hasDT {
		v, err := buildS7WriteVariableDynamic(map[string]any{
			"address":  addr,
			"dataType": dt,
			"value":    msg.Payload(),
		}, 0, n.config.ID)
		if err != nil {
			return nil, err
		}
		return []s7WriteVariable{v}, nil
	}
	return nil, nil
}

// resolveAndEncode picks the effective value for a variable (from static
// config, message path, or inline direct), applies inverse scaling, and
// encodes to wire bytes.
func (n *S7WriteNode) resolveAndEncode(v s7WriteVariable, msg *flow.Message) ([]byte, error) {
	value, err := n.resolveValue(v, msg)
	if err != nil {
		return nil, err
	}

	// Inverse scaling on numeric types only — bool, raw, and the string-shaped
	// types (string, wstring, date, dt, ldt, dtl, wchar) skip scaling.
	if (v.Scale != 1 || v.Offset != 0) && v.DataType != "bool" && !s7IsNonScalable(v.DataType) {
		f, err := UnapplyScale(value, v.Scale, v.Offset)
		if err != nil {
			return nil, fmt.Errorf("scale: %w", err)
		}
		value = f
	}

	switch v.DataType {
	case "bool":
		// One-bit write: encode the value into a single byte (0x00 or 0x01).
		// The S7Item carries the bit position; gos7's AGWriteMulti uses it
		// to compute the wire address as `Start*8 + Bit`.
		b, err := toBool(value)
		if err != nil {
			return nil, fmt.Errorf("bool: %w", err)
		}
		if b {
			return []byte{0x01}, nil
		}
		return []byte{0x00}, nil
	case "string":
		s, err := toStringValue(value)
		if err != nil {
			return nil, fmt.Errorf("string: %w", err)
		}
		return EncodeS7String(v.Item.StringMaxLen, s)
	case "wstring":
		s, err := toStringValue(value)
		if err != nil {
			return nil, fmt.Errorf("wstring: %w", err)
		}
		return EncodeS7WString(v.Item.StringMaxLen, s)
	case "raw":
		bs, err := toByteSlice(value)
		if err != nil {
			return nil, fmt.Errorf("raw: %w", err)
		}
		// raw items expect the byte count to match the configured Amount; we
		// don't pad or truncate.
		if len(bs) != v.Item.Amount {
			return nil, fmt.Errorf("raw: expected %d bytes, got %d", v.Item.Amount, len(bs))
		}
		return bs, nil
	default:
		// signed=false matches the s7-read defaults; users requesting signed
		// byte/word/dword should use the explicit `int`/`dint` types.
		return EncodeS7Scalar(v.DataType, false, value)
	}
}

// resolveValue picks the effective value for a variable based on its source.
// In dynamic mode the value is typically inline (HasDirect). Static mode
// either uses the baked StaticValue or pulls from msg.<ValuePath>.
func (n *S7WriteNode) resolveValue(v s7WriteVariable, msg *flow.Message) (any, error) {
	if v.HasDirect {
		return v.DirectValue, nil
	}
	if v.HasStatic {
		return v.StaticValue, nil
	}
	if msg == nil {
		return nil, fmt.Errorf("no value: msg is nil")
	}
	path := v.ValuePath
	if path == "" {
		path = "payload"
	}
	value := msg.Get(path)
	if value == nil {
		return nil, fmt.Errorf("no value: msg.%s is nil", path)
	}
	return value, nil
}

// buildResultMessage assembles the outgoing message according to emitAck /
// passthrough / silent (default). Returns nil for the silent case.
func (n *S7WriteNode) buildResultMessage(in *flow.Message, results []s7WriteResultRow, allOk bool) *flow.Message {
	if !n.emitAck && !n.passthrough {
		return nil
	}

	resultArr := make([]map[string]any, len(results))
	for i, r := range results {
		entry := map[string]any{
			"address":  r.Address,
			"dataType": r.DataType,
			"ok":       r.Ok,
		}
		if r.Err != "" {
			entry["error"] = r.Err
		}
		resultArr[i] = entry
	}
	meta := map[string]any{
		"plc":     plcTopicSegment(n.plc),
		"results": resultArr,
		"allOk":   allOk,
	}

	if n.passthrough && in != nil {
		// Forward the input message with the s7Write metadata added. We use
		// COWClone so we don't mutate the caller's message.
		out := in.COWClone()
		out.Set("s7Write", meta)
		return out
	}

	out := flow.NewMessage()
	out.SetPayload(allOk)
	out.Set("s7Write", meta)
	return out
}

// handlePLCStatus relays the PLC connection state to the node's status pill.
func (n *S7WriteNode) handlePLCStatus(fill, text string) {
	if n.Status == nil {
		return
	}
	if fill == "green" {
		n.Status("green", "ready")
		return
	}
	n.Status(fill, text)
}

// parseS7WriteVariablesStatic parses the config-time variables list (static
// mode). Each entry follows the shape:
//
//	{address, dataType, valueSource, value (if static), valuePath (if msg),
//	 scale, offset}
func parseS7WriteVariablesStatic(raw []any, nodeID string) ([]s7WriteVariable, error) {
	out := make([]s7WriteVariable, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("s7-write %s: variables[%d] is not an object", nodeID, i)
		}
		v, err := buildS7WriteVariableStatic(obj, i, nodeID)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func buildS7WriteVariableStatic(obj map[string]any, i int, nodeID string) (s7WriteVariable, error) {
	addr, _ := obj["address"].(string)
	if addr == "" {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: variables[%d].address is required", nodeID, i)
	}
	dt, _ := obj["dataType"].(string)
	if dt == "" {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: variables[%d].dataType is required", nodeID, i)
	}
	item, err := ParseS7Address(addr, dt)
	if err != nil {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: variables[%d] (%q): %w", nodeID, i, addr, err)
	}

	v := s7WriteVariable{
		Address:  addr,
		DataType: dt,
		Scale:    readFloatProp(obj, "scale", 1),
		Offset:   readFloatProp(obj, "offset", 0),
		Item:     item,
	}
	if v.Scale == 0 {
		v.Scale = 1
	}

	source, _ := obj["valueSource"].(string)
	if source == "" {
		source = "msg"
	}
	switch source {
	case "static":
		val, has := obj["value"]
		if !has {
			return s7WriteVariable{}, fmt.Errorf("s7-write %s: variables[%d] valueSource=static requires `value`", nodeID, i)
		}
		v.HasStatic = true
		v.StaticValue = val
	case "msg":
		path, _ := obj["valuePath"].(string)
		if path == "" {
			path = "payload"
		}
		v.ValuePath = path
	default:
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: variables[%d] invalid valueSource %q (expected static|msg)", nodeID, i, source)
	}
	return v, nil
}

// parseS7WriteVariablesDynamic parses msg.variables for dynamic-mode writes.
// Each entry carries the value inline:
//
//	{address, dataType, value, scale?, offset?}
func parseS7WriteVariablesDynamic(raw []any, nodeID string) ([]s7WriteVariable, error) {
	out := make([]s7WriteVariable, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("s7-write %s: msg.variables[%d] is not an object", nodeID, i)
		}
		v, err := buildS7WriteVariableDynamic(obj, i, nodeID)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func buildS7WriteVariableDynamic(obj map[string]any, i int, nodeID string) (s7WriteVariable, error) {
	addr, _ := obj["address"].(string)
	if addr == "" {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: msg.variables[%d].address is required", nodeID, i)
	}
	dt, _ := obj["dataType"].(string)
	if dt == "" {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: msg.variables[%d].dataType is required", nodeID, i)
	}
	item, err := ParseS7Address(addr, dt)
	if err != nil {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: msg.variables[%d] (%q): %w", nodeID, i, addr, err)
	}

	v := s7WriteVariable{
		Address:  addr,
		DataType: dt,
		Scale:    readFloatProp(obj, "scale", 1),
		Offset:   readFloatProp(obj, "offset", 0),
		Item:     item,
	}
	if v.Scale == 0 {
		v.Scale = 1
	}
	val, has := obj["value"]
	if !has {
		return s7WriteVariable{}, fmt.Errorf("s7-write %s: msg.variables[%d] requires `value`", nodeID, i)
	}
	v.HasDirect = true
	v.DirectValue = val
	return v, nil
}

// toStringValue coerces a payload to a string. Already-string values pass
// through; other primitive types render via %v as a fallback (intended for
// the `string` data type only — Function-node users typically already have
// a string there).
func toStringValue(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", fmt.Errorf("nil value")
	case string:
		return x, nil
	default:
		return fmt.Sprintf("%v", x), nil
	}
}

// toByteSlice coerces JSON-decoded byte arrays into a Go []byte. The
// JSON-decoder turns numeric arrays into []any (with float64 elements), so
// we handle that case explicitly. Direct []byte and []int are also accepted.
func toByteSlice(v any) ([]byte, error) {
	switch x := v.(type) {
	case []byte:
		out := make([]byte, len(x))
		copy(out, x)
		return out, nil
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
			n, err := toInt64(e)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("element %d (%d) out of byte range [0, 255]", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	}
	return nil, fmt.Errorf("cannot convert %T to []byte", v)
}

// parseS7BlockConfigWrite parses the write node's `block` config object.
// The write side is structurally similar to the read side but allows
// `length` to be optional (when unset, the incoming data length is used)
// and adds the `inputProperty` field (default "payload"). Writes to inputs
// (PE) are rejected upfront because the underlying PLC manager refuses them.
func parseS7BlockConfigWrite(props map[string]any, nodeID string) (s7BlockConfig, error) {
	if props == nil {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: block mode requires a `block` config object", nodeID)
	}
	areaName, _ := props["area"].(string)
	if areaName == "" {
		areaName = "DB"
	}
	area, err := parseS7AreaName(areaName)
	if err != nil {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: %w", nodeID, err)
	}
	if area == S7AreaPE {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: block.area=I (inputs) is not writable", nodeID)
	}
	db := readIntProp(props, "db", 0)
	if area == S7AreaDB && db <= 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: block.db must be > 0 for area=DB", nodeID)
	}
	start := readIntProp(props, "start", 0)
	if start < 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: block.start must be >= 0, got %d", nodeID, start)
	}
	length := readIntProp(props, "length", 0)
	if length < 0 {
		return s7BlockConfig{}, fmt.Errorf("s7-write %s: block.length must be >= 0 (0 = use incoming data length), got %d", nodeID, length)
	}
	inputProp, _ := props["inputProperty"].(string)
	if inputProp == "" {
		inputProp = "payload"
	}
	return s7BlockConfig{
		Area: area, AreaName: areaName, DB: db,
		Start: start, Length: length, InputProperty: inputProp,
	}, nil
}

// handleBlockWrite dispatches a single block write. Per-message overrides
// for area/db/start come via `msg.s7.*`; the byte slice itself comes from
// `msg.<inputProperty>` (default "payload").
func (n *S7WriteNode) handleBlockWrite(msg *flow.Message) ([][]*flow.Message, error) {
	blk := n.effectiveWriteBlock(msg)

	raw, err := toByteSlice(msg.Get(blk.InputProperty))
	if err != nil {
		return nil, fmt.Errorf("s7-write %s: read msg.%s: %w", n.config.ID, blk.InputProperty, err)
	}
	if blk.Length > 0 && len(raw) != blk.Length {
		return nil, fmt.Errorf("s7-write %s: input length %d != configured block.length %d", n.config.ID, len(raw), blk.Length)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("s7-write %s: block write requires a non-empty byte slice in msg.%s", n.config.ID, blk.InputProperty)
	}

	if err := n.plc.WriteArea(blk.Area, blk.DB, blk.Start, raw); err != nil {
		return nil, fmt.Errorf("s7-write %s: %w", n.config.ID, err)
	}

	if !n.emitAck && !n.passthrough {
		return nil, nil
	}
	meta := map[string]any{
		"plc":    plcTopicSegment(n.plc),
		"area":   blk.AreaName,
		"db":     blk.DB,
		"start":  blk.Start,
		"length": len(raw),
		"allOk":  true,
	}
	if n.passthrough {
		out := msg.COWClone()
		out.Set("s7Write", meta)
		return [][]*flow.Message{{out}}, nil
	}
	out := flow.NewMessage()
	out.SetPayload(true)
	out.Set("s7Write", meta)
	return [][]*flow.Message{{out}}, nil
}

// effectiveWriteBlock applies per-message overrides on top of the block
// config. Mirrors the read node's effectiveBlock helper but skips the
// triggerOnInput field (writes always trigger on input).
func (n *S7WriteNode) effectiveWriteBlock(msg *flow.Message) s7BlockConfig {
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
	return blk
}

// S7WriteTypeInfo returns the node type metadata for the palette.
func S7WriteTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "s7-write",
		Category:    "industrial",
		Label:       "S7 Write",
		Description: "Writes variables to a SIEMENS S7 PLC",
		Icon:        "memory",
		Defaults: map[string]any{
			"plc":         "",
			"mode":        "static",
			"variables":   []any{},
			"emitAck":     false,
			"passthrough": false,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
