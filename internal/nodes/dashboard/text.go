// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import "github.com/loopzedev/loopze-edge/internal/flow"

// UITextNode renders the latest value of a configured property as a
// label/value pair on the dashboard. All formatting (number decimals,
// units, JSON pretty-print) happens client-side from the widget config.
type UITextNode struct {
	displayWidget
}

// NewUITextNode is the NodeFactory for ui-text.
func NewUITextNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &UITextNode{}
	n.config = config
	return n, nil
}

// Init parses the shared 'property' field.
func (n *UITextNode) Init() error {
	n.initDisplay()
	return nil
}

// UITextTypeInfo is the editor-side metadata for ui-text.
func UITextTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "ui-text",
		Category:    "dashboard-display",
		Label:       "Text",
		Description: "Dashboard text/label widget. Renders the latest value of a property.",
		Icon:        "ui-text",
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
			"layout":   "row-spread",
			"format":   "text",
			"decimals": 2,
			"unit":     "",
			"color":    "",
		},
		Inputs:  1,
		Outputs: 0,
	}
}
