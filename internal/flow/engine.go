// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loopzedev/loopze-edge/internal/config"
)

// PublishDebugFunc is a callback the server provides to the engine so that
// debug messages can be published externally (e.g. to NATS) without the
// engine needing a direct dependency on the messaging infrastructure.
// subject is the NATS subject (e.g. "debug.flow1.node42").
type PublishDebugFunc func(subject string, msg DebugMessage)

// statusCache stores the last known status for each node, protected by a RWMutex
// for safe concurrent access from node goroutines (writers) and API handlers (readers).
type statusCache struct {
	mu      sync.RWMutex
	entries map[string]StatusMessage
}

func (c *statusCache) Set(nodeID string, msg StatusMessage) {
	c.mu.Lock()
	c.entries[nodeID] = msg
	c.mu.Unlock()
}

func (c *statusCache) GetAll() map[string]StatusMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]StatusMessage, len(c.entries))
	for k, v := range c.entries {
		result[k] = v
	}
	return result
}

func (c *statusCache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]StatusMessage)
	c.mu.Unlock()
}

func (c *statusCache) Delete(nodeID string) {
	c.mu.Lock()
	delete(c.entries, nodeID)
	c.mu.Unlock()
}

// runningNode holds the state for a single instantiated node in a deployed flow.
type runningNode struct {
	instance NodeInstance
	config   NodeConfig
	flowID   string
	inputCh  chan *Message   // receives messages from upstream nodes
	stopCh   chan struct{}   // per-node stop signal
	done     chan struct{}   // closed when goroutine exits

	// Local snapshots set during wireAllNodes(), read lock-free from nodeLoop.
	localWires   [][]string               // snapshot of wires for this node
	localTargets map[string]*runningNode   // targetNodeID → runningNode pointer

	// errorFn is the per-node error callback installed during wireAllNodes,
	// used by nodeLoop for synchronous errors and by ErrorProvider nodes for
	// asynchronous errors raised from background goroutines.
	errorFn ErrorFunc
}

// Engine is the core flow runtime that manages the lifecycle of deployed flows.
// It is responsible for instantiating nodes, wiring them together, and
// coordinating message passing between nodes via Go channels.
//
// Architecture notes:
//   - Each node runs in its own goroutine (goroutine-per-node model).
//   - Node-to-node communication within a flow uses Go channels (nanosecond latency).
//   - External communication (debug, context, fleet) uses embedded NATS.
//   - A workspace contains multiple flows; all flows are deployed together.
type Engine struct {
	cfg      *config.Config
	registry *NodeRegistry

	mu    sync.RWMutex
	flows []Flow // all flows in the workspace

	// Active deployment state.
	nodes        map[string]*runningNode // nodeID → running node
	wires        map[string][][]string   // nodeID → wires[outputPort] = [targetNodeIDs]
	linkRegistry    map[string]*runningNode // nodeID → running link node (link-in, link-out, link-call)
	configs         []ConfigNode           // config node definitions from last deploy
	configInstances map[string]ConfigInstance // configID → running config instance
	statusCache     statusCache            // last known status per node
	publishDebug     PublishDebugFunc   // injected by server for NATS publishing
	publishStatus    PublishStatusFunc  // injected by server for NATS publishing
	globalCtxMemory  ContextStore       // volatile in-memory global context store
	globalCtxPersist ContextStore       // file-backed persistent global context store
	flowCtxFactory   FlowContextFactory // creates dedicated KV stores per flow ID

	// Status fan-out: in-process listeners that observe all node status updates
	// (except updates from Status Nodes themselves; see makeStatusFunc).
	statusListenersMu sync.RWMutex
	statusListenerSeq uint64
	statusListeners   map[uint64]StatusListenerFunc

	// Error fan-out: in-process listeners that observe all node errors
	// (except errors raised by Catch Nodes themselves; see makeErrorFunc).
	errorListenersMu sync.RWMutex
	errorListenerSeq uint64
	errorListeners   map[uint64]ErrorListenerFunc

	// HTTP flow-endpoint integration. Nil when no server is configured
	// (e.g. unit tests of the engine). responseRegistry is engine-owned
	// so handles survive Link-Out → Link-In hops across flows.
	responseRegistry *ResponseRegistry
	httpMuxBuilder   HTTPMuxBuilder
	httpRoot         string

	running bool
}

// NewEngine creates a new flow runtime engine with the given configuration
// and an empty node registry. Register node types on the returned engine's
// Registry() before calling Deploy.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		cfg:      cfg,
		registry: NewNodeRegistry(),
		nodes:           make(map[string]*runningNode),
		wires:           make(map[string][][]string),
		linkRegistry:    make(map[string]*runningNode),
		statusCache:      statusCache{entries: make(map[string]StatusMessage)},
		statusListeners:  make(map[uint64]StatusListenerFunc),
		errorListeners:   make(map[uint64]ErrorListenerFunc),
		responseRegistry: NewResponseRegistry(),
	}
}

// SetPublishDebug sets the callback used to publish debug messages externally.
// Must be called before Deploy.
func (e *Engine) SetPublishDebug(fn PublishDebugFunc) {
	e.publishDebug = fn
}

