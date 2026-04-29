// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/niceclouds/flint/internal/flow"
	"github.com/niceclouds/flint/internal/nodes"
)

// opcuaTestConnectionRequest carries the same Config payload that lands in
// workspace.json#configs[].config when the user saves the server. The handler
// builds a transient OpcuaServer from it, opens a short-lived session and
// returns whether the connection succeeded — without persisting anything.
type opcuaTestConnectionRequest struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

// opcuaTestConnectionResponse is the shape the editor expects for the
// "Test Connection" button outcome.
type opcuaTestConnectionResponse struct {
	OK         bool                   `json:"ok"`
	Error      string                 `json:"error,omitempty"`
	ServerInfo *nodes.OpcuaServerInfo `json:"serverInfo,omitempty"`
}

// opcuaBrowseRequest carries either a serverId (use a deployed session) or a
// full server config (open a transient one). nodeId is optional — empty
// browses the Objects folder, the conventional starting point.
type opcuaBrowseRequest struct {
	ServerID string         `json:"serverId,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	NodeID   string         `json:"nodeId,omitempty"`
}

// handleOpcuaBrowse walks one level below the given NodeID. Tries the running
// engine session first, falls back to an ad-hoc transient session.
//
// POST /api/v1/opcua/browse.
func (d *Deps) handleOpcuaBrowse(w http.ResponseWriter, r *http.Request) {
	var req opcuaBrowseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Path 1: a deployed server with this ID — ride along on its session.
	if req.ServerID != "" {
		if inst, ok := d.Engine.GetConfigInstance(req.ServerID); ok {
			if srv, ok := inst.(*nodes.OpcuaServer); ok {
				if client := srv.Client(); client != nil {
					result, err := nodes.OpcuaBrowse(ctx, client, srv, req.NodeID)
					if err != nil {
						jsonResponse(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
						return
					}
					jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
					return
				}
			}
		}
	}

	// Path 2: ad-hoc browse using the supplied config — typical for editing
	// a server that has not yet been deployed.
	if req.Config == nil {
		jsonError(w, http.StatusBadRequest, "either serverId of a deployed server or config is required")
		return
	}
	cfg := flow.ConfigNode{
		ID:     "browse-adhoc",
		Type:   "opcua-server",
		Config: req.Config,
	}
	result, err := nodes.OpcuaBrowseAdHoc(ctx, cfg, req.NodeID)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

// opcuaReadRequest mirrors opcuaBrowseRequest but with a single NodeID to
// read once. Used by the editor's "Read now" button in the address-space
// browser, separate from the streaming Read node.
type opcuaReadRequest struct {
	ServerID string         `json:"serverId,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	NodeID   string         `json:"nodeId"`
}

// handleOpcuaRead reads the Value attribute of a single NodeID and returns
// the decoded result. Re-uses the same session-routing strategy as Browse:
// deployed engine session first, ad-hoc transient session as fallback.
//
// POST /api/v1/opcua/read.
func (d *Deps) handleOpcuaRead(w http.ResponseWriter, r *http.Request) {
	var req opcuaReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.NodeID == "" {
		jsonError(w, http.StatusBadRequest, "nodeId is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Path 1: deployed server.
	if req.ServerID != "" {
		if inst, ok := d.Engine.GetConfigInstance(req.ServerID); ok {
			if srv, ok := inst.(*nodes.OpcuaServer); ok {
				result, err := nodes.OpcuaReadOnce(ctx, srv, req.NodeID)
				if err != nil {
					jsonResponse(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
					return
				}
				jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
				return
			}
		}
	}

	// Path 2: ad-hoc transient session.
	if req.Config == nil {
		jsonError(w, http.StatusBadRequest, "either serverId of a deployed server or config is required")
		return
	}
	cfg := flow.ConfigNode{ID: "read-adhoc", Type: "opcua-server", Config: req.Config}
	result, err := nodes.OpcuaReadOnceAdHoc(ctx, cfg, req.NodeID)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

// handleOpcuaTestConnection probes a candidate OPC UA server config without
// persisting anything. POST /api/v1/opcua/test-connection.
func (d *Deps) handleOpcuaTestConnection(w http.ResponseWriter, r *http.Request) {
	var req opcuaTestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Config == nil {
		jsonError(w, http.StatusBadRequest, "config is required")
		return
	}

	id := req.ID
	if id == "" {
		id = "test-connection"
	}
	cfg := flow.ConfigNode{
		ID:     id,
		Type:   "opcua-server",
		Name:   req.Name,
		Config: req.Config,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info, err := nodes.OpcuaTestConnect(ctx, cfg)
	if err != nil {
		jsonResponse(w, http.StatusOK, opcuaTestConnectionResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}
	jsonResponse(w, http.StatusOK, opcuaTestConnectionResponse{
		OK:         true,
		ServerInfo: info,
	})
}
