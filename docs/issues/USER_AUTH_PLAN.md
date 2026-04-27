# Plan: Benutzer-Authentifizierung (V1, Lokale Auth)

Implementierungsplan zu [`USER_AUTH.md`](./USER_AUTH.md). Ziel: V1 mit First-Run-Setup, lokalen Usern, Rollen `admin` / `editor` / `viewer`. SSO ist explizit **nicht** Teil von V1.

## Reihenfolge

Backend zuerst, danach Frontend. Innerhalb von Backend: Domäne → Persistenz → Sessions → HTTP-Schicht. So ist nach jedem Schritt etwas Lauffähiges testbar.

```
[1] auth-Package (Domäne, Argon2)
   ↓
[2] UserStorage in storage-Package (users.json)
   ↓
[3] Session-Manager (NATS-KV)
   ↓
[4] Auth-Middleware
   ↓
[5] HTTP-Routen (setup, login, logout, me, users) + Routen-Schutz
   ↓
[6] WebSocket-Schutz
   ↓
[7] Frontend: Auth-Store + API-Hooks
   ↓
[8] Setup-Modal
   ↓
[9] Login-Modal
   ↓
[10] User-Verwaltungs-View
   ↓
[11] Rollen-Gating in vorhandenen UI-Komponenten
   ↓
[12] Tests + Dev-Bypass-Flag
```

Nach jedem Schritt: kompilieren, Server starten, das in dem Schritt definierte „Smoke-Test"-Kriterium prüfen.

---

## Schritt 1 — `internal/auth` Package

**Ziel:** Domäne und Passwort-Hashing als isoliertes Package, ohne HTTP- oder Storage-Abhängigkeit.

**Neue Dateien:**
- `internal/auth/types.go` — `User`, `Role` (`admin`/`editor`/`viewer`), `AuthProvider` (Konstante `local`), Validierungs-Funktionen.
- `internal/auth/password.go` — `HashPassword(plain string) (string, error)`, `VerifyPassword(plain, hash string) bool`. **Argon2id** mit den OWASP-2024-Defaults (m=19MiB, t=2, p=1).
- `internal/auth/store.go` — Interface `UserStore`:
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
- `internal/auth/auth_test.go` — Hash-Roundtrip, Username-Normalisierung (case-insensitive), Validierung.

**Abhängigkeit:** `golang.org/x/crypto/argon2` (Standard-Lib unter `x/`, kein neues Major-Dependency).

**Smoke-Test:** `go test ./internal/auth/...` grün.

---

## Schritt 2 — UserStorage (File-Based)

**Ziel:** `users.json` neben `workspace.json` und `credentials.json`.

**Erweiterung in `internal/storage/storage.go`:**

```go
type Storage interface {
    // ... bestehend ...
    LoadUsers() ([]auth.User, error)
    SaveUsers(users []auth.User) error
}
```

`FileStorage` bekommt ein neues Feld `usersFile string`. Schreiben mit `atomicWriteFile(path, data, 0600)` — **0600**, nicht 0644, weil die Datei Argon2-Hashes enthält.

**Anpassung `internal/config/config.go`:**
- Neuer Default `defaultUsersFile = "users.json"`
- Neue Flag/Env: `--users-file` / `FLINT_USERS_FILE`
- Neue Methode: `(c *Config) UsersFilePath() string`

**Anpassung `internal/server/server.go`:**
- `NewFileStorage(...)` bekommt zusätzlich `cfg.UsersFilePath()`.

**Neue Datei `internal/auth/filestore.go`:**
- `NewFileStore(s storage.Storage) *FileStore` — implementiert `UserStore` über die Storage-Schnittstelle. Lädt einmal beim Start in eine In-Memory-Map, persistiert bei jedem Write.
- Mutex für Concurrent-Reads (analog zu `FileStorage`).

**Smoke-Test:** Server startet, leere `users.json` wird angelegt sobald der erste User geschrieben wird (kein Pre-Touch nötig).

---

## Schritt 3 — Session-Manager (NATS-KV)

**Ziel:** Session-Persistenz mit TTL, kein eigener State-Daemon.

**Neue Datei `internal/auth/session.go`:**

```go
type Session struct {
    ID        string    // ULID, in Cookie
    UserID    string
    CreatedAt time.Time
    ExpiresAt time.Time
}

type SessionManager struct { ... }

func (s *SessionManager) Create(userID string) (*Session, error)
func (s *SessionManager) Get(id string) (*Session, error)
func (s *SessionManager) Refresh(id string) error  // sliding window
func (s *SessionManager) Delete(id string) error
func (s *SessionManager) DeleteAllForUser(userID string) error  // nach Disable / PW-Reset
```

