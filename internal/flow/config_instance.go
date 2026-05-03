// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

// ConfigInstance is the interface that all config node implementations must satisfy.
// Config nodes represent shared resources (MQTT brokers, DB connections, HTTP auth)
// that are referenced by regular canvas nodes. The engine manages their lifecycle:
//
//	Deploy: Config instances Start() BEFORE regular nodes Start()
//	Stop:   Config instances Stop() AFTER regular nodes Stop()
//
// To add a new config node type, implement this interface and register
// a ConfigNodeFactory via NodeRegistry.RegisterConfig().
type ConfigInstance interface {
	// Start initialises the resource (e.g. connect to MQTT broker, open DB pool).
	// Called once during Deploy, before regular nodes are started.
	Start() error

	// Stop releases the resource (e.g. disconnect, close pool).
	// Called during Stop/Re-Deploy, after regular nodes have been stopped.
	Stop() error

	// Status returns the current state for UI display.
	// fill is a color key ("green", "red", "yellow", "blue", "grey").
	// text is a short status label (e.g. "connected", "disconnected").
	Status() (fill string, text string)
}

// ConfigNodeFactory creates a ConfigInstance from a ConfigNode definition.
// Each config node type registers exactly one factory via NodeRegistry.RegisterConfig().
type ConfigNodeFactory func(config ConfigNode) (ConfigInstance, error)

// ConfigLookupFunc is injected into nodes that implement ConfigProvider.
// Nodes call this to retrieve a config instance by its ID.
type ConfigLookupFunc func(configID string) (instance ConfigInstance, ok bool)

// ConfigProvider is an optional interface that nodes can implement to receive
// access to config node instances. The engine checks each NodeInstance after
// creation and injects the lookup function if the node implements this interface.
//
// Usage in a node:
//
//	func (n *MqttInNode) SetConfigLookup(fn flow.ConfigLookupFunc) {
//	    n.configLookup = fn
//	}
//
//	func (n *MqttInNode) Start() error {
//	    inst, ok := n.configLookup(n.brokerID)
//	    if !ok { return fmt.Errorf("broker not found") }
//	    broker := inst.(*MqttBroker) // type-assert to concrete type
//	    ...
//	}
type ConfigProvider interface {
	SetConfigLookup(fn ConfigLookupFunc)
}
