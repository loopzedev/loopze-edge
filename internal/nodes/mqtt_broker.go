// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// BrokerMessageHandler is invoked by the broker for each incoming MQTT publish
// that matches a subscribed pattern. It receives the full v5 publish so callers
// can read user properties, content type, correlation data etc.
type BrokerMessageHandler func(*paho.Publish)

// SubscribeOptions captures the per-subscription parameters that affect both
// how the SUBSCRIBE packet is sent to the broker and how the broker delivers
// matching messages back. All fields except QoS are MQTT v5-only; in a future
// v3.1.1 path they would be silently dropped.
//
// SubscriptionIdentifier == 0 means "not set" (the MQTT spec excludes 0 from
// valid identifiers, so this is a safe sentinel).
//
// UserProperties holds SUBSCRIBE-packet-level user properties. They are rarely
// used; when two subscribers share otherwise-identical options but provide
// different user properties, the first set wins (a warning is logged).
type SubscribeOptions struct {
	QoS                    byte
	NoLocal                bool
	RetainAsPublished      bool
	RetainHandling         byte // 0 = always send retained, 1 = only if new sub, 2 = never
	SubscriptionIdentifier int  // 0 = not set
	UserProperties         map[string]string
}

// muxKey identifies a unique paho-client subscription. Subscribers that share
// the same muxKey share one wire-level subscription; differences in any field
// force a separate SUBSCRIBE packet so the broker honours each set of options.
type muxKey struct {
	topic                  string
	noLocal                bool
	retainAsPublished      bool
	retainHandling         byte
	subscriptionIdentifier int
}

// muxEntry tracks the state of one paho-client subscription: the effective QoS
// (max requested by any sharing subscriber), the handlers to dispatch incoming
// publishes to, and the SUBSCRIBE-packet user properties (first writer wins).
type muxEntry struct {
	qos            byte
	userProperties map[string]string
	handlers       []handlerRef
}

type handlerRef struct {
	subscriberID string
	handler      BrokerMessageHandler
}

// MqttBroker is a config node that manages a shared MQTT v5 client connection.
// Multiple mqtt-in and mqtt-out nodes can reference the same broker instance.
// Implements flow.ConfigInstance.
type MqttBroker struct {
	id          string
	name        string
	cfg         autopaho.ClientConfig
	connectCtx  context.Context
	cancel      context.CancelFunc
	stopTimeout time.Duration

	onConnectMsg    *presenceMessage
	onDisconnectMsg *presenceMessage

	mu               sync.RWMutex
	cm               *autopaho.ConnectionManager
	subscribers      map[string]map[string]muxKey // subscriberID → topic → muxKey of the entry that holds this subscriber's handler
	muxSubscriptions map[muxKey]*muxEntry         // wire-level subscriptions
	statusFuncs      []flow.StatusFunc            // broadcast status to all referencing nodes
	currentFill      string
	currentText      string
}

// presenceMessage holds the configuration for the onConnect / onDisconnect publishes.
type presenceMessage struct {
	topic   string
	payload []byte
	qos     byte
	retain  bool
}

