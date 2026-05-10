// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// monitoredSpec captures the user-side description of a single MonitoredItem.
// It survives across reconnects so the subscription can be rebuilt verbatim.
type monitoredSpec struct {
	nodeID           string
	name             string // optional user-supplied display name
	parsed           *ua.NodeID
	attribute        ua.AttributeID
	samplingInterval float64 // ms; -1 = server default, 0 = "as fast as possible"
	queueSize        uint32
	discardOldest    bool
	deadbandType     uint32  // 0 none, 1 absolute, 2 percent
	deadbandValue    float64
}

// itemState ties a server-assigned MonitoredItemID to its spec so notifications
// can be reverse-mapped to a NodeID without a per-message lookup.
type itemState struct {
	spec       monitoredSpec
	handle     uint32 // client handle we requested
	monitorID  uint32 // server-assigned ID, populated after CreateMonitoredItems
}

// OpcuaSubscribeNode mirrors a set of OPC UA MonitoredItems into the flow.
// Two modes:
//
//   - "static":  items come from the config; created at Start, recreated on
//                reconnect, never reshaped at runtime.
//   - "dynamic": no items at start; control messages with msg.action =
//                "subscribe"|"unsubscribe"|"clear" reshape the active set.
//                Notifications are emitted on the output regardless of mode;
//                control messages themselves are NOT forwarded.
//
// Implements flow.ConfigProvider.
type OpcuaSubscribeNode struct {
	config       flow.NodeConfig
	BaseNode
	configLookup flow.ConfigLookupFunc

	serverID    string
	mode        string
	configItems []monitoredSpec
	subParams   opcua.SubscriptionParameters
	outputShape string // "per-item" | "batch"

	server *OpcuaServer

	mu          sync.Mutex
	subscription *opcua.Subscription
	items       map[uint32]*itemState // handle → state
	nextHandle  uint32

	// rebuildMu serialises rebuildSubscription so the start path and the
	// reconnect-callback path can't race two Subscribe calls in flight —
	// the server gets two CreateMonitoredItems requests and (observed on
	// real servers) hangs both, manifesting as "context deadline exceeded".
	rebuildMu sync.Mutex

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewOpcuaSubscribeNode is the registry factory.
func NewOpcuaSubscribeNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &OpcuaSubscribeNode{config: config, items: make(map[uint32]*itemState)}, nil
}

func (n *OpcuaSubscribeNode) Init() error {
	props := n.config.Properties

	n.serverID, _ = props["server"].(string)
	if n.serverID == "" {
		return fmt.Errorf("opcua-subscribe %s: no server configured", n.config.ID)
	}

	mode, _ := props["mode"].(string)
	switch mode {
	case "":
		mode = "static"
	case "static", "dynamic":
		// ok
	default:
		return fmt.Errorf("opcua-subscribe %s: invalid mode %q", n.config.ID, mode)
	}
	n.mode = mode

	shape, _ := props["outputShape"].(string)
	switch shape {
	case "":
		shape = "per-item"
	case "per-item", "batch":
		// ok
	default:
		return fmt.Errorf("opcua-subscribe %s: invalid outputShape %q", n.config.ID, shape)
	}
	n.outputShape = shape

	if mode == "static" {
		raw, _ := props["monitoredItems"].([]any)
		if len(raw) == 0 {
			return fmt.Errorf("opcua-subscribe %s: at least one monitoredItem is required in static mode", n.config.ID)
		}
		items, err := parseMonitoredSpecs(raw)
		if err != nil {
			return fmt.Errorf("opcua-subscribe %s: %w", n.config.ID, err)
		}
		n.configItems = items
	}

	n.subParams = opcua.SubscriptionParameters{
		Interval:          time.Duration(getFloat(props, "publishingInterval", 500)) * time.Millisecond,
		LifetimeCount:     uint32(getFloat(props, "lifetimeCount", 60)),
		MaxKeepAliveCount: uint32(getFloat(props, "keepAliveCount", 10)),
		Priority:          uint8(getFloat(props, "priority", 0)),
	}

	return nil
}

