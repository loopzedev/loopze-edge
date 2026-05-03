# Plan: User Authentication (V1, Local Auth)

Implementation plan for [`USER_AUTH.md`](./USER_AUTH.md). Goal: V1 with first-run setup, local users, roles `admin` / `editor` / `viewer`. SSO is explicitly **not** part of V1.

## Order

Backend first, then frontend. Within backend: domain → persistence → sessions → HTTP layer. This way something runnable is testable after each step.

```
[1] auth package (domain, Argon2)
   ↓
[2] UserStorage in storage package (users.json)
   ↓
[3] Session manager (NATS-KV)
   ↓
[4] Auth middleware
   ↓
[5] HTTP routes (setup, login, logout, me, users) + route protection
   ↓
[6] WebSocket protection
   ↓
[7] Frontend: auth store + API hooks
   ↓
[8] Setup modal
   ↓
[9] Login modal
   ↓
[10] User management view
   ↓
[11] Role gating in existing UI components
   ↓
[12] Tests + dev-bypass flag
```

After each step: compile, start the server, check the "smoke test" criterion defined in that step.

---

## Step 1 — `internal/auth` Package

**Goal:** Domain and password hashing as an isolated package, without HTTP or storage dependency.

**New files:**
- `internal/auth/types.go` — `User`, `Role` (`admin`/`editor`/`viewer`), `AuthProvider` (constant `local`), validation functions.
- `internal/auth/password.go` — `HashPassword(plain string) (string, error)`, `VerifyPassword(plain, hash string) bool`. **Argon2id** with the OWASP 2024 defaults (m=19MiB, t=2, p=1).
- `internal/auth/store.go` — interface `UserStore`:
  ```go
  type UserStore interface {
      Get(id string) (*User, error)
      GetByUsername(provider, username string) (*User, error)
      List() ([]User, error)
      Create(u User) error
      Update(u User) error
      CountActiveAdmins() (int, error)
  }
  ```
  Errors: `ErrUserNotFound`, `ErrUsernameTaken`, `ErrLastAdmin`.
- `internal/auth/auth_test.go` — hash roundtrip, username normalization (case-insensitive), validation.

**Dependency:** `golang.org/x/crypto/argon2` (standard lib under `x/`, no new major dependency).

**Smoke test:** `go test ./internal/auth/...` green.

---

## Step 2 — UserStorage (File-Based)

**Goal:** `users.json` next to `workspace.json` and `credentials.json`.

**Extension in `internal/storage/storage.go`:**

```go
type Storage interface {
    // ... existing ...
    LoadUsers() ([]auth.User, error)
    SaveUsers(users []auth.User) error
}
```

`FileStorage` gets a new field `usersFile string`. Write with `atomicWriteFile(path, data, 0600)` — **0600**, not 0644, because the file contains Argon2 hashes.

**Adjustment `internal/config/config.go`:**
- New default `defaultUsersFile = "users.json"`
- New flag/env: `--users-file` / `LOOPZE_USERS_FILE`
- New method: `(c *Config) UsersFilePath() string`

**Adjustment `internal/server/server.go`:**
- `NewFileStorage(...)` additionally receives `cfg.UsersFilePath()`.

**New file `internal/auth/filestore.go`:**
- `NewFileStore(s storage.Storage) *FileStore` — implements `UserStore` over the storage interface. Loads once on start into an in-memory map, persists on every write.
- Mutex for concurrent reads (analogous to `FileStorage`).

**Smoke test:** Server starts, an empty `users.json` is created as soon as the first user is written (no pre-touch needed).

---

## Step 3 — Session Manager (NATS-KV)

**Goal:** Session persistence with TTL, no separate state daemon.

**New file `internal/auth/session.go`:**

```go
type Session struct {
    ID        string    // ULID, in cookie
    UserID    string
    CreatedAt time.Time
    ExpiresAt time.Time
}

type SessionManager struct { ... }

func (s *SessionManager) Create(userID string) (*Session, error)
func (s *SessionManager) Get(id string) (*Session, error)
func (s *SessionManager) Refresh(id string) error  // sliding window
func (s *SessionManager) Delete(id string) error
func (s *SessionManager) DeleteAllForUser(userID string) error  // after disable / pw reset
```