// SetPublishStatus sets the callback used to publish status messages externally.
// Must be called before Deploy.
func (e *Engine) SetPublishStatus(fn PublishStatusFunc) {
	e.publishStatus = fn
}

// SetContextStores provides the engine with the global memory and persistent
// ContextStores that are injected into nodes implementing ContextProvider.
// Must be called before Deploy.
func (e *Engine) SetContextStores(memory, persistent ContextStore) {
	e.globalCtxMemory = memory
	e.globalCtxPersist = persistent
}

// SetFlowContextFactory provides a factory the engine calls once per unique
// flow ID during Deploy to obtain dedicated KV stores for that flow.
// Must be called before Deploy.
func (e *Engine) SetFlowContextFactory(fn FlowContextFactory) {
	e.flowCtxFactory = fn
}

// Registry returns the node type registry associated with this engine.
// Use it to register node factories before deploying flows.
func (e *Engine) Registry() *NodeRegistry {
	return e.registry
}

// SetHTTPMuxBuilder wires the engine to the server-owned flow-endpoint
// mux. The builder is invoked once per deploy with the route specs
// collected from every HTTPInProvider node; it atomically swaps the
// live route table and reports any conflicts. root is the configured
// URL prefix (e.g. "/endpoint") and is forwarded to HTTPMuxProvider
// nodes for use in status text. Must be called before Deploy.
func (e *Engine) SetHTTPMuxBuilder(builder HTTPMuxBuilder, root string) {
	e.httpMuxBuilder = builder
	e.httpRoot = root
}

// ResponseRegistry returns the engine-owned response registry. The
// server uses this to publish slot lifecycle events; tests use it to
// inspect slot counts. Always non-nil.
func (e *Engine) ResponseRegistry() *ResponseRegistry {
	return e.responseRegistry
}

// Start initialises the engine and prepares it for flow deployment.
// It does not deploy any flows — call Deploy() to activate flows.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("engine: already running")
	}

	slog.Info("flow engine starting",
		"registered_node_types", e.registry.Count(),
	)

	e.running = true

	// Start the response-registry sweeper so http-in slots can be
	// reaped on timeout. No-op if Run has already been called.
	if e.responseRegistry != nil {
		e.responseRegistry.Run()
	}

	slog.Info("flow engine started")
	return nil
}

// Stop gracefully shuts down the engine, stopping all running flows and
// releasing resources. It is safe to call Stop on an engine that is not running.
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		slog.Debug("engine: stop called but engine is not running")
		return nil
	}

	slog.Info("flow engine stopping", "active_nodes", len(e.nodes))

	e.stopAllNodes()

	// Cancel any HTTP routes still registered with the server-side mux,
	// drain in-flight slots with a 503, and stop the sweeper. Order:
	// swap to an empty router first so new requests get a 404, then
	// drain so any handler still blocked on <-done returns immediately.
	if e.httpMuxBuilder != nil {
		_ = e.httpMuxBuilder(nil)
	}
	if e.responseRegistry != nil {
		e.responseRegistry.DrainAll()
		e.responseRegistry.Stop()
	}

	e.running = false
	e.flows = nil

	slog.Info("flow engine stopped")
	return nil
}

// Deploy accepts a workspace (all flows), applies the given deploy mode,
// and starts execution. The mode controls which nodes are restarted.
func (e *Engine) Deploy(flows []Flow, configs []ConfigNode, mode DeployMode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("engine: cannot deploy, engine is not running")
	}

	totalNodes := countNodes(flows)
	slog.Info("deploying flows",
		"flow_count", len(flows),
		"total_nodes", totalNodes,
		"mode", string(mode),
	)

	switch mode {
	case DeployModifiedNodes:
		return e.deployModifiedNodes(flows, configs)
	case DeployModifiedFlows:
		return e.deployModifiedFlows(flows, configs)
	default:
		return e.deployFull(flows, configs)
	}
}

// ─── Deploy modes ───────────────────────────────────────────────────────────

// deployFull stops all running nodes and redeploys the entire workspace.
// This is the original deploy strategy.
func (e *Engine) deployFull(flows []Flow, configs []ConfigNode) error {
	e.stopAllNodes()

	// Validate all node types are registered.
	for _, f := range flows {
		for _, n := range f.Nodes {
			if !e.registry.Has(n.Type) {
				slog.Warn("unknown node type, skipping", "type", n.Type, "node_id", n.ID)
			}
		}
	}

	// Instantiate and start config nodes (before regular nodes).
	e.deployConfigs(configs)

	// Create and init NodeInstances.
	e.nodes = make(map[string]*runningNode, countNodes(flows))
	e.wires = make(map[string][][]string, countNodes(flows))

	for _, f := range flows {
		if f.Disabled {
			slog.Debug("skipping disabled flow", "flow_id", f.ID, "label", f.Label)
			continue
		}
		for _, n := range f.Nodes {
			e.instantiateNode(n, f.ID)
		}
	}

	// Wire and start all nodes.
	e.wireAllNodes()
	e.startAllNodeLoops()
	e.rebuildHTTPMux()

	e.flows = flows
	e.configs = configs

	slog.Info("flows deployed successfully",
		"flow_count", len(flows),
		"config_count", len(configs),
		"active_nodes", len(e.nodes),
	)
	return nil
}

