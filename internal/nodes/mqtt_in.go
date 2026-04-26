// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/niceclouds/flint/internal/flow"
)

// MqttInNode subscribes to MQTT topics and emits received messages into the flow.
//
// Two modes are supported:
//   - "static" (default): one fixed topic from configuration; node has 0 inputs.
//   - "dynamic": no subscription at start. The node has 1 input and reacts to
//     control messages with msg.action="subscribe", replacing all current
//     subscriptions with the topic(s) in msg.payload (string or []string).
//
// Implements flow.ConfigProvider.
type MqttInNode struct {
	config       flow.NodeConfig
	send         flow.SendFunc
	status       flow.StatusFunc
	debug        flow.DebugFunc
	configLookup flow.ConfigLookupFunc

	mode     string
	brokerID string
	topic    string // static mode only
	qos      byte
	broker   *MqttBroker

	mu           sync.Mutex
	activeTopics []string // currently subscribed topics (both modes track this for clean teardown)
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

	mode, _ := props["mode"].(string)
	if mode == "" {
		mode = "static"
	}
	if mode != "static" && mode != "dynamic" {
		return fmt.Errorf("mqtt-in %s: invalid mode %q (expected static|dynamic)", n.config.ID, mode)
	}
	n.mode = mode

	if mode == "static" {
		n.topic, _ = props["topic"].(string)
		if n.topic == "" {
			return fmt.Errorf("mqtt-in %s: no topic configured", n.config.ID)
		}
	}

	n.qos = extractQoS(props["qos"], 0)

	return nil
}

func (n *MqttInNode) SetSend(fn flow.SendFunc)                 { n.send = fn }
func (n *MqttInNode) SetStatus(fn flow.StatusFunc)              { n.status = fn }
func (n *MqttInNode) SetDebug(fn flow.DebugFunc)                { n.debug = fn }
func (n *MqttInNode) SetConfigLookup(fn flow.ConfigLookupFunc)  { n.configLookup = fn }

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

	// Wrap status callback so connected-state can be enriched with node-local
	// detail (active topic count / static topic) — disconnected/connecting
	// states are passed through unchanged.
	n.broker.RegisterStatusFunc(n.handleBrokerStatus)

	if n.mode == "static" {
		if err := n.broker.Subscribe(n.config.ID, n.topic, n.qos, n.onMessage); err != nil {
			n.status("red", "subscribe failed")
			return fmt.Errorf("mqtt-in %s: subscribe failed: %w", n.config.ID, err)
		}
		n.mu.Lock()
		n.activeTopics = []string{n.topic}
		n.mu.Unlock()
		slog.Info("mqtt-in node started (static)", "node_id", n.config.ID, "topic", n.topic, "qos", n.qos)
	} else {
		slog.Info("mqtt-in node started (dynamic, idle)", "node_id", n.config.ID, "qos", n.qos)
	}

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

// HandleMessage in dynamic mode treats incoming messages as control commands
// and replaces all current subscriptions with the topics from msg.payload.
// Control messages are NOT forwarded to the output.
func (n *MqttInNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if n.mode != "dynamic" {
		return nil, nil
	}

	action, _ := msg.Get("action").(string)
	if action != "subscribe" {
		slog.Debug("mqtt-in: ignoring control message", "node_id", n.config.ID, "action", action)
		return nil, nil
	}

	next := toTopicSlice(msg.Get("payload"))
	qos := extractQoS(msg.Get("qos"), n.qos)

	n.mu.Lock()
	old := n.activeTopics
	n.activeTopics = next
	n.mu.Unlock()

	for _, t := range old {
		if err := n.broker.Unsubscribe(n.config.ID, t); err != nil {
			slog.Warn("mqtt-in: unsubscribe failed", "node_id", n.config.ID, "topic", t, "error", err)
		}
	}
	for _, t := range next {
		if err := n.broker.Subscribe(n.config.ID, t, qos, n.onMessage); err != nil {
			slog.Warn("mqtt-in: subscribe failed", "node_id", n.config.ID, "topic", t, "error", err)
		}
	}
	if len(next) > 0 {
		slog.Info("mqtt-in dynamic subscribe", "node_id", n.config.ID, "topics", next, "qos", qos)
	}

	n.refreshStatus()
	return nil, nil
}

// extractQoS reads a QoS override from a control message. Valid values 0/1/2
// override the fallback; anything else (missing, out of range, unparsable)
// returns the fallback unchanged. Accepts float64 (typical wire format),
// int (in-process callers), and numeric strings ("0"/"1"/"2") for users who
// configure msg.qos as a string-typed inject value.
func extractQoS(v any, fallback byte) byte {
	var n float64
	switch x := v.(type) {
	case nil:
		return fallback
	case float64:
		n = x
	case int:
		n = float64(x)
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return fallback
		}
		n = f
	default:
		return fallback
	}
	if n < 0 || n > 2 {
		return fallback
	}
	return byte(n)
}

// toTopicSlice converts a payload value into a clean list of topic strings.
//
//   - nil or empty string → []
//   - non-empty string    → [s]
//   - []string            → s (filtering empties)
//   - []any of strings    → s (filtering empties; non-strings are skipped)
//
// Anything else returns []. The wire format delivers JSON-decoded arrays as
// []any, so the []any case is the common one.
func toTopicSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func (n *MqttInNode) Stop() error {
	n.mu.Lock()
	topics := append([]string(nil), n.activeTopics...)
	n.activeTopics = nil
	n.mu.Unlock()

	if n.broker != nil {
		for _, t := range topics {
			if err := n.broker.Unsubscribe(n.config.ID, t); err != nil {
				slog.Warn("mqtt-in: unsubscribe on stop failed", "node_id", n.config.ID, "topic", t, "error", err)
			}
		}
	}

	slog.Info("mqtt-in node stopped", "node_id", n.config.ID, "mode", n.mode)
	return nil
}

// handleBrokerStatus is invoked by the broker on connection state changes.
// We pass disconnected / connecting states through verbatim and only override
// the "connected" state with node-local detail.
func (n *MqttInNode) handleBrokerStatus(fill, text string) {
	if fill != "green" {
		n.status(fill, text)
		return
	}
	n.refreshStatus()
}

// refreshStatus computes and sets the green-state status text. Must only be
// called when the broker is connected.
func (n *MqttInNode) refreshStatus() {
	n.mu.Lock()
	count := len(n.activeTopics)
	first := ""
	if count > 0 {
		first = n.activeTopics[0]
	}
	n.mu.Unlock()

	if n.mode == "static" {
		n.status("green", "connected · "+first)
		return
	}
	if count == 0 {
		n.status("green", "connected · idle")
		return
	}
	n.status("green", fmt.Sprintf("connected · %d topic(s)", count))
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
			"mode":   "static",
			"topic":  "",
			"qos":    0,
		},
		Inputs:  0,
		Outputs: 1,
	}
}