**Storage:** NATS JetStream KV bucket `auth-sessions` with TTL = 12 h. `Refresh` writes the same entry with renewed TTL — JetStream KV supports MaxAge per bucket; per entry every write triggers a TTL reset (see `internal/nats/context_store.go` for the KV pattern we already use).

**Cookie signing:** Session IDs are HMAC-SHA256 signed; key from `data/loopze.session.key`. New file `internal/auth/sessionkey.go` with `EnsureSessionKey(path string) ([]byte, error)` — analogous to `credentials.EnsureKeyFile`.

**Wiring in `Broker`:** New method `(*Broker).SetupSessionKV(ctx) (jetstream.KeyValue, error)` in `internal/nats/broker.go`. Called in `server.Start()`, the result passed to the `SessionManager`.

**Smoke test:** Server starts, bucket is created, `nats kv ls` shows `auth-sessions`.

---

## Step 4 — Auth Middleware

**Goal:** A single chi middleware piece that checks requests against sessions+roles.

**New file `internal/auth/middleware.go`:**

```go
type ctxKey int
const userCtxKey ctxKey = 0

func WithUser(r *http.Request) (*User, bool)  // helper for handlers

func RequireRole(min Role) func(http.Handler) http.Handler  // 401 if no session, 403 if role too low
func RequireSetupComplete(...) func(http.Handler) http.Handler  // 503 in setup mode
```

**Role comparison:** `Role` is a `string`, but internally `RoleRank(r) int` (`viewer=1, editor=2, admin=3`) is used for the `>=` comparison. Deliberately flat despite the "not hierarchical" statement in the issue: implementation-wise *rank-based* is simpler, the mapping stays simple (`admin >= editor >= viewer`). The issue meant: no separate permissions, no ACL lists — and that still holds.

**Smoke test:** Unit tests for every rank combination + setup mode.

---

## Step 5 — HTTP Routes + Protection of Existing Routes

**New file `internal/api/auth_handlers.go`:**

| Route | Handler | Body | Response |
|---|---|---|---|
| `POST /api/v1/setup` | `handleSetup` | `{ username, password }` | `201 { user }` or `409` |
| `POST /api/v1/auth/login` | `handleLogin` | `{ username, password }` | `200 { user }` + cookie or `401` |
| `POST /api/v1/auth/logout` | `handleLogout` | – | `204` |
| `GET /api/v1/auth/me` | `handleMe` | – | `200 { user }` or `401` |

**New file `internal/api/user_handlers.go`:**

| Route | Minimum role | Body / Response |
|---|---|---|
| `GET /api/v1/users` | admin | `200 { users: [...] }` |
| `POST /api/v1/users` | admin | `{ username, password, role }` → `201 { user }` |
| `PATCH /api/v1/users/{id}` | admin | `{ role?, disabled? }` |
| `POST /api/v1/users/{id}/password` | admin | `{ password }` |

`/users/{id}/password` invalidates sessions of the target user (`SessionManager.DeleteAllForUser`).

**Extension `Deps`:**
```go
type Deps struct {
    // ... existing ...
    Users    auth.UserStore
    Sessions *auth.SessionManager
    Setup    *auth.SetupGuard  // one-shot lock for setup
}
```

**`internal/api/routes.go`** — route protection with the middleware:

```go
r.Group(func(r chi.Router) {
    r.Use(deps.RequireSetupComplete())  // everything except /setup, /auth/*

    r.Group(func(r chi.Router) {
        r.Use(deps.RequireRole(auth.RoleViewer))
        r.Get("/flows", deps.handleGetFlows)
        r.Get("/nodes", deps.handleGetNodes)
        // … all GETs
    })

    r.Group(func(r chi.Router) {
        r.Use(deps.RequireRole(auth.RoleEditor))
        r.Post("/flows", deps.handleDeployFlows)
        r.Post("/inject/{id}", deps.handleInjectNode)
        r.Delete("/context/...", ...)
    })

    r.Group(func(r chi.Router) {
        r.Use(deps.RequireRole(auth.RoleAdmin))
        r.Mount("/users", userRouter)
    })
})

// open routes (outside the setup gate):
r.Post("/setup", deps.handleSetup)
r.Post("/auth/login", deps.handleLogin)
r.Post("/auth/logout", deps.handleLogout)
r.Get("/auth/me", deps.handleMe)
```

