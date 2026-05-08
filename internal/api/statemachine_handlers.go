// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// handleListStateMachines returns one entry per running state machine node in
// the given flow, used to populate the inspector dropdown.
//
// GET /api/v1/state-machines/flow/{flowID}
func (d *Deps) handleListStateMachines(w http.ResponseWriter, r *http.Request) {
	flowID := chi.URLParam(r, "flowID")
	if flowID == "" {
		jsonError(w, http.StatusBadRequest, "missing flow id")
		return
	}

	machines, err := d.Engine.ListStateMachines(flowID)
	if err != nil {
		stateMachineWriteErr(w, r, flowID, "", err)
		return
	}

	sort.Slice(machines, func(i, j int) bool {
		return machines[i].NodeID < machines[j].NodeID
	})

	jsonResponse(w, http.StatusOK, map[string]any{
		"flowID":   flowID,
		"machines": machines,
	})
}

// handleGetStateMachineSnapshot returns the full snapshot for a single state
// machine node: definition states, current state, context, available events,
// and recent transition history.
//
// GET /api/v1/state-machines/flow/{flowID}/{nodeID}
func (d *Deps) handleGetStateMachineSnapshot(w http.ResponseWriter, r *http.Request) {
	flowID := chi.URLParam(r, "flowID")
	nodeID := chi.URLParam(r, "nodeID")
	if flowID == "" || nodeID == "" {
		jsonError(w, http.StatusBadRequest, "missing flow or node id")
		return
	}

	snap, err := d.Engine.StateMachineSnapshot(flowID, nodeID)
	if err != nil {
		stateMachineWriteErr(w, r, flowID, nodeID, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"flowID":   flowID,
		"nodeID":   nodeID,
		"snapshot": snap,
	})
}

// stateMachineWriteErr maps engine sentinel errors to HTTP status codes.
func stateMachineWriteErr(w http.ResponseWriter, r *http.Request, flowID, nodeID string, err error) {
	switch {
	case errors.Is(err, flow.ErrFlowNotFound):
		jsonError(w, http.StatusNotFound, "flow not found")
	case errors.Is(err, flow.ErrNodeNotFound):
		jsonError(w, http.StatusNotFound, "node not found")
	case errors.Is(err, flow.ErrNotStateMachine):
		jsonError(w, http.StatusBadRequest, "node is not a state machine")
	default:
		slog.Error("state machine inspector error",
			"flow_id", flowID, "node_id", nodeID, "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
	}
}
