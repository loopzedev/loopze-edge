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
	if mode != "static" && mode != "dynamic" {
		return fmt.Errorf("s7-read %s: invalid mode %q (expected static|dynamic; block mode arrives in PR-6)", n.config.ID, mode)
	}
	n.mode = mode

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

	// outputShape: validated against the variables list count. "single" is
	// only meaningful with exactly one variable; >1 variables default to
	// "object" so downstream Function/Switch nodes get a map keyed by name.
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

	if n.mode == "static" {
		n.wg.Add(1)
		go n.pollLoop()
		slog.Info("s7-read started (static)",
			"node_id", n.config.ID, "vars", len(n.variables), "interval", n.pollInterval)
	} else {
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

// HandleMessage in dynamic mode triggers a read using effective variables
// (config defaults overridden by msg.variables, msg.address+msg.dataType, or
// msg.name). The incoming message is NOT forwarded — the output carries only
// the read result, matching the modbus-read convention.
func (n *S7ReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
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

// tick performs one read of the configured variables list and dispatches the
// result. Errors are routed through the catch-node mechanism (errFn) and
// optionally emitted as an error message on port 0.
func (n *S7ReadNode) tick() {
	out, err := n.doRead(n.variables, nil)
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
