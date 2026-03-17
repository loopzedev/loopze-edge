// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package api defines the REST API routes and handlers for the Flint runtime.
// All API endpoints are mounted under /api/v1/ and provide the interface
// between the Vue 3 frontend editor and the Go backend engine.
package api

import (
	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts all Flint API v1 route handlers onto the given router.
// This function is called by the server during initialization to wire up
// the REST endpoints that the frontend and external tools use to interact
// with the flow runtime.
func RegisterRoutes(r chi.Router) {
	// Flow management endpoints.
	// Flows are the primary unit of work — each flow is a tab in the editor
	// containing interconnected nodes.
	r.Get("/flows", handleGetFlows)       // List all deployed flows.
	r.Post("/flows", handleDeployFlows)    // Deploy (create/update) flows.
	r.Get("/flows/{id}", handleGetFlow)    // Get a single flow by ID.

	// Node catalog endpoint.
	// Returns the list of registered node types so the frontend can populate
	// the palette sidebar with available nodes.
	r.Get("/nodes", handleGetNodes)

	// Inject trigger endpoint.
	// Allows the frontend (or external callers) to manually trigger an
	// inject node, simulating the button press in the editor.
	r.Post("/inject/{id}", handleInjectNode)

	// Runtime settings endpoint.
	// Returns the current runtime configuration and metadata (version,
	// available features, etc.) for the frontend settings panel.
	r.Get("/settings", handleGetSettings)

	// Debug message stream endpoint.
	// Returns recent debug messages collected from debug nodes in all
	// active flows. The frontend debug sidebar polls this or uses the
	// WebSocket for live streaming.
	r.Get("/debug/messages", handleGetDebugMessages)
}
