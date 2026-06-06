// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// UIStatNode is the Grafana-style "stat" widget: a big headline number
// with an optional trend delta and a server-side windowed sparkline.
// Unlike ui-text / ui-led / ui-gauge it reads TWO properties from the
// incoming message (value + optional delta) and emits them as a single
// atomic payload so the dashboard never shows a stale delta next to a
// fresh value.
//
// Sparkline samples will be pushed in Slice 3 via a new
// hub.AppendStatSample method — for Slice 1 we only ship the headline
// value path.
type UIStatNode struct {
	nodes.BaseNode
	config flow.NodeConfig

	property        string
	deltaProperty   string
	sparklineWindow int
	showSparkline   bool
	hub             flow.DashboardHub
}

// NewUIStatNode is the NodeFactory for ui-stat.
func NewUIStatNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &UIStatNode{}
	n.config = config
	return n, nil
}

func (n *UIStatNode) Init() error {
	n.property = nodes.StringVal(n.config.Properties, "property", "payload")
	n.deltaProperty = nodes.StringVal(n.config.Properties, "deltaProperty", "delta")
	n.sparklineWindow = nodes.IntVal(n.config.Properties, "sparklineWindow", 60)
	if n.sparklineWindow < 1 {
		n.sparklineWindow = 1
	}
	n.showSparkline = nodes.BoolVal(n.config.Properties, "showSparkline", true)
	return nil
}

// SetDashboardHub satisfies flow.DashboardHubProvider.
func (n *UIStatNode) SetDashboardHub(hub flow.DashboardHub) {
	n.hub = hub
}

func (n *UIStatNode) Start() error {
	if n.hub == nil && n.Status != nil {
		n.Status("yellow", "no dashboard hub")
		return nil
	}
	if n.Status != nil {
		n.Status("blue", "ready")
	}
	return nil
}

func (n *UIStatNode) Stop() error { return nil }

// HandleMessage assembles the {value, delta?} payload and pushes it to
// the hub. The delta field is omitted entirely when the source message
// has no numeric value at the delta property — the dashboard widget
// uses presence/absence to decide whether to render the delta row.
func (n *UIStatNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.hub == nil {
		return nil, nil
	}

	value, hasValue := numericFromMsg(msg, n.property)
	if !hasValue {
		// Non-numeric / missing → don't update. Keeps the last good
		// reading on the dashboard rather than blanking the widget.
		return nil, nil
	}

	payload := map[string]any{"value": value}
	if delta, hasDelta := numericFromMsg(msg, n.deltaProperty); hasDelta {
		payload["delta"] = delta
	}

	now := time.Now()
	n.hub.PushWidgetValue(n.config.ID, payload, now)
	if n.showSparkline {
		n.hub.AppendStatSample(n.config.ID, value, now, n.sparklineWindow)
	}
	return nil, nil
}

// numericFromMsg reads a property and coerces to float64. Accepts the
// common numeric shapes that flow nodes produce; rejects strings even
// if they parse cleanly (explicit numeric source per L-5 in the spec).
func numericFromMsg(msg *flow.Message, property string) (float64, bool) {
	switch v := msg.Get(property).(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	}
	return 0, false
}

// UIStatTypeInfo is the editor-side metadata for ui-stat.
func UIStatTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "ui-stat",
		Category:    "dashboard-display",
		Label:       "Stat",
		Description: "Headline KPI widget: big number with optional trend delta and sparkline.",
		Icon:        "ui-stat",
		Defaults: map[string]any{
			// layout
			"group":   "",
			"order":   0,
			"x":       0,
			"y":       0,
			"width":   3,
			"height":  4,
			"tooltip": "",
			// label
			"label":          "",
			"sublabel":       "",
			"labelSeparator": "·",
			// value
			"property":           "payload",
			"decimals":           0,
			"thousandsSeparator": "space",
			"decimalSeparator":   "dot",
			"prefix":             "",
			"suffix":             "",
			"valueColor":         "",
			// delta
			"deltaProperty":  "delta",
			"showDelta":      true,
			"deltaFormat":    "percent",
			"deltaDecimals":  1,
			"deltaContext":   "",
			"deltaDirection": "up-is-good",
			// sparkline
			"showSparkline":   true,
			"sparklineWindow": 60,
			"sparklineColor":  "",
			"sparklineFill":   true,
			// layout
			"layout": "vertical",
		},
		Inputs:  1,
		Outputs: 0,
	}
}
