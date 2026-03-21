// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/niceclouds/flint/internal/flow"
	flintnats "github.com/niceclouds/flint/internal/nats"
	"github.com/niceclouds/flint/internal/storage"
	"github.com/niceclouds/flint/internal/ws"
)

// Deps holds the dependencies that API handlers need.
type Deps struct {
	Engine  *flow.Engine
	Storage storage.Storage
	Broker  *flintnats.Broker
	Hub     *ws.Hub
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

	d.broadcast(ws.EventDeploy, map[string]any{"action": "deploying", "revision": ""})

	// Save to persistent storage.
	if err := d.Storage.SaveFlows(req.Flows); err != nil {
		slog.Error("failed to save flows", "error", err)
		d.broadcast(ws.EventDeploy, map[string]any{"action": "failed", "revision": "", "message": "failed to save flows"})
		jsonError(w, http.StatusInternalServerError, "failed to save flows")
		return
	}

	// Create flow-scoped context KV buckets.
	ctx := context.Background()
	for _, f := range req.Flows {
		if _, _, err := d.Broker.SetupFlowContextKV(ctx, f.ID); err != nil {
			slog.Error("failed to setup flow context KV", "flow_id", f.ID, "error", err)
		}
	}

	// Deploy to the runtime engine.
	if err := d.Engine.Deploy(req.Flows); err != nil {
		slog.Error("failed to deploy flows to engine", "error", err)
		d.broadcast(ws.EventDeploy, map[string]any{"action": "failed", "revision": "", "message": err.Error()})
		jsonError(w, http.StatusInternalServerError, "failed to deploy flows")
		return
	}

	rev := flowsRevision(req.Flows)

	slog.Info("flows deployed successfully", "rev", rev)

	d.broadcast(ws.EventDeploy, map[string]any{"action": "deployed", "revision": rev})

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"rev":     rev,
		"flows":   req.Flows,
	})
}

// broadcast sends a WebSocket event if the Hub is configured.
func (d *Deps) broadcast(eventType string, payload any) {
	if d.Hub != nil {
		d.Hub.Broadcast(eventType, payload)
	}
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

	if err := d.Engine.TriggerNode(id); err != nil {
		slog.Warn("failed to trigger node", "node_id", id, "error", err)
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}

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

// handleGetNodeStatuses returns the last known status for all running nodes.
//
// GET /api/v1/status/nodes
func (d *Deps) handleGetNodeStatuses(w http.ResponseWriter, r *http.Request) {
	statuses := d.Engine.NodeStatuses()
	jsonResponse(w, http.StatusOK, map[string]any{
		"statuses": statuses,
	})
}

// handleGetDebugMessages returns recent debug messages from the JetStream ring buffer.
//
// GET /api/v1/debug/messages?limit=100&flowId=<id>&nodeId=<id>
//
// Query parameters:
//
//	limit  – max number of messages to return (1–1000, default 100)
//	flowId – optional: restrict to messages from a specific flow
//	nodeId – optional: restrict to a specific node (requires flowId)
func (d *Deps) handleGetDebugMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
	}

	// Build NATS subject filter from optional query parameters.
	var subject string
	flowID := q.Get("flowId")
	nodeID := q.Get("nodeId")
	switch {
	case flowID != "" && nodeID != "":
		subject = fmt.Sprintf("debug.%s.%s", flowID, nodeID)
	case flowID != "":
		subject = fmt.Sprintf("debug.%s.>", flowID)
	default:
		subject = "" // no filter → all debug messages
	}

	raw, err := d.Broker.GetDebugMessages(r.Context(), limit, subject)
	if err != nil {
		slog.Error("failed to read debug messages from JetStream", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to read debug messages")
		return
	}

	// Decode raw JSON bytes into generic maps so we can re-encode as a JSON array.
	messages := make([]json.RawMessage, 0, len(raw))
	for _, b := range raw {
		messages = append(messages, json.RawMessage(b))
	}

	slog.Debug("GET /debug/messages", "limit", limit, "subject", subject, "count", len(messages))

	jsonResponse(w, http.StatusOK, map[string]any{
		"messages": messages,
		"count":    len(messages),
		"limit":    limit,
	})
}
