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

// OpcuaReadNode reads OPC UA node attributes (typically Value) and forwards
// them as flow messages.
//
// Three modes:
//
//   - "static"    — periodic Read with a configured interval, no input port
//   - "triggered" — every input message triggers a Read of the configured NodeIDs
//   - "dynamic"   — every input message triggers a Read of the NodeIDs given in
//                   msg.nodeIds (string or []string); falls back to the configured
//                   list when msg.nodeIds is missing
//
// Implements flow.ConfigProvider.
type OpcuaReadNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc

	serverID        string
	mode            string
	nodeIDs         []string
	attribute       ua.AttributeID
	outputShape     string // "single" | "array" | "object"
	includeMetadata bool

	// static-mode-only timing
	interval    time.Duration
	startupRead bool

	server *OpcuaServer

	mu     sync.Mutex
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewOpcuaReadNode is the factory used by the registry.
func NewOpcuaReadNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &OpcuaReadNode{config: config}, nil
}

func (n *OpcuaReadNode) Init() error {
	props := n.config.Properties

	n.serverID, _ = props["server"].(string)
	if n.serverID == "" {
		return fmt.Errorf("opcua-read %s: no server configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	switch mode {
	case "":
		mode = "triggered"
	case "static", "triggered", "dynamic":
		// ok
	default:
		return fmt.Errorf("opcua-read %s: invalid mode %q", n.config.ID, mode)
	}
	n.mode = mode

	n.nodeIDs = readStringSlice(props["nodeIds"])
	if mode != "dynamic" && len(n.nodeIDs) == 0 {
		return fmt.Errorf("opcua-read %s: at least one nodeId is required in %s mode", n.config.ID, mode)
	}
	for _, s := range n.nodeIDs {
		if _, err := ParseOpcuaNodeID(s); err != nil {
			return fmt.Errorf("opcua-read %s: invalid nodeId %q: %w", n.config.ID, s, err)
		}
	}

	attrName, _ := props["attribute"].(string)
	if attr, ok := OpcuaAttributeIDFromName(attrName); ok {
		n.attribute = attr
	} else {
		n.attribute = ua.AttributeIDValue
	}

	shape, _ := props["outputShape"].(string)
	switch shape {
	case "":
		if len(n.nodeIDs) == 1 && mode != "dynamic" {
			shape = "single"
		} else {
			shape = "array"
		}
	case "single", "array", "object", "per-item":
		// ok
	default:
		return fmt.Errorf("opcua-read %s: invalid outputShape %q", n.config.ID, shape)
	}
	n.outputShape = shape

	if v, ok := props["includeMetadata"].(bool); ok {
		n.includeMetadata = v
	} else {
		n.includeMetadata = true
	}

	if mode == "static" {
		intervalMs, _ := props["interval"].(float64)
		n.interval = time.Duration(intervalMs) * time.Millisecond
		if v, ok := props["startupRead"].(bool); ok {
			n.startupRead = v
		} else {
			n.startupRead = true
		}
	}

	return nil
}

func (n *OpcuaReadNode) SetSend(fn flow.SendFunc)                { n.send = fn }
func (n *OpcuaReadNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *OpcuaReadNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *OpcuaReadNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *OpcuaReadNode) Start() error {
	server, err := resolveConfigInstance[OpcuaServer](n.configLookup, n.serverID, n.status, resolveConfigParams{
		NodeKind:   "opcua-read",
		NodeID:     n.config.ID,
		ConfigKind: "server",
		TypeLabel:  "an OPC UA server",
	})
	if err != nil {
		return err
	}
	n.server = server

	if n.status != nil {
		n.server.RegisterStatusFunc(n.status)
	}

	if n.mode == "static" {
		n.mu.Lock()
		n.stopCh = make(chan struct{})
		n.doneCh = make(chan struct{})
		stop := n.stopCh
		done := n.doneCh
		n.mu.Unlock()
		go n.runStatic(stop, done)
	}

	slog.Info("opcua-read started", "node_id", n.config.ID, "mode", n.mode, "node_count", len(n.nodeIDs))
	return nil
}

// prewarmStructs runs PrewarmStructForVariable for every NodeID in parallel
// with a single shared timeout. Best-effort; never blocks the caller.
func (n *OpcuaReadNode) prewarmStructs(ids []string) {
	ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	defer cancel()
	var wg sync.WaitGroup
	for _, s := range ids {
		parsed, err := ParseOpcuaNodeID(s)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(p *ua.NodeID) {
			defer wg.Done()
			n.server.PrewarmStructForVariable(ctx, p)
		}(parsed)
	}
	wg.Wait()
}

func (n *OpcuaReadNode) Stop() error {
	n.mu.Lock()
	stop := n.stopCh
	done := n.doneCh
	n.stopCh = nil
	n.doneCh = nil
	n.mu.Unlock()

	if stop != nil {
		close(stop)
		<-done
	}
	return nil
}

// HandleMessage drives Read in triggered/dynamic mode. Static mode ignores
// inputs (port count is 0 anyway, but a misconfigured wiring shouldn't crash).
func (n *OpcuaReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	switch n.mode {
	case "triggered":
		outs := n.doRead(n.nodeIDs)
		if len(outs) == 0 {
			return nil, nil
		}
		return [][]*flow.Message{outs}, nil
	case "dynamic":
		ids := extractNodeIDs(msg)
		if len(ids) == 0 {
			ids = n.nodeIDs
		}
		if len(ids) == 0 {
			return nil, nil
		}
		outs := n.doRead(ids)
		if len(outs) == 0 {
			return nil, nil
		}
		return [][]*flow.Message{outs}, nil
	}
	return nil, nil
}

func (n *OpcuaReadNode) runStatic(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	emit := func() {
		for _, msg := range n.doRead(n.nodeIDs) {
			n.send(0, msg)
		}
	}

	if n.startupRead {
		emit()
	}
	if n.interval <= 0 {
		<-stop
		return
	}
	t := time.NewTicker(n.interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			emit()
		}
	}
}

