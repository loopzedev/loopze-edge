// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UIBase is the singleton dashboard config node. It carries the global
// theme, mount path, auth mode, and density. The dashboard.BuildLayout
// reader reads its fields directly from the workspace config map — this
// config instance therefore needs no parsed state and only exists to
// satisfy the lifecycle contract.
type UIBase struct{}

// NewUIBase is the ConfigNodeFactory for ui-base.
func NewUIBase(_ flow.ConfigNode) (flow.ConfigInstance, error) {
	return &UIBase{}, nil
}

// Start is a no-op — ui-base owns no runtime resources.
func (n *UIBase) Start() error { return nil }

// Stop is a no-op.
func (n *UIBase) Stop() error { return nil }

// Status reports the dashboard as ready. The hub layer surfaces the
// real "active connections" count separately.
func (n *UIBase) Status() (fill string, text string) {
	return "blue", "dashboard configured"
}

// UIBaseTypeInfo is the editor-side metadata for ui-base.
func UIBaseTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "ui-base",
		Label:       "Dashboard",
		Description: "Global dashboard configuration: name, theme, auth, mount path. Exactly one ui-base per deployment.",
		Defaults: map[string]any{
			"name":        "LOOPZE Dashboard",
			"path":        "/dashboard",
			"theme":       "dark",
			"accentColor": "#58a6ff",
			"auth":        "session",
			"showNav":     true,
			"navStyle":    "tabs",
			"density":     "default",
		},
	}
}
