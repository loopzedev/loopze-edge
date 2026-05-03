// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/niceclouds/loopze/internal/auth"
)

// createUserRequest is the body of POST /api/v1/users. The role is
// validated to one of "admin"/"editor"/"viewer".
type createUserRequest struct {
	Username string    `json:"username"`
	Password string    `json:"password"`
	Role     auth.Role `json:"role"`
}

// updateUserRequest is the body of PATCH /api/v1/users/{id}. Pointers
// distinguish between "not present" and "set to zero value".
type updateUserRequest struct {
	Role     *auth.Role `json:"role,omitempty"`
	Disabled *bool      `json:"disabled,omitempty"`
}

// passwordRequest is the body of POST /api/v1/users/{id}/password.
type passwordRequest struct {
	Password string `json:"password"`
}

// handleListUsers returns all users in alphabetical order.
//
// GET /api/v1/users
func (d *Deps) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := d.Users.List()
	if err != nil {
		slog.Error("list users", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	out := make([]auth.PublicUser, 0, len(users))
	for _, u := range users {
		out = append(out, u.Public())
	}
	jsonResponse(w, http.StatusOK, map[string]any{"users": out})
}

// handleCreateUser creates a new user. Admin role is allowed but
// requires the caller to also be admin (which the route already
// enforces — handlers in this file are mounted under RequireRole(admin)).
//
// POST /api/v1/users
func (d *Deps) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username, err := auth.ValidateUsername(req.Username)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Role.Valid() {
		jsonError(w, http.StatusBadRequest, "invalid role")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("create user: hash", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	id, err := auth.NewID()
	if err != nil {
		slog.Error("create user: new id", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to allocate id")
		return
	}

	now := time.Now().UTC()
	u := auth.User{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Role:         req.Role,
		AuthProvider: auth.ProviderLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := d.Users.Create(u); err != nil {
		if errors.Is(err, auth.ErrUsernameTaken) {
			jsonError(w, http.StatusConflict, "username already taken")
			return
		}
		slog.Error("create user: persist", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	slog.Info("user created", "user_id", u.ID, "username", u.Username, "role", u.Role)
	jsonResponse(w, http.StatusCreated, map[string]any{"user": u.Public()})
}

// handleUpdateUser changes role or disabled state. Username and
// AuthProvider are immutable in V1.
//
// PATCH /api/v1/users/{id}
func (d *Deps) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing user id")
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role == nil && req.Disabled == nil {
		jsonError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	if req.Role != nil && !req.Role.Valid() {
		jsonError(w, http.StatusBadRequest, "invalid role")
		return
	}

	current, err := d.Users.Get(id)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			jsonError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("update user: lookup", "error", err)
		jsonError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	updated := *current
	if req.Role != nil {
		updated.Role = *req.Role
	}
	if req.Disabled != nil {
		updated.Disabled = *req.Disabled
	}
	updated.UpdatedAt = time.Now().UTC()

	if err := d.Users.Update(updated); err != nil {
		if errors.Is(err, auth.ErrLastAdmin) {
			jsonError(w, http.StatusConflict, "cannot remove the last active admin")
			return
		}
		slog.Error("update user: persist", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	// If the user became disabled, log them out everywhere.
	if updated.Disabled && !current.Disabled {
		if err := d.Sessions.DeleteAllForUser(r.Context(), updated.ID); err != nil {
			slog.Warn("update user: revoke sessions", "user_id", updated.ID, "error", err)
		}
	}

	slog.Info("user updated", "user_id", updated.ID, "role", updated.Role, "disabled", updated.Disabled)
	jsonResponse(w, http.StatusOK, map[string]any{"user": updated.Public()})
}

// handleSetPassword sets a new password for the target user and
// invalidates all of that user's sessions.
//
// POST /api/v1/users/{id}/password
func (d *Deps) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing user id")
		return
	}

	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	current, err := d.Users.Get(id)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			jsonError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("set password: lookup", "error", err)
		jsonError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("set password: hash", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	updated := *current
	updated.PasswordHash = hash
	updated.UpdatedAt = time.Now().UTC()
	if err := d.Users.Update(updated); err != nil {
		slog.Error("set password: persist", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	if err := d.Sessions.DeleteAllForUser(r.Context(), updated.ID); err != nil {
		slog.Warn("set password: revoke sessions", "user_id", updated.ID, "error", err)
	}

	slog.Info("user password reset", "user_id", updated.ID)
	w.WriteHeader(http.StatusNoContent)
}