// NewMqttBroker creates a new MqttBroker config instance from a ConfigNode definition.
func NewMqttBroker(cfg flow.ConfigNode) (flow.ConfigInstance, error) {
	props := cfg.Config

	host, _ := props["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("mqtt-broker %s: host is required", cfg.ID)
	}

	port := 1883
	if v, ok := props["port"].(float64); ok && v > 0 {
		port = int(v)
	}

	clientID, _ := props["clientId"].(string)
	if clientID == "" {
		suffix := cfg.ID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		clientID = fmt.Sprintf("loopze-%s", suffix)
	}

	protocolVersion, _ := props["protocolVersion"].(string)
	if protocolVersion == "" {
		protocolVersion = "5"
	}
	if protocolVersion != "5" {
		return nil, fmt.Errorf("mqtt-broker %s: protocolVersion %q not supported in this build (only v5)", cfg.ID, protocolVersion)
	}

	username, _ := props["username"].(string)
	password, _ := props["password"].(string)

	keepalive := uint16(60)
	if v, ok := props["keepalive"].(float64); ok && v > 0 {
		keepalive = uint16(v)
	}

	cleanStart := true
	if v, ok := props["cleanStart"].(bool); ok {
		cleanStart = v
	}

	sessionExpiry := uint32(0)
	if v, ok := props["sessionExpiry"].(float64); ok && v >= 0 {
		sessionExpiry = uint32(v)
	}

	useTLS := false
	if v, ok := props["useTLS"].(bool); ok {
		useTLS = v
	}

	scheme := "mqtt"
	if useTLS {
		scheme = "mqtts"
	}
	serverURL, err := url.Parse(fmt.Sprintf("%s://%s:%d", scheme, host, port))
	if err != nil {
		return nil, fmt.Errorf("mqtt-broker %s: invalid server url: %w", cfg.ID, err)
	}

	b := &MqttBroker{
		id:               cfg.ID,
		name:             cfg.Name,
		subscribers:      make(map[string]map[string]muxKey),
		muxSubscriptions: make(map[muxKey]*muxEntry),
		currentFill:      "grey",
		currentText:      "disconnected",
		stopTimeout:      2 * time.Second,
		onConnectMsg:     readPresenceMessage(props, "onConnect"),
		onDisconnectMsg:  readPresenceMessage(props, "onDisconnect"),
	}

	clientCfg := autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{serverURL},
		KeepAlive:                     keepalive,
		CleanStartOnInitialConnection: cleanStart,
		SessionExpiryInterval:         sessionExpiry,
		ConnectUsername:               username,
		ConnectPassword:               []byte(password),
		OnConnectionUp:                b.onConnectionUp,
		OnConnectError:                b.onConnectError,
		OnConnectionDown:              b.onConnectionDown,
		ClientConfig: paho.ClientConfig{
			ClientID:          clientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){b.onPublishReceived},
			OnServerDisconnect: func(d *paho.Disconnect) {
				slog.Warn("mqtt broker server disconnect", "id", b.id, "reason", d.ReasonCode)
				b.setStatus("yellow", "reconnecting...")
			},
			OnClientError: func(err error) {
				slog.Warn("mqtt broker client error", "id", b.id, "error", err)
			},
		},
	}

	if useTLS {
		clientCfg.TlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	if will := readWill(props); will != nil {
		clientCfg.WillMessage = &paho.WillMessage{
			Retain:  will.retain,
			QoS:     will.qos,
			Topic:   will.topic,
			Payload: will.payload,
		}
		if will.delayInterval > 0 {
			d := will.delayInterval
			clientCfg.WillProperties = &paho.WillProperties{WillDelayInterval: &d}
		}
	}

	b.cfg = clientCfg
	return b, nil
}

// MqttBrokerConfigTypeInfo returns the config type metadata for the frontend.
func MqttBrokerConfigTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "mqtt-broker",
		Label:       "MQTT Broker",
		Description: "MQTT v5 broker connection",
		Defaults: map[string]any{
			"host":                  "localhost",
			"port":                  1883,
			"clientId":              "",
			"protocolVersion":       "5",
			"username":              "",
			"password":              "",
			"keepalive":             60,
			"cleanStart":            true,
			"sessionExpiry":         0,
			"useTLS":                false,
			"onConnectTopic":        "",
			"onConnectPayload":      "",
			"onConnectQoS":          0,
			"onConnectRetain":       false,
			"onDisconnectTopic":     "",
			"onDisconnectPayload":   "",
			"onDisconnectQoS":       0,
			"onDisconnectRetain":    false,
			"lastWillTopic":         "",
			"lastWillPayload":       "",
			"lastWillQoS":           0,
			"lastWillRetain":        false,
			"lastWillDelayInterval": 0,
		},
	}
}

