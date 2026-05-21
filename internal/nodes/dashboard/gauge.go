// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UIGaugeNode pushes a numeric value to the dashboard. The frontend
// GaugeWidget renders an SVG arc using min/max/thresholds from the
// widget config — no ECharts in Phase 1 PR 3 (ECharts arrives with
// ui-chart in PR 4).
type UIGaugeNode struct {
	displayWidget
}

// NewUIGaugeNode is the NodeFactory for ui-gauge.
func NewUIGaugeNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &UIGaugeNode{}
	n.config = config
	return n, nil
}

func (n *UIGaugeNode) Init() error {
	n.initDisplay()
	return nil
}

// UIGaugeTypeInfo is the editor-side metadata for ui-gauge.
func UIGaugeTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "ui-gauge",
		Category:    "dashboard-display",
		Label:       "Gauge",
		Description: "Single-value gauge with optional threshold colour bands.",
		Icon:        "ui-gauge",
		Defaults: map[string]any{
			"group":      "",
			"order":      0,
			"x":          0,
			"y":          0,
			"width":      6,
			"height":     4,
			"label":      "",
			"tooltip":    "",
			"property":   "payload",
			"min":        0,
			"max":        100,
			"unit":       "",
			"decimals":   1,
			"style":      "arc",
			"thresholds": []any{},
			"showValue":  true,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
