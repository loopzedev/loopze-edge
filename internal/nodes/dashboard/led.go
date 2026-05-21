// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UILedNode pushes a payload value to the dashboard; the frontend LED
// widget evaluates the configured `states` rules client-side to pick
// the active color/label. Server-side rule evaluation would duplicate
// the client logic and lock late ACL/role-aware overrides out of
// scope — the raw value is the right thing to broadcast.
type UILedNode struct {
	displayWidget
}

// NewUILedNode is the NodeFactory for ui-led.
func NewUILedNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &UILedNode{}
	n.config = config
	return n, nil
}

func (n *UILedNode) Init() error {
	n.initDisplay()
	return nil
}

// UILedTypeInfo is the editor-side metadata for ui-led.
func UILedTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "ui-led",
		Category:    "dashboard-display",
		Label:       "LED",
		Description: "Coloured indicator. States are evaluated client-side from configured rules.",
		Icon:        "ui-led",
		Defaults: map[string]any{
			"group":    "",
			"order":    0,
			"x":        0,
			"y":        0,
			"width":    0,
			"height":   1,
			"label":    "",
			"tooltip":  "",
			"property": "payload",
			"states": []any{
				map[string]any{"when": true, "color": "#5eba7d", "label": "ON"},
				map[string]any{"when": false, "color": "#444", "label": "OFF"},
			},
			"offColor": "#444",
			"glow":     false,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
