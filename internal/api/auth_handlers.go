// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/niceclouds/loopze/internal/auth"
)

// authDeps holds the auth-specific dependencies. It is embedded into the
// main Deps struct so handler methods can be defined directly on Deps.
type authDeps struct {
	Users    auth.UserStore
	Sessions *auth.SessionManager
	AuthMW   *auth.Middleware
	Throttle *auth.LoginThrottle

	// setupMu guards the first-run setup endpoint against a race in
	// which two concurrent calls both find an empty store.
	setupMu sync.Mutex
}

// setupRequest is the body of POST /api/v1/setup.
type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginRequest is the body of POST /api/v1/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetup creates the first admin user. It is idempotent: a second
// call after a successful setup returns 409.
//
// On success the handler also issues a session cookie so the operator
// does not need a follow-up login round-trip — that race condition is
// what motivated bundling the two operations here.
//
// POST /api/v1/setup
func (d *Deps) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
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

	d.setupMu.Lock()
	defer d.setupMu.Unlock()

	n, err := d.Users.CountActiveAdmins()
	if err != nil {
		slog.Error("setup: count admins", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to check setup status")
		return
	}
	if n > 0 {
		jsonError(w, http.StatusConflict, "setup already completed")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("setup: hash password", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	id, err := auth.NewID()
	if err != nil {
		slog.Error("setup: new id", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to allocate id")
		return
	}

	now := time.Now().UTC()
	user := auth.User{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Role:         auth.RoleAdmin,
		AuthProvider: auth.ProviderLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := d.Users.Create(user); err != nil {
		slog.Error("setup: create user", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create admin user")
		return
	}

	sess, err := d.Sessions.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("setup: create session", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, d.AuthMW.BuildSessionCookie(d.Sessions.SignCookieValue(sess.ID), d.Sessions.TTL()))
	slog.Info("first-run setup completed", "user_id", user.ID, "username", user.Username)
	jsonResponse(w, http.StatusCreated, map[string]any{"user": user.Public()})
}

// handleLogin authenticates a user and issues a session cookie.
//
// Throttling: per-username 5 failures / 15 min lock. Failed attempts on
// non-existent or disabled users count just like password mismatches so
// account enumeration is harder.
//
// POST /api/v1/auth/login
func (d *Deps) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username := auth.NormaliseUsername(req.Username)
	if username == "" || req.Password == "" {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !d.Throttle.Allow(username) {
		jsonError(w, http.StatusTooManyRequests, "too many failed attempts; try again later")
		return
	}

	user, err := d.Users.GetByUsername(auth.ProviderLocal, username)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			d.Throttle.RecordFailure(username)
			jsonError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		slog.Error("login: lookup user", "error", err)
		jsonError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if user.Disabled {
		d.Throttle.RecordFailure(username)
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		d.Throttle.RecordFailure(username)
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	d.Throttle.RecordSuccess(username)

	sess, err := d.Sessions.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("login: create session", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, d.AuthMW.BuildSessionCookie(d.Sessions.SignCookieValue(sess.ID), d.Sessions.TTL()))
	slog.Info("user logged in", "user_id", user.ID, "username", user.Username)
	jsonResponse(w, http.StatusOK, map[string]any{"user": user.Public()})
}

// handleLogout invalidates the current session and clears the cookie.
//
// POST /api/v1/auth/logout
func (d *Deps) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.CookieName); err == nil {
		if id, verr := d.Sessions.VerifyCookieValue(cookie.Value); verr == nil {
			if derr := d.Sessions.Delete(r.Context(), id); derr != nil {
				slog.Warn("logout: delete session", "error", derr)
			}
		}
	}
	http.SetCookie(w, d.AuthMW.ClearSessionCookie())
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the currently authenticated user.
//
// GET /api/v1/auth/me
func (d *Deps) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.WithUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"user": user.Public()})
}

// handleAuthStatus reports both the setup state and the current user in
// one response, so the frontend can pick its initial UI state (Setup
// modal / Login modal / Editor) from a single call without having to
// distinguish 401 from 503 on /auth/me.
//
// Always returns 200; the body carries the actual state.
//
// GET /api/v1/auth/status
func (d *Deps) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := d.Users.CountActiveAdmins()
	if err != nil {
		slog.Error("auth status: count admins", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to read setup status")
		return
	}
	user, authed := auth.WithUser(r)
	// An authenticated request implies setup is complete from the
	// caller's perspective — including the LOOPZE_AUTH_DISABLE dev
	// bypass, which short-circuits the user store entirely. Without
	// this, the dev bypass flow would report needsSetup=true alongside
	// authenticated=true, sending the frontend into the Setup modal.
	out := map[string]any{
		"needsSetup":    n == 0 && !authed,
		"authenticated": authed,
	}
	if authed {
		out["user"] = user.Public()
	}
	jsonResponse(w, http.StatusOK, out)
}