// Start initiates the MQTT connection asynchronously. Status updates are
// broadcast via the OnConnectionUp / OnConnectError hooks; Deploy is not
// blocked if the broker is unreachable.
func (b *MqttBroker) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.setStatusLocked("yellow", "connecting...")

	ctx, cancel := context.WithCancel(context.Background())
	b.connectCtx = ctx
	b.cancel = cancel

	cm, err := autopaho.NewConnection(ctx, b.cfg)
	if err != nil {
		cancel()
		b.connectCtx = nil
		b.cancel = nil
		b.setStatusLocked("red", err.Error())
		return fmt.Errorf("mqtt-broker %s: connect failed: %w", b.id, err)
	}
	b.cm = cm

	slog.Info("mqtt broker connecting (async)", "id", b.id, "name", b.name)
	return nil
}

// Stop publishes the onDisconnect presence message (if configured) and then
// disconnects the MQTT client.
func (b *MqttBroker) Stop() error {
	b.mu.Lock()
	cm := b.cm
	cancel := b.cancel
	b.cm = nil
	b.cancel = nil
	b.connectCtx = nil
	b.mu.Unlock()

	if cm != nil {
		if msg := b.onDisconnectMsg; msg != nil {
			ctx, cancel := context.WithTimeout(context.Background(), b.stopTimeout)
			if _, err := cm.Publish(ctx, &paho.Publish{
				QoS:     msg.qos,
				Retain:  msg.retain,
				Topic:   msg.topic,
				Payload: msg.payload,
			}); err != nil {
				slog.Warn("mqtt broker onDisconnect publish failed", "id", b.id, "error", err)
			}
			cancel()
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.stopTimeout)
		if err := cm.Disconnect(ctx); err != nil {
			slog.Warn("mqtt broker disconnect failed", "id", b.id, "error", err)
		}
		cancel()
	}

	if cancel != nil {
		cancel()
	}

	b.mu.Lock()
	b.setStatusLocked("grey", "stopped")
	b.mu.Unlock()

	slog.Info("mqtt broker stopped", "id", b.id)
	return nil
}

// Status returns the current connection state.
func (b *MqttBroker) Status() (string, string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentFill, b.currentText
}