func parseMonitoredSpecs(raw []any) ([]monitoredSpec, error) {
	out := make([]monitoredSpec, 0, len(raw))
	for i, r := range raw {
		entry, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("monitoredItems[%d] is not an object", i)
		}
		spec, err := monitoredSpecFromMap(entry)
		if err != nil {
			return nil, fmt.Errorf("monitoredItems[%d]: %w", i, err)
		}
		out = append(out, spec)
	}
	return out, nil
}

func monitoredSpecFromMap(entry map[string]any) (monitoredSpec, error) {
	nodeID, _ := entry["nodeId"].(string)
	if nodeID == "" {
		return monitoredSpec{}, fmt.Errorf("nodeId is required")
	}
	parsed, err := ParseOpcuaNodeID(nodeID)
	if err != nil {
		return monitoredSpec{}, err
	}
	attrName, _ := entry["attribute"].(string)
	attr, _ := OpcuaAttributeIDFromName(attrName)
	if attrName == "" {
		attr = ua.AttributeIDValue
	}

	name, _ := entry["name"].(string)
	spec := monitoredSpec{
		nodeID:           nodeID,
		name:             name,
		parsed:           parsed,
		attribute:        attr,
		samplingInterval: getFloat(entry, "samplingInterval", 1000),
		queueSize:        uint32(getFloat(entry, "queueSize", 1)),
		discardOldest:    true,
	}
	if v, ok := entry["discardOldest"].(bool); ok {
		spec.discardOldest = v
	}
	if dbRaw, ok := entry["deadband"].(map[string]any); ok {
		switch dbRaw["type"] {
		case "absolute":
			spec.deadbandType = uint32(ua.DeadbandTypeAbsolute)
		case "percent":
			spec.deadbandType = uint32(ua.DeadbandTypePercent)
		default:
			spec.deadbandType = uint32(ua.DeadbandTypeNone)
		}
		spec.deadbandValue = getFloat(dbRaw, "value", 0)
	}
	return spec, nil
}

// getFloat reads a number-typed config field with a fallback when missing or
// the wrong type. JSON-decoded numbers always arrive as float64.
func getFloat(m map[string]any, key string, fallback float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int); ok {
		return float64(v)
	}
	return fallback
}

func (n *OpcuaSubscribeNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *OpcuaSubscribeNode) Start() error {
	server, err := ResolveConfigInstance[OpcuaServer](n.configLookup, n.serverID, n.Status, ResolveConfigParams{
		NodeKind:   "opcua-subscribe",
		NodeID:     n.config.ID,
		ConfigKind: "server",
		TypeLabel:  "an OPC UA server",
	})
	if err != nil {
		return err
	}
	n.server = server

	if n.Status != nil {
		n.server.RegisterStatusFunc(n.Status)
	}
	// Recreate the subscription on every reconnect — gopcua tears it down on
	// disconnect and re-registers nothing for us. Items are stored locally so
	// we can replay them.
	n.server.RegisterReconnectCallback(func() {
		n.rebuildSubscription()
	})

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.doneCh = make(chan struct{})
	n.mu.Unlock()

	// Seed the spec list from the config in static mode. Dynamic-mode nodes
	// start empty and grow on demand.
	if n.mode == "static" {
		n.mu.Lock()
		for _, spec := range n.configItems {
			handle := n.allocHandleLocked()
			n.items[handle] = &itemState{spec: spec, handle: handle}
		}
		n.mu.Unlock()
	}

	// Kick off the initial subscription only if the server is already
	// connected — the gopcua Client is allocated synchronously but its
	// session is activated asynchronously, and Subscribe before activation
	// returns BadSessionIDInvalid and leaves the publish loop in a half-state
	// where later Monitor calls hang forever. Pre-deploy / cold-deploy paths
	// are picked up by the reconnect callback we registered above.
	if fill, _ := n.server.Status(); fill == "green" {
		go n.rebuildSubscription()
	}

	slog.Info("opcua-subscribe started", "node_id", n.config.ID, "mode", n.mode)
	return nil
}

