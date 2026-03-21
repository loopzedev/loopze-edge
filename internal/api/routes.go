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
// The Deps struct provides access to the flow engine and storage layer.
func RegisterRoutes(r chi.Router, deps *Deps) {
	// Flow management endpoints.
	r.Get("/flows", deps.handleGetFlows)
	r.Post("/flows", deps.handleDeployFlows)
	r.Get("/flows/{id}", deps.handleGetFlow)

	// Node catalog endpoint.
	r.Get("/nodes", deps.handleGetNodes)

	// Inject trigger endpoint.
	r.Post("/inject/{id}", deps.handleInjectNode)

	// Runtime settings endpoint.
	r.Get("/settings", deps.handleGetSettings)

	// Node status snapshot endpoint.
	r.Get("/status/nodes", deps.handleGetNodeStatuses)

	// Debug message stream endpoint.
	r.Get("/debug/messages", deps.handleGetDebugMessages)
}
