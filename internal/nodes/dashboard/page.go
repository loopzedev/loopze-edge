// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UIPage is a dashboard page config. Same minimal contract as UIBase —
// dashboard.BuildLayout reads the config map directly; this instance
// only participates in the lifecycle.
type UIPage struct{}

// NewUIPage is the ConfigNodeFactory for ui-page.
func NewUIPage(_ flow.ConfigNode) (flow.ConfigInstance, error) {
	return &UIPage{}, nil
}

func (n *UIPage) Start() error { return nil }
func (n *UIPage) Stop() error  { return nil }
func (n *UIPage) Status() (fill string, text string) {
	return "blue", "page"
}

// UIPageTypeInfo is the editor-side metadata for ui-page.
func UIPageTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "ui-page",
		Label:       "Dashboard Page",
		Description: "A page on the dashboard. Multiple pages appear in the side nav.",
		Defaults: map[string]any{
			"name":   "Page 1",
			"path":   "",
			"icon":   "",
			"layout": "grid",
			"cols":   12,
			"order":  0,
		},
	}
}
