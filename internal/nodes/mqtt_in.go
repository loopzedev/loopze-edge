// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/niceclouds/flint/internal/flow"
)

// MqttInNode subscribes to an MQTT topic and emits received messages into the flow.
// Source node: 0 inputs, 1 output. Implements flow.ConfigProvider.
type MqttInNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc

	brokerID string
	topic    string
	qos      byte
	broker   *MqttBroker
}

// NewMqttInNode creates a new MQTT subscribe node.
func NewMqttInNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &MqttInNode{config: config}, nil
}

func (n *MqttInNode) Init() error {
	props := n.config.Properties

	n.brokerID, _ = props["broker"].(string)
	if n.brokerID == "" {
		return fmt.Errorf("mqtt-in %s: no broker configured", n.config.ID)
	}

	n.topic, _ = props["topic"].(string)
	if n.topic == "" {
		return fmt.Errorf("mqtt-in %s: no topic configured", n.config.ID)
	}

	if v, ok := props["qos"].(float64); ok {
		n.qos = byte(v)
	}

	return nil
}

func (n *MqttInNode) SetSend(fn flow.SendFunc)               { n.send = fn }
func (n *MqttInNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *MqttInNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *MqttInNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *MqttInNode) Start() error {
	if n.configLookup == nil {
		return fmt.Errorf("mqtt-in %s: config lookup not available", n.config.ID)
	}

	inst, ok := n.configLookup(n.brokerID)
	if !ok {
		n.status("red", "broker not found")
		return fmt.Errorf("mqtt-in %s: broker %q not found", n.config.ID, n.brokerID)
	}

	broker, ok := inst.(*MqttBroker)
	if !ok {
		return fmt.Errorf("mqtt-in %s: config %q is not an MQTT broker", n.config.ID, n.brokerID)
	}
	n.broker = broker

	// Register status callback so broker state changes update this node.
	n.broker.RegisterStatusFunc(n.status)

	// Subscribe to topic.
	if err := n.broker.Subscribe(n.topic, n.qos, n.onMessage); err != nil {
		n.status("red", "subscribe failed")
		return fmt.Errorf("mqtt-in %s: subscribe failed: %w", n.config.ID, err)
	}

	slog.Info("mqtt-in node started", "node_id", n.config.ID, "topic", n.topic, "qos", n.qos)
	return nil
}

func (n *MqttInNode) onMessage(_ mqtt.Client, mqttMsg mqtt.Message) {
	msg := flow.NewMessage()
	msg.Set("topic", mqttMsg.Topic())
	msg.Set("payload", string(mqttMsg.Payload()))
	msg.Set("qos", int(mqttMsg.Qos()))
	msg.Set("retain", mqttMsg.Retained())
	n.send(0, msg)
}

func (n *MqttInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil // source node, no input handling
}

func (n *MqttInNode) Stop() error {
	slog.Info("mqtt-in node stopped", "node_id", n.config.ID, "topic", n.topic)
	return nil
}

// MqttInTypeInfo returns the node type metadata for the palette.
func MqttInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "mqtt-in",
		Category:    "network",
		Label:       "MQTT Subscribe",
		Description: "Subscribes to an MQTT topic and receives messages",
		Icon:        "wifi",
		Defaults: map[string]any{
			"broker": "",
			"topic":  "",
			"qos":    0,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