// Subscribe registers a topic subscription on behalf of a subscriber (typically
// the node ID) with full v5 subscription options. Subscriptions sharing the same
// topic AND options reuse one wire-level paho-subscribe; if any option differs,
// a separate SUBSCRIBE packet is sent so the broker honours each option set.
//
// If a new subscriber requests a higher QoS than the entry is currently
// subscribed with, the entry is re-subscribed at the paho client with the
// elevated QoS so all sharing subscribers get at least their requested level.
//
// Calling Subscribe again for an existing (subscriberID, topic) pair replaces
// the previous registration — the old muxEntry is cleaned up first.
func (b *MqttBroker) Subscribe(subscriberID, topic string, opts SubscribeOptions, handler BrokerMessageHandler) error {
	key := muxKey{
		topic:                  topic,
		noLocal:                opts.NoLocal,
		retainAsPublished:      opts.RetainAsPublished,
		retainHandling:         opts.RetainHandling,
		subscriptionIdentifier: opts.SubscriptionIdentifier,
	}

	b.mu.Lock()

	// If this (subscriberID, topic) was already subscribed under a different key,
	// detach the old handler before installing the new one. Same-key resubscribes
	// are also tolerated (handler reference is replaced).
	if oldKey, ok := b.subscribers[subscriberID][topic]; ok {
		b.detachHandlerLocked(oldKey, subscriberID)
	}

	entry, hadEntry := b.muxSubscriptions[key]
	prevQoS := byte(0)
	if hadEntry {
		prevQoS = entry.qos
	}
	needPahoSub := !hadEntry
	if hadEntry && opts.QoS > entry.qos {
		entry.qos = opts.QoS
		needPahoSub = true
	}
	if !hadEntry {
		entry = &muxEntry{
			qos:            opts.QoS,
			userProperties: opts.UserProperties,
		}
		b.muxSubscriptions[key] = entry
	} else if len(entry.userProperties) == 0 && len(opts.UserProperties) > 0 {
		entry.userProperties = opts.UserProperties
	} else if len(opts.UserProperties) > 0 && !mapsEqual(entry.userProperties, opts.UserProperties) {
		slog.Warn("mqtt-broker: ignoring divergent SUBSCRIBE user properties for shared subscription",
			"id", b.id, "subscriber", subscriberID, "topic", topic)
	}
	entry.handlers = append(entry.handlers, handlerRef{subscriberID: subscriberID, handler: handler})

	if _, ok := b.subscribers[subscriberID]; !ok {
		b.subscribers[subscriberID] = make(map[string]muxKey)
	}
	b.subscribers[subscriberID][topic] = key

	cm := b.cm
	effectiveQoS := entry.qos
	subProps := buildSubscribeProperties(opts.SubscriptionIdentifier, entry.userProperties)
	b.mu.Unlock()

	if needPahoSub && cm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := cm.Subscribe(ctx, &paho.Subscribe{
			Properties: subProps,
			Subscriptions: []paho.SubscribeOptions{{
				Topic:             topic,
				QoS:               effectiveQoS,
				NoLocal:           opts.NoLocal,
				RetainAsPublished: opts.RetainAsPublished,
				RetainHandling:    opts.RetainHandling,
			}},
		})
		if err != nil {
			// Roll back: remove the just-added handler entry. Restore prior QoS
			// if the entry pre-existed; otherwise drop it entirely.
			b.mu.Lock()
			b.detachHandlerLocked(key, subscriberID)
			delete(b.subscribers[subscriberID], topic)
			if len(b.subscribers[subscriberID]) == 0 {
				delete(b.subscribers, subscriberID)
			}
			if cur, ok := b.muxSubscriptions[key]; ok && hadEntry {
				cur.qos = prevQoS
			}
			b.mu.Unlock()
			return fmt.Errorf("mqtt-broker %s: subscribe to %q failed: %w", b.id, topic, err)
		}
	}
	return nil
}

// Unsubscribe removes the subscription that (subscriberID, topic) refers to. If
// no other subscriber shares the same muxEntry, an UNSUBSCRIBE is sent to the
// broker.
func (b *MqttBroker) Unsubscribe(subscriberID, topic string) error {
	b.mu.Lock()
	key, ok := b.subscribers[subscriberID][topic]
	if !ok {
		b.mu.Unlock()
		return nil
	}

	releasePahoSub := b.detachHandlerLocked(key, subscriberID)
	delete(b.subscribers[subscriberID], topic)
	if len(b.subscribers[subscriberID]) == 0 {
		delete(b.subscribers, subscriberID)
	}
	cm := b.cm
	b.mu.Unlock()

	if releasePahoSub && cm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := cm.Unsubscribe(ctx, &paho.Unsubscribe{Topics: []string{topic}}); err != nil {
			return fmt.Errorf("mqtt-broker %s: unsubscribe %q failed: %w", b.id, topic, err)
		}
	}
	return nil
}

// detachHandlerLocked removes the handler belonging to subscriberID from the
// muxEntry identified by key, deleting the entry if it becomes empty. Returns
// true if the entry was deleted (caller must send UNSUBSCRIBE to the broker).
// Must be called with b.mu held.
func (b *MqttBroker) detachHandlerLocked(key muxKey, subscriberID string) bool {
	entry, ok := b.muxSubscriptions[key]
	if !ok {
		return false
	}
	filtered := entry.handlers[:0]
	for _, h := range entry.handlers {
		if h.subscriberID != subscriberID {
			filtered = append(filtered, h)
		}
	}
	entry.handlers = filtered
	if len(entry.handlers) == 0 {
		delete(b.muxSubscriptions, key)
		return true
	}
	return false
}