func (n *OpcuaSubscribeNode) Stop() error {
	n.mu.Lock()
	stop := n.stopCh
	done := n.doneCh
	sub := n.subscription
	n.subscription = nil
	n.stopCh = nil
	n.doneCh = nil
	n.mu.Unlock()

	if stop != nil {
		close(stop)
	}
	if sub != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = sub.Cancel(ctx)
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

// HandleMessage handles control messages in dynamic mode. In static mode
// inputs are ignored (port count is 0 anyway).
func (n *OpcuaSubscribeNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if n.mode != "dynamic" {
		return nil, nil
	}
	action, _ := msg.Get("action").(string)
	switch action {
	case "subscribe":
		specs := specsFromMessage(msg)
		n.replaceItems(specs)
	case "unsubscribe":
		n.removeItems(extractNodeIDs(msg))
	case "clear":
		n.replaceItems(nil)
	default:
		// Anything else is a no-op — control messages must opt in explicitly.
	}
	return nil, nil
}

// specsFromMessage builds monitoredSpecs from msg.payload in any of the
// accepted forms: a NodeID string, a string array, or an object/object-array
// with full configuration fields.
func specsFromMessage(msg *flow.Message) []monitoredSpec {
	payload := msg.Get("payload")
	switch v := payload.(type) {
	case nil:
		return nil
	case string:
		if v == "" {
			return nil
		}
		spec, err := monitoredSpecFromMap(map[string]any{"nodeId": v})
		if err != nil {
			return nil
		}
		applyMessageDefaults(&spec, msg)
		return []monitoredSpec{spec}
	case []any:
		out := make([]monitoredSpec, 0, len(v))
		for _, e := range v {
			switch x := e.(type) {
			case string:
				if spec, err := monitoredSpecFromMap(map[string]any{"nodeId": x}); err == nil {
					applyMessageDefaults(&spec, msg)
					out = append(out, spec)
				}
			case map[string]any:
				if spec, err := monitoredSpecFromMap(x); err == nil {
					out = append(out, spec)
				}
			}
		}
		return out
	case []string:
		out := make([]monitoredSpec, 0, len(v))
		for _, s := range v {
			if spec, err := monitoredSpecFromMap(map[string]any{"nodeId": s}); err == nil {
				applyMessageDefaults(&spec, msg)
				out = append(out, spec)
			}
		}
		return out
	case map[string]any:
		if spec, err := monitoredSpecFromMap(v); err == nil {
			return []monitoredSpec{spec}
		}
	}
	return nil
}

// applyMessageDefaults lets msg-level overrides (publishingInterval,
// samplingInterval, queueSize, deadband) fill in fields the user didn't set
// on each individual NodeID — so a "subscribe everything at 100ms" call only
// has to specify it once.
func applyMessageDefaults(spec *monitoredSpec, msg *flow.Message) {
	if v, ok := msg.Get("samplingInterval").(float64); ok {
		spec.samplingInterval = v
	}
	if v, ok := msg.Get("queueSize").(float64); ok {
		spec.queueSize = uint32(v)
	}
	if db, ok := msg.Get("deadband").(map[string]any); ok {
		switch db["type"] {
		case "absolute":
			spec.deadbandType = uint32(ua.DeadbandTypeAbsolute)
		case "percent":
			spec.deadbandType = uint32(ua.DeadbandTypePercent)
		}
		spec.deadbandValue = getFloat(db, "value", spec.deadbandValue)
	}
}

// replaceItems is the dynamic-mode "subscribe" implementation: tear down all
// current items, register the new ones. v1 doesn't diff — simplicity over
// micro-optimisation; the brief allows callers to retain unchanged items in
// later versions.
func (n *OpcuaSubscribeNode) replaceItems(specs []monitoredSpec) {
	n.mu.Lock()
	sub := n.subscription
	oldIDs := make([]uint32, 0, len(n.items))
	for _, it := range n.items {
		if it.monitorID != 0 {
			oldIDs = append(oldIDs, it.monitorID)
		}
	}
	n.items = make(map[uint32]*itemState, len(specs))
	newStates := make([]*itemState, 0, len(specs))
	for _, spec := range specs {
		handle := n.allocHandleLocked()
		st := &itemState{spec: spec, handle: handle}
		n.items[handle] = st
		newStates = append(newStates, st)
	}
	n.mu.Unlock()

	if sub == nil {
		return // will be picked up on the next reconnect callback
	}

	if len(oldIDs) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
		_, _ = sub.Unmonitor(ctx, oldIDs...)
		cancel()
	}
	if len(newStates) > 0 {
		_ = n.createItemsOn(sub, newStates)
	}
	n.updateActiveStatus()
}