// doRead executes one Read service call against the shared session and turns
// the response into one or more flow.Messages according to the configured
// output shape. Returns an empty slice when the call cannot be made (server
// not connected) so callers can decide to skip emission.
//
// Output shapes:
//   - "per-item": one Message per NodeID — payload is the value, all
//     metadata fields (statusCode, dataType, timestamps) sit at the
//     message root. Use this when downstream nodes process each variable
//     independently.
//   - "single":   one Message; payload is the first NodeID's value (legacy
//     shape, sensible only for 1-NodeID configurations).
//   - "array":    one Message; payload is an array of result records.
//   - "object":   one Message; payload is a NodeID → value (or full record)
//     map.
func (n *OpcuaReadNode) doRead(nodeIDStrs []string) []*flow.Message {
	client := n.server.Client()
	if client == nil {
		if n.status != nil {
			n.status("red", "no session")
		}
		return nil
	}

	// Synchronous schema prewarm before the actual Read so server-defined
	// structures arrive with their bytes intact and the codec can decode
	// them. The cache makes second and later calls effectively free.
	n.prewarmStructs(nodeIDStrs)

	// Pre-parse and pair NodeID strings with their parsed forms so per-item
	// errors can be reported in the output without poisoning the whole batch.
	type pair struct {
		raw    string
		parsed *ua.NodeID
		err    error
	}
	pairs := make([]pair, 0, len(nodeIDStrs))
	toRead := make([]*ua.ReadValueID, 0, len(nodeIDStrs))
	for _, s := range nodeIDStrs {
		id, err := ParseOpcuaNodeID(s)
		pairs = append(pairs, pair{raw: s, parsed: id, err: err})
		if err == nil {
			toRead = append(toRead, &ua.ReadValueID{NodeID: id, AttributeID: n.attribute})
		}
	}

	var resp *ua.ReadResponse
	if len(toRead) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
		var err error
		resp, err = client.Read(ctx, &ua.ReadRequest{
			TimestampsToReturn: ua.TimestampsToReturnBoth,
			NodesToRead:        toRead,
		})
		cancel()
		if err != nil {
			if n.status != nil {
				n.status("red", err.Error())
			}
			slog.Warn("opcua-read service error", "node_id", n.config.ID, "error", err)
			return nil
		}
	}

	// Walk pairs, pulling results in order from resp.Results when the parse
	// succeeded and synthesising a BadNodeIdInvalid entry otherwise.
	results := make([]map[string]any, 0, len(pairs))
	respIdx := 0
	for _, p := range pairs {
		entry := map[string]any{"nodeId": p.raw}
		if p.err != nil {
			entry["statusCode"] = "BadNodeIDInvalid"
			entry["statusCodeRaw"] = uint32(ua.StatusBadNodeIDInvalid)
			entry["value"] = nil
			results = append(results, entry)
			continue
		}
		if resp == nil || respIdx >= len(resp.Results) {
			entry["statusCode"] = "BadInternalError"
			entry["statusCodeRaw"] = uint32(ua.StatusBadInternalError)
			entry["value"] = nil
			results = append(results, entry)
			continue
		}
		dv := resp.Results[respIdx]
		respIdx++

		entry["statusCode"] = OpcuaStatusCodeName(dv.Status)
		entry["statusCodeRaw"] = uint32(dv.Status)
		if dv.Value != nil {
			converted := OpcuaValueToJSON(dv.Value, n.server)
			entry["value"] = converted
			entry["dataType"] = OpcuaTypeName(dv.Value.Type())
			// Server-defined ExtensionObjects sometimes return a Bad status on
			// Read (e.g. BadDataTypeIDUnknown) even though the wire payload was
			// successfully decoded against the schema. In that case the data
			// is sound — surface Good and keep the original server status as
			// a diagnostic field so the user can still see what happened.
			if dv.Value.Type() == ua.TypeIDExtensionObject {
				if m, ok := converted.(map[string]any); ok {
					if _, hasErr := m["_decodeError"]; !hasErr {
						if _, hasRaw := m["_raw"]; !hasRaw && len(m) > 0 {
							if !OpcuaStatusCodeIsGood(dv.Status) {
								entry["serverStatusCode"] = entry["statusCode"]
								entry["serverStatusCodeRaw"] = entry["statusCodeRaw"]
								entry["statusCode"] = "Good"
								entry["statusCodeRaw"] = uint32(0)
							}
						}
					}
				}
			}
		} else {
			entry["value"] = nil
		}
		if !dv.SourceTimestamp.IsZero() {
			entry["sourceTimestamp"] = dv.SourceTimestamp.UTC().Format(time.RFC3339Nano)
		}
		if !dv.ServerTimestamp.IsZero() {
			entry["serverTimestamp"] = dv.ServerTimestamp.UTC().Format(time.RFC3339Nano)
		}
		results = append(results, entry)
	}

	if n.outputShape == "per-item" {
		return n.shapePerItem(results)
	}

	out := flow.NewMessage()
	out.Set("payload", n.shapePayload(results))

	allGood := true
	for _, r := range results {
		if name, ok := r["statusCode"].(string); ok && name != "Good" {
			allGood = false
			break
		}
	}
	out.Set("allGood", allGood)
	return []*flow.Message{out}
}