**Login cookie:**
- Name: `loopze_session`
- HttpOnly, Secure (by default; `auth.requireSecureCookies = false` via env to disable in dev), SameSite=Lax
- Path `/`
- MaxAge = 12 h (without `Expires` → browser keeps it until window close, sliding window happens server-side)

**Brute-force protection:** `internal/auth/throttle.go` — in-memory map `map[username]struct{ failures int; until time.Time }`, mutex-guarded. 5 failures in 15 min → lock for another 15 min. Map is cleared on restart (accepted; brute-force across restarts is not a realistic threat model for an on-prem instance).

**Smoke test:**
- `curl /api/v1/flows` without setup → 503
- `curl -X POST /api/v1/setup -d '{"username":"admin","password":"hunter22"}'` → 201
- `curl -X POST /api/v1/auth/login ...` → 200 with Set-Cookie
- `curl --cookie ... /api/v1/flows` → 200
- 2nd setup call → 409

---

## Step 6 — WebSocket Protection

**Adjustment `internal/ws/hub.go`:**

`ServeWS` gets an optional auth function injected (server setup):

```go
type AuthFunc func(*http.Request) (*auth.User, error)

func (h *Hub) ServeWSAuthed(authFn AuthFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, err := authFn(r)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        // existing upgrade code, plus store user in client
    }
}
```

The `Client` gets a field `userID string` (for later audit / per-user filter — V1 only logging).

**Session expiration termination:** The `SessionManager` publishes a NATS event `auth.session.deleted` with `{ sessionID, userID }` on `Delete`/`DeleteAllForUser`. The hub subscribes to it and closes all clients with the matching `userID`. This way logout works immediately across all open tabs.

**Smoke test:** WS connection without cookie → 401. WS connection with valid cookie → connect. Logout → WS closes server-side.

---

## Step 7 — Frontend Auth Store

**New file `frontend/src/stores/authStore.ts`** (Pinia, analogous to the existing stores):

```ts
state: () => ({
  user: null as User | null,
  loading: true,
  needsSetup: false,
})
actions:
  init()       // GET /auth/me → user; on 503 → needsSetup=true; on 401 → user=null
  setup(...)   // POST /setup → init()
  login(...)   // POST /auth/login → init()
  logout()     // POST /auth/logout → user=null
getters:
  isAuthenticated, role, can(action) // 'deploy'|'inject'|'manageUsers'|...
```

**Extension `useApi.ts`:** `request<T>` interprets `401` centrally — sets `authStore.user = null` so the login modal appears automatically.

**App.vue:** calls `authStore.init()` on mount and blocks the rendering of the editor content until `authStore.loading === false`. Depending on state:
- `needsSetup` → `<SetupModal>`
- `!user` → `<LoginModal>`
- otherwise → existing editor

**Smoke test:** Frontend build runs, `/auth/me` is called on page load.

---

## Step 8 — Setup Modal

**New file `frontend/src/components/auth/SetupModal.vue`:**
- Username, password, password confirmation, submit button
- Minimum length 8, live validation
- On success: automatically `authStore.login(...)` with the same data (the `user` object returned from the setup endpoint is sufficient; we trigger the login directly after setup in the backend → saves a roundtrip; sets the cookie immediately).

**Backend adjustment:** `handleSetup` sets a session cookie directly after creating the admin. Rationale: a directly following separate login call could fail due to a race with the setup lock, and the UX advantage is clear.

**Smoke test:** On an empty database the UI starts with the modal, after submit the user lands in the editor.

---

## Step 9 — Login Modal