// removeItems unsubscribes the named NodeIDs without touching the rest.
func (n *OpcuaSubscribeNode) removeItems(nodeIDs []string) {
	if len(nodeIDs) == 0 {
		return
	}
	want := make(map[string]struct{}, len(nodeIDs))
	for _, s := range nodeIDs {
		want[s] = struct{}{}
	}

	n.mu.Lock()
	sub := n.subscription
	toUnmonitor := make([]uint32, 0)
	for handle, it := range n.items {
		if _, hit := want[it.spec.nodeID]; !hit {
			continue
		}
		if it.monitorID != 0 {
			toUnmonitor = append(toUnmonitor, it.monitorID)
		}
		delete(n.items, handle)
	}
	n.mu.Unlock()

	if sub != nil && len(toUnmonitor) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
		_, _ = sub.Unmonitor(ctx, toUnmonitor...)
		cancel()
	}
	n.updateActiveStatus()
}

// rebuildSubscription is the central recovery path: called once at start and
// again after every reconnect. It cancels the old Subscription (if any),
// creates a fresh one, and re-registers every item we still know about.
//
// Serialised through rebuildMu: if a build is already running, the second
// caller drops out — the running one already reads from the latest items
// map under n.mu and reflects whatever state existed when its work began,
// so coalescing concurrent calls is safe.
func (n *OpcuaSubscribeNode) rebuildSubscription() {
	if !n.rebuildMu.TryLock() {
		slog.Debug("opcua-subscribe rebuild already in progress, skipping",
			"node_id", n.config.ID)
		return
	}
	defer n.rebuildMu.Unlock()

	client := n.server.Client()
	if client == nil {
		slog.Debug("opcua-subscribe rebuild skipped: no client", "node_id", n.config.ID)
		return // nothing we can do until the connection comes up
	}

	n.mu.Lock()
	oldSub := n.subscription
	n.subscription = nil
	stop := n.stopCh
	specs := make([]*itemState, 0, len(n.items))
	for _, it := range n.items {
		it.monitorID = 0 // reset; new subscription assigns new IDs
		specs = append(specs, it)
	}
	n.mu.Unlock()

	if oldSub != nil {
		ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
		_ = oldSub.Cancel(ctx)
		cancel()
	}
	if stop == nil {
		return // node was stopped between the callback and here
	}

	// Subscribe with a long-lived background context — the cancel below is
	// only used to stop our local watcher goroutine, not the subscription
	// itself (gopcua's publish loop has its own client-scoped context).
	notifyCh := make(chan *opcua.PublishNotificationData, 64)
	sub, err := client.Subscribe(context.Background(), &n.subParams, notifyCh)
	if err != nil {
		if n.Status != nil {
			n.Status("red", err.Error())
		}
		slog.Warn("opcua-subscribe Subscribe failed", "node_id", n.config.ID, "error", err)
		return
	}
	slog.Info("opcua-subscribe subscription created",
		"node_id", n.config.ID,
		"sub_id", sub.SubscriptionID,
		"interval_ms", sub.RevisedPublishingInterval.Milliseconds(),
		"specs", len(specs))

	n.mu.Lock()
	n.subscription = sub
	n.mu.Unlock()

	// One consumer per rebuild — earlier consumers exit naturally when their
	// notifyCh is closed by the cancelled subscription path below. The
	// stop-channel guards against post-Stop() leaks if the channel never
	// reaches close.
	go n.consumeNotifications(notifyCh, stop)

	if len(specs) > 0 {
		if err := n.createItemsOn(sub, specs); err != nil {
			slog.Warn("opcua-subscribe createItemsOn failed",
				"node_id", n.config.ID, "error", err)
		}
	}
	n.updateActiveStatus()
}

