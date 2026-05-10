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

const catchIdleStatusText = "listening"
const catchBlinkDuration = 2 * time.Second

// CatchNode is a source node that observes errors raised by other nodes in
// the engine and emits each observation as a flow message. It has 0 inputs
// and 1 output.
//
// Scope:
//   - "flow" (default): only errors from nodes in the same flow
//   - "selected": only errors from a configured set of node IDs;
//     scope is always restricted to the same flow as the Catch Node so
//     selecting nodes from foreign flows is not possible
//   - "all": errors from all flows
//
// Errors raised by Catch Nodes themselves are filtered out at the engine
// level, so feedback loops between Catch Nodes are impossible regardless
// of scope.
type CatchNode struct {
	config flow.NodeConfig
	BaseNode
	scope       string
	targetNodes map[string]struct{}

	register   func(flow.ErrorListenerFunc) func()
	unregister func()

	blinkSeq atomic.Uint64

	mu      sync.Mutex
	started bool
}

// NewCatchNode is the NodeFactory for the catch node type.
func NewCatchNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &CatchNode{
		config: config,
		scope:  "flow",
	}, nil
}

func (n *CatchNode) Init() error {
	if v, ok := n.config.Properties["scope"].(string); ok {
		switch v {
		case "flow", "selected", "all":
			n.scope = v
		default:
			slog.Warn("catch node: unknown scope, falling back to flow",
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

// SetErrorListener implements flow.ErrorListenerProvider. The engine calls
// this once per wire pass; if a previous registration exists (from an earlier
// wire pass during a modified-nodes deploy) it is released first to avoid
// listener leaks.
func (n *CatchNode) SetErrorListener(register func(flow.ErrorListenerFunc) func()) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.register = register

	if n.started {
		if n.unregister != nil {
			n.unregister()
		}
		n.unregister = register(n.handleError)
	}
}

func (n *CatchNode) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.register == nil {
		slog.Warn("catch node: no listener registration available",
			"node_id", n.config.ID)
		n.started = true
		if n.Status != nil {
			n.Status("yellow", "no listener")
		}
		return nil
	}

	if n.unregister != nil {
		n.unregister()
	}
	n.unregister = n.register(n.handleError)
	n.started = true

	if n.Status != nil {
		n.Status("green", catchIdleStatusText)
	}
	return nil
}

// HandleMessage is a no-op: the Catch Node has no inputs. Returning nil
// signals "no downstream messages".
func (n *CatchNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

func (n *CatchNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.unregister != nil {
		n.unregister()
		n.unregister = nil
	}
	n.started = false
	return nil
}

// handleError is invoked by the engine fan-out for every error from a
// non-catch node. It runs synchronously on the goroutine that produced
// the error — keep it cheap.
func (n *CatchNode) handleError(em flow.ErrorMessage) {
	switch n.scope {
	case "flow":
		if em.FlowID != n.config.FlowID {
			return
		}
	case "selected":
		// Selection is always scoped to the Catch Node's own flow:
		// foreign flows are never observable via "selected".
		if em.FlowID != n.config.FlowID {
			return
		}
		if _, ok := n.targetNodes[em.NodeID]; !ok {
			return
		}
	case "all":
		// no filter
	}
	if n.Send == nil {
		return
	}

	var out *flow.Message
	if em.Msg != nil {
		out = em.Msg.COWClone()
	} else {
		out = flow.NewMessage()
	}
	out.Set("_error", map[string]any{
		"message": em.Error,
		"source": map[string]any{
			"id":     em.NodeID,
			"type":   em.SourceType,
			"name":   em.SourceName,
			"flowId": em.FlowID,
		},
	})
	// Loop guard: any error raised by a downstream node along this catch
	// branch must not re-trigger catch fan-out. The engine reads this flag
	// in makeErrorFunc.
	out.Set("_caught", true)

	n.Send(0, out)
	n.blink(em)
}

func (n *CatchNode) blink(em flow.ErrorMessage) {
	if n.Status == nil {
		return
	}
	seq := n.blinkSeq.Add(1)
	label := em.SourceName
	if label == "" {
		label = em.NodeID
	}
	text := label + ": error"
	n.Status("red", catchTruncate(text, 32))

	go func() {
		time.Sleep(catchBlinkDuration)
		if n.blinkSeq.Load() != seq {
			return
		}
		n.Status("green", catchIdleStatusText)
	}()
}

func catchTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// CatchTypeInfo returns the NodeTypeInfo for registering the catch node.
func CatchTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "catch",
		Category:    "common",
		Label:       "Catch",
		Description: "Emits a message whenever another node raises an error",
		Icon:        "mdi-alert-octagon-outline",
		Defaults: map[string]any{
			"scope":       "flow",
			"targetNodes": []any{},
		},
		Inputs:  0,
		Outputs: 1,
	}
}
