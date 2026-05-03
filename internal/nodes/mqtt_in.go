// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
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

	mode         string
	brokerID     string
	topic        string // static mode only
	qos          byte
	outputFormat string // "string" (default) | "json" | "buffer"
	broker       *MqttBroker

	// MQTT v5 subscription options (default values used in static mode and as
	// fallbacks in dynamic mode when the control message doesn't override them).
	noLocal                bool
	retainAsPublished      bool
	retainHandling         byte
	subscriptionIdentifier int
	subscribeUserProperties map[string]string

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

	n.outputFormat, _ = props["outputFormat"].(string)
	switch n.outputFormat {
	case "", "string":
		n.outputFormat = "string"
	case "json", "buffer":
	default:
		slog.Warn("mqtt-in: unknown outputFormat, falling back to string",
			"node_id", n.config.ID, "outputFormat", n.outputFormat)
		n.outputFormat = "string"
	}

	if v, ok := props["noLocal"].(bool); ok {
		n.noLocal = v
	}
	if v, ok := props["retainAsPublished"].(bool); ok {
		n.retainAsPublished = v
	}
	n.retainHandling = extractRetainHandling(props["retainHandling"], 0)
	if v, ok := props["subscriptionIdentifier"].(float64); ok && v > 0 {
		n.subscriptionIdentifier = int(v)
	}
	n.subscribeUserProperties = readStringMap(props["subscribeUserProperties"])

	return nil
}

// setPayload writes the MQTT payload onto the flow message in the requested
// format. On JSON parse failure it falls back to the raw string and adds a
// `parseError` field — the message is still emitted so downstream catch nodes
// can react.
//
// Buffer format note: we emit []int rather than []byte. Go's encoding/json
// serializes []byte as base64, which is opaque on the debug viewer and — more
// importantly — not round-trip-safe (after JSON decode the value is a string,
// not bytes). []int serializes as a JSON number array that survives JSON
// round-trips and is human-readable in the inspector. mqtt-out's payload
// converter accepts the array form and reconstructs the bytes when publishing.
func setPayload(msg *flow.Message, raw []byte, format string) {
	switch format {
	case "buffer":
		buf := make([]int, len(raw))
		for i, b := range raw {
			buf[i] = int(b)
		}
		msg.Set("payload", buf)
	case "json":
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			msg.Set("payload", string(raw))
			msg.Set("parseError", err.Error())
			return
		}
		msg.Set("payload", v)
	default: // "string" or unknown
		msg.Set("payload", string(raw))
	}
}

// extractRetainHandling parses the v5 retain-handling option (0/1/2). Anything
// out of range or unparseable returns the fallback.
func extractRetainHandling(v any, fallback byte) byte {
	switch x := v.(type) {
	case nil:
		return fallback
	case float64:
		if x < 0 || x > 2 {
			return fallback
		}
		return byte(x)
	case int:
		if x < 0 || x > 2 {
			return fallback
		}
		return byte(x)
	default:
		return fallback
	}
}