**Speicherung:** NATS-JetStream-KV-Bucket `auth-sessions` mit TTL = 12 h. `Refresh` schreibt den gleichen Eintrag mit erneuertem TTL — JetStream-KV unterstützt MaxAge pro Bucket; pro Eintrag triggert jeder Write den TTL-Reset (siehe `internal/nats/context_store.go` für das KV-Muster, das wir schon nutzen).

**Cookie-Signing:** Session-IDs werden HMAC-SHA256 signiert; Schlüssel aus `data/flint.session.key`. Neue Datei `internal/auth/sessionkey.go` mit `EnsureSessionKey(path string) ([]byte, error)` — analog zu `credentials.EnsureKeyFile`.

**Wiring im `Broker`:** Neue Methode `(*Broker).SetupSessionKV(ctx) (jetstream.KeyValue, error)` in `internal/nats/broker.go`. In `server.Start()` aufrufen, das Ergebnis an den `SessionManager` geben.

**Smoke-Test:** Server startet, Bucket wird angelegt, `nats kv ls` zeigt `auth-sessions`.

---

## Schritt 4 — Auth-Middleware

**Ziel:** Ein Chi-Middleware-Stück, das Requests gegen Sessions+Rollen prüft.

**Neue Datei `internal/auth/middleware.go`:**

```go
type ctxKey int
const userCtxKey ctxKey = 0

func WithUser(r *http.Request) (*User, bool)  // helper für Handler

func RequireRole(min Role) func(http.Handler) http.Handler  // 401 wenn keine Session, 403 wenn Rolle zu niedrig
func RequireSetupComplete(...) func(http.Handler) http.Handler  // 503 im Setup-Modus
```

**Rollen-Vergleich:** `Role` ist eine `string`, aber intern wird `RoleRank(r) int` (`viewer=1, editor=2, admin=3`) für den `>=`-Vergleich verwendet. Bewusst flach trotz der „nicht hierarchisch"-Aussage im Issue: Implementierungs-seitig ist *Rang-basiert* einfacher, das Mapping bleibt einfach (`admin >= editor >= viewer`). Das Issue meinte: keine getrennten Permissions, keine ACL-Listen — und das gilt weiterhin.

**Smoke-Test:** Unit-Tests für jede Rang-Kombination + Setup-Modus.

---

## Schritt 5 — HTTP-Routen + Schutz bestehender Routen

**Neue Datei `internal/api/auth_handlers.go`:**

| Route | Handler | Body | Antwort |
|---|---|---|---|
| `POST /api/v1/setup` | `handleSetup` | `{ username, password }` | `201 { user }` oder `409` |
| `POST /api/v1/auth/login` | `handleLogin` | `{ username, password }` | `200 { user }` + Cookie oder `401` |
| `POST /api/v1/auth/logout` | `handleLogout` | – | `204` |
| `GET /api/v1/auth/me` | `handleMe` | – | `200 { user }` oder `401` |

**Neue Datei `internal/api/user_handlers.go`:**

| Route | Mindestrolle | Body / Antwort |
|---|---|---|
| `GET /api/v1/users` | admin | `200 { users: [...] }` |
| `POST /api/v1/users` | admin | `{ username, password, role }` → `201 { user }` |
| `PATCH /api/v1/users/{id}` | admin | `{ role?, disabled? }` |
| `POST /api/v1/users/{id}/password` | admin | `{ password }` |

`/users/{id}/password` invalidiert Sessions des Ziel-Users (`SessionManager.DeleteAllForUser`).

**Erweiterung `Deps`:**
```go
type Deps struct {
    // ... bestehend ...
    Users    auth.UserStore
    Sessions *auth.SessionManager
    Setup    *auth.SetupGuard  // ein-Schuss-Lock für Setup
}
```

**`internal/api/routes.go`** — Routen-Schutz mit der Middleware:

```go
r.Group(func(r chi.Router) {
    r.Use(deps.RequireSetupComplete())  // alles außer /setup, /auth/*

    r.Group(func(r chi.Router) {
        r.Use(deps.RequireRole(auth.RoleViewer))
        r.Get("/flows", deps.handleGetFlows)
        r.Get("/nodes", deps.handleGetNodes)
        // … alle GETs
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

// offene Routen (außerhalb der Setup-Gate):
r.Post("/setup", deps.handleSetup)
r.Post("/auth/login", deps.handleLogin)
r.Post("/auth/logout", deps.handleLogout)
r.Get("/auth/me", deps.handleMe)
```

**Login-Cookie:**
- Name: `flint_session`
- HttpOnly, Secure (per Default; `auth.requireSecureCookies = false` per Env zum Abschalten in Dev), SameSite=Lax
- Path `/`
- MaxAge = 12 h (ohne `Expires` → Browser hält bei Window-Close, sliding-Window passiert serverseitig)

