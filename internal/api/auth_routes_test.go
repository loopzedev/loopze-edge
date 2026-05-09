// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/auth"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// memStorage implements storage.Storage for the API tests. The
// non-auth methods panic — they should never be called from auth tests.
type memStorage struct {
	users       []byte
	credentials []byte
	workspace   *flow.Workspace
}

func (m *memStorage) LoadFlows() ([]flow.Flow, error) { return []flow.Flow{}, nil }
func (m *memStorage) SaveFlows([]flow.Flow) error     { return nil }
func (m *memStorage) LoadWorkspace() (flow.Workspace, error) {
	if m.workspace != nil {
		return *m.workspace, nil
	}
	return flow.Workspace{Flows: []flow.Flow{}}, nil
}
func (m *memStorage) SaveWorkspace(ws flow.Workspace) error {
	cp := ws
	m.workspace = &cp
	return nil
}
func (m *memStorage) LoadCredentials() ([]byte, error) { return m.credentials, nil }
func (m *memStorage) SaveCredentials(b []byte) error {
	m.credentials = make([]byte, len(b))
	copy(m.credentials, b)
	return nil
}
func (m *memStorage) LoadUsers() ([]byte, error)  { return m.users, nil }
func (m *memStorage) SaveUsers(data []byte) error { m.users = data; return nil }

// testServer wraps an httptest.Server with the deps that produced it,
// so individual tests can mutate state directly (e.g. seed users).
type testServer struct {
	srv      *httptest.Server
	deps     *Deps
	users    *auth.FileStore
	sessions *auth.SessionManager
	url      string
}

func (ts *testServer) close() { ts.srv.Close() }

func (ts *testServer) jarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	storage := &memStorage{}
	users, err := auth.NewFileStore(storage)
	if err != nil {
		t.Fatal(err)
	}
	signKey := make([]byte, 32)
	for i := range signKey {
		signKey[i] = byte(i + 1)
	}
	sm, err := auth.NewSessionManager(auth.NewMemorySessionStore(), signKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mw := &auth.Middleware{Users: users, Sessions: sm}

	deps := &Deps{Storage: storage}
	deps.Users = users
	deps.Sessions = sm
	deps.AuthMW = mw
	deps.Throttle = auth.NewLoginThrottle(nil)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		RegisterRoutes(r, deps)
	})

	srv := httptest.NewServer(r)
	return &testServer{srv: srv, deps: deps, users: users, sessions: sm, url: srv.URL}
}

// post / get / patch helpers that do JSON body marshalling and capture
// status + decoded body. Tests should never have to remember to set
// the Content-Type header.

func doJSON(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if len(raw) > 0 && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		_ = json.Unmarshal(raw, &parsed)
	}
	return resp.StatusCode, parsed
}

// ── Setup-flow tests ─────────────────────────────────────────────────────

func TestSetupFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)

	t.Run("pre-setup gates protected routes", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/flows", nil)
		if code != http.StatusServiceUnavailable {
			t.Errorf("got %d, want 503", code)
		}
	})

	t.Run("setup rejects short password", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/setup",
			map[string]string{"username": "admin", "password": "x"})
		if code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", code)
		}
	})

	t.Run("setup creates admin and issues cookie", func(t *testing.T) {
		code, body := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/setup",
			map[string]string{"username": "admin", "password": "hunter22"})
		if code != http.StatusCreated {
			t.Fatalf("got %d, want 201; body=%v", code, body)
		}
		// Auto-login: /auth/me should now succeed.
		code, _ = doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/me", nil)
		if code != http.StatusOK {
			t.Errorf("/auth/me after setup: got %d, want 200", code)
		}
	})

	t.Run("second setup is rejected with 409", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/setup",
			map[string]string{"username": "admin2", "password": "hunter22"})
		if code != http.StatusConflict {
			t.Errorf("got %d, want 409", code)
		}
	})
}

// ── Login / logout / throttle tests ──────────────────────────────────────

