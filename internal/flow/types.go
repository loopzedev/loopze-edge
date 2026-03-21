// Copyright 2024 NiceClouds GmbH. All rights reserved.
// Licensed under the Elastic License 2.0 (ELv2).

// Package flow implements the Flint flow runtime engine and core types.
package flow

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Flow represents a single flow (tab) containing a set of interconnected nodes.
// Each flow is an independent unit of execution within the Flint runtime.
type Flow struct {
	// ID is the unique identifier for this flow.
	ID string `json:"id"`

	// Type is the flow type identifier, always "tab" for normal flows.
	Type string `json:"type"`

	// Label is the human-readable name displayed in the editor tab.
	Label string `json:"label"`

	// Nodes contains all nodes that belong to this flow.
	Nodes []Node `json:"nodes"`

	// Wires defines the connections between nodes in this flow.
	Wires []Wire `json:"wires"`

	// Info is an optional description or documentation for the flow.
	Info string `json:"info,omitempty"`

	// Disabled indicates whether this flow should be skipped during deployment.
	Disabled bool `json:"disabled,omitempty"`

	// Env holds flow-scoped environment variables available to all nodes in this flow.
	Env map[string]string `json:"env,omitempty"`
}

// Node represents a single processing unit within a flow.
// Nodes receive messages, process them, and optionally send messages to connected nodes.
type Node struct {
	// ID is the unique identifier for this node.
	ID string `json:"id"`

	// Type identifies the node type (e.g., "inject", "debug", "function", "mqtt-in").
	Type string `json:"type"`

	// Name is an optional user-defined label for this node instance.
	Name string `json:"name,omitempty"`

	// X is the horizontal position of the node in the editor canvas.
	X float64 `json:"x"`

	// Y is the vertical position of the node in the editor canvas.
	Y float64 `json:"y"`

	// Z references the flow (tab) this node belongs to.
	Z string `json:"z"`

	// Inputs is the number of input ports (0 for source nodes like inject).
	Inputs int `json:"inputs"`

	// Outputs is the number of output ports.
	Outputs int `json:"outputs"`

	// Wires defines the output connections. Each index corresponds to an output port,
	// and the slice contains the IDs of connected target nodes.
	Wires [][]string `json:"wires"`

	// Config holds user-configured properties for this node instance.
	// The frontend sends this as "config" (e.g., interval, payload, topic for inject).
	Config map[string]any `json:"config,omitempty"`

	// Disabled indicates whether this node should be skipped during deployment.
	Disabled bool `json:"disabled,omitempty"`
}

// Wire represents a connection between two nodes, linking a source output port
// to a target node's input port.
type Wire struct {
	// ID is the unique identifier for this wire.
	ID string `json:"id"`

	// SourceNode is the ID of the source node.
	SourceNode string `json:"sourceNode"`

	// SourcePort is the index of the output port on the source node.
	SourcePort int `json:"sourcePort"`

	// TargetNode is the ID of the destination node.
	TargetNode string `json:"targetNode"`

	// TargetPort is the index of the input port on the target node.
	TargetPort int `json:"targetPort"`
}

// Message is the fundamental data unit passed between nodes in a flow.
// Internally it is a free-form map[string]any — just like Node-RED's msg object.
// Every field (payload, topic, custom fields) is equal and accessed uniformly.
//
// The "_id" field is set once at creation and cannot be changed or deleted
// through Set/Delete. It is always included in JSON output.
type Message struct {
	id   string         // immutable, assigned at creation
	data map[string]any // all user-visible fields (payload, topic, …)
}

// NewMessage creates a new Message with a unique, immutable ID and a timestamp.
func NewMessage() *Message {
	return &Message{
		id: generateID(),
		data: map[string]any{
			"_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
}

// NewMessageFromData creates a Message from a flat map (e.g. from a JS object).
// If the map contains an "_id" key, that value is used as the message ID so that
// the original message identity is preserved across function node execution.
// The "_id" key is removed from the data map.
func NewMessageFromData(data map[string]any) *Message {
	id, _ := data["_id"].(string)
	if id == "" {
		id = generateID()
	}
	clean := make(map[string]any, len(data))
	for k, v := range data {
		if k != "_id" {
			clean[k] = v
		}
	}
	return &Message{id: id, data: clean}
}

// ID returns the immutable message identifier.
func (m *Message) ID() string {
	return m.id
}

// Get retrieves a value by dot-separated path (e.g. "payload", "payload.id", "topic").
// Returns nil if the path does not exist.
func (m *Message) Get(path string) any {
	if path == "_id" {
		return m.id
	}
	parts := strings.Split(path, ".")
	var current any = m.data
	for _, key := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[key]
		if !ok {
			return nil
		}
	}
	return current
}

// Set sets a value at the given dot-separated path.
// The "_id" field is protected and cannot be overwritten.
func (m *Message) Set(path string, val any) {
	if path == "_id" || strings.HasPrefix(path, "_id.") {
		return // immutable
	}
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		m.data[parts[0]] = val
		return
	}
	// Walk/create intermediate maps for nested paths.
	current := m.data
	for _, key := range parts[:len(parts)-1] {
		next, ok := current[key]
		if !ok {
			nested := make(map[string]any)
			current[key] = nested
			current = nested
			continue
		}
		nested, ok := next.(map[string]any)
		if !ok {
			nested = make(map[string]any)
			current[key] = nested
			current = nested
			continue
		}
		current = nested
	}
	current[parts[len(parts)-1]] = val
}

