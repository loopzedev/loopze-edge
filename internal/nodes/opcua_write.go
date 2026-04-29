// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/niceclouds/flint/internal/flow"
)

// writeSpec describes a single write operation either resolved from the static
// configuration or extracted from the incoming control message.
type writeSpec struct {
	nodeID        string  // raw form, kept for the result entry
	parsed        *ua.NodeID
	dataType      string  // optional: explicit DataType name, "" → resolve via cache
	structureType string  // ExtensionObject only: NodeID of the struct DataType
	value         any     // raw JSON-decoded value
	pathError     string  // set when the spec came from msg with a missing/bad field
}

// staticWrite is the parsed form of one row in the config-side "writes" array.
type staticWrite struct {
	nodeID        string
	parsed        *ua.NodeID
	dataType      string
	structureType string
	valueSource   string // "static" | "msg"
	staticValue   any
	valuePath     string
}

// OpcuaWriteNode publishes values to OPC UA nodes via the Write service.
//
// Two modes:
//
//   - "static": every input message triggers the writes pre-defined in the
//     config (values pulled either from a static config field or from a path
//     on the incoming message).
//   - "dynamic": the input message itself carries a "writes" array describing
//     what to write where; the config is unused.
//
// Implements flow.ConfigProvider.
type OpcuaWriteNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc

	serverID         string
	mode             string // "static" | "dynamic"
	staticWrites     []staticWrite
	passthrough      bool
	disableTypeCache bool

	server *OpcuaServer

	mu sync.Mutex
}

// NewOpcuaWriteNode is the registry factory.
func NewOpcuaWriteNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &OpcuaWriteNode{config: config}, nil
}

func (n *OpcuaWriteNode) Init() error {
	props := n.config.Properties

	n.serverID, _ = props["server"].(string)
	if n.serverID == "" {
		return fmt.Errorf("opcua-write %s: no server configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	switch mode {
	case "":
		mode = "static"
	case "static", "dynamic":
		// ok
	default:
		return fmt.Errorf("opcua-write %s: invalid mode %q", n.config.ID, mode)
	}
	n.mode = mode

	if v, ok := props["passthrough"].(bool); ok {
		n.passthrough = v
	}
	if v, ok := props["disableTypeCache"].(bool); ok {
		n.disableTypeCache = v
	}

	if mode == "static" {
		raw, _ := props["writes"].([]any)
		if len(raw) == 0 {
			return fmt.Errorf("opcua-write %s: at least one entry in 'writes' is required in static mode", n.config.ID)
		}
		writes, err := parseStaticWrites(raw)
		if err != nil {
			return fmt.Errorf("opcua-write %s: %w", n.config.ID, err)
		}
		n.staticWrites = writes
	}

	return nil
}

func parseStaticWrites(raw []any) ([]staticWrite, error) {
	out := make([]staticWrite, 0, len(raw))
	for i, r := range raw {
		entry, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("writes[%d] is not an object", i)
		}
		nodeID, _ := entry["nodeId"].(string)
		if nodeID == "" {
			return nil, fmt.Errorf("writes[%d]: nodeId is required", i)
		}
		parsed, err := ParseOpcuaNodeID(nodeID)
		if err != nil {
			return nil, fmt.Errorf("writes[%d]: %w", i, err)
		}

		dataType, _ := entry["dataType"].(string)
		structureType, _ := entry["structureType"].(string)

		valueSource, _ := entry["valueSource"].(string)
		if valueSource == "" {
			valueSource = "msg"
		}
		if valueSource != "msg" && valueSource != "static" {
			return nil, fmt.Errorf("writes[%d]: invalid valueSource %q", i, valueSource)
		}

		valuePath, _ := entry["valuePath"].(string)
		if valuePath == "" {
			valuePath = "payload"
		}

		out = append(out, staticWrite{
			nodeID:        nodeID,
			parsed:        parsed,
			dataType:      dataType,
			structureType: structureType,
			valueSource:   valueSource,
			staticValue:   entry["value"],
			valuePath:     valuePath,
		})
	}
	return out, nil
}

