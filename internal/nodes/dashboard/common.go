// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// displayWidget is the shared shell for ui-text / ui-led / ui-gauge.
// They all read a single property from the incoming message and push
// it to the dashboard hub; the difference between them is purely
// frontend rendering. Sharing the shell keeps the three Go files at
// ~30 LOC each and guarantees identical lifecycle / DI behaviour.
//
// HandleMessage swallows the message — display widgets have zero
// outputs. Returning nil tells the engine "no fan-out".
type displayWidget struct {
	nodes.BaseNode
	config flow.NodeConfig

	property string
	hub      flow.DashboardHub
}

// initDisplay parses the common 'property' field with a "payload"
// fallback. Concrete nodes call this from their own Init.
func (n *displayWidget) initDisplay() {
	n.property = nodes.StringVal(n.config.Properties, "property", "payload")
}

// SetDashboardHub satisfies flow.DashboardHubProvider.
func (n *displayWidget) SetDashboardHub(hub flow.DashboardHub) {
	n.hub = hub
}

// Start is a no-op for display widgets — they have no callback to
// register. A nil hub is tolerated so the node works in engine-only
// unit tests.
func (n *displayWidget) Start() error {
	if n.hub == nil && n.Status != nil {
		n.Status("yellow", "no dashboard hub")
		return nil
	}
	if n.Status != nil {
		n.Status("blue", "ready")
	}
	return nil
}

// Stop is a no-op.
func (n *displayWidget) Stop() error { return nil }

// HandleMessage reads the configured property from the message and
// pushes it to the dashboard hub. Display widgets have no output
// port — the message ends here.
func (n *displayWidget) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.hub == nil {
		return nil, nil
	}
	value := msg.Get(n.property)
	n.hub.PushWidgetValue(n.config.ID, value, time.Now())
	return nil, nil
}
