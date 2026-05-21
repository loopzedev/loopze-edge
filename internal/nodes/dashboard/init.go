// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package dashboard implements the LOOPZE dashboard widget nodes
// (ui-base, ui-page, ui-group, ui-spacer config nodes and ui-button,
// ui-text, ui-chart, ui-gauge, ui-led flow nodes). They feed the
// dashboard runtime hub in internal/dashboard.
package dashboard

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the dashboard node group with the central registry hub.
// Phase 1 ships ui-base / ui-page / ui-group as ConfigNodes and
// ui-button as the lone flow node, proving the bidirectional protocol
// end-to-end. Display widgets (ui-text, ui-chart, ui-gauge, ui-led)
// follow in PR 3/4.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "dashboard",
		Description: "Real-time dashboard widgets rendered by the LOOPZE dashboard SPA at /dashboard",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "ui-button", Factory: NewUIButtonNode, Info: UIButtonTypeInfo()},
			{Type: "ui-text", Factory: NewUITextNode, Info: UITextTypeInfo()},
			{Type: "ui-led", Factory: NewUILedNode, Info: UILedTypeInfo()},
			{Type: "ui-gauge", Factory: NewUIGaugeNode, Info: UIGaugeTypeInfo()},
		},
		ConfigNodes: []nodes.ConfigNodeRegistration{
			{Type: "ui-base", Factory: NewUIBase, Info: UIBaseTypeInfo()},
			{Type: "ui-page", Factory: NewUIPage, Info: UIPageTypeInfo()},
			{Type: "ui-group", Factory: NewUIGroup, Info: UIGroupTypeInfo()},
		},
	})
}
