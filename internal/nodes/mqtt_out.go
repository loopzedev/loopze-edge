// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/niceclouds/flint/internal/flow"
)

// MqttOutNode publishes incoming flow messages to an MQTT topic.
// Sink node: 1 input, 0 outputs. Implements flow.ConfigProvider.
type MqttOutNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc

	brokerID string
	topic    string
	qos      byte
	retain   bool
	broker   *MqttBroker
}

// NewMqttOutNode creates a new MQTT publish node.
func NewMqttOutNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &MqttOutNode{config: config}, nil
}

func (n *MqttOutNode) Init() error {
	props := n.config.Properties

	n.brokerID, _ = props["broker"].(string)
	if n.brokerID == "" {
		return fmt.Errorf("mqtt-out %s: no broker configured", n.config.ID)
	}

	n.topic, _ = props["topic"].(string)
	// topic is optional — can come from msg.topic at runtime

	n.qos = extractQoS(props["qos"], 0)

	if v, ok := props["retain"].(bool); ok {
		n.retain = v
	}

	return nil
}

func (n *MqttOutNode) SetSend(fn flow.SendFunc)               { n.send = fn }
func (n *MqttOutNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *MqttOutNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *MqttOutNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *MqttOutNode) Start() error {
	if n.configLookup == nil {
		return fmt.Errorf("mqtt-out %s: config lookup not available", n.config.ID)
	}

	inst, ok := n.configLookup(n.brokerID)
	if !ok {
		n.status("red", "broker not found")
		return fmt.Errorf("mqtt-out %s: broker %q not found", n.config.ID, n.brokerID)
	}

	broker, ok := inst.(*MqttBroker)
	if !ok {
		return fmt.Errorf("mqtt-out %s: config %q is not an MQTT broker", n.config.ID, n.brokerID)
	}
	n.broker = broker

	// Register status callback so broker state changes update this node.
	n.broker.RegisterStatusFunc(n.status)

	slog.Info("mqtt-out node started", "node_id", n.config.ID, "topic", n.topic, "qos", n.qos)
	return nil
}

func (n *MqttOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	topic := n.topic
	if topic == "" {
		topic = msg.Topic()
	}
	if topic == "" {
		return nil, fmt.Errorf("mqtt-out %s: no topic configured and msg.topic is empty", n.config.ID)
	}

	payload, err := toBytes(msg.Payload())
	if err != nil {
		return nil, fmt.Errorf("mqtt-out %s: payload conversion failed: %w", n.config.ID, err)
	}

	if err := n.broker.Publish(topic, n.qos, n.retain, payload); err != nil {
		n.status("red", err.Error())
		return nil, fmt.Errorf("mqtt-out %s: publish failed: %w", n.config.ID, err)
	}

	return nil, nil // sink node, no output
}

func (n *MqttOutNode) Stop() error {
	slog.Info("mqtt-out node stopped", "node_id", n.config.ID)
	return nil
}

// toBytes converts a payload value to a byte slice for MQTT publishing.
func toBytes(v any) ([]byte, error) {
	switch val := v.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	case nil:
		return []byte{}, nil
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
}

// MqttOutTypeInfo returns the node type metadata for the palette.
func MqttOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "mqtt-out",
		Category:    "network",
		Label:       "MQTT Publish",
		Description: "Publishes messages to an MQTT topic",
		Icon:        "wifi",
		Defaults: map[string]any{
			"broker": "",
			"topic":  "",
			"qos":    0,
			"retain": false,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
