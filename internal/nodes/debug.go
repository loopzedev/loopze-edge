// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/niceclouds/flint/internal/flow"
)

// DebugNode is a sink node that captures incoming messages and publishes
// them as debug output. It has 1 input and 0 outputs.
type DebugNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Parsed from Properties.
	property string // which msg field to display (default: "payload")
	active   bool   // whether debug output is enabled
}

// NewDebugNode is the NodeFactory for the debug node type.
func NewDebugNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &DebugNode{
		config:   config,
		property: "payload",
		active:   true,
	}, nil
}

// Init parses and validates the node configuration.
func (n *DebugNode) Init() error {
	props := n.config.Properties

	if v, ok := props["property"].(string); ok && v != "" {
		n.property = v
	}

	if v, ok := props["active"].(bool); ok {
		n.active = v
	}

	return nil
}

// SetSend stores the send callback (debug node doesn't send downstream).
func (n *DebugNode) SetSend(fn flow.SendFunc) {
	n.send = fn
}

// SetStatus stores the status callback.
func (n *DebugNode) SetStatus(fn flow.StatusFunc) {
	n.status = fn
}

// SetDebug stores the debug callback for publishing debug messages.
func (n *DebugNode) SetDebug(fn flow.DebugFunc) {
	n.debug = fn
}

// Start is a no-op for the debug node (it only reacts to incoming messages).
func (n *DebugNode) Start() error {
	slog.Info("debug node started",
		"node_id", n.config.ID,
		"property", n.property,
		"active", n.active,
	)
	return nil
}

// HandleMessage captures the incoming message and publishes it as a debug event.
func (n *DebugNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if !n.active || n.debug == nil {
		return nil, nil
	}

	// Extract the value for the configured property.
	var payload any
	if n.property == "complete" {
		payload = msg.Data()
	} else {
		payload = msg.Get(n.property)
	}

	n.debug(flow.DebugMessage{
		ID:        msg.ID(),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    "debug",
		Payload:   payload,
		Format:    detectFormat(payload),
		Property:  n.property,
		NodeName:  n.config.Name,
	})

	return nil, nil
}

// Stop is a no-op for the debug node.
func (n *DebugNode) Stop() error {
	slog.Info("debug node stopped", "node_id", n.config.ID)
	return nil
}

// detectFormat returns a format hint string for the given value.
func detectFormat(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case float64, float32, int, int64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// DebugTypeInfo returns the NodeTypeInfo for registering the debug node.
func DebugTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "debug",
		Category:    "common",
		Label:       "Debug",
		Description: "Displays incoming messages in the debug panel",
		Icon:        "mdi-bug",
		Defaults: map[string]any{
			"property": "payload",
			"active":   true,
		},
		Inputs:  1,
		Outputs: 0,
	}
}
