// Copyright 2024 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2)

package flow

import "sync"

// SendFunc is a callback that nodes use to asynchronously send messages
// to a specific output port. The engine provides this function via SetSend
// before calling Start(). Source nodes (e.g. inject, mqtt-in) use this to
// push messages downstream from background goroutines.
type SendFunc func(port int, msg *Message)

// NodeFactory is a constructor function that creates a new NodeInstance
// from a given NodeConfig. Each registered node type provides its own factory.
type NodeFactory func(config NodeConfig) (NodeInstance, error)

// NodeInstance is the interface that all executable node implementations must satisfy.
// The runtime engine calls these methods during the flow lifecycle.
//
// Lifecycle order: Factory → Init → SetSend → Start → HandleMessage… → Stop
type NodeInstance interface {
	// Init is called once after the node is created, before the flow starts.
	// Use it to validate configuration and allocate resources.
	Init() error

	// SetSend provides the node with a callback to send messages to output ports.
	// Called by the engine after Init() and before Start(). Source nodes use this
	// to push messages from background goroutines (timers, subscriptions, etc.).
	SetSend(fn SendFunc)

	// Start is called when the flow is deployed and begins execution.
	// Long-running nodes (e.g. MQTT subscriber, Inject timer) should
	// start their background goroutines here.
	Start() error

	// HandleMessage processes an incoming message and returns zero or more
	// output messages. The outer slice index corresponds to the output port,
	// so a node with two outputs might return []*Message for port 0 and
	// []*Message for port 1. Returning nil or an empty slice means no
	// messages are sent downstream from this invocation.
	HandleMessage(msg *Message) ([]*Message, error)

	// Stop is called when the flow is stopped or re-deployed.
	// Nodes must release resources and stop background goroutines.
	Stop() error
}

// NodeTypeInfo describes a registered node type for the palette / node catalog.
// This information is sent to the frontend so it can render the node palette.
type NodeTypeInfo struct {
	// Type is the unique identifier for this node type, e.g. "inject", "debug", "mqtt-in".
	Type string `json:"type"`

	// Category groups the node in the palette, e.g. "common", "function", "network", "industrial".
	Category string `json:"category"`

	// Label is the human-readable display name shown in the palette.
	Label string `json:"label"`

	// Description is a short help text explaining what the node does.
	Description string `json:"description"`

	// Icon is the icon identifier used by the frontend (e.g. "mdi-play", "flash", "bug").
	Icon string `json:"icon"`

	// Defaults holds the default property values for new instances of this node.
	Defaults map[string]any `json:"defaults"`

	// Inputs is the number of input ports (0 for source nodes like inject, 1 for most nodes).
	Inputs int `json:"inputs"`

	// Outputs is the number of output ports (0 for sink nodes like debug, 1+ for others).
	Outputs int `json:"outputs"`
}

// NodeConfig holds the configuration for a specific node instance within a flow.
// It is passed to the NodeFactory when creating a new NodeInstance.
type NodeConfig struct {
	// ID is the unique identifier of this node instance.
	ID string `json:"id"`

	// Type is the node type identifier, matching a registered NodeTypeInfo.Type.
	Type string `json:"type"`

	// Name is the optional user-defined label for this node instance.
	Name string `json:"name"`

	// Properties holds all user-configured properties for this node instance.
	// Populated from Node.Config during deployment.
	Properties map[string]any `json:"properties"`
}

// NodeRegistry maintains the catalog of available node types.
// Node types are registered at startup and queried by the engine during
// flow deployment and by the API when the frontend requests the palette.
type NodeRegistry struct {
	mu        sync.RWMutex
	factories map[string]NodeFactory
	typeInfos map[string]NodeTypeInfo
}

// NewNodeRegistry creates an empty NodeRegistry ready for node type registration.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		factories: make(map[string]NodeFactory),
		typeInfos: make(map[string]NodeTypeInfo),
	}
}

// Register adds a new node type to the registry. The nodeType string must be
// unique (e.g. "inject", "debug", "function"). The factory is called each time
// an instance of this node type needs to be created during flow deployment.
// The info provides metadata for the frontend palette.
func (r *NodeRegistry) Register(nodeType string, factory NodeFactory, info NodeTypeInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info.Type = nodeType
	r.factories[nodeType] = factory
	r.typeInfos[nodeType] = info
}

// Get retrieves the NodeFactory for a given node type.
// Returns the factory and true if found, or nil and false if the type is not registered.
func (r *NodeRegistry) Get(nodeType string) (NodeFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[nodeType]
	return factory, ok
}

// List returns metadata for all registered node types, suitable for
// sending to the frontend to populate the node palette.
func (r *NodeRegistry) List() []NodeTypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]NodeTypeInfo, 0, len(r.typeInfos))
	for _, info := range r.typeInfos {
		types = append(types, info)
	}
	return types
}

// Has reports whether a node type is registered.
func (r *NodeRegistry) Has(nodeType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.factories[nodeType]
	return ok
}

// Count returns the number of registered node types.
func (r *NodeRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.factories)
}