**Brute-Force-Schutz:** `internal/auth/throttle.go` — In-Memory-Map `map[username]struct{ failures int; until time.Time }`, Mutex-geschützt. 5 Fehler in 15 min → Lock auf weitere 15 min. Map wird beim Restart geleert (akzeptiert; Brute-Force über Restarts hinweg ist kein realistisches Bedrohungsmodell für eine On-Prem-Instanz).

**Smoke-Test:**
- `curl /api/v1/flows` ohne Setup → 503
- `curl -X POST /api/v1/setup -d '{"username":"admin","password":"hunter22"}'` → 201
- `curl -X POST /api/v1/auth/login ...` → 200 mit Set-Cookie
- `curl --cookie ... /api/v1/flows` → 200
- 2. Setup-Aufruf → 409

---

## Schritt 6 — WebSocket-Schutz

**Anpassung `internal/ws/hub.go`:**

`ServeWS` bekommt eine optionale Auth-Funktion injiziert (Server-Setup):

```go
type AuthFunc func(*http.Request) (*auth.User, error)

func (h *Hub) ServeWSAuthed(authFn AuthFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, err := authFn(r)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        // bestehender Upgrade-Code, plus user in Client speichern
    }
}
```

Der `Client` bekommt ein Feld `userID string` (für späteres Audit / Per-User-Filter — V1 nur logging).

**Session-Ablauf-Termination:** Der `SessionManager` veröffentlicht beim `Delete`/`DeleteAllForUser` ein NATS-Event `auth.session.deleted` mit `{ sessionID, userID }`. Der Hub abonniert das und schließt alle Clients mit passender `userID`. So funktioniert Logout sofort über alle offenen Tabs hinweg.

**Smoke-Test:** WS-Verbindung ohne Cookie → 401. WS-Verbindung mit gültigem Cookie → connect. Logout → WS schließt sich serverseitig.

---

## Schritt 7 — Frontend Auth-Store

**Neue Datei `frontend/src/stores/authStore.ts`** (Pinia, analog zu den bestehenden Stores):

```ts
state: () => ({
  user: null as User | null,
  loading: true,
  needsSetup: false,
})
actions:
  init()       // GET /auth/me → user; bei 503 → needsSetup=true; bei 401 → user=null
  setup(...)   // POST /setup → init()
  login(...)   // POST /auth/login → init()
  logout()     // POST /auth/logout → user=null
getters:
  isAuthenticated, role, can(action) // 'deploy'|'inject'|'manageUsers'|...
```

**Erweiterung `useApi.ts`:** `request<T>` interpretiert `401` zentral — setzt `authStore.user = null`, damit Login-Modal automatisch erscheint.

**App.vue:** ruft beim Mounten `authStore.init()` auf und blockiert das Rendern des Editor-Inhalts, bis `authStore.loading === false`. Je nach State:
- `needsSetup` → `<SetupModal>`
- `!user` → `<LoginModal>`
- sonst → bestehender Editor

**Smoke-Test:** Frontend-Build läuft, `/auth/me` wird beim Page-Load gerufen.

---

## Schritt 8 — Setup-Modal

**Neue Datei `frontend/src/components/auth/SetupModal.vue`:**
- Username, Passwort, Passwort-Wiederholung, Submit-Button
- Mindestlänge 8, Live-Validierung
- Bei Erfolg: automatisch `authStore.login(...)` mit denselben Daten (vom Setup-Endpoint zurückkommendes `user`-Objekt reicht; wir lösen den Login direkt nach Setup im Backend mit aus → spart einen Roundtrip; setzt das Cookie sofort).

**Backend-Anpassung:** `handleSetup` setzt direkt nach Anlage des Admin auch ein Session-Cookie. Begründung: ein direkt anschließender separater Login-Call kann durch eine Race mit dem Setup-Lock fehlschlagen, und der UX-Vorteil ist klar.

**Smoke-Test:** Auf leerer Datenbasis startet das UI mit dem Modal, nach Submit landet der User im Editor.

---

## Schritt 9 — Login-Modal

**Neue Datei `frontend/src/components/auth/LoginModal.vue`:**
- Username, Passwort
- Fehleranzeige für `401` und `429` (Rate-Limit)
- Optional ausgeblendete Sektion „Login mit ..." als Vorbereitung für SSO (V1: leer, kein Code)

**Smoke-Test:** Falsches Passwort → Fehlermeldung. Korrektes → Editor lädt.

---

## Schritt 10 — User-Verwaltung-View

**Neue Datei `frontend/src/views/UsersView.vue`** + Route `/users` in `frontend/src/router/index.ts` (mit Route-Guard: nur sichtbar wenn `authStore.role === 'admin'`).

