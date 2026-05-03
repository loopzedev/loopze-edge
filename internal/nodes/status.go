// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// idleStatusText is shown on the Status Node when no status event has been
// observed recently.
const idleStatusText = "listening"

// statusBlinkDuration is how long the Status Node displays the last forwarded
// status before reverting to the idle text.
const statusBlinkDuration = 2 * time.Second

// StatusNode is a source node that observes status updates of other nodes in
// the engine and emits each observation as a flow message. It has 0 inputs
// and 1 output.
//
// Scope:
//   - "flow" (default): only status events from nodes in the same flow
//   - "selected": only status events from a configured set of node IDs;
//     scope is always restricted to the same flow as the Status Node so
//     selecting nodes from foreign flows is not possible
//   - "all": status events from all flows
//
// Status events emitted by Status Nodes themselves are filtered out at the
// engine level, so feedback loops between Status Nodes are impossible
// regardless of scope.
type StatusNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	scope       string              // "flow", "selected" or "all"
	targetNodes map[string]struct{} // populated when scope == "selected"

	register   func(flow.StatusListenerFunc) func()
	unregister func()

	// blinkSeq increments on every forwarded status; the most recent
	// reset goroutine compares its captured seq against the current one
	// to avoid clobbering a newer blink.
	blinkSeq atomic.Uint64

	mu      sync.Mutex
	started bool
}

// NewStatusNode is the NodeFactory for the status node type.
func NewStatusNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &StatusNode{
		config: config,
		scope:  "flow",
	}, nil
}

// Init parses and validates the node configuration.
func (n *StatusNode) Init() error {
	if v, ok := n.config.Properties["scope"].(string); ok {
		switch v {
		case "flow", "selected", "all":
			n.scope = v
		default:
			slog.Warn("status node: unknown scope, falling back to flow",
				"node_id", n.config.ID, "scope", v)
		}
	}

	if raw, ok := n.config.Properties["targetNodes"].([]any); ok {
		n.targetNodes = make(map[string]struct{}, len(raw))
		for _, item := range raw {
			if id, ok := item.(string); ok && id != "" {
				n.targetNodes[id] = struct{}{}
			}
		}
	}
	return nil
}

// SetSend stores the engine-provided callback for sending messages downstream.
func (n *StatusNode) SetSend(fn flow.SendFunc) {
	n.send = fn
}

// SetStatus stores the engine-provided callback for reporting node status.
func (n *StatusNode) SetStatus(fn flow.StatusFunc) {
	n.status = fn
}

// SetDebug stores the engine-provided callback for emitting debug messages.
func (n *StatusNode) SetDebug(fn flow.DebugFunc) {
	n.debug = fn
}

// SetStatusListener implements flow.StatusListenerProvider. The engine calls
// this once per wire pass; if a previous registration exists (from an earlier
// wire pass during a modified-nodes deploy) it is released first to avoid
// listener leaks.
func (n *StatusNode) SetStatusListener(register func(flow.StatusListenerFunc) func()) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.register = register

	// If we are already started and re-wired, replace the active subscription.
	if n.started {
		if n.unregister != nil {
			n.unregister()
		}
		n.unregister = register(n.handleStatus)
	}
}

// Start activates the status listener and sets the idle status.
func (n *StatusNode) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.register == nil {
		// No engine-provided registration: nothing we can do, but don't fail
		// deployment — the node will simply stay idle.
		slog.Warn("status node: no listener registration available",
			"node_id", n.config.ID)
		n.started = true
		if n.status != nil {
			n.status("yellow", "no listener")
		}
		return nil
	}

	// Drop any previous registration before installing a new one.
	if n.unregister != nil {
		n.unregister()
	}
	n.unregister = n.register(n.handleStatus)
	n.started = true

	if n.status != nil {
		n.status("green", idleStatusText)
	}
	return nil
}

// HandleMessage is a no-op: the Status Node has no inputs, but the engine's
// nodeLoop is still allocated for it and may receive trigger messages from
// future API features. Returning nil signals "no downstream messages".
func (n *StatusNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// Stop releases the listener subscription.
func (n *StatusNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.unregister != nil {
		n.unregister()
		n.unregister = nil
	}
	n.started = false
	return nil
}

// handleStatus is invoked by the engine fan-out for every (non-status-node)
// status update in the engine. It runs synchronously on the goroutine that
// emitted the status — keep it cheap.
func (n *StatusNode) handleStatus(sm flow.StatusMessage) {
	switch n.scope {
	case "flow":
		if sm.FlowID != n.config.FlowID {
			return
		}
	case "selected":
		// Selection is always scoped to the Status Node's own flow:
		// foreign flows are never observable via "selected".
		if sm.FlowID != n.config.FlowID {
			return
		}
		if _, ok := n.targetNodes[sm.NodeID]; !ok {
			return
		}
	case "all":
		// no filter
	}
	if n.send == nil {
		return
	}

	msg := flow.NewMessage()
	msg.Set("status", map[string]any{
		"fill": sm.Status.Fill,
		"text": sm.Status.Text,
		"source": map[string]any{
			"id":     sm.NodeID,
			"type":   sm.SourceType,
			"name":   sm.SourceName,
			"flowId": sm.FlowID,
		},
	})
	msg.SetPayload(sm.Status.Text)

	n.send(0, msg)
	n.blink(sm)
}

// blink shows the most recent forwarded status on the Status Node itself for
// statusBlinkDuration, then reverts to the idle text — provided no newer
// blink has started in the meantime.
func (n *StatusNode) blink(sm flow.StatusMessage) {
	if n.status == nil {
		return
	}

	seq := n.blinkSeq.Add(1)
	label := sm.SourceName
	if label == "" {
		label = sm.NodeID
	}
	text := label + ": " + sm.Status.Fill
	n.status("grey", truncate(text, 32))

	go func() {
		time.Sleep(statusBlinkDuration)
		if n.blinkSeq.Load() != seq {
			return
		}
		n.status("green", idleStatusText)
	}()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// StatusTypeInfo returns the NodeTypeInfo for registering the status node.
func StatusTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "status",
		Category:    "common",
		Label:       "Status",
		Description: "Emits status events of other nodes as flow messages",
		Icon:        "mdi-pulse",
		Defaults: map[string]any{
			"scope": "flow",
		},
		Inputs:  0,
		Outputs: 1,
	}
}
