// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"net/http"
	"sync"

	"github.com/loopzedev/loopze-edge/internal/credentials"
)

// SendFunc is a callback that nodes use to asynchronously send messages
// to a specific output port. The engine provides this function via SetSend
// before calling Start(). Source nodes (e.g. inject, mqtt-in) use this to
// push messages downstream from background goroutines.
type SendFunc func(port int, msg *Message)

// StatusFunc is a callback that nodes use to report their current status
// to the editor UI. The fill color and text are displayed on the node.
// Valid fill values: "green", "red", "yellow", "blue", "grey".
type StatusFunc func(fill string, text string)

// DebugMessage represents a debug output from a node, sent to the debug panel.
type DebugMessage struct {
	ID        string `json:"id"`
	NodeID    string `json:"nodeId"`
	NodeName  string `json:"nodeName"`
	FlowID    string `json:"flowId"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"` // "debug", "warn", "error"
	Payload   any    `json:"payload"`
	Format    string `json:"format"`   // "string", "number", "boolean", "object", "array", "null"
	Property  string `json:"property"` // which msg field, e.g. "payload"
}

// NodeStatusPayload is the nested status object within a StatusMessage.
type NodeStatusPayload struct {
	Fill string `json:"fill"`
	Text string `json:"text"`
}

// StatusMessage represents a status update from a node, sent to the editor UI.
// Matches the frontend StatusEvent type: { nodeId, flowId, status: { fill, text } }
type StatusMessage struct {
	NodeID     string            `json:"nodeId"`
	FlowID     string            `json:"flowId"`
	Status     NodeStatusPayload `json:"status"`
	SourceType string            `json:"sourceType,omitempty"`
	SourceName string            `json:"sourceName,omitempty"`
}

// PublishStatusFunc is a callback the server provides to the engine so that
// status messages can be published externally (e.g. to NATS) without the
// engine needing a direct dependency on the messaging infrastructure.
type PublishStatusFunc func(subject string, msg StatusMessage)

// StatusListenerFunc is invoked by the engine for every status update of any
// node, except status updates emitted by Status Nodes themselves (those are
// filtered out at the source to prevent feedback loops between Status Nodes).
type StatusListenerFunc func(msg StatusMessage)

// StatusListenerProvider is implemented by nodes that want to observe status
// events of other nodes (e.g. the Status Node). The engine injects a
// registration function that returns an unregister closure; the node is
// responsible for calling unregister in Stop() to release the listener slot.
type StatusListenerProvider interface {
	SetStatusListener(register func(StatusListenerFunc) (unregister func()))
}

// ErrorMessage represents a runtime error raised by a node, carrying enough
// context for a Catch Node downstream to react. Msg is the message that was
// being processed when the error occurred (may be nil for errors that are not
// tied to a specific incoming message, e.g. background subscription failures).
type ErrorMessage struct {
	NodeID     string
	FlowID     string
	SourceType string
	SourceName string
	Error      string
	Msg        *Message
}

// ErrorFunc is a callback nodes can use to report an asynchronous error to the
// engine — for errors that surface after HandleMessage has returned (e.g. an
// MQTT publish callback, an HTTP response that fails). For synchronous errors
// the engine calls the same pipeline automatically based on the HandleMessage
// return value, so most nodes never need to invoke this directly.
type ErrorFunc func(err error, msg *Message)

// ErrorListenerFunc is invoked by the engine for every node error, except
// errors raised by Catch Nodes themselves (those are filtered out at the
// source to prevent feedback loops between Catch Nodes).
type ErrorListenerFunc func(msg ErrorMessage)

// ErrorListenerProvider is implemented by nodes that want to observe errors
// raised by other nodes (e.g. the Catch Node). The engine injects a
// registration function that returns an unregister closure; the node is
// responsible for calling unregister in Stop() to release the listener slot.
type ErrorListenerProvider interface {
	SetErrorListener(register func(ErrorListenerFunc) (unregister func()))
}

// ErrorProvider is an optional interface nodes can implement to receive an
// ErrorFunc for reporting asynchronous errors (errors that occur after
// HandleMessage has returned). The engine injects the callback during wiring.
type ErrorProvider interface {
	SetError(fn ErrorFunc)
}

