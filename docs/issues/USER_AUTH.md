# Issue: User Authentication – First-Run Admin Setup, Roles, SSO Preparation

## Status: Concept

## Problem Description

LOOPZE currently has **no access protection whatsoever** — anyone who opens the editor in the browser can deploy flows, change connector configurations, and delete context data. For any deployment outside of an isolated lab network this is untenable.

This issue drafts the concept for **user authentication** with the following pillars:

1. **First-run setup**: On the first start, LOOPZE asks via popup for the credentials of an initial admin account. The UI is locked beforehand.
2. **Local user management**: The admin can create additional users with the **Editor** or **Viewer** role.
3. **SSO extensibility**: Later integration of OAuth2 / Azure AD / OIDC without breaking the existing model.

The issue describes the **concept**, not the code. Goal is alignment before the implementation plan is written.

## Out of Scope

**Not part of this issue:**

- **Connector passwords** (MQTT broker, later HTTP/DB connectors). These are already managed via `internal/credentials/credentials.go` (AES-256-GCM, master key in `data/loopze.key`) — that is a separate topic with its own crypto and own lifecycle. **Touchpoint only**: Both mechanisms need server-side secrets on disk (see "Open Questions"). Tracker for connector credentials → separate issue.
- **Audit log** (who changed what when). Useful extension, but separate.
- **Multi-tenancy** / tenant separation. LOOPZE remains single-tenant for now.

## Requirements

### 1. First-Run Setup

On start the backend checks whether a user store exists and contains at least **one admin**.

- **No admin present** → the backend is in **setup mode**. All API routes except `/api/v1/setup` and frontend serving respond with `503 Service Unavailable` (or a dedicated setup-required status).
- **Setup endpoint**: `POST /api/v1/setup` with `{ username, password }` — creates the first admin user. Works **only once** (idempotent: second call returns `409 Conflict`).
- **Frontend**: shows a modal setup popup (non-cancelable) as long as the backend reports setup mode. After successful setup → automatic login with the freshly entered credentials.

**Rationale:** A hardcoded default admin (`admin/admin`) would be insecure and would in practice never be changed. Enforced first-run setup is the only clean solution.

### 2. User Model

Three roles, **flat** (not hierarchical — admin is *not* automatically editor + viewer, but a separate role that encompasses all rights):

| Role | Read flows | Deploy flows | Trigger inject | View context | Delete context | Manage users |
|---|---|---|---|---|---|---|
| **Admin** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Editor** | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ |
| **Viewer** | ✓ | ✗ | ✗ | ✓ | ✗ | ✗ |

**Deliberate decision:** Only three roles. No permission matrix, no custom roles, no per-flow ACLs. This is the simple solution that does not overwhelm operators.

**User fields:**
- `id` (string, UUID)
- `username` (string, unique, case-insensitive)
- `passwordHash` (string, **argon2id**) — empty for SSO users (see below)
- `role` (`admin` | `editor` | `viewer`)
- `createdAt`, `updatedAt` (RFC3339)
- `disabled` (bool) — a user is **disabled**, not deleted (audit-friendly; later deletion possible, but soft-disable is the default)
- `authProvider` (string) — `local` or later `oauth2:google`, `azure-ad`, etc. Separates local from SSO users.

**Constraints:**
- At least **one** active admin must always exist. API refuses to disable / role-downgrade the last admin.
- Username unique per `authProvider` — the same `alice` may exist locally and via SSO (but treated as two different users; **no** automatic account linking).

### 3. Login & Session

- **Login endpoint**: `POST /api/v1/auth/login` with `{ username, password }` → sets a **HttpOnly, Secure, SameSite=Lax session cookie** with a signed session ID. Response contains `{ user: { id, username, role } }`.
- **Logout endpoint**: `POST /api/v1/auth/logout` → invalidates the session.
- **Me endpoint**: `GET /api/v1/auth/me` → returns the currently logged-in user or `401`.
- **Session storage**: Server-side in a NATS-KV bucket `auth-sessions` (TTL: 12 h sliding). Advantage: consistent with existing persistence, no extra state.
- **Session signing key**: 256-bit random key, generated on first start in `data/loopze.session.key` (analogous to `loopze.key`). Rotation = manually deleting the file, all sessions invalid.
- **Brute-force protection**: Per username max 5 failed attempts / 15 min, then locked for another 15 min. A simple in-memory map suffices — no external rate-limit solution.

**Deliberate decision — cookie instead of JWT:** Server-side sessions are simpler (logout works immediately, token revocation is no problem) and LOOPZE has a central instance anyway. JWT would be over-engineering for a single-node system.

### 4. WebSocket Authentication

The `/ws` endpoint is open today. With auth:

- WS upgrade only accepts requests with a valid session cookie (browser sends the cookie automatically on the WS handshake).
- On session expiration the connection is closed server-side; frontend shows the login modal.

### 5. Frontend

Three UI areas:

1. **Setup modal** (first-run, blocking, not closable) — username, password, password confirmation.
2. **Login modal** (when not logged in) — username, password, error display on `401`.
3. **User management** (only visible to admins) — new sidebar/header entry "Users":
   - List of all users: username, role, status (active/disabled), auth provider
   - "Create user" — username, password, role (editor / viewer; admin only by another admin)
   - "Reset password" — sets a new password (no email reset, no reset-token flow — the admin types the new password, the user gets it out-of-band)
   - "Disable" / "Enable"
   - **No** "Delete" in V1 (see soft-disable above).