// deployModifiedFlows restarts only flows that contain changes.
// Unmodified flows keep running without interruption.
func (e *Engine) deployModifiedFlows(flows []Flow, configs []ConfigNode) error {
	// First deploy ever: fall back to full deploy.
	if e.flows == nil {
		return e.deployFull(flows, configs)
	}

	oldWs := Workspace{Flows: e.flows, Configs: e.configs}
	newWs := Workspace{Flows: flows, Configs: configs}
	diff := DiffWorkspaces(oldWs, newWs)

	if diff.IsEmpty() {
		slog.Info("no changes detected, skipping deploy")
		e.flows = flows
		e.configs = configs
		return nil
	}

	// Handle config node changes.
	e.applyConfigDiff(diff, configs)

	// Determine which flows are affected.
	affectedFlows := make(map[string]bool)
	for _, id := range diff.RemovedFlows {
		affectedFlows[id] = true
	}
	for _, id := range diff.ModifiedFlows {
		affectedFlows[id] = true
	}
	for _, id := range diff.AddedFlows {
		affectedFlows[id] = true
	}

	// Expand: if a config changed, all flows containing dependent nodes are affected.
	if len(diff.ModifiedConfigs) > 0 {
		ExpandConfigDependents(&diff, newWs)
		for _, nodeID := range diff.ModifiedNodes {
			if rn, ok := e.nodes[nodeID]; ok {
				affectedFlows[rn.flowID] = true
			}
			// Also check new flows for the node's flow.
			for _, f := range flows {
				for _, n := range f.Nodes {
					if n.ID == nodeID {
						affectedFlows[f.ID] = true
					}
				}
			}
		}
	}

	// Stop nodes in affected flows.
	for flowID := range affectedFlows {
		e.stopNodesInFlow(flowID)
	}

	// Re-instantiate nodes for affected flows.
	for _, f := range flows {
		if f.Disabled || !affectedFlows[f.ID] {
			continue
		}
		for _, n := range f.Nodes {
			e.instantiateNode(n, f.ID)
		}
	}

	// Rewire all nodes (including unchanged ones whose targets may have been replaced).
	e.wireAllNodes()
	e.startAllNodeLoops()
	e.rebuildHTTPMux()

	e.flows = flows
	e.configs = configs

	slog.Info("modified-flows deploy completed",
		"affected_flows", len(affectedFlows),
		"active_nodes", len(e.nodes),
	)
	return nil
}

// deployModifiedNodes restarts only individual nodes that have changed.
// Unchanged nodes keep running without interruption.
func (e *Engine) deployModifiedNodes(flows []Flow, configs []ConfigNode) error {
	// First deploy ever: fall back to full deploy.
	if e.flows == nil {
		return e.deployFull(flows, configs)
	}

	oldWs := Workspace{Flows: e.flows, Configs: e.configs}
	newWs := Workspace{Flows: flows, Configs: configs}
	diff := DiffWorkspaces(oldWs, newWs)

	if diff.IsEmpty() {
		slog.Info("no changes detected, skipping deploy")
		e.flows = flows
		e.configs = configs
		return nil
	}

	// Handle config node changes and expand dependent nodes.
	e.applyConfigDiff(diff, configs)
	ExpandConfigDependents(&diff, newWs)

	// Build a set of all node IDs that need restart.
	restartSet := make(map[string]bool)
	for _, id := range diff.ModifiedNodes {
		restartSet[id] = true
	}
	for _, id := range diff.AddedNodes {
		restartSet[id] = true
	}

	// Stop and remove deleted + modified nodes in one bulk pass so the routing
	// rewire happens once and the stop ordering is correct.
	toStop := make([]string, 0, len(diff.RemovedNodes)+len(diff.ModifiedNodes))
	toStop = append(toStop, diff.RemovedNodes...)
	toStop = append(toStop, diff.ModifiedNodes...)
	e.stopAndRemoveNodes(toStop)

	// Instantiate added and modified nodes.
	newNodeIndex := make(map[string]struct{ node Node; flowID string })
	for _, f := range flows {
		for _, n := range f.Nodes {
			newNodeIndex[n.ID] = struct{ node Node; flowID string }{n, f.ID}
		}
	}

	for nodeID := range restartSet {
		entry, ok := newNodeIndex[nodeID]
		if !ok {
			continue
		}
		e.instantiateNode(entry.node, entry.flowID)
	}

	// Update wires for all nodes — unchanged nodes may reference new/changed targets.
	// Update the wires map from the new flow definitions.
	for _, f := range flows {
		for _, n := range f.Nodes {
			e.wires[n.ID] = n.Wires
		}
	}

	// Rewire all nodes (SendFunc needs updating when wires change).
	e.wireAllNodes()

	// Start only the new/restarted node loops. Unchanged nodes keep their goroutines.
	for nodeID := range restartSet {
		rn, ok := e.nodes[nodeID]
		if !ok {
			continue
		}
		if err := rn.instance.Start(); err != nil {
			slog.Error("failed to start node",
				"node_id", nodeID, "type", rn.config.Type, "error", err)
			e.publishNodeError(nodeID, rn, err)
			close(rn.done) // so stopAllNodes won't hang
			continue
		}
		go e.nodeLoop(nodeID, rn)
	}

	e.rebuildHTTPMux()

	e.flows = flows
	e.configs = configs

	slog.Info("modified-nodes deploy completed",
		"restarted_nodes", len(restartSet),
		"removed_nodes", len(diff.RemovedNodes),
		"active_nodes", len(e.nodes),
	)
	return nil
}