// HTTPRouteSpec is one route an http-in node wants to expose under the
// flow-endpoint prefix. The engine collects these from every
// HTTPInProvider node after wireAllNodes and hands them to the
// server-supplied HTTPMuxBuilder for atomic publication.
//
// Method may be a concrete verb ("GET", "POST", …) or "*" / "" for
// any-verb. Path is chi-style (must start with "/"; supports ":name"
// params and "*" catch-all). Handler is invoked by the flow-endpoint
// router for every matching request.
type HTTPRouteSpec struct {
	NodeID  string
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// HTTPRouteConflict reports a spec that the builder could not install
// (typically a duplicate method+path pair). The engine surfaces these
// to the affected nodes via their errorFn so they can flag themselves
// red without a hard deploy failure.
type HTTPRouteConflict struct {
	NodeID string
	Method string
	Path   string
	Reason string
}

// HTTPMuxBuilder is provided by the server to the engine. The engine
// invokes it once per deploy with the full set of route specs; the
// builder atomically swaps the live flow-endpoint route table and
// returns any conflicts it dropped.
type HTTPMuxBuilder func(specs []HTTPRouteSpec) []HTTPRouteConflict

// HTTPInProvider is implemented by nodes that contribute one or more
// routes to the flow-endpoint mux (typically the http-in node, which
// may register a second OPTIONS route for CORS preflight). The engine
// calls HTTPRoutes() once per deploy after wireAllNodes.
type HTTPInProvider interface {
	HTTPRoutes() []HTTPRouteSpec
}

// HTTPInConflictReporter is an optional companion interface a node can
// implement to receive a callback when one of its routes was rejected
// at mux build time (typically a duplicate method+path with another
// node). Nodes use this to flip their status to red. The engine still
// also dispatches the conflict via the standard ErrorFunc pipeline so
// it surfaces in the debug stream and to Catch nodes.
type HTTPInConflictReporter interface {
	OnHTTPRouteConflict(reason string)
}

// HTTPMuxProvider is implemented by nodes that need the engine-owned
// response registry and the configured flow-endpoint prefix. http-in
// uses both (registry to register live request slots; root for status
// text). http-response only uses the registry to resolve handles.
type HTTPMuxProvider interface {
	SetHTTPMux(registry *ResponseRegistry, root string)
}

// SessionRegistryProvider is implemented by nodes that need access to
// the engine-owned TCP session registry. tcp-in (server mode) uses it
// to register every accepted connection and to publish msg.session
// handles into the flow; tcp-out (reply / server-broadcast modes)
// uses it to resolve those handles back to a writable net.Conn.
type SessionRegistryProvider interface {
	SetSessionRegistry(registry *SessionRegistry)
}

// CertStoreProvider is implemented by nodes that need access to the
// shared TLS certificate store. The engine injects the store BEFORE
// Init runs so nodes can resolve cert references while validating
// their TLS configuration. Nodes that operate without TLS may ignore
// the injected value.
//
// The store may be nil in lightweight test setups; nodes must treat a
// nil store as "no centrally-managed certs are available" and either
// fall back to inline-only handling or surface a clear error if a
// flow references a cert ID without a store wired in.
type CertStoreProvider interface {
	SetCertStore(store *credentials.CertStore)
}

// DebugFunc is a callback that nodes use to emit debug messages.
// The engine provides this function via SetDebug before calling Start().
// The debug node uses this to publish captured messages; any node can
// use it for diagnostic output.
type DebugFunc func(msg DebugMessage)

// NodeFactory is a constructor function that creates a new NodeInstance
// from a given NodeConfig. Each registered node type provides its own factory.
type NodeFactory func(config NodeConfig) (NodeInstance, error)

// NodeInstance is the interface that all executable node implementations must satisfy.
// The runtime engine calls these methods during the flow lifecycle.
//
// Lifecycle order: Factory → Init → SetSend → SetStatus → SetDebug → Start → HandleMessage… → Stop
type NodeInstance interface {
	// Init is called once after the node is created, before the flow starts.
	// Use it to validate configuration and allocate resources.
	Init() error

	// SetSend provides the node with a callback to send messages to output ports.
	// Called by the engine after Init() and before Start(). Source nodes use this
	// to push messages from background goroutines (timers, subscriptions, etc.).
	SetSend(fn SendFunc)

	// SetStatus provides the node with a callback to report its current status
	// to the editor UI (e.g. "connected", "error", "waiting").
	// Called by the engine after Init() and before Start().
	SetStatus(fn StatusFunc)

	// SetDebug provides the node with a callback to emit debug messages.
	// Called by the engine after Init() and before Start().
	SetDebug(fn DebugFunc)

	// Start is called when the flow is deployed and begins execution.
	// Long-running nodes (e.g. MQTT subscriber, Inject timer) should
	// start their background goroutines here.
	Start() error

	// HandleMessage processes an incoming message and returns output messages
	// grouped by output port. The outer slice index corresponds to the output
	// port, and the inner slice contains messages for that port.
	//
	// Example for a switch node with 3 outputs:
	//   results := make([][]*Message, 3)
	//   results[0] = []*Message{msg}  // port 0: match
	//   results[1] = nil              // port 1: no match
	//   results[2] = nil              // port 2: no match
	//
	// Returning nil means no messages are sent downstream.
	HandleMessage(msg *Message) ([][]*Message, error)

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

	// FlowID is the ID of the flow that this node belongs to.
	FlowID string `json:"flowId"`

	// Properties holds all user-configured properties for this node instance.
	// Populated from Node.Config during deployment.
	Properties map[string]any `json:"properties"`
}

// ConfigTypeInfo describes a registered config node type for the frontend.
// Sent to the frontend so it knows which config types exist and their defaults.
type ConfigTypeInfo struct {
	// Type is the unique identifier for this config type (e.g. "mqtt-broker").
	Type string `json:"type"`

	// Label is the human-readable name (e.g. "MQTT Broker").
	Label string `json:"label"`

	// Description is a short help text.
	Description string `json:"description"`

	// Defaults holds default property values for new config node instances.
	Defaults map[string]any `json:"defaults"`
}

// NodeRegistry maintains the catalog of available node types and config node types.
// Node types are registered at startup and queried by the engine during
// flow deployment and by the API when the frontend requests the palette.
type NodeRegistry struct {
	mu              sync.RWMutex
	factories       map[string]NodeFactory
	typeInfos       map[string]NodeTypeInfo
	typeOrder       []string // preserves registration order for stable palette ordering
	configFactories map[string]ConfigNodeFactory
	configTypeInfos map[string]ConfigTypeInfo
}

// NewNodeRegistry creates an empty NodeRegistry ready for node type registration.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		factories:       make(map[string]NodeFactory),
		typeInfos:       make(map[string]NodeTypeInfo),
		configFactories: make(map[string]ConfigNodeFactory),
		configTypeInfos: make(map[string]ConfigTypeInfo),
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
	if _, exists := r.typeInfos[nodeType]; !exists {
		r.typeOrder = append(r.typeOrder, nodeType)
	}
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

// GetTypeInfo retrieves the NodeTypeInfo for a given node type.
func (r *NodeRegistry) GetTypeInfo(nodeType string) (NodeTypeInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.typeInfos[nodeType]
	return info, ok
}

// List returns metadata for all registered node types in registration order,
// suitable for sending to the frontend to populate the node palette.
func (r *NodeRegistry) List() []NodeTypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]NodeTypeInfo, 0, len(r.typeOrder))
	for _, t := range r.typeOrder {
		types = append(types, r.typeInfos[t])
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

// RegisterConfig adds a new config node type to the registry.
// The configType string must be unique (e.g. "mqtt-broker", "http-auth").
func (r *NodeRegistry) RegisterConfig(configType string, factory ConfigNodeFactory, info ConfigTypeInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info.Type = configType
	r.configFactories[configType] = factory
	r.configTypeInfos[configType] = info
}

// GetConfigFactory retrieves the ConfigNodeFactory for a given config type.
func (r *NodeRegistry) GetConfigFactory(configType string) (ConfigNodeFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.configFactories[configType]
	return factory, ok
}

// ListConfigTypes returns metadata for all registered config node types.
func (r *NodeRegistry) ListConfigTypes() []ConfigTypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]ConfigTypeInfo, 0, len(r.configTypeInfos))
	for _, info := range r.configTypeInfos {
		types = append(types, info)
	}
	return types
}
