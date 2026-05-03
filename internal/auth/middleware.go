// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// CookieName is the name of the session cookie set by the auth handlers
// and read by Authenticate. It is exported so tests and the HTTP layer
// can construct cookies without duplicating the literal.
const CookieName = "loopze_session"

// ctxKey is an unexported type used as the key for the user value in the
// request context. Using a private type prevents collisions with keys
// defined in other packages.
type ctxKey int

const userCtxKey ctxKey = 0

// WithUser returns the authenticated user attached to the request by the
// Authenticate middleware. The bool is false if no user is attached.
func WithUser(r *http.Request) (*User, bool) {
	u, ok := r.Context().Value(userCtxKey).(*User)
	return u, ok && u != nil
}

// setUserOnRequest returns a copy of r with the user attached to its
// context. Used both by Authenticate (after a successful cookie lookup)
// and by the dev-bypass path.
func setUserOnRequest(r *http.Request, u *User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
}

// Middleware bundles the dependencies the auth middleware functions need.
// Construct one in server wiring and reuse it across all routes.
type Middleware struct {
	// Users is the user store, used by RequireSetupComplete to count
	// active admins.
	Users UserStore

	// Sessions is the session manager, used by Authenticate to validate
	// cookies and refresh sessions.
	Sessions *SessionManager

	// DevUser, if non-nil, replaces the cookie-based authentication: the
	// user is injected into every request and the setup gate is skipped.
	// Used by LOOPZE_DISABLE_AUTH for local development.
	DevUser *User

	// CookieSecure controls the Secure flag on session cookies. Should be
	// true in any deployment served over HTTPS. May be disabled for
	// localhost development; the user accepts the lower bar.
	CookieSecure bool
}

// BuildSessionCookie returns a session cookie carrying the given signed
// value. ttl controls MaxAge; pass m.Sessions.TTL() in normal flow.
func (m *Middleware) BuildSessionCookie(value string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	}
}

// ClearSessionCookie returns a cookie that, when set on the response,
// instructs the browser to delete the session cookie.
func (m *Middleware) ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

// jsonError writes a JSON error response. Kept local to the auth package
// so the middleware does not depend on internal/api.
func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   http.StatusText(status),
		"message": message,
	})
}

// loadUserFromCookie validates the session cookie on r and returns the
// user it points to, refreshing the session as a side effect. Returns
// (nil, nil) for any "not authenticated" reason (no cookie, bad cookie,
// expired session, missing user, disabled user). A non-nil error is
// returned only for internal failures (KV / store outages).
func (m *Middleware) loadUserFromCookie(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return nil, nil
	}
	sessionID, err := m.Sessions.VerifyCookieValue(cookie.Value)
	if err != nil {
		return nil, nil
	}

	sess, err := m.Sessions.Get(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil
		}
		return nil, err
	}

	user, err := m.Users.Get(sess.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if user.Disabled {
		return nil, nil
	}

	// Sliding-window refresh on every authenticated request. Best-effort:
	// a refresh failure must not break the request.
	if _, err := m.Sessions.Refresh(r.Context(), sessionID); err != nil {
		slog.Warn("auth: session refresh failed", "session_id", sessionID, "error", err)
	}

	return user, nil
}

// Authenticate is the outermost auth middleware. It loads the user (if
// any) into the request context. It does NOT reject unauthenticated
// requests — that is RequireRole's job. Apply this once on the entire
// /api/v1 subtree so RequireRole and handler code can read the user via
// WithUser without doing the cookie lookup themselves.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.DevUser != nil {
			next.ServeHTTP(w, setUserOnRequest(r, m.DevUser))
			return
		}
		user, err := m.loadUserFromCookie(r)
		if err != nil {
			slog.Error("auth: failed to load user", "error", err)
			// Continue as unauthenticated; downstream RequireRole will
			// turn this into 401 if the route requires a user.
		}
		if user != nil {
			r = setUserOnRequest(r, user)
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole returns middleware that allows only requests whose
// authenticated user has rank >= min. Unauthenticated requests get 401;
// authenticated users with insufficient role get 403.
//
// Authenticate must run before this middleware on the same request.
func (m *Middleware) RequireRole(min Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := WithUser(r)
			if !ok {
				jsonError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			if user.Role.Rank() < min.Rank() {
				jsonError(w, http.StatusForbidden, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSetupComplete returns middleware that responds with 503 when no
// active admin exists in the user store. Apply it to every API route
// except /setup itself and the /auth/* family (login/logout/me).
//
// In dev-bypass mode the gate is open regardless of store contents.
func (m *Middleware) RequireSetupComplete() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m.DevUser != nil {
				next.ServeHTTP(w, r)
				return
			}
			n, err := m.Users.CountActiveAdmins()
			if err != nil {
				slog.Error("auth: failed to count admins", "error", err)
				jsonError(w, http.StatusInternalServerError, "failed to check setup status")
				return
			}
			if n == 0 {
				jsonError(w, http.StatusServiceUnavailable, "first-run setup required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
