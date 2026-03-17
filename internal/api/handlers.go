// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/niceclouds/flint/internal/flow"
	"github.com/niceclouds/flint/internal/storage"
)

// Deps holds the dependencies that API handlers need.
type Deps struct {
	Engine  *flow.Engine
	Storage storage.Storage
}

// deployRequest is the JSON body sent by the frontend on deploy.
type deployRequest struct {
	Flows []flow.Flow `json:"flows"`
	Rev   string      `json:"rev,omitempty"`
}

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
		"error":   http.StatusText(status),
		"message": message,
	})
}

// flowsRevision computes a short hash over the flows for revision tracking.
func flowsRevision(flows []flow.Flow) string {
	data, _ := json.Marshal(flows)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

// handleGetFlows returns all deployed flows.
//
// GET /api/v1/flows
func (d *Deps) handleGetFlows(w http.ResponseWriter, r *http.Request) {
	flows, err := d.Storage.LoadFlows()
	if err != nil {
		slog.Error("failed to load flows", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to load flows")
		return
	}

	rev := flowsRevision(flows)

	slog.Debug("GET /flows", "count", len(flows), "rev", rev)

	jsonResponse(w, http.StatusOK, map[string]any{
		"rev":   rev,
		"flows": flows,
	})
}

// handleDeployFlows deploys a new set of flows to the runtime.
//
// POST /api/v1/flows
func (d *Deps) handleDeployFlows(w http.ResponseWriter, r *http.Request) {
	var req deployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("invalid deploy request body", "error", err)
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	slog.Info("deploying flows",
		"flow_count", len(req.Flows),
		"client_rev", req.Rev,
	)

	// Save to persistent storage.
	if err := d.Storage.SaveFlows(req.Flows); err != nil {
		slog.Error("failed to save flows", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to save flows")
		return
	}

	// Deploy to the runtime engine.
	if err := d.Engine.Deploy(req.Flows); err != nil {
		slog.Error("failed to deploy flows to engine", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to deploy flows")
		return
	}

	rev := flowsRevision(req.Flows)

	slog.Info("flows deployed successfully", "rev", rev)

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"rev":     rev,
		"flows":   req.Flows,
	})
}

// handleGetFlow returns a single flow by its ID.
//
// GET /api/v1/flows/{id}
func (d *Deps) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing flow id")
		return
	}

	flows := d.Engine.Flows()
	for _, f := range flows {
		if f.ID == id {
			jsonResponse(w, http.StatusOK, f)
			return
		}
	}

	jsonError(w, http.StatusNotFound, "flow not found")
}

// handleGetNodes returns a list of all registered node types for the palette.
//
// GET /api/v1/nodes
func (d *Deps) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	types := d.Engine.Registry().List()

	slog.Debug("GET /nodes", "count", len(types))

	jsonResponse(w, http.StatusOK, map[string]any{
		"nodes": types,
	})
}

// handleInjectNode triggers an inject node manually by its ID.
//
// POST /api/v1/inject/{id}
func (d *Deps) handleInjectNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing node id")
		return
	}

	slog.Debug("POST /inject", "node_id", id)

	// TODO: Find the inject node by ID in the running engine and trigger it.

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ok",
		"id":     id,
	})
}

// handleGetSettings returns the current runtime settings and configuration.
//
// GET /api/v1/settings
func (d *Deps) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]any{
		"settings": map[string]any{
			"httpNodeRoot":    "/",
			"version":         "0.1.0",
			"context":         map[string]any{"stores": []string{"memory"}},
			"flowFilePretty":  true,
		},
	})
}

// handleGetDebugMessages returns recent debug messages from the runtime.
//
// GET /api/v1/debug/messages
func (d *Deps) handleGetDebugMessages(w http.ResponseWriter, r *http.Request) {
	// TODO: Retrieve from NATS JetStream ring buffer.
	jsonResponse(w, http.StatusOK, map[string]any{
		"messages": []any{},
	})
}