**Komponenten:**
- Tabelle: Username, Rolle (Dropdown zum Ändern), Status (Toggle), Auth-Provider, Aktionen (Passwort setzen)
- „Neuer User"-Dialog: Username, Passwort, Rolle (`editor` / `viewer`; `admin` nur per separatem Klick mit Bestätigung)
- „Passwort setzen"-Dialog pro Zeile
- „Deaktivieren"-Toggle pro Zeile

**Header-Eintrag:** In `HeaderBar.vue` einen Link „Users" einblenden, wenn `can('manageUsers')`.

**Smoke-Test:** Als Admin: User anlegen, deaktivieren, Passwort setzen, Rolle ändern. Als letzter Admin → Versuch sich selbst zu deaktivieren wird mit `409` abgelehnt und im UI als Fehler gezeigt.

---

## Schritt 11 — Rollen-Gating in vorhandenen Komponenten

**Anpassungen** (Liste, jeweils mit `v-if="authStore.can('...')"`):
- `HeaderBar.vue` — Deploy-Button: `can('deploy')`
- `NodePalette.vue` — Drag-out und Edit-Modus: `can('deploy')` (Viewer kann Flows ansehen, aber nicht modifizieren)
- `FlowProperties.vue` — Editieren der Felder: `can('deploy')`
- `PropertyPanel.vue` — Inputs `disabled` wenn nicht `can('deploy')`
- `ContextPanel.vue` — Delete-Buttons: `can('deploy')` (Editor löscht; Viewer sieht read-only)
- Inject-Node-Trigger im Canvas: `can('inject')`

**Wichtig:** UI-Gating ist Zucker. Die Backend-Routen sind die Sicherheitslinie.

**Smoke-Test:** Mit Viewer-User einloggen — keine Deploy-/Inject-/Delete-Buttons sichtbar.

---

## Schritt 12 — Tests, Doku, Dev-Bypass

**Backend-Tests** (in den jeweiligen Packages):
- `internal/auth/password_test.go` — Roundtrip, Tampering
- `internal/auth/store_test.go` — CRUD, last-admin-Schutz, Username-Eindeutigkeit
- `internal/auth/middleware_test.go` — alle Rang-Kombinationen × Setup-Modus
- `internal/api/auth_handlers_test.go` — Setup-Idempotenz, Login-Cookie, Logout, Throttle
- `internal/api/user_handlers_test.go` — Rollen-Enforcement-Matrix (Tabellen-Tests)

**Dev-Bypass:**
- Env-Variable `FLINT_DISABLE_AUTH=1` → Middleware lässt alles durch und injiziert einen virtuellen `dev-admin`-User in den Context. Bei Server-Start: dicker `slog.Warn` mit Banner.
- Implementiert in der Middleware als erste Prüfung, vor Cookie-Lookup.

**Doku-Update:**
- `PLANNING.md` — Auth-Block ergänzen, Status `🚧 In Arbeit` / dann `✅ Fertig`
- `README.md` — Setup-Hinweis: „Nach erstem Start: Browser öffnen → Admin anlegen"
- *Kein* eigenes Auth-Doku-File. Das Issue + Plan in `docs/issues/` reichen, Code ist selbst-dokumentierend.

**Smoke-Test:** Vollständiger Durchlauf:
1. `rm -rf data/`
2. Server starten
3. Browser → Setup-Modal → Admin anlegen
4. Editor lädt mit Admin-Berechtigungen
5. Users-View → Editor- und Viewer-User anlegen
6. Logout → Login als Editor → keine Users-View
7. Logout → Login als Viewer → keine Deploy-/Inject-Buttons

---

## Migrations-Hinweise

Bestehende Installationen (mit `workspace.json`, aber ohne `users.json`) landen beim ersten Start mit dieser Version automatisch im **Setup-Modus** — der vorhandene Workspace wird dabei *nicht* gelöscht; er wird einfach erst freigegeben, sobald der Admin angelegt ist und sich eingeloggt hat. Backups-Hinweis im README ergänzen.

## Was bewusst weggelassen wurde

- **Custom Roles, Permissions** → nur drei Rollen, fertig.
- **Email-basiertes Passwort-Reset** → Admin setzt out-of-band, Self-Service ist V2.
- **2FA / TOTP** → V2 oder mit SSO.
- **Audit-Log** → eigenes Issue, separat.
- **CSRF-Token** → wir setzen `SameSite=Lax`. Für lokale Editor-Nutzung (gleicher Origin, kein Embedding) reicht das. Wenn Flint später per `<iframe>` eingebettet werden soll, kommt CSRF separat.
- **JWT** → siehe Issue, bewusst nicht.

## Geschätzte Größe

Pi mal Daumen, ohne Garantien:
- Backend: ~800 LOC inkl. Tests
- Frontend: ~600 LOC

Nach Bestätigung des Plans → Schritt 1 starten.
