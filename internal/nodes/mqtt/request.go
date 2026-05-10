// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package mqtt

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

const (
	mqttRequestTimeoutModeError       = "error"
	mqttRequestTimeoutModePassthrough = "passthrough"

	defaultMqttRequestPrefix  = "loopze/response"
	defaultMqttRequestTimeout = 30 * time.Second
)

// MqttRequestNode implements the MQTT v5 request/response pattern. For each
// inbound message it generates a unique response topic and correlation data,
// opens a one-shot subscription on the response topic, publishes the request
// with v5 ResponseTopic + CorrelationData properties, and waits for the
// matching response (or for the timeout). On match it forwards the response
// on output 0 and tears down the subscription; on timeout it either emits a
// catchable error (timeoutMode=error) or a passthrough message with
// msg.timedOut=true (timeoutMode=passthrough).
//
// Multiple inflight requests are supported — every call has its own response
// topic and is tracked under the hex-encoded correlation data.
//
// Implements flow.ConfigProvider and flow.ErrorProvider.
type MqttRequestNode struct {
	config       flow.NodeConfig
	nodes.BaseNode
	configLookup flow.ConfigLookupFunc
	errorFn      flow.ErrorFunc

	brokerID       string
	topic          string
	qos            byte
	retain         bool
	prefix         string
	timeout        time.Duration
	timeoutMode    string
	responseFormat string

	// MQTT v5 default publish properties, merged with msg overrides at
	// HandleMessage time. ResponseTopic and CorrelationData are NOT
	// configurable here — they are generated per request.
	defaultUserProperties map[string]string
	defaultContentType    string
	defaultMessageExpiry  *uint32
	defaultPayloadFormat  *byte

	broker *MqttBroker

	mu      sync.Mutex
	pending map[string]*mqttRequestInflight // key = hex(correlationData)
}

type mqttRequestInflight struct {
	correlation   []byte
	responseTopic string
	timer         *time.Timer
	inMsg         *flow.Message // input msg for passthrough on timeout / source for downstream metadata
}

// NewMqttRequestNode creates a new MQTT request node.
func NewMqttRequestNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &MqttRequestNode{
		config:  config,
		pending: make(map[string]*mqttRequestInflight),
	}, nil
}

func (n *MqttRequestNode) Init() error {
	props := n.config.Properties

	n.brokerID, _ = props["broker"].(string)
	if n.brokerID == "" {
		return fmt.Errorf("mqtt-request %s: no broker configured", n.config.ID)
	}

	n.topic, _ = props["topic"].(string)
	// topic is optional — can come from msg.topic at runtime

	n.qos = extractQoS(props["qos"], 0)

	if v, ok := props["retain"].(bool); ok {
		n.retain = v
	}

	prefix, _ := props["responseTopicPrefix"].(string)
	if prefix == "" {
		prefix = defaultMqttRequestPrefix
	}
	n.prefix = strings.TrimRight(prefix, "/")

	timeoutSec, ok := readUint32(props["timeout"])
	if !ok {
		n.timeout = defaultMqttRequestTimeout
	} else {
		n.timeout = time.Duration(timeoutSec) * time.Second
	}

	mode, _ := props["timeoutMode"].(string)
	switch mode {
	case "", mqttRequestTimeoutModeError:
		n.timeoutMode = mqttRequestTimeoutModeError
	case mqttRequestTimeoutModePassthrough:
		n.timeoutMode = mqttRequestTimeoutModePassthrough
	default:
		return fmt.Errorf("mqtt-request %s: invalid timeoutMode %q (expected error|passthrough)", n.config.ID, mode)
	}

	n.responseFormat, _ = props["responseFormat"].(string)
	switch n.responseFormat {
	case "", "string":
		n.responseFormat = "string"
	case "json", "buffer":
	default:
		slog.Warn("mqtt-request: unknown responseFormat, falling back to string",
			"node_id", n.config.ID, "responseFormat", n.responseFormat)
		n.responseFormat = "string"
	}

	n.defaultUserProperties = readStringMap(props["defaultUserProperties"])
	if ct, ok := props["defaultContentType"].(string); ok && ct != "" {
		n.defaultContentType = ct
	}
	if me, ok := readUint32(props["defaultMessageExpiry"]); ok && me > 0 {
		n.defaultMessageExpiry = &me
	}
	if pf, ok := readPayloadFormat(props["defaultPayloadFormat"]); ok {
		n.defaultPayloadFormat = &pf
	}

	return nil
}

