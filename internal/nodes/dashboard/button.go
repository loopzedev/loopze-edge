// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"fmt"
	"strconv"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// UIButtonNode is the canonical dashboard input widget: a button on the
// dashboard that, when clicked, emits a configured payload on the
// node's single output port.
//
// Lifecycle:
//   - SetDashboardHub: called by the engine before Start (Provider DI).
//   - Start: register the click callback with the hub.
//   - Stop: unregister.
//   - HandleMessage: not used (input widgets have no input port) — the
//     interface still requires the method, so it is a no-op.
type UIButtonNode struct {
	nodes.BaseNode
	config flow.NodeConfig

	// Parsed from Properties.
	topic       string
	payloadType string
	payloadRaw  string

	hub        flow.DashboardHub
	unregister func()
}

// NewUIButtonNode is the NodeFactory for ui-button.
func NewUIButtonNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &UIButtonNode{config: config}, nil
}

// Init validates and caches the static fields used to build the click
// payload. The button does not access the hub here — Start is the only
// place that touches the engine-injected hub reference.
func (n *UIButtonNode) Init() error {
	props := n.config.Properties

	n.topic = nodes.StringVal(props, "topic", "")
	n.payloadType = nodes.StringVal(props, "payloadType", "bool")
	n.payloadRaw = nodes.StringVal(props, "payload", "")

	switch n.payloadType {
	case "bool", "string", "number", "timestamp":
		// supported in Phase 1
	default:
		return fmt.Errorf("ui-button: unsupported payloadType %q (bool|string|number|timestamp)", n.payloadType)
	}
	return nil
}

// SetDashboardHub satisfies flow.DashboardHubProvider. The engine calls
// this in wireAllNodes before Start, so n.hub is non-nil by the time
// Start runs (unless the engine was constructed without a dashboard
// hub — engine-only unit tests).
func (n *UIButtonNode) SetDashboardHub(hub flow.DashboardHub) {
	n.hub = hub
}

// Start registers the click handler with the hub. A nil hub (no
// dashboard configured in this engine) is tolerated as a no-op so the
// node still participates in the flow lifecycle for non-dashboard
// deployments.
func (n *UIButtonNode) Start() error {
	if n.hub == nil {
		if n.Status != nil {
			n.Status("yellow", "no dashboard hub")
		}
		return nil
	}
	n.unregister = n.hub.RegisterInputWidget(n.config.ID, n.onClick)
	if n.Status != nil {
		n.Status("blue", "ready")
	}
	return nil
}

// Stop unregisters the click handler so a re-deploy does not leak
// callbacks.
func (n *UIButtonNode) Stop() error {
	if n.unregister != nil {
		n.unregister()
		n.unregister = nil
	}
	return nil
}

// HandleMessage: ui-button has no input port. Returning nil tells the
// engine there are no fan-out messages.
func (n *UIButtonNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// onClick fires for every dashboard click that targets this widget.
// Builds a flow.Message from the configured payload, attaches client
// metadata, and pushes it down the wire via the engine-injected Send
// callback.
func (n *UIButtonNode) onClick(evt flow.WidgetEvent) {
	if n.Send == nil {
		return
	}
	payload := n.buildPayload(evt)

	msg := flow.NewMessage()
	msg.SetPayload(payload)
	if n.topic != "" {
		msg.SetTopic(n.topic)
	}
	msg.Set("_client", map[string]any{
		"userID":    evt.Client.UserID,
		"sessionID": evt.Client.SessionID,
		"socketID":  evt.Client.SocketID,
	})
	msg.Set("_widget", map[string]any{
		"nodeId": n.config.ID,
		"type":   "ui-button",
	})

	n.Send(0, msg)
}

// buildPayload converts the configured (payloadType, payload) into the
// runtime value emitted on click. Unrecognised payloadTypes are caught
// in Init; this function returns sensible defaults if a value cannot
// be parsed at click time.
func (n *UIButtonNode) buildPayload(evt flow.WidgetEvent) any {
	switch n.payloadType {
	case "bool":
		if n.payloadRaw == "false" {
			return false
		}
		return true
	case "string":
		return n.payloadRaw
	case "number":
		if v, err := strconv.ParseFloat(n.payloadRaw, 64); err == nil {
			return v
		}
		return 0.0
	case "timestamp":
		if evt.TS.IsZero() {
			return time.Now().UnixMilli()
		}
		return evt.TS.UnixMilli()
	default:
		return true
	}
}

// UIButtonTypeInfo is the editor-side metadata for ui-button.
func UIButtonTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "ui-button",
		Category:    "dashboard-input",
		Label:       "Button",
		Description: "Dashboard button. Emits a configured payload on the output when clicked.",
		Icon:        "ui-button",
		Defaults: map[string]any{
			"group":       "",
			"order":       0,
			"x":           0,
			"y":           0,
			"width":       0,
			"height":      1,
			"label":       "Click me",
			"tooltip":     "",
			"payload":     "",
			"payloadType": "bool",
			"topic":       "",
			"color":       "",
			"icon":        "",
		},
		Inputs:  0,
		Outputs: 1,
	}
}