// createItemsOn submits a CreateMonitoredItems request and stitches the
// returned MonitoredItemIDs back onto the local itemState list. Per-item
// failures are logged but do not fail the batch.
//
// Before submitting the create request we run a best-effort schema prewarm
// for every item: if the variable's DataType is a server-defined structure
// (ExtensionObject), we load its definition and register the encoding NodeID
// with gopcua. Without that step the first incoming notification arrives
// with the body silently stripped by gopcua's "unknown extobj type" path
// and the user sees only {typeId} in their flow message.
func (n *OpcuaSubscribeNode) createItemsOn(sub *opcua.Subscription, states []*itemState) error {
	if sub == nil || len(states) == 0 {
		return nil
	}

	// Schema prewarm: parallel reads with one shared timeout so the slowest
	// schema doesn't block Monitor for longer than RequestTimeout.
	prewarmCtx, prewarmCancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	var prewarmWG sync.WaitGroup
	for _, st := range states {
		prewarmWG.Add(1)
		go func(parsed *ua.NodeID) {
			defer prewarmWG.Done()
			n.server.PrewarmStructForVariable(prewarmCtx, parsed)
		}(st.spec.parsed)
	}
	prewarmWG.Wait()
	prewarmCancel()
	reqs := make([]*ua.MonitoredItemCreateRequest, 0, len(states))
	for _, st := range states {
		slog.Debug("opcua-subscribe item params",
			"node_id", n.config.ID,
			"item", st.spec.nodeID,
			"client_handle", st.handle,
			"sampling_interval", st.spec.samplingInterval,
			"queue_size", st.spec.queueSize,
			"discard_oldest", st.spec.discardOldest,
			"deadband_type", st.spec.deadbandType,
			"deadband_value", st.spec.deadbandValue,
			"attribute_id", uint32(st.spec.attribute))
		params := &ua.MonitoringParameters{
			ClientHandle:     st.handle,
			SamplingInterval: st.spec.samplingInterval,
			QueueSize:        st.spec.queueSize,
			DiscardOldest:    st.spec.discardOldest,
		}
		if st.spec.deadbandType != uint32(ua.DeadbandTypeNone) {
			params.Filter = ua.NewExtensionObject(&ua.DataChangeFilter{
				Trigger:       ua.DataChangeTriggerStatusValue,
				DeadbandType:  st.spec.deadbandType,
				DeadbandValue: st.spec.deadbandValue,
			})
		}
		reqs = append(reqs, &ua.MonitoredItemCreateRequest{
			ItemToMonitor: &ua.ReadValueID{
				NodeID:       st.spec.parsed,
				AttributeID:  st.spec.attribute,
				DataEncoding: &ua.QualifiedName{},
			},
			MonitoringMode:      ua.MonitoringModeReporting,
			RequestedParameters: params,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.server.RequestTimeout())
	defer cancel()
	resp, err := sub.Monitor(ctx, ua.TimestampsToReturnBoth, reqs...)
	if err != nil {
		if n.Status != nil {
			n.Status("red", err.Error())
		}
		return err
	}

	n.mu.Lock()
	good, bad := 0, 0
	for i, res := range resp.Results {
		if i >= len(states) {
			break
		}
		if !OpcuaStatusCodeIsGood(res.StatusCode) {
			bad++
			slog.Warn("opcua-subscribe item rejected",
				"node_id", n.config.ID,
				"item", states[i].spec.nodeID,
				"status", OpcuaStatusCodeName(res.StatusCode))
			continue
		}
		states[i].monitorID = res.MonitoredItemID
		good++
		slog.Debug("opcua-subscribe item registered",
			"node_id", n.config.ID,
			"item", states[i].spec.nodeID,
			"client_handle", states[i].handle,
			"server_id", res.MonitoredItemID,
			"sampling_ms", res.RevisedSamplingInterval,
			"queue_size", res.RevisedQueueSize)
	}
	n.mu.Unlock()
	slog.Info("opcua-subscribe items processed",
		"node_id", n.config.ID, "good", good, "bad", bad)
	return nil
}

// consumeNotifications turns server PublishNotificationData into Flow
// messages. A single PublishResponse can carry many MonitoredItemNotifications;
// per-item mode emits one Flow message per change, batch mode bundles them.
//
// Exits when the node is Stop()'d (stop channel closed) — gopcua does not
// close the notify channel itself, so the stop-channel guard prevents the
// goroutine from leaking after Stop.
func (n *OpcuaSubscribeNode) consumeNotifications(ch <-chan *opcua.PublishNotificationData, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			if data == nil {
				continue
			}
			if data.Error != nil {
				slog.Warn("opcua-subscribe notification error",
					"node_id", n.config.ID,
					"sub_id", data.SubscriptionID,
					"error", data.Error)
				continue
			}
			switch v := data.Value.(type) {
			case *ua.DataChangeNotification:
				if v == nil {
					slog.Warn("opcua-subscribe nil DataChangeNotification",
						"node_id", n.config.ID, "sub_id", data.SubscriptionID)
					continue
				}
				slog.Debug("opcua-subscribe data change",
					"node_id", n.config.ID,
					"sub_id", data.SubscriptionID,
					"items", len(v.MonitoredItems))
				n.dispatchDataChange(v)
			case *ua.EventNotificationList:
				// Events are out of scope for v1 (separate issue).
			default:
				slog.Debug("opcua-subscribe ignored notification value",
					"node_id", n.config.ID, "type", fmt.Sprintf("%T", v))
			}
		}
	}
}

