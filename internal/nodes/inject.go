// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/niceclouds/flint/internal/flow"
)

// InjectNode is a source node that generates messages on a timer or once at startup.
// It has 0 inputs and 1 output.
type InjectNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Parsed from Properties.
	once     bool          // send one message immediately on Start
	interval time.Duration // recurring interval (0 = disabled)
	payload  any           // payload value (nil = current timestamp)
	topic    string        // message topic

	done chan struct{}
	wg   sync.WaitGroup
}

// NewInjectNode is the NodeFactory for the inject node type.
func NewInjectNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &InjectNode{
		config: config,
		done:   make(chan struct{}),
	}
	return n, nil
}

// Init parses and validates the node configuration from Properties.
func (n *InjectNode) Init() error {
	props := n.config.Properties

	if v, ok := props["once"].(bool); ok {
		n.once = v
	}

	if v, ok := props["interval"].(float64); ok && v > 0 {
		n.interval = time.Duration(v) * time.Millisecond
	}

	if v, exists := props["payload"]; exists {
		n.payload = v
	}

	if v, ok := props["topic"].(string); ok {
		n.topic = v
	}

	if !n.once && n.interval == 0 {
		slog.Warn("inject node has no trigger configured (neither once nor interval)",
			"node_id", n.config.ID,
		)
	}

	return nil
}

// SetSend stores the engine-provided callback for sending messages downstream.
func (n *InjectNode) SetSend(fn flow.SendFunc) {
	n.send = fn
}

// SetStatus stores the engine-provided callback for reporting node status.
func (n *InjectNode) SetStatus(fn flow.StatusFunc) {
	n.status = fn
}

// SetDebug stores the engine-provided callback for emitting debug messages.
func (n *InjectNode) SetDebug(fn flow.DebugFunc) {
	n.debug = fn
}

// Start begins message generation. If once is true, a message is sent immediately.
// If interval is set, a background goroutine sends messages at the configured rate.
func (n *InjectNode) Start() error {
	if n.send == nil {
		return fmt.Errorf("inject node %s: send function not set", n.config.ID)
	}

	if n.once {
		n.emit()
	}

	if n.interval > 0 {
		n.wg.Add(1)
		go n.tickerLoop()
	}

	slog.Info("inject node started",
		"node_id", n.config.ID,
		"once", n.once,
		"interval", n.interval,
	)
	return nil
}

// HandleMessage allows manual triggering of the inject node (e.g. via API).
// The incoming message is ignored; a new message is generated and sent.
func (n *InjectNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	n.emit()
	return nil, nil
}

// Stop signals the background goroutine to exit and waits for it to finish.
func (n *InjectNode) Stop() error {
	close(n.done)
	n.wg.Wait()

	slog.Info("inject node stopped", "node_id", n.config.ID)
	return nil
}

// emit creates a new message and sends it to output port 0.
func (n *InjectNode) emit() {
	msg := flow.NewMessage()

	if n.payload != nil {
		msg.SetPayload(n.payload)
	} else {
		msg.SetPayload(time.Now().UTC().Format(time.RFC3339Nano))
	}

	if n.topic != "" {
		msg.SetTopic(n.topic)
	}

	n.send(0, msg)
}

// tickerLoop runs in a goroutine and emits messages at the configured interval.
func (n *InjectNode) tickerLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	for {
		select {
		case <-n.done:
			return
		case <-ticker.C:
			n.emit()
		}
	}
}

// InjectTypeInfo returns the NodeTypeInfo for registering the inject node.
func InjectTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "inject",
		Category:    "common",
		Label:       "Inject",
		Description: "Injects a message into the flow at startup or at a recurring interval",
		Icon:        "mdi-play",
		Defaults: map[string]any{
			"once":     false,
			"interval": 0,
			"payload":  nil,
			"topic":    "",
		},
		Inputs:  0,
		Outputs: 1,
	}
}
