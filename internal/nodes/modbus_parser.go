// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"

	"github.com/niceclouds/loopze/internal/flow"
)

// modbusField describes one entry of the parser node's register layout.
// Offsets are 0-based on the input register block (NOT the absolute Modbus
// address) so a layout is portable between different application addresses.
//
// Bit == -1 means "not set" (the codec's bit sentinel). For type=bool, a
// non-negative Bit selects a single bit out of the host register; Bit < 0
// treats the whole register as a 0/1 boolean.
//
// Length is only consumed for "string" and "raw" — for other types the
// register count is implied by the dataType.
type modbusField struct {
	Offset      int
	Name        string
	Type        string
	Length      int
	ByteOrder   ByteOrder // empty = inherit node default
	WordOrder   WordOrder // empty = inherit node default
	Scale       float64   // 0 → treated as 1
	OffsetValue float64
	Unit        string
	Bit         int // -1 = not set
}

// ModbusParserNode parses register blocks into structured objects (parse) or
// builds register blocks from objects (encode). The layout is configured
// declaratively as a list of fields and is shared by both directions.
//
// Action modes:
//   - auto:   array/buffer input → parse, map input → encode
//   - parse:  forces parse, errors on map input
//   - encode: forces encode, errors on array/buffer input
type ModbusParserNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc
	errFn  flow.ErrorFunc

	action     string // "auto" | "parse" | "encode"
	parseFrom  string // msg path for parse input (default "bytes")
	encodeFrom string // msg path for encode input (default "payload")
	byteOrder  ByteOrder
	wordOrder  WordOrder
	layout     []modbusField

	// inErrorState tracks whether the last conversion failed so we can clear
	// the status pill on the next success. Safe without locking — HandleMessage
	// runs on a single goroutine per node.
	inErrorState bool
}

// NewModbusParserNode is the NodeFactory for the modbus-parser node type.
func NewModbusParserNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &ModbusParserNode{config: config}, nil
}

// Init parses and validates the layout. Returns an error if the configuration
// is structurally invalid (missing required fields, duplicate names, etc.).
// Overlapping fields produce a slog.Warn but are not rejected — bit-mixing on
// the same offset is a legitimate use case.
func (n *ModbusParserNode) Init() error {
	props := n.config.Properties

	n.action = stringVal(props, "action", "auto")
	switch n.action {
	case "auto", "parse", "encode":
	default:
		slog.Warn("modbus-parser: unknown action, falling back to auto",
			"node_id", n.config.ID, "action", n.action)
		n.action = "auto"
	}

	n.parseFrom = stringVal(props, "parseFrom", "bytes")
	n.encodeFrom = stringVal(props, "encodeFrom", "payload")
	n.byteOrder = parseByteOrder(stringVal(props, "byteOrder", ""))
	n.wordOrder = parseWordOrder(stringVal(props, "wordOrder", ""))

	rawLayout, ok := props["layout"].([]any)
	if !ok {
		if _, present := props["layout"]; present {
			return fmt.Errorf("modbus-parser %s: layout must be an array", n.config.ID)
		}
		return fmt.Errorf("modbus-parser %s: layout is empty (define at least one field)", n.config.ID)
	}
	if len(rawLayout) == 0 {
		return fmt.Errorf("modbus-parser %s: layout is empty (define at least one field)", n.config.ID)
	}

	seen := make(map[string]struct{}, len(rawLayout))
	n.layout = make([]modbusField, 0, len(rawLayout))
	for i, raw := range rawLayout {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("modbus-parser %s: layout[%d] is not an object", n.config.ID, i)
		}
		f, err := parseLayoutField(entry)
		if err != nil {
			return fmt.Errorf("modbus-parser %s: layout[%d]: %w", n.config.ID, i, err)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("modbus-parser %s: duplicate field name %q", n.config.ID, f.Name)
		}
		seen[f.Name] = struct{}{}
		n.layout = append(n.layout, f)
	}

	checkLayoutOverlap(n.layout, n.config.ID)

	return nil
}

