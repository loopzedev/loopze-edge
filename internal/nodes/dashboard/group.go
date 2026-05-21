// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UIGroup is a layout container inside a ui-page. Widgets reference a
// group's ID via their `group` config field. Same minimal contract as
// UIBase / UIPage.
type UIGroup struct{}

// NewUIGroup is the ConfigNodeFactory for ui-group.
func NewUIGroup(_ flow.ConfigNode) (flow.ConfigInstance, error) {
	return &UIGroup{}, nil
}

func (n *UIGroup) Start() error { return nil }
func (n *UIGroup) Stop() error  { return nil }
func (n *UIGroup) Status() (fill string, text string) {
	return "blue", "group"
}

// UIGroupTypeInfo is the editor-side metadata for ui-group.
func UIGroupTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "ui-group",
		Label:       "Dashboard Group",
		Description: "A group of widgets inside a dashboard page.",
		Defaults: map[string]any{
			"name":        "Group 1",
			"page":        "",
			"x":           0,
			"y":           0,
			"width":       12,
			"height":      6,
			"collapsible": false,
			"order":       0,
		},
	}
}