// buildSubscribeProperties returns the SubscribeProperties for a SUBSCRIBE
// packet, or nil if neither identifier nor user properties are set.
func buildSubscribeProperties(subID int, userProps map[string]string) *paho.SubscribeProperties {
	if subID == 0 && len(userProps) == 0 {
		return nil
	}
	props := &paho.SubscribeProperties{}
	if subID != 0 {
		v := subID
		props.SubscriptionIdentifier = &v
	}
	for k, val := range userProps {
		props.User.Add(k, val)
	}
	return props
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// Publish sends a message to the broker. v5 properties are optional — pass nil
// to publish without any.
func (b *MqttBroker) Publish(topic string, qos byte, retain bool, payload []byte, props *paho.PublishProperties) error {
	b.mu.RLock()
	cm := b.cm
	b.mu.RUnlock()

	if cm == nil {
		return fmt.Errorf("mqtt-broker %s: not connected", b.id)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := cm.Publish(ctx, &paho.Publish{
		QoS:        qos,
		Retain:     retain,
		Topic:      topic,
		Payload:    payload,
		Properties: props,
	})
	return err
}

// RegisterStatusFunc allows MQTT nodes to register their status callback so the
// broker can broadcast connection state changes to all of them.
func (b *MqttBroker) RegisterStatusFunc(fn flow.StatusFunc) {
	b.mu.Lock()
	b.statusFuncs = append(b.statusFuncs, fn)
	fill, text := b.currentFill, b.currentText
	b.mu.Unlock()

	fn(fill, text)
}

// onPublishReceived is the single mux handler registered at the paho client.
// It dispatches each incoming publish to every subscriber whose pattern matches
// the concrete topic. We iterate muxSubscriptions (one entry per unique
// topic+option-set) and dispatch to all handlers grouped under it.
func (b *MqttBroker) onPublishReceived(pr paho.PublishReceived) (bool, error) {
	topic := pr.Packet.Topic

	b.mu.RLock()
	handlers := make([]BrokerMessageHandler, 0, 4)
	for key, entry := range b.muxSubscriptions {
		if topicMatchesPattern(key.topic, topic) {
			for _, h := range entry.handlers {
				handlers = append(handlers, h.handler)
			}
		}
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		h(pr.Packet)
	}
	return len(handlers) > 0, nil
}

// onConnectionUp is invoked by autopaho after every successful CONNACK
// (initial connect and reconnect). We publish the onConnect presence message,
// rebuild all subscriptions and broadcast green status.
func (b *MqttBroker) onConnectionUp(cm *autopaho.ConnectionManager, _ *paho.Connack) {
	slog.Info("mqtt broker connected", "id", b.id, "name", b.name)

	if msg := b.onConnectMsg; msg != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if _, err := cm.Publish(ctx, &paho.Publish{
			QoS:     msg.qos,
			Retain:  msg.retain,
			Topic:   msg.topic,
			Payload: msg.payload,
		}); err != nil {
			slog.Warn("mqtt broker onConnect publish failed", "id", b.id, "error", err)
		}
		cancel()
	}

	// Snapshot the muxSubscriptions before sending. We must release the lock
	// before the network calls; SubscribeProperties.SubscriptionIdentifier and
	// User Properties apply per-packet, so each muxEntry needs its own
	// SUBSCRIBE — no batching across entries.
	b.mu.Lock()
	type pendingSub struct {
		key   muxKey
		qos   byte
		props *paho.SubscribeProperties
	}
	pending := make([]pendingSub, 0, len(b.muxSubscriptions))
	for key, entry := range b.muxSubscriptions {
		pending = append(pending, pendingSub{
			key:   key,
			qos:   entry.qos,
			props: buildSubscribeProperties(key.subscriptionIdentifier, entry.userProperties),
		})
	}
	b.setStatusLocked("green", "connected")
	b.mu.Unlock()

	for _, p := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := cm.Subscribe(ctx, &paho.Subscribe{
			Properties: p.props,
			Subscriptions: []paho.SubscribeOptions{{
				Topic:             p.key.topic,
				QoS:               p.qos,
				NoLocal:           p.key.noLocal,
				RetainAsPublished: p.key.retainAsPublished,
				RetainHandling:    p.key.retainHandling,
			}},
		})
		cancel()
		if err != nil {
			slog.Warn("mqtt broker re-subscribe failed", "id", b.id, "topic", p.key.topic, "error", err)
		}
	}
}