func (n *MqttRequestNode) SetConfigLookup(fn flow.ConfigLookupFunc) { n.configLookup = fn }
func (n *MqttRequestNode) SetError(fn flow.ErrorFunc)                { n.errorFn = fn }

func (n *MqttRequestNode) Start() error {
	broker, err := nodes.ResolveConfigInstance[MqttBroker](n.configLookup, n.brokerID, n.Status, nodes.ResolveConfigParams{
		NodeKind:   "mqtt-request",
		NodeID:     n.config.ID,
		ConfigKind: "broker",
		TypeLabel:  "an MQTT broker",
	})
	if err != nil {
		return err
	}
	n.broker = broker

	n.broker.RegisterStatusFunc(n.handleBrokerStatus)
	n.broker.RegisterConnectionDownFunc(n.config.ID, n.failAllInflight)

	slog.Info("mqtt-request node started",
		"node_id", n.config.ID, "topic", n.topic, "qos", n.qos,
		"timeout", n.timeout, "timeoutMode", n.timeoutMode,
	)
	return nil
}

func (n *MqttRequestNode) Stop() error {
	if n.broker != nil {
		n.broker.UnregisterConnectionDownFunc(n.config.ID)
	}
	n.failAllInflight()
	slog.Info("mqtt-request node stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage builds a fresh request context (random response topic +
// correlation data), subscribes for the reply, and publishes the request.
// IMPORTANT: subscribe MUST complete before publish, so the broker has the
// subscription installed before any quick reply lands. MqttBroker.Subscribe
// awaits SUBACK, so the synchronous order Subscribe → Publish here is safe.
func (n *MqttRequestNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		msg = flow.NewMessage()
	}

	topic := n.topic
	if topic == "" {
		topic = msg.Topic()
	}
	if topic == "" {
		return nil, fmt.Errorf("mqtt-request %s: no topic configured and msg.topic is empty", n.config.ID)
	}

	qos := extractQoS(msg.Get("qos"), n.qos)

	correlation, err := randomCorrelationData()
	if err != nil {
		return nil, fmt.Errorf("mqtt-request %s: correlation data: %w", n.config.ID, err)
	}
	responseTopic := n.prefix + "/" + randomResponseSuffix()
	key := hex.EncodeToString(correlation)

	ctx := &mqttRequestInflight{
		correlation:   correlation,
		responseTopic: responseTopic,
		inMsg:         msg,
	}
	n.mu.Lock()
	n.pending[key] = ctx
	n.mu.Unlock()

	// NoLocal MUST be false here. We share one broker connection (= one
	// MQTT client ID) with every other mqtt-* node referencing the same
	// broker config, and a paired mqtt-out target=responseTopic on the same
	// broker would have its response filtered out by the broker if NoLocal
	// were true (MQTT v5 §3.8.3.1). The response topic itself is a random
	// UUID, so there is no echo-loop risk to guard against.
	if err := n.broker.Subscribe(n.config.ID, responseTopic, SubscribeOptions{
		QoS:            qos,
		NoLocal:        false,
		RetainHandling: 2, // never deliver retained on this short-lived subscription
	}, n.onResponse); err != nil {
		n.removePending(key)
		return nil, fmt.Errorf("mqtt-request %s: subscribe: %w", n.config.ID, err)
	}

	payload, err := toBytes(msg.Payload())
	if err != nil {
		_ = n.broker.Unsubscribe(n.config.ID, responseTopic)
		n.removePending(key)
		return nil, fmt.Errorf("mqtt-request %s: payload conversion: %w", n.config.ID, err)
	}

	props := n.buildPublishProperties(msg, responseTopic, correlation)

	if err := n.broker.Publish(topic, qos, n.retain, payload, props); err != nil {
		_ = n.broker.Unsubscribe(n.config.ID, responseTopic)
		n.removePending(key)
		return nil, fmt.Errorf("mqtt-request %s: publish: %w", n.config.ID, err)
	}

	if n.timeout > 0 {
		ctx.timer = time.AfterFunc(n.timeout, func() { n.onTimeout(key) })
	}

	n.refreshStatus()
	return nil, nil
}

// onResponse is the callback invoked by the broker when a publish lands on
// our temporary response topic. We match by correlation data — anything
// without correlation, or with mismatched correlation, is dropped.
func (n *MqttRequestNode) onResponse(p *paho.Publish) {
	if p == nil || p.Properties == nil || len(p.Properties.CorrelationData) == 0 {
		return
	}
	key := hex.EncodeToString(p.Properties.CorrelationData)

	ctx := n.takePending(key)
	if ctx == nil {
		return // unknown / late response, or already timed out
	}
	if ctx.timer != nil {
		ctx.timer.Stop()
	}
	if err := n.broker.Unsubscribe(n.config.ID, ctx.responseTopic); err != nil {
		slog.Warn("mqtt-request: unsubscribe on response failed",
			"node_id", n.config.ID, "topic", ctx.responseTopic, "error", err)
	}

	out := ctx.inMsg.COWClone()
	if reqTopic := ctx.inMsg.Topic(); reqTopic != "" {
		out.Set("requestTopic", reqTopic)
	}
	out.Set("topic", p.Topic)

	value, parseErr := decodeMqttPayload(p.Payload, n.responseFormat)
	out.Set("payload", value)
	if parseErr != "" {
		out.Set("parseError", parseErr)
	}
	out.Set("qos", int(p.QoS))
	out.Set("retain", p.Retain)
	out.Set("correlationData", append([]byte(nil), p.Properties.CorrelationData...))

	if props := p.Properties; props != nil {
		if len(props.User) > 0 {
			up := make(map[string]string, len(props.User))
			for _, kv := range props.User {
				up[kv.Key] = kv.Value
			}
			out.Set("userProperties", up)
		}
		if props.ContentType != "" {
			out.Set("contentType", props.ContentType)
		}
		if props.MessageExpiry != nil {
			out.Set("messageExpiry", *props.MessageExpiry)
		}
		if props.PayloadFormat != nil {
			out.Set("payloadFormat", int(*props.PayloadFormat))
		}
	}

	n.Send(0, out)
	n.refreshStatus()
}

// onTimeout is fired by the inflight context's timer when no response
// arrived in time. Either raises a catchable error (timeoutMode=error) or
// emits a passthrough message with msg.timedOut=true.
func (n *MqttRequestNode) onTimeout(key string) {
	ctx := n.takePending(key)
	if ctx == nil {
		return // response already received between timer fire and lookup — race-safe drop
	}
	if err := n.broker.Unsubscribe(n.config.ID, ctx.responseTopic); err != nil {
		slog.Warn("mqtt-request: unsubscribe on timeout failed",
			"node_id", n.config.ID, "topic", ctx.responseTopic, "error", err)
	}

	if n.timeoutMode == mqttRequestTimeoutModePassthrough {
		out := ctx.inMsg.COWClone()
		out.Set("timedOut", true)
		n.Send(0, out)
	} else if n.errorFn != nil {
		n.errorFn(errors.New("mqtt-request: timeout"), ctx.inMsg)
	}

	n.refreshStatus()
}

// failAllInflight drains every pending context — used both by Stop() and on
// broker disconnect. Each context is treated like a timeout: passthrough or
// catchable error per the configured timeoutMode.
func (n *MqttRequestNode) failAllInflight() {
	n.mu.Lock()
	if len(n.pending) == 0 {
		n.mu.Unlock()
		return
	}
	pending := n.pending
	n.pending = make(map[string]*mqttRequestInflight)
	n.mu.Unlock()

	for _, ctx := range pending {
		if ctx.timer != nil {
			ctx.timer.Stop()
		}
		if n.broker != nil {
			_ = n.broker.Unsubscribe(n.config.ID, ctx.responseTopic)
		}
		if n.timeoutMode == mqttRequestTimeoutModePassthrough {
			if n.Send != nil {
				out := ctx.inMsg.COWClone()
				out.Set("timedOut", true)
				n.Send(0, out)
			}
		} else if n.errorFn != nil {
			n.errorFn(errors.New("mqtt-request: connection lost"), ctx.inMsg)
		}
	}

	n.refreshStatus()
}

// takePending atomically removes and returns the inflight context for key.
// Returns nil if the key is not present (already taken, late, or unknown).
func (n *MqttRequestNode) takePending(key string) *mqttRequestInflight {
	n.mu.Lock()
	defer n.mu.Unlock()
	ctx, ok := n.pending[key]
	if !ok {
		return nil
	}
	delete(n.pending, key)
	return ctx
}

// removePending discards a pending context without firing any callbacks. Used
// during the setup error path (subscribe / publish failed) where the caller
// reports the error via the HandleMessage return value instead.
func (n *MqttRequestNode) removePending(key string) {
	n.mu.Lock()
	delete(n.pending, key)
	n.mu.Unlock()
}

// buildPublishProperties is mqtt-request's variant of mqtt-out's merge logic.
// ResponseTopic and CorrelationData are ALWAYS set by the node; user-supplied
// msg.responseTopic / msg.correlationData are deliberately ignored — the
// request/response correlation is owned by this node.
func (n *MqttRequestNode) buildPublishProperties(msg *flow.Message, responseTopic string, correlation []byte) *paho.PublishProperties {
	props := &paho.PublishProperties{
		ResponseTopic:   responseTopic,
		CorrelationData: correlation,
	}

	merged := mergeUserProperties(n.defaultUserProperties, msg.Get("userProperties"))
	for k, v := range merged {
		props.User.Add(k, v)
	}

	contentType := n.defaultContentType
	if ct, ok := msg.Get("contentType").(string); ok && ct != "" {
		contentType = ct
	}
	if contentType != "" {
		props.ContentType = contentType
	}

	if me, ok := readUint32(msg.Get("messageExpiry")); ok {
		props.MessageExpiry = &me
	} else if n.defaultMessageExpiry != nil {
		v := *n.defaultMessageExpiry
		props.MessageExpiry = &v
	}

	if pf, ok := readPayloadFormat(msg.Get("payloadFormat")); ok {
		props.PayloadFormat = &pf
	} else if n.defaultPayloadFormat != nil {
		v := *n.defaultPayloadFormat
		props.PayloadFormat = &v
	}

	return props
}

// handleBrokerStatus mirrors mqtt-in's pattern: pass disconnected/connecting
// through verbatim and override the green state with node-local detail.
func (n *MqttRequestNode) handleBrokerStatus(fill, text string) {
	if fill != "green" {
		n.Status(fill, text)
		return
	}
	n.refreshStatus()
}

// refreshStatus updates the green-state status string based on inflight count.
func (n *MqttRequestNode) refreshStatus() {
	if n.Status == nil {
		return
	}
	n.mu.Lock()
	count := len(n.pending)
	n.mu.Unlock()

	if count == 0 {
		n.Status("green", "idle")
		return
	}
	n.Status("green", fmt.Sprintf("%d inflight", count))
}

// randomCorrelationData returns 16 cryptographically random bytes for v5
// CorrelationData. The bytes are opaque to the broker but the mqtt-request
// node uses them to match the response back to the originating request.
func randomCorrelationData() ([]byte, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// randomResponseSuffix returns a hex string used as the per-request unique
// segment of the response topic. We use math/rand/v2 here because the suffix
// is for collision avoidance among the node's own pending requests, not for
// security; with 128 random bits the probability of collision is negligible
// even at very high request rates.
func randomResponseSuffix() string {
	var b [16]byte
	binary.LittleEndian.PutUint64(b[:8], mathrand.Uint64())
	binary.LittleEndian.PutUint64(b[8:], mathrand.Uint64())
	return hex.EncodeToString(b[:])
}

// MqttRequestTypeInfo returns the node type metadata for the palette.
func MqttRequestTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "mqtt-request",
		Category:    "network",
		Label:       "MQTT Request",
		Description: "Sends an MQTT request and waits for a v5 response (Response Topic + Correlation Data)",
		Icon:        "wifi",
		Defaults: map[string]any{
			"broker":                "",
			"topic":                 "",
			"qos":                   0,
			"retain":                false,
			"responseTopicPrefix":   defaultMqttRequestPrefix,
			"timeout":               int(defaultMqttRequestTimeout / time.Second),
			"timeoutMode":           mqttRequestTimeoutModeError,
			"responseFormat":        "string",
			"defaultUserProperties": map[string]string{},
			"defaultContentType":    "",
			"defaultMessageExpiry":  0,
			"defaultPayloadFormat":  0,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