// readStringMap converts a config-side user-properties map into a clean
// string-to-string map. Accepts both map[string]string (from Go callers) and
// map[string]any (from JSON-decoded wire format); non-string values are dropped.
func readStringMap(v any) map[string]string {
	switch m := v.(type) {
	case map[string]string:
		if len(m) == 0 {
			return nil
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	case map[string]any:
		if len(m) == 0 {
			return nil
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			if s, ok := val.(string); ok {
				out[k] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func (n *MqttInNode) SetSend(fn flow.SendFunc)                 { n.send = fn }
func (n *MqttInNode) SetStatus(fn flow.StatusFunc)              { n.status = fn }
func (n *MqttInNode) SetDebug(fn flow.DebugFunc)                { n.debug = fn }
func (n *MqttInNode) SetConfigLookup(fn flow.ConfigLookupFunc)  { n.configLookup = fn }

func (n *MqttInNode) Start() error {
	broker, err := resolveConfigInstance[MqttBroker](n.configLookup, n.brokerID, n.status, resolveConfigParams{
		NodeKind:   "mqtt-in",
		NodeID:     n.config.ID,
		ConfigKind: "broker",
		TypeLabel:  "an MQTT broker",
	})
	if err != nil {
		return err
	}
	n.broker = broker

	// Wrap status callback so connected-state can be enriched with node-local
	// detail (active topic count / static topic) — disconnected/connecting
	// states are passed through unchanged.
	n.broker.RegisterStatusFunc(n.handleBrokerStatus)

	if n.mode == "static" {
		if err := n.broker.Subscribe(n.config.ID, n.topic, n.subscribeOptionsFromConfig(), n.onMessage); err != nil {
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

// subscribeOptionsFromConfig builds SubscribeOptions from the static
// configuration. Used directly in static mode and as the default in dynamic
// mode before applying per-message overrides.
func (n *MqttInNode) subscribeOptionsFromConfig() SubscribeOptions {
	return SubscribeOptions{
		QoS:                    n.qos,
		NoLocal:                n.noLocal,
		RetainAsPublished:      n.retainAsPublished,
		RetainHandling:         n.retainHandling,
		SubscriptionIdentifier: n.subscriptionIdentifier,
		UserProperties:         n.subscribeUserProperties,
	}
}

func (n *MqttInNode) onMessage(p *paho.Publish) {
	msg := flow.NewMessage()
	msg.Set("topic", p.Topic)
	setPayload(msg, p.Payload, n.outputFormat)
	msg.Set("qos", int(p.QoS))
	msg.Set("retain", p.Retain)

	if props := p.Properties; props != nil {
		if len(props.User) > 0 {
			up := make(map[string]string, len(props.User))
			for _, kv := range props.User {
				up[kv.Key] = kv.Value
			}
			msg.Set("userProperties", up)
		}
		if props.ContentType != "" {
			msg.Set("contentType", props.ContentType)
		}
		if props.ResponseTopic != "" {
			msg.Set("responseTopic", props.ResponseTopic)
		}
		if len(props.CorrelationData) > 0 {
			msg.Set("correlationData", props.CorrelationData)
		}
		if props.MessageExpiry != nil {
			msg.Set("messageExpiry", *props.MessageExpiry)
		}
		if props.PayloadFormat != nil {
			msg.Set("payloadFormat", int(*props.PayloadFormat))
		}
		if props.SubscriptionIdentifier != nil {
			msg.Set("subscriptionIdentifier", *props.SubscriptionIdentifier)
		}
	}

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
	opts := n.applyDynamicOverrides(msg)

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
		if err := n.broker.Subscribe(n.config.ID, t, opts, n.onMessage); err != nil {
			slog.Warn("mqtt-in: subscribe failed", "node_id", n.config.ID, "topic", t, "error", err)
		}
	}
	if len(next) > 0 {
		slog.Info("mqtt-in dynamic subscribe", "node_id", n.config.ID, "topics", next, "qos", opts.QoS)
	}

	n.refreshStatus()
	return nil, nil
}

// applyDynamicOverrides starts from the configured SubscribeOptions and
// overlays any msg.* fields the control message provides. Missing or invalid
// fields fall back to the configured value.
func (n *MqttInNode) applyDynamicOverrides(msg *flow.Message) SubscribeOptions {
	opts := n.subscribeOptionsFromConfig()
	opts.QoS = extractQoS(msg.Get("qos"), opts.QoS)

	if v, ok := readBool(msg.Get("noLocal")); ok {
		opts.NoLocal = v
	}
	if v, ok := readBool(msg.Get("retainAsPublished")); ok {
		opts.RetainAsPublished = v
	}
	opts.RetainHandling = extractRetainHandling(msg.Get("retainHandling"), opts.RetainHandling)
	if v, ok := readPositiveInt(msg.Get("subscriptionIdentifier")); ok {
		opts.SubscriptionIdentifier = v
	}
	return opts
}

// readBool extracts a boolean from typical wire formats. Returns ok=false if
// the value is absent or not a bool.
func readBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}

// readPositiveInt parses a positive integer from typical wire formats. Returns
// ok=false for absent, zero, or negative values.
func readPositiveInt(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		if x <= 0 {
			return 0, false
		}
		return int(x), true
	case int:
		if x <= 0 {
			return 0, false
		}
		return x, true
	default:
		return 0, false
	}
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
			"broker":                  "",
			"mode":                    "static",
			"topic":                   "",
			"qos":                     0,
			"outputFormat":            "string",
			"noLocal":                 false,
			"retainAsPublished":       false,
			"retainHandling":          0,
			"subscriptionIdentifier":  0,
			"subscribeUserProperties": map[string]string{},
		},
		Inputs:  0,
		Outputs: 1,
	}
}
