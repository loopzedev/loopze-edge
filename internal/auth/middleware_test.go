// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testHandler is a minimal http.Handler that records that it ran and lets
// tests inspect the user the middleware put in context.
type testHandler struct {
	called bool
	user   *User
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	if u, ok := WithUser(r); ok {
		h.user = u
	}
	w.WriteHeader(http.StatusOK)
}

// newTestMiddleware builds a fully wired Middleware (FileStore + memory
// session manager). Returns the middleware plus the user store and
// sessions so tests can seed state.
func newTestMiddleware(t *testing.T) (*Middleware, *FileStore, *SessionManager) {
	t.Helper()
	store, err := NewFileStore(&memStorage{})
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, sessionKeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	sm, err := NewSessionManager(NewMemorySessionStore(), key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return &Middleware{Users: store, Sessions: sm}, store, sm
}

// loginCookie creates a session for the given user and returns the
// cookie that Authenticate would accept.
func loginCookie(t *testing.T, sm *SessionManager, userID string) *http.Cookie {
	t.Helper()
	sess, err := sm.Create(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: CookieName, Value: sm.SignCookieValue(sess.ID)}
}

func TestAuthenticateNoCookie(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)
	h := &testHandler{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.Authenticate(h).ServeHTTP(rec, req)

	if !h.called {
		t.Error("handler not called")
	}
	if h.user != nil {
		t.Errorf("expected no user, got %v", h.user)
	}
}

func TestAuthenticateValidCookie(t *testing.T) {
	mw, store, sm := newTestMiddleware(t)
	u := makeUser(t, "alice", RoleEditor)
	if err := store.Create(u); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(loginCookie(t, sm, u.ID))

	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.Authenticate(h).ServeHTTP(rec, req)

	if h.user == nil {
		t.Fatal("expected user in context")
	}
	if h.user.ID != u.ID {
		t.Errorf("got user %q, want %q", h.user.ID, u.ID)
	}
}

func TestAuthenticateTamperedCookieIsIgnored(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "bogus.value"})

	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.Authenticate(h).ServeHTTP(rec, req)

	// Tampered cookies are silently treated as "not authenticated"; the
	// request still reaches the next handler.
	if !h.called {
		t.Error("handler not called")
	}
	if h.user != nil {
		t.Error("tampered cookie should not yield a user")
	}
}

func TestAuthenticateDisabledUserNotAttached(t *testing.T) {
	mw, store, sm := newTestMiddleware(t)
	u := makeUser(t, "alice", RoleEditor)
	if err := store.Create(u); err != nil {
		t.Fatal(err)
	}
	// Disable after session creation: cookie is valid but the user is
	// no longer permitted to act.
	cookie := loginCookie(t, sm, u.ID)
	u.Disabled = true
	// Need a second admin so Update doesn't trip ErrLastAdmin (alice is
	// editor anyway, but verify the helper logic).
	if err := store.Update(u); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)

	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.Authenticate(h).ServeHTTP(rec, req)

	if h.user != nil {
		t.Errorf("disabled user should not be attached, got %v", h.user)
	}
}

func TestAuthenticateDevBypass(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)
	mw.DevUser = &User{ID: "dev", Username: "dev", Role: RoleAdmin}

	req := httptest.NewRequest(http.MethodGet, "/", nil) // no cookie
	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.Authenticate(h).ServeHTTP(rec, req)

	if h.user == nil || h.user.Username != "dev" {
		t.Errorf("expected dev user, got %v", h.user)
	}
}

func TestRequireRoleMatrix(t *testing.T) {
	cases := []struct {
		userRole Role
		minRole  Role
		want     int
	}{
		{RoleAdmin, RoleAdmin, http.StatusOK},
		{RoleAdmin, RoleEditor, http.StatusOK},
		{RoleAdmin, RoleViewer, http.StatusOK},
		{RoleEditor, RoleAdmin, http.StatusForbidden},
		{RoleEditor, RoleEditor, http.StatusOK},
		{RoleEditor, RoleViewer, http.StatusOK},
		{RoleViewer, RoleAdmin, http.StatusForbidden},
		{RoleViewer, RoleEditor, http.StatusForbidden},
		{RoleViewer, RoleViewer, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(string(c.userRole)+"_vs_"+string(c.minRole), func(t *testing.T) {
			mw, store, sm := newTestMiddleware(t)
			u := makeUser(t, "u-"+string(c.userRole), c.userRole)
			if err := store.Create(u); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(loginCookie(t, sm, u.ID))

			h := &testHandler{}
			rec := httptest.NewRecorder()
			handler := mw.Authenticate(mw.RequireRole(c.minRole)(h))
			handler.ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Errorf("got %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestRequireRoleNoAuth(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h := &testHandler{}
	rec := httptest.NewRecorder()
	handler := mw.Authenticate(mw.RequireRole(RoleViewer)(h))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rec.Code)
	}
	if h.called {
		t.Error("handler should not have been called")
	}
}

func TestRequireSetupCompleteEmpty(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.RequireSetupComplete()(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", rec.Code)
	}
}

func TestRequireSetupCompleteWithAdmin(t *testing.T) {
	mw, store, _ := newTestMiddleware(t)
	if err := store.Create(makeUser(t, "alice", RoleAdmin)); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.RequireSetupComplete()(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rec.Code)
	}
}

func TestRequireSetupCompleteIgnoresDisabledAdmin(t *testing.T) {
	mw, store, _ := newTestMiddleware(t)
	u := makeUser(t, "alice", RoleAdmin)
	u.Disabled = true
	if err := store.Create(u); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.RequireSetupComplete()(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("disabled admin should not unlock setup gate; got %d", rec.Code)
	}
}

func TestRequireSetupCompleteDevBypass(t *testing.T) {
	mw, _, _ := newTestMiddleware(t)
	mw.DevUser = &User{ID: "dev", Username: "dev", Role: RoleAdmin}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h := &testHandler{}
	rec := httptest.NewRecorder()
	mw.RequireSetupComplete()(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("dev bypass should pass setup gate; got %d", rec.Code)
	}
}
