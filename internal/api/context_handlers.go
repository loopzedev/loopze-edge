// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"

	"github.com/go-chi/chi/v5"
	loopzenats "github.com/loopzedev/loopze-edge/internal/nats"
)

// resolveContextStore returns a KVContextStore for the given scope/storage.
// flowID is required when scope == "flow" and ignored otherwise.
// Both scope and storage are validated; unknown values yield ("", err).
func (d *Deps) resolveContextStore(r *http.Request, scope, flowID, storage string) (*loopzenats.KVContextStore, int, error) {
	if storage != "memory" && storage != "persistent" {
		return nil, http.StatusBadRequest, errors.New("invalid storage type, must be 'memory' or 'persistent'")
	}

	ctx := r.Context()

	switch scope {
	case "global":
		mem, pers, err := d.Broker.SetupContextKV(ctx)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if storage == "memory" {
			return loopzenats.NewKVContextStore(mem), 0, nil
		}
		return loopzenats.NewKVContextStore(pers), 0, nil

	case "flow":
		if flowID == "" {
			return nil, http.StatusBadRequest, errors.New("missing flow id")
		}
		// Validate the flow actually exists to avoid creating orphan KV buckets
		// for arbitrary IDs sent by clients.
		found := false
		for _, f := range d.Engine.Flows() {
			if f.ID == flowID {
				found = true
				break
			}
		}
		if !found {
			return nil, http.StatusNotFound, errors.New("flow not found")
		}

		mem, pers, err := d.Broker.SetupFlowContextKV(ctx, flowID)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if storage == "memory" {
			return loopzenats.NewKVContextStore(mem), 0, nil
		}
		return loopzenats.NewKVContextStore(pers), 0, nil

	default:
		return nil, http.StatusBadRequest, errors.New("invalid scope, must be 'global' or 'flow'")
	}
}

// contextEntry is the JSON shape for a single context key/value pair.
type contextEntry struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// scopeFromRequest infers the scope from the URL: presence of {flowID}
// in the route means scope=flow, otherwise scope=global.
func scopeFromRequest(r *http.Request) (scope, flowID string) {
	flowID = chi.URLParam(r, "flowID")
	if flowID != "" {
		return "flow", flowID
	}
	return "global", ""
}

// handleGetContextStore returns all key/value pairs in a context store.
//
// GET /api/v1/context/global/{storage}
// GET /api/v1/context/flow/{flowID}/{storage}
func (d *Deps) handleGetContextStore(w http.ResponseWriter, r *http.Request) {
	scope, flowID := scopeFromRequest(r)
	storage := chi.URLParam(r, "storage")

	store, status, err := d.resolveContextStore(r, scope, flowID, storage)
	if err != nil {
		jsonError(w, status, err.Error())
		return
	}

	keys, err := store.Keys()
	if err != nil {
		slog.Error("failed to list context keys", "scope", scope, "flow_id", flowID, "storage", storage, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to list context keys")
		return
	}

	sort.Strings(keys)

	entries := make([]contextEntry, 0, len(keys))
	for _, k := range keys {
		v, err := store.Get(k)
		if err != nil {
			slog.Warn("failed to read context key", "scope", scope, "flow_id", flowID, "storage", storage, "key", k, "error", err)
			continue
		}
		entries = append(entries, contextEntry{Key: k, Value: v})
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"scope":   scope,
		"storage": storage,
		"flowId":  flowID,
		"entries": entries,
	})
}

// handleGetContextKey returns a single key/value pair.
//
// GET /api/v1/context/global/{storage}/{key}
// GET /api/v1/context/flow/{flowID}/{storage}/{key}
func (d *Deps) handleGetContextKey(w http.ResponseWriter, r *http.Request) {
	scope, flowID := scopeFromRequest(r)
	storage := chi.URLParam(r, "storage")
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		jsonError(w, http.StatusBadRequest, "invalid or missing key")
		return
	}

	store, status, err := d.resolveContextStore(r, scope, flowID, storage)
	if err != nil {
		jsonError(w, status, err.Error())
		return
	}

	v, err := store.Get(key)
	if err != nil {
		slog.Error("failed to read context key", "scope", scope, "flow_id", flowID, "storage", storage, "key", key, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to read context key")
		return
	}
	if v == nil {
		jsonError(w, http.StatusNotFound, "key not found")
		return
	}

	jsonResponse(w, http.StatusOK, contextEntry{Key: key, Value: v})
}

// handleDeleteContextKey removes a single key.
//
// DELETE /api/v1/context/global/{storage}/{key}
// DELETE /api/v1/context/flow/{flowID}/{storage}/{key}
func (d *Deps) handleDeleteContextKey(w http.ResponseWriter, r *http.Request) {
	scope, flowID := scopeFromRequest(r)
	storage := chi.URLParam(r, "storage")
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	if err != nil || key == "" {
		jsonError(w, http.StatusBadRequest, "invalid or missing key")
		return
	}

	store, status, err := d.resolveContextStore(r, scope, flowID, storage)
	if err != nil {
		jsonError(w, status, err.Error())
		return
	}

	if err := store.Delete(key); err != nil {
		slog.Error("failed to delete context key", "scope", scope, "flow_id", flowID, "storage", storage, "key", key, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to delete context key")
		return
	}

	slog.Debug("context key deleted", "scope", scope, "flow_id", flowID, "storage", storage, "key", key)
	w.WriteHeader(http.StatusNoContent)
}

// handleClearContextStore deletes all keys in a context store.
//
// DELETE /api/v1/context/global/{storage}
// DELETE /api/v1/context/flow/{flowID}/{storage}
func (d *Deps) handleClearContextStore(w http.ResponseWriter, r *http.Request) {
	scope, flowID := scopeFromRequest(r)
	storage := chi.URLParam(r, "storage")

	store, status, err := d.resolveContextStore(r, scope, flowID, storage)
	if err != nil {
		jsonError(w, status, err.Error())
		return
	}

	keys, err := store.Keys()
	if err != nil {
		slog.Error("failed to list context keys for clear", "scope", scope, "flow_id", flowID, "storage", storage, "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to list context keys")
		return
	}

	deleted := 0
	for _, k := range keys {
		if err := store.Delete(k); err != nil {
			slog.Warn("failed to delete context key during clear", "scope", scope, "flow_id", flowID, "storage", storage, "key", k, "error", err)
			continue
		}
		deleted++
	}

	slog.Info("context store cleared", "scope", scope, "flow_id", flowID, "storage", storage, "deleted", deleted)
	jsonResponse(w, http.StatusOK, map[string]any{
		"scope":   scope,
		"storage": storage,
		"flowId":  flowID,
		"deleted": deleted,
	})
}