func seedAdmin(t *testing.T, ts *testServer) {
	t.Helper()
	hash, err := auth.HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := auth.NewID()
	now := time.Now().UTC()
	if err := ts.users.Create(auth.User{
		ID:           id,
		Username:     "admin",
		PasswordHash: hash,
		Role:         auth.RoleAdmin,
		AuthProvider: auth.ProviderLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
}

func login(t *testing.T, ts *testServer, c *http.Client, username, password string) int {
	t.Helper()
	code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/auth/login",
		map[string]string{"username": username, "password": password})
	return code
}

func TestLoginAndLogout(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	seedAdmin(t, ts)

	c := ts.jarClient(t)

	t.Run("wrong password", func(t *testing.T) {
		if code := login(t, ts, c, "admin", "wrong"); code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", code)
		}
	})

	t.Run("unknown user looks the same", func(t *testing.T) {
		if code := login(t, ts, c, "ghost", "anything-long-enough"); code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", code)
		}
	})

	t.Run("correct credentials", func(t *testing.T) {
		if code := login(t, ts, c, "admin", "hunter22"); code != http.StatusOK {
			t.Errorf("got %d, want 200", code)
		}

		// Cookie should now be in the jar; /auth/me succeeds.
		code, _ := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/me", nil)
		if code != http.StatusOK {
			t.Errorf("/auth/me after login: got %d, want 200", code)
		}
	})

	t.Run("logout clears cookie", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/auth/logout", nil)
		if code != http.StatusNoContent {
			t.Errorf("logout: got %d, want 204", code)
		}
		code, _ = doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/me", nil)
		if code != http.StatusUnauthorized {
			t.Errorf("/auth/me after logout: got %d, want 401", code)
		}
	})
}

func TestLoginThrottle(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	seedAdmin(t, ts)

	c := ts.jarClient(t)

	for i := 0; i < 5; i++ {
		if code := login(t, ts, c, "admin", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i+1, code)
		}
	}
	// 6th attempt gets locked out — even with the correct password.
	if code := login(t, ts, c, "admin", "hunter22"); code != http.StatusTooManyRequests {
		t.Errorf("post-threshold: got %d, want 429", code)
	}
}

// ── Auth status endpoint (used by frontend init) ─────────────────────────

func TestAuthStatusEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)

	t.Run("pre-setup", func(t *testing.T) {
		code, body := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/status", nil)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
		if body["needsSetup"] != true || body["authenticated"] != false {
			t.Errorf("unexpected body: %v", body)
		}
	})

	seedAdmin(t, ts)

	t.Run("post-setup, anon", func(t *testing.T) {
		code, body := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/status", nil)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
		if body["needsSetup"] != false || body["authenticated"] != false {
			t.Errorf("unexpected body: %v", body)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		login(t, ts, c, "admin", "hunter22")
		code, body := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/auth/status", nil)
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
		if body["authenticated"] != true {
			t.Errorf("expected authenticated=true; got %v", body)
		}
		if _, ok := body["user"]; !ok {
			t.Errorf("expected user in body; got %v", body)
		}
	})
}

// ── Role enforcement matrix ──────────────────────────────────────────────

// seedRoleUsers creates one user per role and returns clients already
// logged in as each. Useful for matrix tests.
func seedRoleUsers(t *testing.T, ts *testServer) map[auth.Role]*http.Client {
	t.Helper()
	seedAdmin(t, ts)

	clients := map[auth.Role]*http.Client{
		auth.RoleAdmin: ts.jarClient(t),
	}
	if code := login(t, ts, clients[auth.RoleAdmin], "admin", "hunter22"); code != http.StatusOK {
		t.Fatalf("admin login failed: %d", code)
	}

	// Use the admin to create editor and viewer through the API so the
	// route is exercised.
	for role, name := range map[auth.Role]string{
		auth.RoleEditor: "alice",
		auth.RoleViewer: "bob",
	} {
		code, _ := doJSON(t, clients[auth.RoleAdmin], http.MethodPost, ts.url+"/api/v1/users",
			map[string]any{"username": name, "password": "hunter22", "role": string(role)})
		if code != http.StatusCreated {
			t.Fatalf("create %s: got %d, want 201", role, code)
		}
		c := ts.jarClient(t)
		if code := login(t, ts, c, name, "hunter22"); code != http.StatusOK {
			t.Fatalf("login %s: got %d, want 200", role, code)
		}
		clients[role] = c
	}
	return clients
}