// ─── Granular node lifecycle ─────────────────────────────────────────────────

// instantiateNode creates, initialises, and registers a single node.
// It does NOT start the node's goroutine — call startAllNodeLoops or start manually.
func (e *Engine) instantiateNode(n Node, flowID string) {
	if n.Disabled {
		slog.Debug("skipping disabled node", "node_id", n.ID, "type", n.Type)
		return
	}

	factory, ok := e.registry.Get(n.Type)
	if !ok {
		return
	}

	nc := NodeConfig{
		ID:         n.ID,
		Type:       n.Type,
		Name:       n.Name,
		FlowID:     flowID,
		Properties: n.Config,
	}

	instance, err := factory(nc)
	if err != nil {
		slog.Error("failed to create node instance",
			"node_id", n.ID, "type", n.Type, "error", err)
		return
	}

	if err := instance.Init(); err != nil {
		slog.Error("failed to init node",
			"node_id", n.ID, "type", n.Type, "error", err)
		return
	}

	rn := &runningNode{
		instance: instance,
		config:   nc,
		flowID:   flowID,
		inputCh:  make(chan *Message, 64),
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	e.nodes[n.ID] = rn
	e.wires[n.ID] = n.Wires
}

// wireAllNodes sets up callbacks (Send, Status, Debug, Context, Link, Config)
// for all registered nodes. Safe to call multiple times — overwrites previous callbacks.
func (e *Engine) wireAllNodes() {
	type flowCtxPair struct{ mem, pers ContextStore }
	flowCtxCache := make(map[string]flowCtxPair)

	// First pass: build local wire snapshots and target maps for each node.
	for nodeID, rn := range e.nodes {
		nodeWires := e.wires[nodeID]
		rn.localWires = nodeWires

		targets := make(map[string]*runningNode)
		for _, portTargets := range nodeWires {
			for _, targetID := range portTargets {
				if tn, ok := e.nodes[targetID]; ok {
					targets[targetID] = tn
				}
			}
		}
		rn.localTargets = targets
	}

	// Second pass: wire callbacks using the local snapshots.
	for nodeID, rn := range e.nodes {
		rn.instance.SetSend(e.makeSendFunc(nodeID, rn.localWires, rn.localTargets))
		rn.instance.SetStatus(e.makeStatusFunc(nodeID, rn))
		rn.instance.SetDebug(e.makeDebugFunc(nodeID, rn))

		// Inject context stores for nodes that opt in via ContextProvider.
		if cp, ok := rn.instance.(ContextProvider); ok {
			var flowMem, flowPers ContextStore
			if e.flowCtxFactory != nil {
				if cached, hit := flowCtxCache[rn.flowID]; hit {
					flowMem, flowPers = cached.mem, cached.pers
				} else {
					flowMem, flowPers = e.flowCtxFactory(rn.flowID)
					flowCtxCache[rn.flowID] = flowCtxPair{flowMem, flowPers}
				}
			}
			cp.SetContext(e.globalCtxMemory, e.globalCtxPersist, flowMem, flowPers)
		}
	}

	// Build link registry for cross-flow messaging.
	e.linkRegistry = make(map[string]*runningNode)
	for nodeID, rn := range e.nodes {
		switch rn.config.Type {
		case "link-in", "link-out", "link-call":
			e.linkRegistry[nodeID] = rn
		}
	}

	// Inject LinkSendFunc for nodes that implement LinkProvider.
	linkSend := e.makeLinkSendFunc()
	for _, rn := range e.nodes {
		if lp, ok := rn.instance.(LinkProvider); ok {
			lp.SetLinkSend(linkSend)
		}
	}

	// Inject ConfigLookupFunc for nodes that implement ConfigProvider.
	configLookup := func(id string) (ConfigInstance, bool) {
		inst, ok := e.configInstances[id]
		return inst, ok
	}
	for _, rn := range e.nodes {
		if cp, ok := rn.instance.(ConfigProvider); ok {
			cp.SetConfigLookup(configLookup)
		}
	}

	// Inject status listener registration for nodes that implement
	// StatusListenerProvider (e.g. the Status Node). Re-wire is safe:
	// nodes are expected to drop their previous registration before
	// installing a new one — see StatusNode.SetStatusListener.
	for _, rn := range e.nodes {
		if slp, ok := rn.instance.(StatusListenerProvider); ok {
			slp.SetStatusListener(e.registerStatusListener)
		}
	}

	// Build the per-node error callback once and cache it on runningNode so
	// the nodeLoop can use it for synchronous errors. The same closure is
	// injected into nodes that implement ErrorProvider so they can report
	// asynchronous errors from background goroutines.
	for nodeID, rn := range e.nodes {
		rn.errorFn = e.makeErrorFunc(nodeID, rn)
		if ep, ok := rn.instance.(ErrorProvider); ok {
			ep.SetError(rn.errorFn)
		}
	}

	// Inject error listener registration for nodes that implement
	// ErrorListenerProvider (e.g. the Catch Node). Re-wire is safe:
	// nodes are expected to drop their previous registration before
	// installing a new one — see CatchNode.SetErrorListener.
	for _, rn := range e.nodes {
		if elp, ok := rn.instance.(ErrorListenerProvider); ok {
			elp.SetErrorListener(e.registerErrorListener)
		}
	}

	// Inject the response registry and configured prefix into HTTP
	// nodes (http-in for slot registration; http-response for slot
	// resolution). When the engine has no response registry (defensive,
	// it normally always has one), nodes still receive nil and can
	// no-op — but that's an unsupported deployment.
	for _, rn := range e.nodes {
		if hp, ok := rn.instance.(HTTPMuxProvider); ok {
			hp.SetHTTPMux(e.responseRegistry, e.httpRoot)
		}
	}
}

// rebuildHTTPMux collects the HTTP route specs from every HTTPInProvider
// node and asks the server's builder to atomically swap the live route
// table. After the swap, any in-flight request slots from the previous
// route table are drained with a 503 so blocked handlers return.
//
// No-op when no builder has been injected (engine-only unit tests).
func (e *Engine) rebuildHTTPMux() {
	if e.httpMuxBuilder == nil {
		return
	}

	specs := make([]HTTPRouteSpec, 0)
	for nodeID, rn := range e.nodes {
		hp, ok := rn.instance.(HTTPInProvider)
		if !ok {
			continue
		}
		spec := hp.HTTPRoute()
		spec.NodeID = nodeID
		specs = append(specs, spec)
	}

	conflicts := e.httpMuxBuilder(specs)

	// Drain in-flight slots from the previous route table so any
	// handlers still blocked on <-done return with a 503.
	if e.responseRegistry != nil {
		e.responseRegistry.DrainAll()
	}

	for _, c := range conflicts {
		rn, ok := e.nodes[c.NodeID]
		if !ok || rn.errorFn == nil {
			continue
		}
		rn.errorFn(fmt.Errorf("http-in route conflict: %s", c.Reason), nil)
	}
}

// startAllNodeLoops starts a goroutine for every node whose goroutine
// is not already running (done channel still open, stopCh still open).
func (e *Engine) startAllNodeLoops() {
	for nodeID, rn := range e.nodes {
		// Skip nodes that already have a running goroutine.
		// A running goroutine has stopCh open and done open.
		// A freshly created node also has both open — we distinguish
		// by checking if done was already closed (goroutine finished).
		select {
		case <-rn.stopCh:
			// stopCh closed → was stopped, skip (shouldn't be in e.nodes)
			continue
		default:
		}

		if err := rn.instance.Start(); err != nil {
			slog.Error("failed to start node",
				"node_id", nodeID, "type", rn.config.Type, "error", err)
			e.publishNodeError(nodeID, rn, err)
			// Close done so stopAllNodes won't hang waiting for this node.
			close(rn.done)
			continue
		}

		go e.nodeLoop(nodeID, rn)
	}
}

// stopAndRemoveNode stops a single node and removes it from all engine maps.
// Convenience wrapper around stopAndRemoveNodes for the single-node case.
func (e *Engine) stopAndRemoveNode(nodeID string) {
	e.stopAndRemoveNodes([]string{nodeID})
}

// stopAndRemoveNodes stops a batch of nodes safely, even when other nodes are
// still running and might be sending to them.
//
// Order matters to avoid "send on closed channel" panics: we first detach the
// targets from the routing maps and re-wire the remaining nodes so nothing
// resolves to these nodes anymore. Only then do we stop the source goroutines
// (Inject tickers, MQTT subscribers, …) and finally close the input channels.
func (e *Engine) stopAndRemoveNodes(nodeIDs []string) {
	if len(nodeIDs) == 0 {
		return
	}

	// Snapshot the running nodes that actually exist.
	rns := make([]*runningNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if rn, ok := e.nodes[id]; ok {
			rns = append(rns, rn)
		}
	}
	if len(rns) == 0 {
		return
	}

	// Step 1: detach from routing maps so wireAllNodes won't pick them up
	// as targets anymore.
	for _, id := range nodeIDs {
		delete(e.nodes, id)
		delete(e.wires, id)
		delete(e.linkRegistry, id)
	}

	// Step 2: rewire the remaining nodes so their cached SendFuncs forget
	// the removed targets. After this, no SendFunc resolves to a removed node.
	e.wireAllNodes()

	// Refresh the flow-endpoint mux so removed http-in routes go away.
	e.rebuildHTTPMux()

	// Step 3: stop instances — terminates source goroutines (Inject tickers,
	// MQTT subscribers, …) so nothing new is enqueued anywhere.
	for _, rn := range rns {
		if err := rn.instance.Stop(); err != nil {
			slog.Error("error stopping node",
				"node_id", rn.config.ID, "type", rn.config.Type, "error", err)
		}
	}

	// Step 4: stop the per-node consumer goroutines.
	for _, rn := range rns {
		close(rn.stopCh)
	}
	for _, rn := range rns {
		<-rn.done
	}

	// Step 5: close input channels — safe now, no sender remains.
	for _, rn := range rns {
		close(rn.inputCh)
	}

	// Cleanup status cache.
	for _, id := range nodeIDs {
		e.statusCache.Delete(id)
	}
}

// stopNodesInFlow stops and removes all nodes belonging to a specific flow.
func (e *Engine) stopNodesInFlow(flowID string) {
	var nodeIDs []string
	for id, rn := range e.nodes {
		if rn.flowID == flowID {
			nodeIDs = append(nodeIDs, id)
		}
	}
	e.stopAndRemoveNodes(nodeIDs)
}

// stopAllNodes stops all currently running node instances, waits for goroutines
// to finish, and clears the active node state.
//
// Order matters to avoid "send on closed channel" panics: stop the instances
// first (which terminates source goroutines like Inject tickers), then stop
// the per-node consumer goroutines, and only after all senders are gone do we
// close the input channels.
func (e *Engine) stopAllNodes() {
	if len(e.nodes) == 0 {
		return
	}

	slog.Debug("stopping active nodes", "count", len(e.nodes))

	// Step 1: stop instances — terminates source goroutines (Inject tickers,
	// MQTT subscribers, …) so nothing new is enqueued anywhere.
	for nodeID, rn := range e.nodes {
		if err := rn.instance.Stop(); err != nil {
			slog.Error("error stopping node",
				"node_id", nodeID, "type", rn.config.Type, "error", err)
		}
	}

	// Step 2: signal all nodeLoop consumer goroutines to exit.
	for _, rn := range e.nodes {
		close(rn.stopCh)
	}
	// Step 3: wait for all consumer goroutines to finish.
	for _, rn := range e.nodes {
		<-rn.done
	}

	// Step 4: close all input channels — safe now, no sender remains.
	for _, rn := range e.nodes {
		close(rn.inputCh)
	}

	e.nodes = make(map[string]*runningNode)
	e.wires = make(map[string][][]string)
	e.linkRegistry = make(map[string]*runningNode)
	e.statusCache.Clear()

	// Stop config node instances AFTER regular nodes.
	for id, inst := range e.configInstances {
		if err := inst.Stop(); err != nil {
			slog.Error("error stopping config node", "config_id", id, "error", err)
		}
	}
	e.configInstances = nil
}

// ─── Config node lifecycle ──────────────────────────────────────────────────

// deployConfigs stops all existing config instances and starts new ones.
func (e *Engine) deployConfigs(configs []ConfigNode) {
	// Stop existing config instances.
	for id, inst := range e.configInstances {
		if err := inst.Stop(); err != nil {
			slog.Error("error stopping config node", "config_id", id, "error", err)
		}
	}

	e.configInstances = make(map[string]ConfigInstance)
	for _, cfg := range configs {
		e.startConfigNode(cfg)
	}
}

// applyConfigDiff handles config node changes: stops removed/modified, starts added/modified.
func (e *Engine) applyConfigDiff(diff WorkspaceDiff, configs []ConfigNode) {
	// Index new configs for lookup.
	newCfgMap := make(map[string]ConfigNode, len(configs))
	for _, c := range configs {
		newCfgMap[c.ID] = c
	}

	// Stop removed config nodes.
	for _, id := range diff.RemovedConfigs {
		if inst, ok := e.configInstances[id]; ok {
			if err := inst.Stop(); err != nil {
				slog.Error("error stopping config node", "config_id", id, "error", err)
			}
			delete(e.configInstances, id)
		}
	}

	// Restart modified config nodes.
	for _, id := range diff.ModifiedConfigs {
		if inst, ok := e.configInstances[id]; ok {
			if err := inst.Stop(); err != nil {
				slog.Error("error stopping config node", "config_id", id, "error", err)
			}
			delete(e.configInstances, id)
		}
		if cfg, ok := newCfgMap[id]; ok {
			e.startConfigNode(cfg)
		}
	}

	// Start added config nodes.
	for _, id := range diff.AddedConfigs {
		if cfg, ok := newCfgMap[id]; ok {
			e.startConfigNode(cfg)
		}
	}
}

// GetConfigInstance returns the running config node instance for the given
// ID, or (nil, false) if nothing is registered. Used by API handlers that
// want to ride along on an already-deployed connection rather than opening
// a fresh ad-hoc session.
func (e *Engine) GetConfigInstance(id string) (ConfigInstance, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	inst, ok := e.configInstances[id]
	return inst, ok
}

// startConfigNode creates and starts a single config node instance.
func (e *Engine) startConfigNode(cfg ConfigNode) {
	factory, ok := e.registry.GetConfigFactory(cfg.Type)
	if !ok {
		slog.Warn("unknown config node type, skipping", "type", cfg.Type, "config_id", cfg.ID)
		return
	}

	instance, err := factory(cfg)
	if err != nil {
		slog.Error("failed to create config node instance",
			"config_id", cfg.ID, "type", cfg.Type, "error", err)
		return
	}

	if err := instance.Start(); err != nil {
		slog.Error("failed to start config node",
			"config_id", cfg.ID, "type", cfg.Type, "error", err)
		return
	}

	e.configInstances[cfg.ID] = instance
	slog.Info("config node started", "config_id", cfg.ID, "type", cfg.Type, "name", cfg.Name)
}

// ─── Message routing ────────────────────────────────────────────────────────

// makeSendFunc creates a SendFunc closure for a specific node that routes
// messages to downstream nodes based on the wire configuration.
func (e *Engine) makeSendFunc(sourceID string, wires [][]string, targets map[string]*runningNode) SendFunc {
	return func(port int, msg *Message) {
		if port < 0 || port >= len(wires) {
			return
		}
		for _, targetID := range wires[port] {
			targetNode := targets[targetID]
			if targetNode == nil {
				slog.Debug("wire target not active (disabled or unknown)",
					"source", sourceID, "target", targetID, "port", port)
				continue
			}
			// COW clone: the target shares data until first mutation.
			select {
			case targetNode.inputCh <- msg.COWClone():
			default:
				slog.Warn("message dropped, target buffer full",
					"source", sourceID, "target", targetID)
			}
		}
	}
}

// makeDebugFunc creates a DebugFunc closure for a specific node that publishes
// debug messages via the engine's publishDebug callback (typically to NATS).
func (e *Engine) makeDebugFunc(nodeID string, rn *runningNode) DebugFunc {
	return func(msg DebugMessage) {
		// Fill in node/flow context.
		if msg.ID == "" {
			msg.ID = generateID()
		}
		msg.NodeID = nodeID
		msg.FlowID = rn.flowID
		if msg.NodeName == "" {
			msg.NodeName = rn.config.Name
		}

		if e.publishDebug != nil {
			subject := fmt.Sprintf("debug.%s.%s", rn.flowID, nodeID)
			e.publishDebug(subject, msg)
		}
	}
}

// publishNodeError publishes a node error as an error-level debug message
// so it appears in the frontend debug panel.
func (e *Engine) publishNodeError(nodeID string, rn *runningNode, err error) {
	if e.publishDebug == nil {
		return
	}
	subject := fmt.Sprintf("debug.%s.%s", rn.flowID, nodeID)
	e.publishDebug(subject, DebugMessage{
		ID:        generateID(),
		NodeID:    nodeID,
		NodeName:  rn.config.Name,
		FlowID:    rn.flowID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    "error",
		Payload:   err.Error(),
		Format:    "string",
		Property:  "error",
	})
}

// makeStatusFunc creates a StatusFunc closure for a specific node that publishes
// status messages via the engine's publishStatus callback (typically to NATS)
// and dispatches them to in-process status listeners (e.g. the Status Node).
//
// Status updates emitted by Status Nodes themselves are intentionally excluded
// from the in-process fan-out: this prevents Status Nodes from triggering each
// other and makes feedback loops architecturally impossible. Their updates
// still reach the frontend via NATS so the node UI keeps rendering them.
func (e *Engine) makeStatusFunc(nodeID string, rn *runningNode) StatusFunc {
	return func(fill string, text string) {
		msg := StatusMessage{
			NodeID:     nodeID,
			FlowID:     rn.flowID,
			Status:     NodeStatusPayload{Fill: fill, Text: text},
			SourceType: rn.config.Type,
			SourceName: rn.config.Name,
		}
		e.statusCache.Set(nodeID, msg)
		if rn.config.Type != "status" {
			e.fanoutStatus(msg)
		}
		if e.publishStatus != nil {
			subject := fmt.Sprintf("status.%s.%s", rn.flowID, nodeID)
			e.publishStatus(subject, msg)
		}
	}
}

// registerStatusListener adds a listener to the in-process status fan-out and
// returns an unregister closure that the caller must invoke to release the
// slot (typically in the node's Stop() method).
func (e *Engine) registerStatusListener(fn StatusListenerFunc) func() {
	e.statusListenersMu.Lock()
	e.statusListenerSeq++
	id := e.statusListenerSeq
	e.statusListeners[id] = fn
	e.statusListenersMu.Unlock()
	return func() {
		e.statusListenersMu.Lock()
		delete(e.statusListeners, id)
		e.statusListenersMu.Unlock()
	}
}

// fanoutStatus dispatches a status message to all registered listeners.
// Listeners run synchronously on the caller's goroutine — they must be cheap
// and non-blocking; long work belongs in the listener's own goroutine.
func (e *Engine) fanoutStatus(msg StatusMessage) {
	e.statusListenersMu.RLock()
	defer e.statusListenersMu.RUnlock()
	for _, fn := range e.statusListeners {
		fn(msg)
	}
}

// makeErrorFunc creates an ErrorFunc closure for a specific node that publishes
// the error to the debug panel via publishNodeError and dispatches it to
// in-process error listeners (e.g. the Catch Node).
//
// Errors raised by Catch Nodes themselves are intentionally excluded from the
// in-process fan-out: this prevents Catch Nodes from triggering each other and
// makes feedback loops architecturally impossible. Their errors still reach
// the frontend debug panel via publishNodeError.
func (e *Engine) makeErrorFunc(nodeID string, rn *runningNode) ErrorFunc {
	return func(err error, msg *Message) {
		if err == nil {
			return
		}
		e.publishNodeError(nodeID, rn, err)
		// Loop guard 1: errors raised by Catch Nodes themselves never
		// fan out — Catch cannot catch Catch.
		if rn.config.Type == "catch" {
			return
		}
		// Loop guard 2: messages that already carry the _caught marker
		// originated from a Catch Node's output branch. Fanning them out
		// again would let a failing branch re-trigger any catch in scope,
		// which is a fast path to an infinite loop.
		if msg != nil {
			if v, ok := msg.Get("_caught").(bool); ok && v {
				return
			}
		}
		e.fanoutError(ErrorMessage{
			NodeID:     nodeID,
			FlowID:     rn.flowID,
			SourceType: rn.config.Type,
			SourceName: rn.config.Name,
			Error:      err.Error(),
			Msg:        msg,
		})
	}
}

// registerErrorListener adds a listener to the in-process error fan-out and
// returns an unregister closure that the caller must invoke to release the
// slot (typically in the node's Stop() method).
func (e *Engine) registerErrorListener(fn ErrorListenerFunc) func() {
	e.errorListenersMu.Lock()
	e.errorListenerSeq++
	id := e.errorListenerSeq
	e.errorListeners[id] = fn
	e.errorListenersMu.Unlock()
	return func() {
		e.errorListenersMu.Lock()
		delete(e.errorListeners, id)
		e.errorListenersMu.Unlock()
	}
}

// fanoutError dispatches an error message to all registered listeners.
// Listeners run synchronously on the caller's goroutine — they must be cheap
// and non-blocking; long work belongs in the listener's own goroutine.
func (e *Engine) fanoutError(msg ErrorMessage) {
	e.errorListenersMu.RLock()
	defer e.errorListenersMu.RUnlock()
	for _, fn := range e.errorListeners {
		fn(msg)
	}
}

// makeLinkSendFunc creates a LinkSendFunc closure that sends messages directly
// to a node by its ID via the link registry — independent of wire-based routing.
func (e *Engine) makeLinkSendFunc() LinkSendFunc {
	return func(targetNodeID string, msg *Message) {
		targetNode, ok := e.linkRegistry[targetNodeID]
		if !ok {
			slog.Warn("link target not found in registry",
				"target", targetNodeID)
			return
		}
		select {
		case targetNode.inputCh <- msg.COWClone():
		default:
			slog.Warn("link message dropped, target buffer full",
				"target", targetNodeID)
		}
	}
}

// nodeLoop runs in a goroutine for each node that accepts input messages.
// It reads from the node's input channel, calls HandleMessage, and routes
// output messages to downstream nodes.
func (e *Engine) nodeLoop(nodeID string, rn *runningNode) {
	defer close(rn.done)

	for {
		select {
		case <-rn.stopCh:
			return
		case msg, ok := <-rn.inputCh:
			if !ok {
				return
			}
			outputs, err := rn.instance.HandleMessage(msg)
			if err != nil {
				slog.Error("node HandleMessage error",
					"node_id", nodeID, "error", err)
				if rn.errorFn != nil {
					rn.errorFn(err, msg)
				} else {
					e.publishNodeError(nodeID, rn, err)
				}
				continue
			}
			// Use node-local wire snapshot (set by wireAllNodes under write lock).
			nodeWires := rn.localWires

			// Route output messages to downstream nodes.
			for port, msgs := range outputs {
				if port >= len(nodeWires) {
					continue
				}
				for _, targetID := range nodeWires[port] {
					targetNode := rn.localTargets[targetID]
					if targetNode == nil {
						continue
					}
					for _, outMsg := range msgs {
						// COW clone: the target shares data until first mutation.
						select {
						case targetNode.inputCh <- outMsg.COWClone():
						default:
							slog.Warn("message dropped, target buffer full",
								"source", nodeID, "target", targetID)
						}
					}
				}
			}
		}
	}
}

// ─── Public accessors ───────────────────────────────────────────────────────

// NodeStatuses returns a snapshot of the last known status for all nodes.
func (e *Engine) NodeStatuses() map[string]StatusMessage {
	return e.statusCache.GetAll()
}

// TriggerNode sends a trigger message to a running node's input channel,
// used for manual triggering (e.g. inject button in the editor).
// The message is processed by the node's goroutine to avoid race conditions.
func (e *Engine) TriggerNode(nodeID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rn, ok := e.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %q not found in running deployment", nodeID)
	}

	// Send a trigger message via the channel so it's processed
	// by the node's own goroutine — no concurrent access.
	trigger := NewMessage()
	trigger.Set("_trigger", true)
	select {
	case rn.inputCh <- trigger:
		return nil
	default:
		return fmt.Errorf("node %q trigger dropped, buffer full", nodeID)
	}
}

// Flows returns a copy of the currently deployed flow definitions.
func (e *Engine) Flows() []Flow {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Flow, len(e.flows))
	copy(result, e.flows)
	return result
}

// IsRunning reports whether the engine is currently started and ready for deployment.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// countNodes is a helper that returns the total number of nodes across all flows.
func countNodes(flows []Flow) int {
	total := 0
	for _, f := range flows {
		total += len(f.Nodes)
	}
	return total
}