func (n *OpcuaWriteNode) SetSend(fn flow.SendFunc)                { n.send = fn }
func (n *OpcuaWriteNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *OpcuaWriteNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *OpcuaWriteNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *OpcuaWriteNode) Start() error {
	if n.configLookup == nil {
		return fmt.Errorf("opcua-write %s: config lookup not available", n.config.ID)
	}
	inst, ok := n.configLookup(n.serverID)
	if !ok {
		if n.status != nil {
			n.status("red", "server not found")
		}
		return fmt.Errorf("opcua-write %s: server %q not found", n.config.ID, n.serverID)
	}
	server, ok := inst.(*OpcuaServer)
	if !ok {
		return fmt.Errorf("opcua-write %s: config %q is not an OPC UA server", n.config.ID, n.serverID)
	}
	n.server = server

	if n.status != nil {
		n.server.RegisterStatusFunc(n.status)
	}

	slog.Info("opcua-write started", "node_id", n.config.ID, "mode", n.mode)
	return nil
}

func (n *OpcuaWriteNode) Stop() error { return nil }

func (n *OpcuaWriteNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	specs := n.specsForMessage(msg)
	if len(specs) == 0 {
		return nil, nil
	}

	results := n.executeWrites(specs)

	allGood := true
	for _, r := range results {
		if name, ok := r["statusCode"].(string); ok && name != "Good" {
			allGood = false
			break
		}
	}

	out := msg
	if !n.passthrough {
		out = flow.NewMessage()
	}
	out.Set("writeResult", toAnySliceFromMaps(results))
	out.Set("allGood", allGood)

	if n.config.Properties != nil {
		// HandleMessage is configured for 1 input; output is 0 by default. We
		// only emit when the wiring leaves a port to write to — the engine
		// handles the dropped second slot if outputs == 0.
	}
	return [][]*flow.Message{{out}}, nil
}

// specsForMessage builds the actionable list of writes for a given input.
// In static mode it walks the config; in dynamic mode it parses msg.writes
// (or the convenience single-write form msg.nodeId + msg.payload).
func (n *OpcuaWriteNode) specsForMessage(msg *flow.Message) []writeSpec {
	if n.mode == "static" {
		return n.staticSpecs(msg)
	}
	return n.dynamicSpecs(msg)
}

func (n *OpcuaWriteNode) staticSpecs(msg *flow.Message) []writeSpec {
	out := make([]writeSpec, 0, len(n.staticWrites))
	for _, w := range n.staticWrites {
		spec := writeSpec{nodeID: w.nodeID, parsed: w.parsed, dataType: w.dataType, structureType: w.structureType}
		if w.valueSource == "static" {
			spec.value = w.staticValue
		} else {
			spec.value = msg.Get(w.valuePath)
			if spec.value == nil {
				spec.pathError = fmt.Sprintf("msg.%s missing", w.valuePath)
			}
		}
		out = append(out, spec)
	}
	return out
}

func (n *OpcuaWriteNode) dynamicSpecs(msg *flow.Message) []writeSpec {
	if writesAny, ok := msg.Get("writes").([]any); ok {
		out := make([]writeSpec, 0, len(writesAny))
		for i, raw := range writesAny {
			entry, ok := raw.(map[string]any)
			if !ok {
				out = append(out, writeSpec{
					nodeID:    fmt.Sprintf("[%d]", i),
					pathError: "writes entry is not an object",
				})
				continue
			}
			out = append(out, writeSpecFromMap(entry))
		}
		return out
	}
	// Convenience single-write form: msg.nodeId + msg.payload + optional dataType.
	if nodeID, ok := msg.Get("nodeId").(string); ok && nodeID != "" {
		spec := writeSpec{
			nodeID: nodeID,
			value:  msg.Get("payload"),
		}
		if dt, ok := msg.Get("dataType").(string); ok {
			spec.dataType = dt
		}
		if parsed, err := ParseOpcuaNodeID(nodeID); err == nil {
			spec.parsed = parsed
		} else {
			spec.pathError = err.Error()
		}
		return []writeSpec{spec}
	}
	return nil
}