// shapePerItem turns the per-NodeID result list into one flow.Message per
// entry. payload is the bare value; metadata (nodeId, statusCode, etc.) is
// spread on the message root so consumers can route on it without dipping
// into the payload.
func (n *OpcuaReadNode) shapePerItem(results []map[string]any) []*flow.Message {
	out := make([]*flow.Message, 0, len(results))
	for _, r := range results {
		msg := flow.NewMessage()
		if n.includeMetadata {
			for k, v := range r {
				msg.Set(k, v)
			}
		} else {
			if id, ok := r["nodeId"].(string); ok {
				msg.Set("nodeId", id)
			}
		}
		msg.Set("payload", r["value"])
		out = append(out, msg)
	}
	return out
}

// shapePayload turns the canonical per-NodeID list of result maps into the
// payload form selected by the user. "single" and "array"/"object" with
// includeMetadata=false strip everything but the value(s).
func (n *OpcuaReadNode) shapePayload(results []map[string]any) any {
	switch n.outputShape {
	case "single":
		if len(results) == 0 {
			return nil
		}
		first := results[0]
		if n.includeMetadata {
			return first
		}
		return first["value"]
	case "object":
		obj := make(map[string]any, len(results))
		for _, r := range results {
			id, _ := r["nodeId"].(string)
			if n.includeMetadata {
				obj[id] = r
			} else {
				obj[id] = r["value"]
			}
		}
		return obj
	default: // "array"
		if n.includeMetadata {
			out := make([]any, len(results))
			for i, r := range results {
				out[i] = r
			}
			return out
		}
		out := make([]any, len(results))
		for i, r := range results {
			out[i] = r["value"]
		}
		return out
	}
}

// extractNodeIDs reads msg.nodeIds in either string-or-string-array form. An
// empty/missing field returns nil so callers can fall back to the config list.
func extractNodeIDs(msg *flow.Message) []string {
	switch v := msg.Get("nodeIds").(type) {
	case nil:
		return nil
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// readStringSlice normalises the JSON-decoded form of a string-array config
// field. Workspace JSON delivers []any with string elements; in tests we may
// pass []string directly.
func readStringSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	}
	return nil
}

// OpcuaReadTypeInfo registers the node type with the palette.
func OpcuaReadTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "opcua-read",
		Category:    "industrial",
		Label:       "OPC UA Read",
		Description: "Read OPC UA node attributes",
		Icon:        "mdi-database-arrow-down",
		Defaults: map[string]any{
			"server":          "",
			"mode":            "triggered",
			"nodeIds":         []string{},
			"attribute":       "Value",
			"outputShape":     "array",
			"includeMetadata": true,
			"interval":        1000,
			"startupRead":     true,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
