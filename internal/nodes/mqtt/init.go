// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package mqtt

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the MQTT node group with the central registry hub.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "mqtt",
		Description: "MQTT v3.1/v3.1.1/v5 publish / subscribe / request-reply nodes",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "mqtt-in", Factory: NewMqttInNode, Info: MqttInTypeInfo()},
			{Type: "mqtt-out", Factory: NewMqttOutNode, Info: MqttOutTypeInfo()},
			{Type: "mqtt-request", Factory: NewMqttRequestNode, Info: MqttRequestTypeInfo()},
		},
		ConfigNodes: []nodes.ConfigNodeRegistration{
			{Type: "mqtt-broker", Factory: NewMqttBroker, Info: MqttBrokerConfigTypeInfo()},
		},
	})
}