func writeSpecFromMap(entry map[string]any) writeSpec {
	nodeID, _ := entry["nodeId"].(string)
	spec := writeSpec{nodeID: nodeID}
	if nodeID == "" {
		spec.pathError = "nodeId missing"
		return spec
	}
	parsed, err := ParseOpcuaNodeID(nodeID)
	if err != nil {
		spec.pathError = err.Error()
		return spec
	}
	spec.parsed = parsed

	if dt, ok := entry["dataType"].(string); ok {
		spec.dataType = dt
	}
	if st, ok := entry["structureType"].(string); ok {
		spec.structureType = st
	}
	spec.value = entry["value"]
	return spec
}

// executeWrites is where the network call happens. It never returns an error;
// per-item issues are reported in the result entries so a partial batch can
// still emit a useful output message.
func (n *OpcuaWriteNode) executeWrites(specs []writeSpec) []map[string]any {
	results := make([]map[string]any, len(specs))
	for i := range specs {
		results[i] = map[string]any{"nodeId": specs[i].nodeID}
	}

	client := n.server.Client()
	if client == nil {
		for i := range results {
			results[i]["statusCode"] = "BadConnectionClosed"
			results[i]["statusCodeRaw"] = uint32(ua.StatusBadConnectionClosed)
		}
		if n.status != nil {
			n.status("red", "no session")
		}
		return results
	}

	// Resolve target TypeIDs. Spec-level errors short-circuit before the call.
	type prepared struct {
		idx     int
		spec    writeSpec
		typeID  ua.TypeID
		coerced any
	}
	toSubmit := make([]prepared, 0, len(specs))
	for i, spec := range specs {
		if spec.pathError != "" {
			results[i]["statusCode"] = "BadInvalidArgument"
			results[i]["statusCodeRaw"] = uint32(ua.StatusBadInvalidArgument)
			results[i]["error"] = spec.pathError
			continue
		}
		typeID, err := n.resolveTypeID(spec)
		if err != nil {
			results[i]["statusCode"] = "BadTypeMismatch"
			results[i]["statusCodeRaw"] = uint32(ua.StatusBadTypeMismatch)
			results[i]["error"] = err.Error()
			continue
		}
		coerced, err := n.coerceOrEncode(spec, typeID)
		if err != nil {
			results[i]["statusCode"] = "BadTypeMismatch"
			results[i]["statusCodeRaw"] = uint32(ua.StatusBadTypeMismatch)
			results[i]["error"] = err.Error()
			continue
		}
		toSubmit = append(toSubmit, prepared{idx: i, spec: spec, typeID: typeID, coerced: coerced})
	}

	if len(toSubmit) == 0 {
		return results
	}

	nodesToWrite := make([]*ua.WriteValue, len(toSubmit))
	for j, p := range toSubmit {
		variant, err := ua.NewVariant(p.coerced)
		if err != nil {
			results[p.idx]["statusCode"] = "BadEncodingError"
			results[p.idx]["statusCodeRaw"] = uint32(ua.StatusBadEncodingError)
			results[p.idx]["error"] = fmt.Sprintf("variant: %v", err)
			continue
		}
		nodesToWrite[j] = &ua.WriteValue{
			NodeID:      p.spec.parsed,
			AttributeID: ua.AttributeIDValue,
			Value:       &ua.DataValue{EncodingMask: ua.DataValueValue, Value: variant},
		}
	}

	// Compact nodesToWrite — entries that failed variant encoding above are nil.
	pairs := make([]prepared, 0, len(toSubmit))
	compact := nodesToWrite[:0]
	for j, p := range toSubmit {
		if nodesToWrite[j] != nil {
			compact = append(compact, nodesToWrite[j])
			pairs = append(pairs, p)
		}
	}
	if len(compact) == 0 {
		return results
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	defer cancel()
	resp, err := client.Write(ctx, &ua.WriteRequest{NodesToWrite: compact})
	if err != nil {
		// Service-level failure → mark every submitted item as bad.
		for _, p := range pairs {
			results[p.idx]["statusCode"] = "BadCommunicationError"
			results[p.idx]["statusCodeRaw"] = uint32(ua.StatusBadCommunicationError)
			results[p.idx]["error"] = err.Error()
		}
		if n.status != nil {
			n.status("red", err.Error())
		}
		return results
	}
	for j, code := range resp.Results {
		if j >= len(pairs) {
			break
		}
		idx := pairs[j].idx
		results[idx]["statusCode"] = OpcuaStatusCodeName(code)
		results[idx]["statusCodeRaw"] = uint32(code)
	}

	n.pulseStatus(results)
	return results
}

// coerceOrEncode produces the Go value to wrap in a Variant. For built-in
// types the path is the simple primitive coercion; for ExtensionObjects we
// route through the type resolver so the body bytes are encoded against the
// server-side schema.
func (n *OpcuaWriteNode) coerceOrEncode(spec writeSpec, typeID ua.TypeID) (any, error) {
	if typeID != ua.TypeIDExtensionObject {
		return CoerceOpcuaValue(spec.value, typeID)
	}
	if spec.structureType == "" {
		return nil, fmt.Errorf("ExtensionObject write needs structureType (NodeID of the struct DataType)")
	}
	structType, err := ParseOpcuaNodeID(spec.structureType)
	if err != nil {
		return nil, fmt.Errorf("invalid structureType %q: %w", spec.structureType, err)
	}
	obj, ok := spec.value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ExtensionObject value must be an object, got %T", spec.value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	defer cancel()
	return n.server.EncodeStructValue(ctx, structType, obj)
}

// resolveTypeID figures out which ua.TypeID applies for a given spec. The
// explicit dataType field wins; otherwise the server-side cache (or a Read of
// the DataType attribute) is consulted.
func (n *OpcuaWriteNode) resolveTypeID(spec writeSpec) (ua.TypeID, error) {
	if spec.dataType != "" {
		t, ok := OpcuaTypeIDFromName(spec.dataType)
		if !ok {
			return 0, fmt.Errorf("unknown dataType %q", spec.dataType)
		}
		return t, nil
	}
	if n.disableTypeCache {
		return 0, fmt.Errorf("dataType is required when type cache is disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	defer cancel()
	t, err := n.server.LookupDataType(ctx, spec.parsed)
	if err != nil {
		return 0, fmt.Errorf("DataType lookup: %w", err)
	}
	return t, nil
}

// pulseStatus reports a brief summary like "3/4 ok" so the editor shows the
// last write outcome at a glance. Long-lived state (red on connection loss
// etc.) keeps coming through the server's RegisterStatusFunc broadcast.
func (n *OpcuaWriteNode) pulseStatus(results []map[string]any) {
	if n.status == nil {
		return
	}
	good := 0
	for _, r := range results {
		if name, ok := r["statusCode"].(string); ok && name == "Good" {
			good++
		}
	}
	if good == len(results) {
		n.status("green", fmt.Sprintf("%d/%d ok", good, len(results)))
	} else {
		n.status("yellow", fmt.Sprintf("%d/%d ok", good, len(results)))
	}
	// Don't restore the prior status — the next state-change broadcast or the
	// next message will overwrite this anyway. Keeps the implementation tight.
	_ = time.Time{}
}

// toAnySliceFromMaps converts the result list into the JSON-friendly form
// used in the outgoing message payload.
func toAnySliceFromMaps(in []map[string]any) []any {
	out := make([]any, len(in))
	for i, m := range in {
		out[i] = m
	}
	return out
}

// OpcuaWriteTypeInfo registers the node type with the palette.
func OpcuaWriteTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "opcua-write",
		Category:    "industrial",
		Label:       "OPC UA Write",
		Description: "Write OPC UA node attributes",
		Icon:        "mdi-database-arrow-up",
		Defaults: map[string]any{
			"server":           "",
			"mode":             "static",
			"writes":           []any{},
			"passthrough":      false,
			"disableTypeCache": false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
