// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package flow

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/niceclouds/flint/internal/config"
)

// PublishDebugFunc is a callback the server provides to the engine so that
// debug messages can be published externally (e.g. to NATS) without the
// engine needing a direct dependency on the messaging infrastructure.
// subject is the NATS subject (e.g. "debug.flow1.node42").
type PublishDebugFunc func(subject string, msg DebugMessage)

// runningNode holds the state for a single instantiated node in a deployed flow.
type runningNode struct {
	instance NodeInstance
	config   NodeConfig
	flowID   string
	inputCh  chan *Message // receives messages from upstream nodes
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
	publishDebug     PublishDebugFunc   // injected by server for NATS publishing
	globalCtxMemory  ContextStore       // volatile in-memory global context store
	globalCtxPersist ContextStore       // file-backed persistent global context store
	flowCtxFactory   FlowContextFactory // creates dedicated KV stores per flow ID
	running      bool
	wg           sync.WaitGroup
	stopCh       chan struct{}
}

// NewEngine creates a new flow runtime engine with the given configuration
// and an empty node registry. Register node types on the returned engine's
// Registry() before calling Deploy.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		cfg:      cfg,
		registry: NewNodeRegistry(),
		nodes:    make(map[string]*runningNode),
		wires:    make(map[string][][]string),
		stopCh:   make(chan struct{}),
	}
}

// SetPublishDebug sets the callback used to publish debug messages externally.
// Must be called before Deploy.
func (e *Engine) SetPublishDebug(fn PublishDebugFunc) {
	e.publishDebug = fn
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
	e.stopCh = make(chan struct{})

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

	e.stopNodes()

	e.running = false
	e.flows = nil

	slog.Info("flow engine stopped")
	return nil
}

// Deploy accepts a workspace (all flows), stops any currently running
// flows, instantiates nodes from the registry, wires them together, and
// starts execution. All flows in the workspace run concurrently.
//
// This implements a full-restart deploy strategy: all flows are stopped and
// restarted. A future optimisation could diff the old and new flows to only
// restart changed flows (modified-nodes deploy).
func (e *Engine) Deploy(flows []Flow) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return fmt.Errorf("engine: cannot deploy, engine is not running")
	}

	totalNodes := countNodes(flows)
	slog.Info("deploying flows",
		"flow_count", len(flows),
		"total_nodes", totalNodes,
	)

	// Step 1 — Stop currently running nodes.
	e.stopNodes()

	// Step 2 — Validate all node types are registered.
	for _, f := range flows {
		for _, n := range f.Nodes {
			if !e.registry.Has(n.Type) {
				slog.Warn("unknown node type, skipping", "type", n.Type, "node_id", n.ID)
			}
		}
	}

	// Step 3+4 — Create and init NodeInstances.
	e.nodes = make(map[string]*runningNode, totalNodes)
	e.wires = make(map[string][][]string, totalNodes)

	for _, f := range flows {
		if f.Disabled {
			slog.Debug("skipping disabled flow", "flow_id", f.ID, "label", f.Label)
			continue
		}

		for _, n := range f.Nodes {
			if n.Disabled {
				slog.Debug("skipping disabled node", "node_id", n.ID, "type", n.Type)
				continue
			}

			factory, ok := e.registry.Get(n.Type)
			if !ok {
				continue
			}

			nc := NodeConfig{
				ID:         n.ID,
				Type:       n.Type,
				Name:       n.Name,
				Properties: n.Config,
			}

			instance, err := factory(nc)
			if err != nil {
				slog.Error("failed to create node instance",
					"node_id", n.ID, "type", n.Type, "error", err)
				continue
			}

			if err := instance.Init(); err != nil {
				slog.Error("failed to init node",
					"node_id", n.ID, "type", n.Type, "error", err)
				continue
			}

			rn := &runningNode{
				instance: instance,
				config:   nc,
				flowID:   f.ID,
				inputCh:  make(chan *Message, 64),
			}
			e.nodes[n.ID] = rn
			e.wires[n.ID] = n.Wires
		}
	}

	// Step 5 — Wire: build callbacks for each node.
	// Cache per-flow context stores so the factory is called at most once per flow ID.
	type flowCtxPair struct{ mem, pers ContextStore }
	flowCtxCache := make(map[string]flowCtxPair)

	for nodeID, rn := range e.nodes {
		nodeWires := e.wires[nodeID]
		rn.instance.SetSend(e.makeSendFunc(nodeID, nodeWires))
		rn.instance.SetStatus(e.makeStatusFunc(nodeID))
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

	// Step 6 — Start each node in its own goroutine.
	e.stopCh = make(chan struct{})

	for nodeID, rn := range e.nodes {
		if err := rn.instance.Start(); err != nil {
			slog.Error("failed to start node",
				"node_id", nodeID, "type", rn.config.Type, "error", err)
			e.publishNodeError(nodeID, rn, err)
			continue
		}

		// Start a message listener goroutine for nodes that accept inputs.
		// Source nodes (inputs=0) only produce messages via SendFunc.
		typeInfo, hasInfo := e.registry.GetTypeInfo(rn.config.Type)
		if !hasInfo || typeInfo.Inputs > 0 {
			e.wg.Add(1)
			go e.nodeLoop(nodeID, rn)
		}
	}

	e.flows = flows

	slog.Info("flows deployed successfully",
		"flow_count", len(flows),
		"active_nodes", len(e.nodes),
	)

	return nil
}

