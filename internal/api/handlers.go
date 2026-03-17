// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// jsonResponse is a helper that writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// jsonError is a helper that writes a JSON error response.
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]any{
		"error":  http.StatusText(status),
		"message": message,
	})
}

// handleGetFlows returns all deployed flows.
//
// GET /api/v1/flows
func handleGetFlows(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GET /flows: listing all flows")

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ok",
		"flows":  []any{},
	})
}

// handleDeployFlows deploys a new set of flows to the runtime.
//
// POST /api/v1/flows
func handleDeployFlows(w http.ResponseWriter, r *http.Request) {
	slog.Debug("POST /flows: deploying flows")

	// TODO: Parse request body containing flow definitions.
	// TODO: Validate flows and pass them to the engine for deployment.
	// TODO: Return the deployed flow revision.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "flows deployed",
	})
}

// handleGetFlow returns a single flow by its ID.
//
// GET /api/v1/flows/{id}
func handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	slog.Debug("GET /flows/{id}: fetching flow", "id", id)

	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing flow id")
		return
	}

	// TODO: Look up the flow by ID in the engine/storage.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ok",
		"id":     id,
		"flow":   nil,
	})
}

// handleGetNodes returns a list of all registered node types for the palette.
//
// GET /api/v1/nodes
func handleGetNodes(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GET /nodes: listing available node types")

	// TODO: Query the node registry for all registered node types
	// and return their metadata (type, category, label, icon, defaults, etc.).

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ok",
		"nodes":  []any{},
	})
}

// handleInjectNode triggers an inject node manually by its ID.
//
// POST /api/v1/inject/{id}
func handleInjectNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	slog.Debug("POST /inject/{id}: triggering inject node", "id", id)

	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing node id")
		return
	}

	// TODO: Find the inject node by ID in the running flow engine
	// and trigger it to emit a message.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "inject triggered",
		"id":      id,
	})
}

// handleGetSettings returns the current runtime settings and configuration.
//
// GET /api/v1/settings
func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GET /settings: returning runtime settings")

	// TODO: Return actual runtime settings including version, node types,
	// available palettes, editor configuration, etc.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ok",
		"settings": map[string]any{
			"httpNodeRoot":    "/",
			"version":        "0.1.0",
			"context":        map[string]any{"stores": []string{"memory"}},
			"flowFilePretty": true,
		},
	})
}

// handleGetDebugMessages returns recent debug messages from the runtime.
//
// GET /api/v1/debug/messages
func handleGetDebugMessages(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GET /debug/messages: returning recent debug messages")

	// TODO: Retrieve the last N debug messages from the NATS JetStream
	// ring buffer or in-memory debug message store.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"messages": []any{},
	})
}
