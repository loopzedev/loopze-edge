// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package flow

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/niceclouds/flint/internal/config"
)

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

	mu      sync.RWMutex
	flows   []Flow // all flows in the workspace
	running bool

	// stopCh signals all running flow goroutines to shut down.
	stopCh chan struct{}
}

// NewEngine creates a new flow runtime engine with the given configuration
// and an empty node registry. Register node types on the returned engine's
// Registry() before calling Deploy.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		cfg:      cfg,
		registry: NewNodeRegistry(),
		stopCh:   make(chan struct{}),
	}
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

	slog.Info("flow engine stopping", "active_flows", len(e.flows))

	// Signal all goroutines to stop.
	close(e.stopCh)

	// TODO: iterate over running node instances and call Stop() on each.
	// TODO: wait for all node goroutines to finish (use sync.WaitGroup).

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

	slog.Info("deploying flows",
		"flow_count", len(flows),
		"total_nodes", countNodes(flows),
	)

	// TODO: Step 1 — Stop currently running flows and node instances.
	// TODO: Step 2 — Validate all node types in the new flows are registered.
	// TODO: Step 3 — Create NodeInstance for each node using the registry factory.
	// TODO: Step 4 — Call Init() on each NodeInstance.
	// TODO: Step 5 — Wire output channels between connected nodes.
	// TODO: Step 6 — Start a goroutine per node, calling Start() and then
	//                 listening for incoming messages on the node's input channel.
	// TODO: Step 7 — Store the active flow state for introspection.

	e.flows = flows

	slog.Info("flows deployed successfully",
		"flow_count", len(flows),
	)

	return nil
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