func TestRoleMatrix(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	clients := seedRoleUsers(t, ts)

	type expect struct {
		method string
		path   string
		role   auth.Role
		want   int
	}
	cases := []expect{
		// Read access — every role.
		{http.MethodGet, "/api/v1/flows", auth.RoleAdmin, http.StatusOK},
		{http.MethodGet, "/api/v1/flows", auth.RoleEditor, http.StatusOK},
		{http.MethodGet, "/api/v1/flows", auth.RoleViewer, http.StatusOK},

		// User listing — admin only.
		{http.MethodGet, "/api/v1/users", auth.RoleAdmin, http.StatusOK},
		{http.MethodGet, "/api/v1/users", auth.RoleEditor, http.StatusForbidden},
		{http.MethodGet, "/api/v1/users", auth.RoleViewer, http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(string(tc.role)+"_"+tc.method+"_"+tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, ts.url+tc.path, nil)
			resp, err := clients[tc.role].Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// ── User CRUD ────────────────────────────────────────────────────────────

func TestUserCRUDLastAdminInvariant(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	seedAdmin(t, ts)
	c := ts.jarClient(t)
	login(t, ts, c, "admin", "hunter22")

	// Find admin's id.
	adminID := ""
	for _, u := range listAllUsers(t, ts, c) {
		if u.Username == "admin" {
			adminID = u.ID
			break
		}
	}
	if adminID == "" {
		t.Fatal("admin user not found")
	}

	t.Run("disable last admin → 409", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPatch, ts.url+"/api/v1/users/"+adminID,
			map[string]any{"disabled": true})
		if code != http.StatusConflict {
			t.Errorf("got %d, want 409", code)
		}
	})

	t.Run("downgrade last admin → 409", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPatch, ts.url+"/api/v1/users/"+adminID,
			map[string]any{"role": "editor"})
		if code != http.StatusConflict {
			t.Errorf("got %d, want 409", code)
		}
	})
}

func TestPasswordResetRevokesSessions(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	seedAdmin(t, ts)

	// Admin client (will issue the reset).
	admin := ts.jarClient(t)
	login(t, ts, admin, "admin", "hunter22")

	// Create a target user via API.
	code, _ := doJSON(t, admin, http.MethodPost, ts.url+"/api/v1/users",
		map[string]any{"username": "alice", "password": "hunter22", "role": "viewer"})
	if code != http.StatusCreated {
		t.Fatalf("create alice: %d", code)
	}

	// Alice logs in with her own client.
	alice := ts.jarClient(t)
	if code := login(t, ts, alice, "alice", "hunter22"); code != http.StatusOK {
		t.Fatalf("alice login: %d", code)
	}
	if code, _ := doJSON(t, alice, http.MethodGet, ts.url+"/api/v1/auth/me", nil); code != http.StatusOK {
		t.Fatalf("alice /me: %d", code)
	}

	// Find alice id.
	var aliceID string
	for _, u := range listAllUsers(t, ts, admin) {
		if u.Username == "alice" {
			aliceID = u.ID
			break
		}
	}
	if aliceID == "" {
		t.Fatal("alice not found")
	}

	// Admin resets alice's password.
	code, _ = doJSON(t, admin, http.MethodPost, ts.url+"/api/v1/users/"+aliceID+"/password",
		map[string]any{"password": "new-password-12"})
	if code != http.StatusNoContent {
		t.Fatalf("password reset: %d", code)
	}

	// Alice's old cookie must no longer authenticate.
	if code, _ := doJSON(t, alice, http.MethodGet, ts.url+"/api/v1/auth/me", nil); code != http.StatusUnauthorized {
		t.Errorf("alice's old session should be revoked, got %d", code)
	}
}

// publicUser mirrors auth.PublicUser for test JSON unmarshal.
type publicUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func listAllUsers(t *testing.T, ts *testServer, c *http.Client) []publicUser {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+"/api/v1/users", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Users []publicUser `json:"users"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body.Users
}

// ── Dev-bypass test ──────────────────────────────────────────────────────

func TestDevBypassSkipsSetupGate(t *testing.T) {
	users, _ := auth.NewFileStore(&memStorage{})
	signKey := make([]byte, 32)
	for i := range signKey {
		signKey[i] = byte(i + 1)
	}
	sm, _ := auth.NewSessionManager(auth.NewMemorySessionStore(), signKey, time.Hour)
	mw := &auth.Middleware{
		Users:    users,
		Sessions: sm,
		DevUser: &auth.User{
			ID:       "dev",
			Username: "dev",
			Role:     auth.RoleAdmin,
		},
	}
	deps := &Deps{}
	deps.Users = users
	deps.Sessions = sm
	deps.AuthMW = mw
	deps.Throttle = auth.NewLoginThrottle(nil)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		RegisterRoutes(r, deps)
	})

	// /users would require admin + setup-complete normally. With
	// DevUser set, both gates are satisfied even though the user store
	// is empty.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("dev bypass: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// silence unused-import warnings in the rare case an import is needed
// only for one of the conditional blocks.
var _ = strings.TrimSpace
var _ = url.Parse
var _ = context.Background