**New file `frontend/src/components/auth/LoginModal.vue`:**
- Username, password
- Error display for `401` and `429` (rate limit)
- Optionally hidden section "Login with ..." as preparation for SSO (V1: empty, no code)

**Smoke test:** Wrong password → error message. Correct → editor loads.

---

## Step 10 — User Management View

**New file `frontend/src/views/UsersView.vue`** + route `/users` in `frontend/src/router/index.ts` (with route guard: only visible when `authStore.role === 'admin'`).

**Components:**
- Table: username, role (dropdown to change), status (toggle), auth provider, actions (set password)
- "New user" dialog: username, password, role (`editor` / `viewer`; `admin` only via separate click with confirmation)
- "Set password" dialog per row
- "Disable" toggle per row

**Header entry:** In `HeaderBar.vue` show a link "Users" when `can('manageUsers')`.

**Smoke test:** As admin: create user, disable, set password, change role. As the last admin → an attempt to disable yourself is rejected with `409` and shown as an error in the UI.

---

## Step 11 — Role Gating in Existing Components

**Adjustments** (list, each with `v-if="authStore.can('...')"`):
- `HeaderBar.vue` — Deploy button: `can('deploy')`
- `NodePalette.vue` — drag-out and edit mode: `can('deploy')` (viewer can view flows but not modify)
- `FlowProperties.vue` — editing the fields: `can('deploy')`
- `PropertyPanel.vue` — inputs `disabled` when not `can('deploy')`
- `ContextPanel.vue` — Delete buttons: `can('deploy')` (editor deletes; viewer sees read-only)
- Inject node trigger in the canvas: `can('inject')`

**Important:** UI gating is sugar. The backend routes are the security line.

**Smoke test:** Log in with viewer user — no Deploy/Inject/Delete buttons visible.

---

## Step 12 — Tests, Docs, Dev Bypass

**Backend tests** (in the respective packages):
- `internal/auth/password_test.go` — roundtrip, tampering
- `internal/auth/store_test.go` — CRUD, last-admin protection, username uniqueness
- `internal/auth/middleware_test.go` — all rank combinations × setup mode
- `internal/api/auth_handlers_test.go` — setup idempotency, login cookie, logout, throttle
- `internal/api/user_handlers_test.go` — role enforcement matrix (table tests)

**Dev bypass:**
- Env variable `LOOPZE_DISABLE_AUTH=1` → middleware lets everything through and injects a virtual `dev-admin` user into the context. On server start: bold `slog.Warn` with banner.
- Implemented in the middleware as the first check, before cookie lookup.

**Doc update:**
- `PLANNING.md` — add auth block, status `🚧 In Progress` / then `✅ Done`
- `README.md` — setup hint: "After first start: open browser → create admin"
- *No* dedicated auth doc file. The issue + plan in `docs/issues/` are enough, code is self-documenting.

**Smoke test:** Full run-through:
1. `rm -rf data/`
2. Start server
3. Browser → setup modal → create admin
4. Editor loads with admin permissions
5. Users view → create editor and viewer users
6. Logout → log in as editor → no users view
7. Logout → log in as viewer → no Deploy/Inject buttons

---

## Migration Notes

Existing installations (with `workspace.json` but without `users.json`) automatically land in **setup mode** on first start with this version — the existing workspace is *not* deleted; it simply remains gated until the admin is created and logged in. Add a backups note in the README.

## What Was Deliberately Left Out

- **Custom roles, permissions** → only three roles, done.
- **Email-based password reset** → admin sets out-of-band, self-service is V2.
- **2FA / TOTP** → V2 or with SSO.
- **Audit log** → separate issue, on its own.
- **CSRF token** → we set `SameSite=Lax`. For local editor use (same origin, no embedding) that is sufficient. If LOOPZE is later to be embedded via `<iframe>`, CSRF comes separately.
- **JWT** → see issue, deliberately not.

## Estimated Size

Rough thumb, no guarantees:
- Backend: ~800 LOC incl. tests
- Frontend: ~600 LOC

After confirmation of the plan → start step 1.