func (b *MqttBroker) onConnectError(err error) {
	slog.Warn("mqtt broker connect error", "id", b.id, "error", err)
	b.setStatus("red", err.Error())
}

func (b *MqttBroker) onConnectionDown() bool {
	slog.Warn("mqtt broker connection down", "id", b.id)
	b.setStatus("yellow", "reconnecting...")
	return true
}

// setStatus updates the status and broadcasts to all registered nodes.
func (b *MqttBroker) setStatus(fill, text string) {
	b.mu.Lock()
	b.setStatusLocked(fill, text)
	b.mu.Unlock()
}

// setStatusLocked must be called with b.mu held.
func (b *MqttBroker) setStatusLocked(fill, text string) {
	b.currentFill = fill
	b.currentText = text
	for _, fn := range b.statusFuncs {
		fn(fill, text)
	}
}

// readPresenceMessage returns nil if no topic is configured for the given prefix
// (e.g. "onConnect" → reads "onConnectTopic", "onConnectPayload", …).
func readPresenceMessage(props map[string]any, prefix string) *presenceMessage {
	topic, _ := props[prefix+"Topic"].(string)
	if topic == "" {
		return nil
	}
	payload, _ := props[prefix+"Payload"].(string)
	qos := extractQoS(props[prefix+"QoS"], 0)
	retain := false
	if v, ok := props[prefix+"Retain"].(bool); ok {
		retain = v
	}
	return &presenceMessage{
		topic:   topic,
		payload: []byte(payload),
		qos:     qos,
		retain:  retain,
	}
}

// willConfig is the parsed lastWill section of the broker properties.
type willConfig struct {
	topic         string
	payload       []byte
	qos           byte
	retain        bool
	delayInterval uint32
}

func readWill(props map[string]any) *willConfig {
	topic, _ := props["lastWillTopic"].(string)
	if topic == "" {
		return nil
	}
	payload, _ := props["lastWillPayload"].(string)
	qos := extractQoS(props["lastWillQoS"], 0)
	retain := false
	if v, ok := props["lastWillRetain"].(bool); ok {
		retain = v
	}
	delay := uint32(0)
	if v, ok := props["lastWillDelayInterval"].(float64); ok && v >= 0 {
		delay = uint32(v)
	}
	return &willConfig{
		topic:         topic,
		payload:       []byte(payload),
		qos:           qos,
		retain:        retain,
		delayInterval: delay,
	}
}

// topicMatchesPattern reports whether the concrete MQTT topic matches the
// subscription pattern. Supports the `+` (single-level) and `#` (multi-level
// trailing) wildcards as defined by MQTT v5 §4.7.
func topicMatchesPattern(pattern, topic string) bool {
	if pattern == topic {
		return true
	}
	pSegs := strings.Split(pattern, "/")
	tSegs := strings.Split(topic, "/")
	for i, p := range pSegs {
		if p == "#" {
			// `#` must be the final segment in a valid pattern; matches all remaining.
			return i <= len(tSegs)
		}
		if i >= len(tSegs) {
			return false
		}
		if p == "+" {
			continue
		}
		if p != tSegs[i] {
			return false
		}
	}
	return len(pSegs) == len(tSegs)
}
