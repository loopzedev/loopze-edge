// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes/s7"
)

// s7TestConnectionRequest carries the same Config payload that lands in
// workspace.json#configs[].config when the user saves an S7 PLC. The handler
// builds a transient S7PLC from it, opens a short-lived session and returns
// whether the connection succeeded — without persisting anything.
type s7TestConnectionRequest struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

// s7TestConnectionResponse is the shape the editor expects for the
// "Test Connection" button outcome. On the success path Info carries the
// negotiated PDU plus any CPU info the PLC returned; empty CPU strings are
// substituted with "unknown" so the UI never displays blank cells.
type s7TestConnectionResponse struct {
	OK    bool                         `json:"ok"`
	Error string                       `json:"error,omitempty"`
	Info  *s7.S7TestConnectionInfo  `json:"info,omitempty"`
}

// handleS7TestConnection probes a candidate S7 PLC config without persisting
// anything. POST /api/v1/s7/test-connection.
func (d *Deps) handleS7TestConnection(w http.ResponseWriter, r *http.Request) {
	var req s7TestConnectionRequest
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
		Type:   "s7-plc",
		Name:   req.Name,
		Config: req.Config,
	}

	// Cap the probe at 10s end-to-end. The handler's own connect/timeout
	// kicks in earlier for unreachable hosts; this just protects us against
	// a misconfigured very-long timeout in the request.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info, err := s7.S7TestConnect(ctx, cfg)
	if err != nil {
		jsonResponse(w, http.StatusOK, s7TestConnectionResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}
	jsonResponse(w, http.StatusOK, s7TestConnectionResponse{
		OK:   true,
		Info: fillUnknown(info),
	})
}

// fillUnknown replaces empty CPU/order strings with "unknown" so the editor
// can render a stable "PLC reachable but no CPU info available" hint without
// branching on emptiness for each field. The python-snap7 demo server is the
// main producer of empty fields; real CPUs return populated values.
func fillUnknown(info *s7.S7TestConnectionInfo) *s7.S7TestConnectionInfo {
	if info == nil {
		return nil
	}
	if info.CPUType == "" {
		info.CPUType = "unknown"
	}
	if info.OrderCode == "" {
		info.OrderCode = "unknown"
	}
	if info.ModuleName == "" {
		info.ModuleName = "unknown"
	}
	if info.SerialNumber == "" {
		info.SerialNumber = "unknown"
	}
	return info
}
