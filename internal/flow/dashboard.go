// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import "time"

// DashboardHub is the runtime-facing interface to the dashboard hub. It
// is implemented by *internal/dashboard.Hub and injected into widget
// nodes via DashboardHubProvider before any node's Start runs.
//
// The hub also receives engine deploy events through DeployListener,
// installed via Engine.SetDeployListener — that path is engine→hub,
// uses the concrete *dashboard.Hub, and does not appear on this
// interface.
type DashboardHub interface {
	// PushWidgetValue records the latest value for a display widget and
	// broadcasts it to every connected dashboard client.
	PushWidgetValue(nodeID string, value any, ts time.Time)

	// RegisterInputWidget connects an input widget's emit callback to
	// the hub so user interactions in the browser reach the flow. The
	// returned unregister func must be called on widget Stop.
	RegisterInputWidget(nodeID string, fn func(WidgetEvent)) (unregister func())
}

// DashboardHubProvider is implemented by widget nodes that need access
// to the dashboard hub. The engine calls SetDashboardHub in
// wireAllNodes — before Start — so input widgets can register their
// callback during Start without races.
type DashboardHubProvider interface {
	SetDashboardHub(hub DashboardHub)
}

// WidgetEvent is delivered to an input widget node when a dashboard
// client interacts with its widget. The widget node turns this into an
// outgoing flow.Message and sends it on its output port.
type WidgetEvent struct {
	Value  any
	Client WidgetClient
	TS     time.Time
}

// WidgetClient identifies the dashboard client that produced an event.
// UserID is empty when ui-base.auth == "none".
type WidgetClient struct {
	UserID    string
	SessionID string
	SocketID  string
}

// DeployListener is invoked by the engine after every successful Deploy
// with the workspace that was just deployed. The server wires this to
// dashboard.Hub.RebuildLayout (and, in PR 5, dashboard.Hub.NotifyDeploy).
type DeployListener func(workspace Workspace)
