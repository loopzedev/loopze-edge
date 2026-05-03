// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
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

	// MQTT v5 default publish properties. These act as fallbacks: msg.* fields
	// always win when present. UserProperties are merged (msg keys override
	// config keys; non-overlapping keys from both sides survive).
	defaultUserProperties map[string]string
	defaultContentType    string
	defaultResponseTopic  string
	defaultMessageExpiry  *uint32
	defaultPayloadFormat  *byte
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

	n.defaultUserProperties = readStringMap(props["defaultUserProperties"])
	if ct, ok := props["defaultContentType"].(string); ok && ct != "" {
		n.defaultContentType = ct
	}
	if rt, ok := props["defaultResponseTopic"].(string); ok && rt != "" {
		n.defaultResponseTopic = rt
	}
	if me, ok := readUint32(props["defaultMessageExpiry"]); ok && me > 0 {
		n.defaultMessageExpiry = &me
	}
	if pf, ok := readPayloadFormat(props["defaultPayloadFormat"]); ok {
		n.defaultPayloadFormat = &pf
	}

	return nil
}

// readPayloadFormat parses the v5 payload-format-indicator (0=bytes, 1=UTF-8).
// Anything outside that range, or absent, returns ok=false.
func readPayloadFormat(v any) (byte, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		if x != 0 && x != 1 {
			return 0, false
		}
		return byte(x), true
	case int:
		if x != 0 && x != 1 {
			return 0, false
		}
		return byte(x), true
	default:
		return 0, false
	}
}

func (n *MqttOutNode) SetSend(fn flow.SendFunc)               { n.send = fn }
func (n *MqttOutNode) SetStatus(fn flow.StatusFunc)            { n.status = fn }
func (n *MqttOutNode) SetDebug(fn flow.DebugFunc)              { n.debug = fn }
func (n *MqttOutNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }

func (n *MqttOutNode) Start() error {
	broker, err := resolveConfigInstance[MqttBroker](n.configLookup, n.brokerID, n.status, resolveConfigParams{
		NodeKind:   "mqtt-out",
		NodeID:     n.config.ID,
		ConfigKind: "broker",
		TypeLabel:  "an MQTT broker",
	})
	if err != nil {
		return err
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

	props := n.mergeV5PublishProperties(msg)

	if err := n.broker.Publish(topic, n.qos, n.retain, payload, props); err != nil {
		n.status("red", err.Error())
		return nil, fmt.Errorf("mqtt-out %s: publish failed: %w", n.config.ID, err)
	}

	return nil, nil // sink node, no output
}

// mergeV5PublishProperties combines the node's configured v5 defaults with
// any v5 fields present on the incoming flow message. Rules:
//
//   - Scalars (contentType, responseTopic, messageExpiry, payloadFormat):
//     msg.* wins when present and non-empty/valid; otherwise the configured
//     default is used.
//   - userProperties are merged: keys from defaultUserProperties are added
//     first, then keys from msg.userProperties — so msg keys override config
//     keys at the same name, and non-overlapping keys from both sides survive.
//   - correlationData has no static default (per design — it's per-request);
//     only msg.correlationData applies.
//
// Returns nil if neither defaults nor msg contribute any field — paho-go is
// happy with a nil PublishProperties on the wire.
func (n *MqttOutNode) mergeV5PublishProperties(msg *flow.Message) *paho.PublishProperties {
	var props *paho.PublishProperties
	ensure := func() *paho.PublishProperties {
		if props == nil {
			props = &paho.PublishProperties{}
		}
		return props
	}

	// User properties: config first, msg overrides per-key.
	merged := mergeUserProperties(n.defaultUserProperties, msg.Get("userProperties"))
	for k, v := range merged {
		ensure().User.Add(k, v)
	}

	contentType := n.defaultContentType
	if ct, ok := msg.Get("contentType").(string); ok && ct != "" {
		contentType = ct
	}
	if contentType != "" {
		ensure().ContentType = contentType
	}

	responseTopic := n.defaultResponseTopic
	if rt, ok := msg.Get("responseTopic").(string); ok && rt != "" {
		responseTopic = rt
	}
	if responseTopic != "" {
		ensure().ResponseTopic = responseTopic
	}

	if cd := msg.Get("correlationData"); cd != nil {
		switch v := cd.(type) {
		case []byte:
			if len(v) > 0 {
				ensure().CorrelationData = v
			}
		case string:
			if v != "" {
				ensure().CorrelationData = []byte(v)
			}
		}
	}

	if me, ok := readUint32(msg.Get("messageExpiry")); ok {
		ensure().MessageExpiry = &me
	} else if n.defaultMessageExpiry != nil {
		v := *n.defaultMessageExpiry
		ensure().MessageExpiry = &v
	}

	if pf, ok := readPayloadFormat(msg.Get("payloadFormat")); ok {
		ensure().PayloadFormat = &pf
	} else if n.defaultPayloadFormat != nil {
		v := *n.defaultPayloadFormat
		ensure().PayloadFormat = &v
	}

	return props
}

// mergeUserProperties returns a single map that has all keys from defaults,
// overlaid with all keys from msgValue. msgValue may be nil, map[string]string
// or map[string]any (the latter from JSON-decoded wire format). Returns nil if
// the merge result is empty.
func mergeUserProperties(defaults map[string]string, msgValue any) map[string]string {
	out := make(map[string]string, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	switch m := msgValue.(type) {
	case map[string]string:
		for k, v := range m {
			out[k] = v
		}
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// readUint32 accepts the typical wire formats (float64 from JSON, int, uint32)
// and reports whether the value is a valid non-negative integer.
func readUint32(v any) (uint32, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		if x < 0 {
			return 0, false
		}
		return uint32(x), true
	case int:
		if x < 0 {
			return 0, false
		}
		return uint32(x), true
	case uint32:
		return x, true
	default:
		return 0, false
	}
}

func (n *MqttOutNode) Stop() error {
	slog.Info("mqtt-out node stopped", "node_id", n.config.ID)
	return nil
}

// toBytes converts a payload value to a byte slice for MQTT publishing.
//
// Numeric arrays are recognised so that buffer-mode payloads from mqtt-in (and
// JSON-round-tripped equivalents) publish as their original bytes rather than
// the literal "[1,2,3]" string.
func toBytes(v any) ([]byte, error) {
	switch val := v.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	case []int:
		return intsToBytes(val), nil
	case []any:
		if buf, ok := anyToBytes(val); ok {
			return buf, nil
		}
		data, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		return data, nil
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

// intsToBytes packs an []int into []byte, clamping each element to a single
// byte (out-of-range values are taken modulo 256).
func intsToBytes(in []int) []byte {
	out := make([]byte, len(in))
	for i, n := range in {
		out[i] = byte(n)
	}
	return out
}

// anyToBytes recognises a []any whose elements are all numeric and packs them
// as a byte slice. This is the format produced by JSON-decoding a buffer-mode
// payload — every number arrives as float64. Returns ok=false if any element
// is not a number, leaving the caller to fall back to JSON-marshaling.
func anyToBytes(in []any) ([]byte, bool) {
	out := make([]byte, len(in))
	for i, e := range in {
		switch n := e.(type) {
		case float64:
			out[i] = byte(int(n))
		case int:
			out[i] = byte(n)
		default:
			return nil, false
		}
	}
	return out, true
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
			"broker":                "",
			"topic":                 "",
			"qos":                   0,
			"retain":                false,
			"defaultUserProperties": map[string]string{},
			"defaultContentType":    "",
			"defaultResponseTopic":  "",
			"defaultMessageExpiry":  0,
			"defaultPayloadFormat":  0,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