// Delete removes a field at the given path. The "_id" field cannot be deleted.
func (m *Message) Delete(path string) {
	if path == "_id" || strings.HasPrefix(path, "_id.") {
		return // immutable
	}
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(m.data, parts[0])
		return
	}
	current := m.data
	for _, key := range parts[:len(parts)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
}

// Clone creates a deep copy of the message. The clone receives a NEW unique ID.
func (m *Message) Clone() *Message {
	cloned := &Message{
		id:   generateID(),
		data: deepCopyMap(m.data),
	}
	return cloned
}

// Data returns the underlying map for direct iteration.
// Modifications to the returned map are reflected in the message.
func (m *Message) Data() map[string]any {
	return m.data
}

// Payload is a convenience accessor for m.Get("payload").
func (m *Message) Payload() any {
	return m.data["payload"]
}

// SetPayload is a convenience accessor for m.Set("payload", v).
func (m *Message) SetPayload(v any) {
	m.data["payload"] = v
}

// Topic is a convenience accessor for m.Get("topic").
func (m *Message) Topic() string {
	t, _ := m.data["topic"].(string)
	return t
}

// SetTopic is a convenience accessor for m.Set("topic", v).
func (m *Message) SetTopic(v string) {
	m.data["topic"] = v
}

// MarshalJSON serializes the message as a flat JSON object with "_id" included.
func (m *Message) MarshalJSON() ([]byte, error) {
	flat := make(map[string]any, len(m.data)+1)
	for k, v := range m.data {
		flat[k] = v
	}
	flat["_id"] = m.id
	return json.Marshal(flat)
}

// UnmarshalJSON deserializes a flat JSON object into the message.
// If "_id" is present in the JSON, it is used; otherwise a new ID is generated.
func (m *Message) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if id, ok := raw["_id"].(string); ok && id != "" {
		m.id = id
	} else {
		m.id = generateID()
	}
	delete(raw, "_id")
	m.data = raw
	return nil
}

// generateID produces a random hex ID (16 bytes = 32 hex chars).
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// deepCopyMap recursively copies a map[string]any.
func deepCopyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case map[string]any:
			dst[k] = deepCopyMap(val)
		case []any:
			cp := make([]any, len(val))
			copy(cp, val)
			dst[k] = cp
		default:
			dst[k] = v
		}
	}
	return dst
}

// Workspace is the top-level container for all flow and config node data.
// It replaces the flat []Flow format for persistence and API transport.
type Workspace struct {
	// Flows contains all flow definitions in the workspace.
	Flows []Flow `json:"flows"`

	// Configs contains all config node definitions (e.g. MQTT broker, DB connection).
	// Config nodes are workspace-global and can be referenced by nodes in any flow.
	Configs []ConfigNode `json:"configs,omitempty"`
}

// ConfigNode is a configuration entity that does not appear on the canvas.
// It stores shared connection/resource settings (e.g. MQTT broker, HTTP auth)
// that are referenced by regular nodes via the config node's ID.
type ConfigNode struct {
	// ID is the unique identifier for this config node.
	ID string `json:"id"`

	// Type identifies the config node type (e.g. "mqtt-broker", "http-auth").
	Type string `json:"type"`

	// Name is an optional user-defined label (e.g. "Production Broker").
	Name string `json:"name,omitempty"`

	// Config holds the type-specific configuration properties.
	Config map[string]any `json:"config,omitempty"`
}

// Port describes a single input or output connector on a node type definition.
type Port struct {
	// Name is the display name of the port (e.g., "output", "true", "false").
	Name string `json:"name"`

	// Index is the zero-based position of this port on the node.
	Index int `json:"index"`

	// Type is an optional hint about the expected data type (e.g., "string", "number", "any").
	Type string `json:"type,omitempty"`
}