// parseLayoutField converts one wire-format layout entry into a modbusField,
// applying defaults and validating the per-field constraints.
func parseLayoutField(m map[string]any) (modbusField, error) {
	f := modbusField{
		Offset: intVal(m, "offset", -1),
		Name:   stringVal(m, "name", ""),
		Type:   stringVal(m, "type", ""),
		Length: intVal(m, "length", 0),
		Bit:    intVal(m, "bit", -1),
		Unit:   stringVal(m, "unit", ""),
	}
	if bo, ok := m["byteOrder"].(string); ok && bo != "" {
		f.ByteOrder = parseByteOrder(bo)
	}
	if wo, ok := m["wordOrder"].(string); ok && wo != "" {
		f.WordOrder = parseWordOrder(wo)
	}
	if v, ok := m["scale"]; ok {
		switch x := v.(type) {
		case float64:
			f.Scale = x
		case int:
			f.Scale = float64(x)
		}
	}
	if v, ok := m["offsetValue"]; ok {
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

	// ── Required-field validation ────────────────────────────────────
	if f.Name == "" {
		return f, fmt.Errorf("name is required")
	}
	if f.Offset < 0 {
		return f, fmt.Errorf("field %q: offset must be ≥ 0", f.Name)
	}
	switch f.Type {
	case "bool", "int16", "uint16", "int32", "uint32", "float32",
		"int64", "uint64", "float64", "string", "raw":
	case "":
		return f, fmt.Errorf("field %q: type is required", f.Name)
	default:
		return f, fmt.Errorf("field %q: unknown type %q", f.Name, f.Type)
	}

	// ── Per-type constraints ─────────────────────────────────────────
	if (f.Type == "string" || f.Type == "raw") && f.Length < 1 {
		return f, fmt.Errorf("field %q: type %s requires length ≥ 1", f.Name, f.Type)
	}
	if f.Type != "bool" && f.Bit >= 0 {
		return f, fmt.Errorf("field %q: bit only valid with type=bool, got %s", f.Name, f.Type)
	}
	if f.Bit >= 16 {
		return f, fmt.Errorf("field %q: bit must be 0..15, got %d", f.Name, f.Bit)
	}

	return f, nil
}

// checkLayoutOverlap warns about fields that occupy the same register slot.
// Bit-mixing on the same offset is legitimate (status bits packed into one
// register), so we only emit a warning, not an error. Two non-bit fields on
// the exact same offset are caught here as well (still a warning — the user
// might be parsing the same register two different ways intentionally).
func checkLayoutOverlap(layout []modbusField, nodeID string) {
	type slot struct {
		end int // inclusive
		idx int
	}
	occupied := map[int]slot{}
	for i, f := range layout {
		regs := RegistersForType(f.Type, f.Length)
		end := f.Offset + regs - 1

		// Skip overlap checks for bit fields against non-bit fields on the
		// same register — they are designed to coexist.
		if f.Type == "bool" && f.Bit >= 0 {
			continue
		}

		for offset := f.Offset; offset <= end; offset++ {
			if existing, ok := occupied[offset]; ok && layout[existing.idx].Type != "bool" {
				slog.Warn("modbus-parser: overlapping fields",
					"node_id", nodeID,
					"field_a", layout[existing.idx].Name,
					"field_b", f.Name,
					"register", offset)
				break
			}
			occupied[offset] = slot{end: end, idx: i}
		}
	}
}

// SetSend stores the engine-provided callback for sending output messages.
func (n *ModbusParserNode) SetSend(fn flow.SendFunc) { n.send = fn }

// SetStatus stores the engine-provided callback for status pill updates.
func (n *ModbusParserNode) SetStatus(fn flow.StatusFunc) { n.status = fn }

// SetDebug stores the engine-provided callback for debug output.
func (n *ModbusParserNode) SetDebug(fn flow.DebugFunc) { n.debug = fn }

// SetError implements flow.ErrorProvider so runtime decode/encode errors can
// be routed to Catch nodes.
func (n *ModbusParserNode) SetError(fn flow.ErrorFunc) { n.errFn = fn }

// Start logs the node startup and clears any leftover status pill from a
// previous deploy. Without this clear, a node that died with status=red and
// got redeployed with a fixed config would keep showing the red pill until
// the FIRST conversion error happened — because successful runs only call
// status("", "") when n.inErrorState is true, and a fresh node always starts
// with inErrorState=false.
func (n *ModbusParserNode) Start() error {
	if n.status != nil {
		n.status("", "")
	}
	slog.Info("modbus-parser started",
		"node_id", n.config.ID,
		"action", n.action,
		"fields", len(n.layout))
	return nil
}

// Stop is a no-op for the parser — it holds no goroutines or open resources.
func (n *ModbusParserNode) Stop() error {
	slog.Info("modbus-parser stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage dispatches to parse or encode based on the configured action
// and the input value's type. Errors are returned so the engine routes them
// to Catch nodes; the status pill is also updated red on failure and cleared
// on the next success.
func (n *ModbusParserNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	out, err := n.dispatch(msg)
	if err != nil {
		if n.status != nil {
			n.status("red", "parse error")
		}
		n.inErrorState = true
		return nil, fmt.Errorf("modbus-parser %s: %w", n.config.ID, err)
	}

	if n.inErrorState && n.status != nil {
		n.status("", "")
	}
	n.inErrorState = false

	return [][]*flow.Message{{out}}, nil
}

// dispatch picks parse vs encode and applies it to the right msg property.
// Returns the mutated message (the same object the engine handed us) ready
// for the downstream wire.
func (n *ModbusParserNode) dispatch(msg *flow.Message) (*flow.Message, error) {
	switch n.action {
	case "parse":
		return n.dispatchParse(msg)
	case "encode":
		return n.dispatchEncode(msg)
	case "auto":
		// Pick by looking at the input shape. Encode-side input lives at
		// `encodeFrom` (default "payload"); parse-side at `parseFrom`
		// (default "bytes"). In auto mode we look at both and let the type
		// decide which path to take.
		encodeInput := msg.Get(n.encodeFrom)
		if isMapInput(encodeInput) {
			return n.dispatchEncode(msg)
		}
		return n.dispatchParse(msg)
	default:
		return nil, fmt.Errorf("unknown action %q", n.action)
	}
}

// dispatchParse reads parseFrom, normalises to []uint16, parses, and writes
// the result object to msg.payload.
func (n *ModbusParserNode) dispatchParse(msg *flow.Message) (*flow.Message, error) {
	value := msg.Get(n.parseFrom)
	regs, err := normaliseRegistersInput(value)
	if err != nil {
		return nil, fmt.Errorf("parse input %q: %w", n.parseFrom, err)
	}
	out, err := n.parse(regs)
	if err != nil {
		return nil, err
	}
	msg.SetPayload(out)
	return msg, nil
}

// dispatchEncode reads encodeFrom (must be a map), encodes via the layout,
// and writes the result word-array to msg.payload + bytes to msg.bytes +
// the minimum offset to msg.address (only when not already set).
func (n *ModbusParserNode) dispatchEncode(msg *flow.Message) (*flow.Message, error) {
	value := msg.Get(n.encodeFrom)
	input, ok := normaliseMapInput(value)
	if !ok {
		return nil, fmt.Errorf("encode input %q: expected object/map, got %T", n.encodeFrom, value)
	}
	regs, minOffset, err := n.encode(input)
	if err != nil {
		return nil, err
	}

	// Word-array as []int (JSON-friendly, matches modbus-read raw output).
	wordArr := make([]int, len(regs))
	for i, r := range regs {
		wordArr[i] = int(r)
	}
	msg.SetPayload(wordArr)

	// Wire bytes for downstream Buffer-API consumers.
	rawBytes := RegistersToBytes(regs)
	bytesArr := make([]int, len(rawBytes))
	for i, b := range rawBytes {
		bytesArr[i] = int(b)
	}
	msg.Set("bytes", bytesArr)

	// Only set msg.address if the user hasn't already provided one — this
	// lets the user override the encode-derived offset by setting msg.address
	// upstream, but provides a sensible default for direct attachment to
	// modbus-write.
	if msg.Get("address") == nil {
		msg.Set("address", minOffset)
	}

	return msg, nil
}

// isMapInput reports whether the value looks like a structured object (the
// encode-side input shape).
func isMapInput(v any) bool {
	switch v.(type) {
	case map[string]any:
		return true
	default:
		return false
	}
}

// normaliseMapInput converts the wire value into a map[string]any, accepting
// the typical JSON-decoded shape directly.
func normaliseMapInput(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// normaliseRegistersInput accepts the various wire shapes a register block
// may arrive in and returns a []uint16 ready for parse(). Bytes (msg.bytes
// from modbus-read) are 2:1-collapsed back to registers; word arrays
// (msg.payload from raw read) pass through.
func normaliseRegistersInput(v any) ([]uint16, error) {
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("value is nil")
	case []byte:
		return BytesToRegisters(x), nil
	case []uint16:
		return x, nil
	case []int:
		// May be either bytes (uint8) or registers (uint16). Heuristic: if
		// every element fits into a byte, treat as bytes; otherwise registers.
		// We need to be safe both ways because modbus-read may emit either
		// representation under different dataType settings.
		allBytes := true
		for _, n := range x {
			if n < 0 || n > 0xFF {
				allBytes = false
				break
			}
		}
		if allBytes && len(x)%2 == 0 {
			b := make([]byte, len(x))
			for i, n := range x {
				b[i] = byte(n)
			}
			return BytesToRegisters(b), nil
		}
		// Treat as registers.
		return toUint16Slice(x)
	case []any:
		// JSON-decoded numbers — same heuristic.
		allBytes := true
		for _, e := range x {
			n, err := toInt64(e)
			if err != nil {
				return nil, fmt.Errorf("element: %w", err)
			}
			if n < 0 || n > 0xFF {
				allBytes = false
				break
			}
		}
		if allBytes && len(x)%2 == 0 {
			b := make([]byte, len(x))
			for i, e := range x {
				n, _ := toInt64(e)
				b[i] = byte(n)
			}
			return BytesToRegisters(b), nil
		}
		return toUint16Slice(x)
	default:
		return nil, fmt.Errorf("expected register or byte array, got %T", v)
	}
}

// effectiveOrders resolves the byte/word order for a given field, falling
// back to the node-level defaults when the field has no override.
func (n *ModbusParserNode) effectiveOrders(f modbusField) (ByteOrder, WordOrder) {
	bo := f.ByteOrder
	if bo == "" {
		bo = n.byteOrder
	}
	wo := f.WordOrder
	if wo == "" {
		wo = n.wordOrder
	}
	return bo, wo
}

// parse decodes a register block into a flat object keyed by field name.
//
// Errors mention the failing field by name so users can correct their layout
// without guessing which field caused the trouble.
func (n *ModbusParserNode) parse(regs []uint16) (map[string]any, error) {
	out := make(map[string]any, len(n.layout))
	for _, f := range n.layout {
		regCount := RegistersForType(f.Type, f.Length)
		if f.Offset+regCount > len(regs) {
			return nil, fmt.Errorf("field %q: needs %d register(s) at offset %d but input only has %d",
				f.Name, regCount, f.Offset, len(regs))
		}
		slice := regs[f.Offset : f.Offset+regCount]

		bo, wo := n.effectiveOrders(f)

		// Bool fields are decoded via uint16 — either masked (Bit ≥ 0) or
		// truthy-zero. Decode-side the codec has no "bool" type for registers
		// (its bool path is FC1/FC2 coils); we synthesise the semantic here.
		if f.Type == "bool" {
			raw, err := DecodeRegisters("uint16", bo, WordOrderBig, slice)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", f.Name, err)
			}
			rawInt, _ := toInt64(raw)
			if f.Bit >= 0 {
				out[f.Name] = (rawInt>>uint(f.Bit))&1 == 1
			} else {
				out[f.Name] = rawInt != 0
			}
			continue
		}

		v, err := DecodeRegisters(f.Type, bo, wo, slice)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}

		// Scale only applies to numeric scalars — string/raw pass through.
		if f.Type != "string" && f.Type != "raw" && f.Type != "bool" {
			v = ApplyScale(v, f.Scale, f.OffsetValue)
		}
		out[f.Name] = v
	}
	return out, nil
}

// encode builds a register block from a structured input. Fields not present
// in the input are left at zero (sparse-zero semantics, see PARSER_MODBUS_NODE.md).
//
// Bit-fields with the same Offset are OR-aggregated in a pre-pass so multiple
// status bits can share one host register without overwriting each other.
//
// The returned slice is sized to cover the highest field's end offset. The
// minimum offset is reported so callers can attach msg.address for direct
// wiring into modbus-write.
func (n *ModbusParserNode) encode(input map[string]any) ([]uint16, int, error) {
	if len(input) == 0 {
		return nil, 0, fmt.Errorf("encode: input object is empty")
	}

	// Pass 1 — find the block size and minimum offset.
	maxEnd := 0
	minOffset := -1
	for _, f := range n.layout {
		regs := RegistersForType(f.Type, f.Length)
		if f.Offset+regs > maxEnd {
			maxEnd = f.Offset + regs
		}
		if minOffset < 0 || f.Offset < minOffset {
			minOffset = f.Offset
		}
	}
	regs := make([]uint16, maxEnd)

	// Pass 2 — bit fields. Group by host offset, OR-aggregate the masks.
	bitWords := map[int]uint16{}
	for _, f := range n.layout {
		if f.Type != "bool" || f.Bit < 0 {
			continue
		}
		v, present := input[f.Name]
		if !present {
			continue
		}
		b, err := toBool(v)
		if err != nil {
			return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
		}
		if b {
			bitWords[f.Offset] |= 1 << uint(f.Bit)
		}
	}

	// Pass 3 — non-bit fields (and whole-register bools).
	for _, f := range n.layout {
		// Bit fields are handled by Pass 2.
		if f.Type == "bool" && f.Bit >= 0 {
			continue
		}
		v, present := input[f.Name]
		if !present {
			continue // sparse-zero
		}

		// Inverse scale before encoding (only meaningful for numeric scalars).
		if f.Type != "string" && f.Type != "raw" && f.Type != "bool" {
			if f.Scale != 1 || f.OffsetValue != 0 {
				if scaled, err := UnapplyScale(v, f.Scale, f.OffsetValue); err == nil {
					v = scaled
				}
			}
		}

		bo, wo := n.effectiveOrders(f)

		// Whole-register bool → uint16 (0/1).
		encType := f.Type
		if encType == "bool" {
			encType = "uint16"
			b, err := toBool(v)
			if err != nil {
				return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
			}
			if b {
				v = 1
			} else {
				v = 0
			}
		}

		encoded, err := EncodeRegisters(encType, bo, wo, v)
		if err != nil {
			return nil, 0, fmt.Errorf("field %q: %w", f.Name, err)
		}
		copy(regs[f.Offset:], encoded)
	}

	// Pass 4 — apply the bit masks. OR with whatever the field-level pass
	// wrote so a uint16 field on the same offset survives if the bit field
	// happens to coexist (edge case; documented in the issue).
	for offset, mask := range bitWords {
		if offset < len(regs) {
			regs[offset] |= mask
		}
	}

	if minOffset < 0 {
		minOffset = 0
	}
	return regs, minOffset, nil
}

// ModbusParserTypeInfo returns the node type metadata for the palette.
func ModbusParserTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "modbus-parser",
		Category:    "industrial",
		Label:       "Modbus Parser",
		Description: "Parse register blocks into objects and back",
		Icon:        "memory",
		Defaults: map[string]any{
			"action":     "auto",
			"parseFrom":  "bytes",
			"encodeFrom": "payload",
			"byteOrder":  "bigEndian",
			"wordOrder":  "bigEndian",
			"layout":     []any{},
		},
		Inputs:  1,
		Outputs: 1,
	}
}
