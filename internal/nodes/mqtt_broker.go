// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/niceclouds/flint/internal/flow"
)

// MqttBroker is a config node that manages a shared MQTT v3.1.1 client connection.
// Multiple mqtt-in and mqtt-out nodes can reference the same broker instance.
// Implements flow.ConfigInstance.
type MqttBroker struct {
	id   string
	name string
	opts *mqtt.ClientOptions

	mu          sync.RWMutex
	client      mqtt.Client
	subscribers map[string]brokerSub     // topic → subscription info
	statusFuncs []flow.StatusFunc        // broadcast status to all referencing nodes
	currentFill string                   // current status fill color
	currentText string                   // current status text
}

// brokerSub holds a subscription registered by an mqtt-in node.
type brokerSub struct {
	qos     byte
	handler mqtt.MessageHandler
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
		clientID = fmt.Sprintf("flint-%s", cfg.ID[:8])
	}

	username, _ := props["username"].(string)
	password, _ := props["password"].(string)

	keepalive := 60
	if v, ok := props["keepalive"].(float64); ok && v > 0 {
		keepalive = int(v)
	}

	cleanSession := true
	if v, ok := props["cleanSession"].(bool); ok {
		cleanSession = v
	}

	broker := fmt.Sprintf("tcp://%s:%d", host, port)

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetKeepAlive(time.Duration(keepalive) * time.Second).
		SetCleanSession(cleanSession).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(30 * time.Second)

	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	b := &MqttBroker{
		id:          cfg.ID,
		name:        cfg.Name,
		opts:        opts,
		subscribers: make(map[string]brokerSub),
		currentFill: "grey",
		currentText: "disconnected",
	}

	opts.SetOnConnectHandler(b.onConnect)
	opts.SetConnectionLostHandler(b.onConnectionLost)
	opts.SetReconnectingHandler(b.onReconnecting)

	return b, nil
}

// MqttBrokerConfigTypeInfo returns the config type metadata for the frontend.
func MqttBrokerConfigTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "mqtt-broker",
		Label:       "MQTT Broker",
		Description: "MQTT v3.1.1 broker connection",
		Defaults: map[string]any{
			"host":         "localhost",
			"port":         1883,
			"clientId":     "",
			"username":     "",
			"password":     "",
			"keepalive":    60,
			"cleanSession": true,
			"useTLS":       false,
		},
	}
}

// Start initiates the MQTT connection asynchronously.
// The connection is established in the background so that Deploy is not blocked
// if the broker is unreachable. Status updates are broadcast via onConnect/onConnectionLost.
func (b *MqttBroker) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.setStatus("yellow", "connecting...")

	b.client = mqtt.NewClient(b.opts)
	// Connect asynchronously — paho handles retries via ConnectRetry option.
	// onConnect callback will set status to green and re-subscribe topics.
	go b.client.Connect()

	slog.Info("mqtt broker connecting (async)", "id", b.id, "name", b.name)
	return nil
}

// Stop disconnects the MQTT client.
func (b *MqttBroker) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil && b.client.IsConnected() {
		b.client.Disconnect(1000)
		slog.Info("mqtt broker disconnected", "id", b.id)
	}

	b.setStatus("grey", "stopped")
	return nil
}

// Status returns the current connection state.
func (b *MqttBroker) Status() (string, string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentFill, b.currentText
}

// Subscribe registers a topic subscription. If the client is connected,
// the subscription is established immediately.
func (b *MqttBroker) Subscribe(topic string, qos byte, handler mqtt.MessageHandler) error {
	b.mu.Lock()
	b.subscribers[topic] = brokerSub{qos: qos, handler: handler}
	client := b.client
	b.mu.Unlock()

	if client != nil && client.IsConnected() {
		token := client.Subscribe(topic, qos, handler)
		token.WaitTimeout(5 * time.Second)
		if token.Error() != nil {
			return fmt.Errorf("mqtt-broker %s: subscribe to %q failed: %w", b.id, topic, token.Error())
		}
	}
	return nil
}

// Publish sends a message to the broker.
func (b *MqttBroker) Publish(topic string, qos byte, retain bool, payload []byte) error {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("mqtt-broker %s: not connected", b.id)
	}

	token := client.Publish(topic, qos, retain, payload)
	token.WaitTimeout(5 * time.Second)
	return token.Error()
}

// RegisterStatusFunc allows MQTT nodes to register their status callback
// so the broker can broadcast connection state changes to all of them.
func (b *MqttBroker) RegisterStatusFunc(fn flow.StatusFunc) {
	b.mu.Lock()
	b.statusFuncs = append(b.statusFuncs, fn)
	// Immediately send current status to the new subscriber.
	fill, text := b.currentFill, b.currentText
	b.mu.Unlock()

	fn(fill, text)
}

func (b *MqttBroker) onConnect(client mqtt.Client) {
	slog.Info("mqtt broker connected", "id", b.id, "name", b.name)
	b.mu.Lock()
	b.setStatus("green", "connected")
	// Re-subscribe all topics after reconnect.
	for topic, sub := range b.subscribers {
		client.Subscribe(topic, sub.qos, sub.handler)
	}
	b.mu.Unlock()
}

func (b *MqttBroker) onConnectionLost(_ mqtt.Client, err error) {
	slog.Warn("mqtt broker connection lost", "id", b.id, "error", err)
	b.mu.Lock()
	b.setStatus("red", "disconnected")
	b.mu.Unlock()
}

func (b *MqttBroker) onReconnecting(_ mqtt.Client, _ *mqtt.ClientOptions) {
	slog.Debug("mqtt broker reconnecting", "id", b.id)
	b.mu.Lock()
	b.setStatus("yellow", "reconnecting...")
	b.mu.Unlock()
}

// setStatus updates the status and broadcasts to all registered nodes.
// Must be called with b.mu held (at least read lock for broadcasting).
func (b *MqttBroker) setStatus(fill, text string) {
	b.currentFill = fill
	b.currentText = text
	for _, fn := range b.statusFuncs {
		fn(fill, text)
	}
}