// makeSendFunc creates a SendFunc closure for a specific node that routes
// messages to downstream nodes based on the wire configuration.
func (e *Engine) makeSendFunc(sourceID string, wires [][]string) SendFunc {
	return func(port int, msg *Message) {
		if port < 0 || port >= len(wires) {
			return
		}
		targets := wires[port]
		for _, targetID := range targets {
			targetNode, ok := e.nodes[targetID]
			if !ok {
				slog.Warn("wire target not found",
					"source", sourceID, "target", targetID, "port", port)
				continue
			}
			// Non-blocking send; drop if buffer full.
			select {
			case targetNode.inputCh <- msg:
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

// makeStatusFunc creates a StatusFunc closure for a specific node.
func (e *Engine) makeStatusFunc(nodeID string) StatusFunc {
	return func(fill string, text string) {
		slog.Debug("node status",
			"node_id", nodeID, "fill", fill, "text", text)
		// TODO: broadcast status via WebSocket to frontend
	}
}

// nodeLoop runs in a goroutine for each node that accepts input messages.
// It reads from the node's input channel, calls HandleMessage, and routes
// output messages to downstream nodes.
func (e *Engine) nodeLoop(nodeID string, rn *runningNode) {
	defer e.wg.Done()

	nodeWires := e.wires[nodeID]

	for {
		select {
		case <-e.stopCh:
			return
		case msg, ok := <-rn.inputCh:
			if !ok {
				return
			}
			outputs, err := rn.instance.HandleMessage(msg)
			if err != nil {
				slog.Error("node HandleMessage error",
					"node_id", nodeID, "error", err)
				e.publishNodeError(nodeID, rn, err)
				continue
			}
			// Route output messages to downstream nodes.
			for port, msgs := range outputs {
				if port >= len(nodeWires) {
					continue
				}
				for _, targetID := range nodeWires[port] {
					targetNode, ok := e.nodes[targetID]
					if !ok {
						continue
					}
					for _, outMsg := range msgs {
						select {
						case targetNode.inputCh <- outMsg:
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

// stopNodes stops all currently running node instances, waits for goroutines
// to finish, and clears the active node state.
func (e *Engine) stopNodes() {
	if len(e.nodes) == 0 {
		return
	}

	slog.Debug("stopping active nodes", "count", len(e.nodes))

	// Signal all nodeLoop goroutines to exit.
	close(e.stopCh)
	e.wg.Wait()

	// Call Stop() on each node instance.
	for nodeID, rn := range e.nodes {
		if err := rn.instance.Stop(); err != nil {
			slog.Error("error stopping node",
				"node_id", nodeID, "type", rn.config.Type, "error", err)
		}
		close(rn.inputCh)
	}

	e.nodes = make(map[string]*runningNode)
	e.wires = make(map[string][][]string)
}

// TriggerNode sends a nil message to a running node's HandleMessage,
// used for manual triggering (e.g. inject button in the editor).
func (e *Engine) TriggerNode(nodeID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rn, ok := e.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %q not found in running deployment", nodeID)
	}

	// Call HandleMessage directly with nil (inject ignores the input).
	_, err := rn.instance.HandleMessage(nil)
	return err
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