// dispatchDataChange resolves each MonitoredItemNotification back to its
// NodeID via the client handle and writes Flow messages according to the
// configured output shape.
func (n *OpcuaSubscribeNode) dispatchDataChange(dcn *ua.DataChangeNotification) {
	if dcn == nil || len(dcn.MonitoredItems) == 0 {
		return
	}
	if n.Send == nil {
		slog.Warn("opcua-subscribe dispatch skipped: no send func",
			"node_id", n.config.ID)
		return
	}

	type entry struct {
		spec monitoredSpec
		dv   *ua.DataValue
	}
	resolved := make([]entry, 0, len(dcn.MonitoredItems))

	n.mu.Lock()
	missingHandles := make([]uint32, 0)
	for _, m := range dcn.MonitoredItems {
		st, ok := n.items[m.ClientHandle]
		if !ok {
			missingHandles = append(missingHandles, m.ClientHandle)
			continue
		}
		resolved = append(resolved, entry{spec: st.spec, dv: m.Value})
	}
	knownHandles := make([]uint32, 0, len(n.items))
	for h := range n.items {
		knownHandles = append(knownHandles, h)
	}
	n.mu.Unlock()

	if len(missingHandles) > 0 {
		slog.Warn("opcua-subscribe notification with unknown client handles",
			"node_id", n.config.ID,
			"unknown", missingHandles,
			"known", knownHandles)
	}
	if len(resolved) == 0 {
		return
	}

	if n.outputShape == "batch" {
		batch := make([]any, len(resolved))
		for i, e := range resolved {
			batch[i] = n.buildItemRecord(e.spec, e.dv)
		}
		out := flow.NewMessage()
		out.Set("payload", batch)
		n.Send(0, out)
		return
	}

	for _, e := range resolved {
		out := flow.NewMessage()
		rec := n.buildItemRecord(e.spec, e.dv)
		for k, v := range rec {
			out.Set(k, v)
		}
		// Convenience: the value sits both inside the record and at
		// msg.payload so consumers can wire either way.
		out.Set("payload", rec["value"])
		n.Send(0, out)
	}
}