**Role gating in the UI:**
- Viewer does not see the Deploy button (instead of showing it and getting `403` on click).
- Viewer does not see the Inject button.
- Viewer does not see the context "Delete" buttons.

UI gating is **not a security feature** — the server must secure all actions independently of the UI. It is pure UX so viewers don't stare at buttons that always fail.

### 6. Backend Route Protection

Middleware in `internal/api/routes.go` that annotates each route with the required minimum role:

| Route | Minimum role |
|---|---|
| `GET /api/v1/flows`, `/nodes`, `/configs/types`, `/settings`, `/status/...`, `/debug/messages`, `/logs`, `/context/...` (GET) | viewer |
| `POST /api/v1/flows` (deploy) | editor |
| `POST /api/v1/inject/{id}` | editor |
| `DELETE /api/v1/context/...` | editor |
| `/api/v1/users/*` | admin |
| `/api/v1/auth/login`, `/auth/logout`, `/auth/me`, `/setup` | (open) |

### 7. Persistence

New file: `data/users.json` (analogous to `workspace.json`). Format:

```json
{
  "users": [
    {
      "id": "01HX...",
      "username": "alice",
      "passwordHash": "$argon2id$...",
      "role": "admin",
      "authProvider": "local",
      "disabled": false,
      "createdAt": "2026-04-27T10:00:00Z",
      "updatedAt": "2026-04-27T10:00:00Z"
    }
  ]
}
```

Atomic write (temp file + rename) as with `workspace.json`.

**Why a JSON file and not NATS-KV?** Consistent with the existing storage pattern (flows, credentials are also file-based). User data changes rarely, a few hundred entries are no problem for JSON.

### 8. SSO Preparation (V2, not implemented in V1)

V1 implements **only** local users. But the model must **architecturally not exclude** SSO. Concretely:

- `authProvider` as a field on the user exists from the start.
- Login endpoint clearly separates credential verification from session creation. A future `/auth/oauth2/callback` route would use the same session logic, just with a different credential verification in front.
- The frontend login modal has a placeholder "Login with ..." (hidden in V1, in V2 the configured providers appear there).

**What V1 explicitly does not contain** (to avoid speculation):
- Provider configuration UI
- OAuth2 / OIDC library integration
- Account linking local ↔ SSO
- Group mapping "Azure AD group X → LOOPZE editor role"

This is drafted in a **separate V2 issue** as soon as V1 is running and a concrete SSO need exists.

## Acceptance Criteria

**V1 (Local Auth):**

- [ ] On first start without `data/users.json` the frontend shows a non-cancelable setup modal; all API routes except setup are gated.
- [ ] After setup the first user is automatically logged in and sees the editor.
- [ ] Logout works; afterwards the login modal appears.
- [ ] Login with wrong password fails, with correct succeeds.
- [ ] 5 failed attempts / 15 min temporarily lock the account.
- [ ] Admin can create additional editor and viewer users under "Users".
- [ ] Editor can deploy and inject but not manage users (`/api/v1/users/*` → 403).
- [ ] Viewer sees flows and context, but Deploy / Inject / Delete buttons are hidden; server returns `403` for these actions.
- [ ] The last active admin cannot be disabled / downgraded (`409 Conflict`).
- [ ] WebSocket connections without a valid session are rejected; on session expiration the open connection is closed server-side.
- [ ] Sessions survive a server restart (KV-persisted).

## Open Questions

1. **Master-key relation**: Should the connector credentials master key (`loopze.key`) optionally be coupled to the logged-in admin (e.g., "connector passwords are only decryptable when an admin is logged in")? — **Proposal: No.** The server must execute flows even without a logged-in user (boot-time deploy). The master key remains a pure server secret. User auth only protects UI/API access.
2. **Password policy**: Minimum length / complexity? — **Proposal**: Only minimum length 8 characters, no complexity rules (NIST 800-63B compliant).
3. **Session duration**: 12 h sliding is a start. Configurable? — **Proposal**: Hardcoded for now, later a setting.
4. **HTTPS enforcement**: Cookie is `Secure` — then does not work over HTTP. Acceptable or do we need an "insecure dev mode"? — **Proposal**: Setting `auth.requireSecureCookies` (default: on, only disable via flag/env).
5. **Default login on running dev server**: Should there be a dev mode that completely disables auth (`LOOPZE_DISABLE_AUTH=1`)? — **Proposal**: Yes, but with a bold warn log on start. Useful for local development and tests.

## Implementation Plan (Sketch, before Detail Plan)

1. **Backend skeleton**: `internal/auth/` package with user store, Argon2 hashing, session KV.
2. **API routes**: `/setup`, `/auth/login`, `/auth/logout`, `/auth/me`, `/users/*`. Hang middleware `RequireRole(...)` on the existing routes.
3. **Frontend**: auth Pinia store, setup modal, login modal, "Users" view, role gating in existing components.
4. **WebSocket protection**: cookie check on upgrade, connection termination on session expiration.
5. **Tests**: setup flow, login flow, role enforcement (for every protected route a 200/403 test).

After confirmation of this concept → detail plan in `docs/issues/USER_AUTH_PLAN.md`.
