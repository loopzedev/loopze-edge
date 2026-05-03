// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

// ContextStore provides key-value storage for shared node context data.
// Implementations are backed by NATS KV (memory or persistent).
type ContextStore interface {
	// Get retrieves a value by key. Returns nil if the key does not exist.
	Get(key string) (any, error)

	// Set stores a value under the given key.
	Set(key string, value any) error

	// Delete removes a key from the store. A missing key is not an error.
	Delete(key string) error

	// Keys returns all keys currently in the store.
	Keys() ([]string, error)
}

// ContextProvider is an optional interface nodes can implement to receive
// global and flow-scoped ContextStores before Start() is called.
// Nodes that do not need context access do not implement this interface.
//
// The engine checks each NodeInstance after wiring:
//
//	if cp, ok := instance.(ContextProvider); ok {
//	    cp.SetContext(globalMem, globalPers, flowMem, flowPers)
//	}
type ContextProvider interface {
	SetContext(globalMem, globalPers, flowMem, flowPers ContextStore)
}

// FlowContextFactory is called by the engine once per unique flow ID during
// Deploy to obtain dedicated KV-backed stores for that flow.
// Returning nil stores for a flow is allowed; the node will simply have no
// flow context available.
type FlowContextFactory func(flowID string) (memory, persistent ContextStore)