func (n *OpcuaSubscribeNode) buildItemRecord(spec monitoredSpec, dv *ua.DataValue) map[string]any {
	rec := map[string]any{
		"nodeId": spec.nodeID,
	}
	if spec.name != "" {
		rec["name"] = spec.name
	}
	if dv == nil {
		rec["statusCode"] = "BadInternalError"
		rec["statusCodeRaw"] = uint32(ua.StatusBadInternalError)
		rec["value"] = nil
		return rec
	}
	rec["statusCode"] = OpcuaStatusCodeName(dv.Status)
	rec["statusCodeRaw"] = uint32(dv.Status)
	if dv.Value != nil {
		converted := OpcuaValueToJSON(dv.Value, n.server)
		rec["value"] = converted
		rec["dataType"] = OpcuaTypeName(dv.Value.Type())
		// Override server-side Bad status when our schema-driven decoder
		// successfully turned the wire bytes into structured fields. See
		// opcua_read.go.doRead for the same trick — server limitations
		// reporting BadDataTypeIDUnknown shouldn't gate working data.
		if dv.Value.Type() == ua.TypeIDExtensionObject {
			if m, ok := converted.(map[string]any); ok {
				if _, hasErr := m["_decodeError"]; !hasErr {
					if _, hasRaw := m["_raw"]; !hasRaw && len(m) > 0 {
						if !OpcuaStatusCodeIsGood(dv.Status) {
							rec["serverStatusCode"] = rec["statusCode"]
							rec["serverStatusCodeRaw"] = rec["statusCodeRaw"]
							rec["statusCode"] = "Good"
							rec["statusCodeRaw"] = uint32(0)
						}
					}
				}
			}
		}
	} else {
		rec["value"] = nil
	}
	if !dv.SourceTimestamp.IsZero() {
		rec["sourceTimestamp"] = dv.SourceTimestamp.UTC().Format(time.RFC3339Nano)
	}
	if !dv.ServerTimestamp.IsZero() {
		rec["serverTimestamp"] = dv.ServerTimestamp.UTC().Format(time.RFC3339Nano)
	}
	return rec
}

// allocHandleLocked produces a fresh, never-zero ClientHandle. Caller holds n.mu.
func (n *OpcuaSubscribeNode) allocHandleLocked() uint32 {
	n.nextHandle++
	if n.nextHandle == 0 {
		n.nextHandle = 1
	}
	return n.nextHandle
}

// updateActiveStatus reports the live item count so the editor surfaces a
// useful number even when the server-level status is otherwise unchanged.
func (n *OpcuaSubscribeNode) updateActiveStatus() {
	if n.Status == nil {
		return
	}
	n.mu.Lock()
	count := len(n.items)
	connected := n.subscription != nil
	n.mu.Unlock()

	if !connected {
		return
	}
	if count == 0 {
		n.Status("green", "active · idle")
	} else {
		n.Status("green", fmt.Sprintf("active · %d items", count))
	}
}

// OpcuaSubscribeTypeInfo registers the node type with the palette.
func OpcuaSubscribeTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "opcua-subscribe",
		Category:    "industrial",
		Label:       "OPC UA Subscribe",
		Description: "Subscribe to OPC UA MonitoredItems with push updates",
		Icon:        "mdi-bell-outline",
		Defaults: map[string]any{
			"server":             "",
			"mode":               "static",
			"monitoredItems":     []any{},
			"publishingInterval": 500,
			"lifetimeCount":      60,
			"keepAliveCount":     10,
			"priority":           0,
			"outputShape":        "per-item",
		},
		Inputs:  0,
		Outputs: 1,
	}
}
