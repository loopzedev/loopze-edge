// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// kvProvider is a local interface for accessing the underlying NATS KV bucket
// from a ContextStore. This avoids importing NATS types into the flow package.
type kvProvider interface {
	KeyValue() jetstream.KeyValue
}

// ContextWatchNode watches a NATS KV context store for changes and emits
// messages downstream when keys matching a pattern are created, updated, or deleted.
// It is a source node (0 inputs, 1 output).
type ContextWatchNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Context stores received via ContextProvider.
	globalMem  flow.ContextStore
	globalPers flow.ContextStore
	flowMem    flow.ContextStore
	flowPers   flow.ContextStore

	// Parsed config.
	scope       string // "global" or "flow"
	keyPattern  string // NATS KV watch pattern (e.g. ">" for all)
	storage     string // "memory" or "persistent"
	emitDeletes bool   // whether to emit messages for delete operations

	done chan struct{}
	wg   sync.WaitGroup
}

// NewContextWatchNode is the NodeFactory for the context-watch node type.
func NewContextWatchNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &ContextWatchNode{
		config: config,
		done:   make(chan struct{}),
	}, nil
}

// Init parses the node configuration.
func (n *ContextWatchNode) Init() error {
	props := n.config.Properties

	n.scope = "global"
	if v, ok := props["scope"].(string); ok && v != "" {
		n.scope = v
	}

	n.keyPattern = ">"
	if v, ok := props["keyPattern"].(string); ok && v != "" {
		n.keyPattern = v
	}

	n.storage = "memory"
	if v, ok := props["storage"].(string); ok && v != "" {
		n.storage = v
	}

	if v, ok := props["emitDeletes"].(bool); ok {
		n.emitDeletes = v
	}

	return nil
}

func (n *ContextWatchNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *ContextWatchNode) SetStatus(fn flow.StatusFunc)  { n.status = fn }
func (n *ContextWatchNode) SetDebug(fn flow.DebugFunc)    { n.debug = fn }

// SetContext implements flow.ContextProvider to receive context stores.
func (n *ContextWatchNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.globalMem = globalMem
	n.globalPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start selects the configured context store, starts a NATS KV watcher,
// and launches a goroutine that emits messages for each change event.
func (n *ContextWatchNode) Start() error {
	if n.send == nil {
		return fmt.Errorf("context-watch node %s: send function not set", n.config.ID)
	}

	store := n.pickStore()
	if store == nil {
		return fmt.Errorf("context-watch node %s: no %s/%s context store available", n.config.ID, n.scope, n.storage)
	}

	kvp, ok := store.(kvProvider)
	if !ok {
		return fmt.Errorf("context-watch node %s: context store does not support watch", n.config.ID)
	}

	kv := kvp.KeyValue()

	// Start watching — UpdatesOnly skips replaying existing values.
	var watcher jetstream.KeyWatcher
	var err error
	if n.keyPattern == ">" || n.keyPattern == "*" || n.keyPattern == "" {
		watcher, err = kv.WatchAll(context.Background(), jetstream.UpdatesOnly())
	} else {
		watcher, err = kv.Watch(context.Background(), n.keyPattern, jetstream.UpdatesOnly())
	}
	if err != nil {
		return fmt.Errorf("context-watch node %s: failed to start watcher: %w", n.config.ID, err)
	}

	n.wg.Add(1)
	go n.watchLoop(watcher)

	if n.status != nil {
		n.status("green", "watching")
	}

	slog.Info("context-watch node started",
		"node_id", n.config.ID,
		"scope", n.scope,
		"storage", n.storage,
		"pattern", n.keyPattern,
	)
	return nil
}

// HandleMessage is a no-op for source nodes.
func (n *ContextWatchNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// Stop signals the watcher goroutine to exit and waits for cleanup.
func (n *ContextWatchNode) Stop() error {
	close(n.done)
	n.wg.Wait()
	slog.Info("context-watch node stopped", "node_id", n.config.ID)
	return nil
}

func (n *ContextWatchNode) pickStore() flow.ContextStore {
	if n.scope == "flow" {
		if n.storage == "persistent" {
			return n.flowPers
		}
		return n.flowMem
	}
	// default: global
	if n.storage == "persistent" {
		return n.globalPers
	}
	return n.globalMem
}

func (n *ContextWatchNode) watchLoop(watcher jetstream.KeyWatcher) {
	defer n.wg.Done()
	defer watcher.Stop()

	updates := watcher.Updates()
	for {
		select {
		case <-n.done:
			return
		case entry, ok := <-updates:
			if !ok {
				// Watcher channel closed — try to reconnect after a delay.
				slog.Warn("context-watch watcher channel closed, stopping",
					"node_id", n.config.ID,
				)
				if n.status != nil {
					n.status("red", "disconnected")
				}
				return
			}
			if entry == nil {
				continue // nil sentinel means initial values done (shouldn't happen with UpdatesOnly)
			}
			n.handleEntry(entry)
		}
	}
}

func (n *ContextWatchNode) handleEntry(entry jetstream.KeyValueEntry) {
	op := entry.Operation()

	// Skip deletes if not configured.
	if !n.emitDeletes && (op == jetstream.KeyValueDelete || op == jetstream.KeyValuePurge) {
		return
	}

	msg := flow.NewMessage()
	msg.SetTopic(entry.Key())
	msg.Set("key", entry.Key())
	msg.Set("scope", n.scope)
	msg.Set("storage", n.storage)
	msg.Set("timestamp", entry.Created().UTC().Format(time.RFC3339Nano))

	switch op {
	case jetstream.KeyValuePut:
		msg.Set("operation", "put")
		// Deserialize the JSON value.
		var val any
		if err := json.Unmarshal(entry.Value(), &val); err != nil {
			val = string(entry.Value())
		}
		msg.SetPayload(val)
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		msg.Set("operation", "delete")
		msg.SetPayload(nil)
	}

	n.send(0, msg)
}

// ContextWatchTypeInfo returns the NodeTypeInfo for registering the context-watch node.
func ContextWatchTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "context-watch",
		Category:    "context",
		Label:       "Context Watch",
		Description: "Watches for changes in context stores and emits change events",
		Icon:        "mdi-eye",
		Defaults: map[string]any{
			"scope":       "global",
			"keyPattern":  ">",
			"storage":     "memory",
			"emitDeletes": false,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
