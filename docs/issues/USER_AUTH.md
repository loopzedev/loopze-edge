# Issue: Benutzer-Authentifizierung – First-Run Admin-Setup, Rollen, SSO-Vorbereitung

## Status: Konzept

## Problembeschreibung

LOOPZE hat aktuell **keinerlei Zugriffsschutz** — wer den Editor im Browser öffnet, kann Flows deployen, Connector-Konfigurationen ändern und Context-Daten löschen. Für jeden Einsatz außerhalb eines abgeschotteten Lab-Netzes ist das nicht tragbar.

Dieses Issue entwirft das Konzept für eine **Benutzer-Authentifizierung** mit den folgenden Eckpfeilern:

1. **First-Run-Setup**: Beim erstmaligen Start fragt LOOPZE per Popup nach den Daten für einen initialen Admin-Account. Vorher ist das UI gesperrt.
2. **Lokale Benutzerverwaltung**: Der Admin kann weitere Benutzer mit den Rollen **Editor** oder **Viewer** anlegen.
3. **SSO-Erweiterbarkeit**: Spätere Einbindung von OAuth2 / Azure AD / OIDC ohne Bruch des bestehenden Modells.

Das Issue beschreibt das **Konzept**, nicht den Code. Ziel ist Alignment, bevor der Implementierungsplan geschrieben wird.

## Abgrenzung

**Nicht Teil dieses Issues:**

- **Connector-Passwörter** (MQTT-Broker, später HTTP/DB-Connectors). Die werden bereits über `internal/credentials/credentials.go` (AES-256-GCM, Master-Key in `data/loopze.key`) verwaltet — das ist ein separates Thema mit eigener Krypto und eigenem Lifecycle. **Berührungspunkt nur**: Beide Mechanismen brauchen Server-seitige Geheimnisse auf der Platte (siehe „Offene Fragen"). Tracker für Connector-Credentials → eigenes Issue.
- **Audit-Log** (wer hat wann was geändert). Sinnvolle Erweiterung, aber separat.
- **Multi-Tenancy** / Mandanten-Trennung. LOOPZE bleibt vorerst Single-Tenant.

## Anforderungen

### 1. First-Run Setup

Beim Start prüft das Backend, ob ein User-Store existiert und mindestens **einen Admin** enthält.

- **Kein Admin vorhanden** → das Backend befindet sich im **Setup-Modus**. Alle API-Routen außer `/api/v1/setup` und der Frontend-Auslieferung antworten mit `503 Service Unavailable` (oder einem dezidierten Setup-Required-Status).
- **Setup-Endpoint**: `POST /api/v1/setup` mit `{ username, password }` — legt den ersten Admin-User an. Funktioniert **nur einmal** (idempotent: zweiter Aufruf liefert `409 Conflict`).
- **Frontend**: zeigt ein modales Setup-Popup (nicht abbruchbar), solange das Backend Setup-Modus meldet. Nach erfolgreichem Setup → automatischer Login mit den frisch eingegebenen Daten.

**Begründung:** Ein im Code hartcodierter Default-Admin (`admin/admin`) wäre unsicher und würde in der Praxis nie geändert. Erzwungenes First-Run-Setup ist die einzig saubere Lösung.

### 2. Benutzer-Modell

Drei Rollen, **flach** (nicht hierarchisch — Admin ist *nicht* automatisch Editor + Viewer, sondern eine eigene Rolle, die alle Rechte umfasst):

| Rolle | Flows lesen | Flows deployen | Inject auslösen | Context anzeigen | Context löschen | User verwalten |
|---|---|---|---|---|---|---|
| **Admin** | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Editor** | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ |
| **Viewer** | ✓ | ✗ | ✗ | ✓ | ✗ | ✗ |

**Bewusste Entscheidung:** Nur drei Rollen. Keine Permission-Matrix, keine Custom-Roles, keine Per-Flow-ACLs. Das ist die einfache Lösung, die die Bediener nicht überfordert.

**User-Felder:**
- `id` (string, UUID)
- `username` (string, eindeutig, case-insensitive)
- `passwordHash` (string, **argon2id**) — leer bei SSO-Usern (siehe unten)
- `role` (`admin` | `editor` | `viewer`)
- `createdAt`, `updatedAt` (RFC3339)
- `disabled` (bool) — ein User wird **deaktiviert**, nicht gelöscht (Audit-freundlich; spätere Löschung möglich, aber Soft-Disable ist Default)
- `authProvider` (string) — `local` oder später `oauth2:google`, `azure-ad` etc. Trennt lokale von SSO-Usern.

**Constraints:**
- Mindestens **ein** aktiver Admin muss immer existieren. API verweigert das Deaktivieren / Rollendowngrade des letzten Admins.
- Username eindeutig pro `authProvider` — derselbe `alice` kann lokal und SSO existieren (wird aber als zwei verschiedene User behandelt; **kein** automatisches Account-Linking).

### 3. Login & Session

- **Login-Endpoint**: `POST /api/v1/auth/login` mit `{ username, password }` → setzt ein **HttpOnly, Secure, SameSite=Lax Session-Cookie** mit einer signierten Session-ID. Antwort enthält `{ user: { id, username, role } }`.
- **Logout-Endpoint**: `POST /api/v1/auth/logout` → invalidiert die Session.
- **Me-Endpoint**: `GET /api/v1/auth/me` → liefert den aktuell eingeloggten User oder `401`.
- **Session-Speicherung**: Server-seitig in einem NATS-KV-Bucket `auth-sessions` (TTL: 12 h sliding). Vorteil: Konsistent mit der bestehenden Persistenz, kein zusätzlicher State.
- **Session-Signing-Key**: 256-bit Random Key, beim ersten Start in `data/loopze.session.key` generiert (analog zu `loopze.key`). Rotation = manuelles Löschen der Datei, alle Sessions ungültig.
- **Brute-Force-Schutz**: Pro Username max. 5 Fehlversuche / 15 min, danach Sperre auf weitere 15 min. Einfache In-Memory-Map reicht — keine externe Rate-Limit-Lösung.

**Bewusste Entscheidung — Cookie statt JWT:** Server-seitige Sessions sind einfacher (Logout funktioniert sofort, Token-Revocation kein Problem) und LOOPZE hat ohnehin eine zentrale Instanz. JWT wäre Over-Engineering für ein Single-Node-System.

### 4. WebSocket-Authentifizierung

Der `/ws`-Endpunkt ist heute offen. Mit Auth:

- WS-Upgrade akzeptiert nur Requests mit gültigem Session-Cookie (Browser sendet das Cookie automatisch beim WS-Handshake).
- Beim Session-Ablauf wird die Verbindung serverseitig geschlossen; Frontend zeigt Login-Modal.

### 5. Frontend

Drei UI-Bereiche:

1. **Setup-Modal** (First-Run, blockierend, nicht schließbar) — Username, Passwort, Passwort-Wiederholung.
2. **Login-Modal** (wenn nicht angemeldet) — Username, Passwort, Fehleranzeige bei `401`.
3. **User-Verwaltung** (nur Admins sichtbar) — neuer Sidebar-/Header-Eintrag „Users":
   - Liste aller User: Username, Rolle, Status (aktiv/deaktiviert), Auth-Provider
   - „User anlegen" — Username, Passwort, Rolle (Editor / Viewer; Admin nur durch anderen Admin)
   - „Passwort zurücksetzen" — setzt ein neues Passwort (kein E-Mail-Reset, kein Reset-Token-Flow — der Admin tippt das neue Passwort ein, der User kriegt es out-of-band)
   - „Deaktivieren" / „Aktivieren"
   - **Kein** „Löschen" in V1 (siehe Soft-Disable oben).

**Rollen-Gating im UI:**
- Viewer sieht den Deploy-Button nicht (statt ihn zu zeigen und beim Klick `403` zu kassieren).
- Viewer sieht den Inject-Button nicht.
- Viewer sieht die Context-„Delete"-Buttons nicht.

Das UI-Gating ist **kein Sicherheits-Feature** — der Server muss alle Aktionen unabhängig vom UI absichern. Es ist reine UX, damit Viewer nicht auf Buttons starren, die immer fehlschlagen.

### 6. Backend-Routen-Schutz

Middleware in `internal/api/routes.go`, die jede Route mit der nötigen Mindestrolle annotiert:

| Route | Mindestrolle |
|---|---|
| `GET /api/v1/flows`, `/nodes`, `/configs/types`, `/settings`, `/status/...`, `/debug/messages`, `/logs`, `/context/...` (GET) | viewer |
| `POST /api/v1/flows` (deploy) | editor |
| `POST /api/v1/inject/{id}` | editor |
| `DELETE /api/v1/context/...` | editor |
| `/api/v1/users/*` | admin |
| `/api/v1/auth/login`, `/auth/logout`, `/auth/me`, `/setup` | (offen) |

### 7. Persistenz

Neue Datei: `data/users.json` (analog zu `workspace.json`). Format:

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

Atomic-Write (temp-Datei + rename) wie bei `workspace.json`.

**Warum JSON-Datei und nicht NATS-KV?** Konsistent mit dem bestehenden Storage-Pattern (Flows, Credentials sind auch File-Based). User-Daten ändern sich selten, ein paar hundert Einträge sind kein Problem für JSON.

### 8. SSO-Vorbereitung (V2, nicht in V1 implementiert)

V1 implementiert **nur** lokale User. Aber das Modell muss SSO **architektonisch nicht ausschließen**. Konkret:

- `authProvider` als Feld am User existiert von Anfang an.
- Login-Endpoint trennt klar zwischen Credential-Verifikation und Session-Erzeugung. Eine künftige `/auth/oauth2/callback`-Route würde dieselbe Session-Logik nutzen, nur mit einer anderen Credential-Verifikation davor.
- Das Frontend-Login-Modal hat einen Platzhalter „Login mit ..." (in V1 ausgeblendet, in V2 erscheinen dort die konfigurierten Provider).

**Was V1 explizit nicht enthält** (um nicht zu spekulieren):
- Provider-Konfigurations-UI
- OAuth2 / OIDC Library-Integration
- Account-Linking lokal ↔ SSO
- Group-Mapping „Azure-AD-Group X → LOOPZE-Rolle Editor"

Das wird in einem **separaten V2-Issue** entworfen, sobald V1 läuft und ein konkreter SSO-Bedarf da ist.

## Acceptance Criteria

**V1 (Lokale Auth):**

- [ ] Beim ersten Start ohne `data/users.json` zeigt das Frontend ein nicht abbrechbares Setup-Modal; alle API-Routen außer Setup sind gesperrt.
- [ ] Nach Setup ist der erste User automatisch eingeloggt und sieht den Editor.
- [ ] Logout funktioniert; danach erscheint das Login-Modal.
- [ ] Login mit falschem Passwort schlägt fehl, mit korrektem gelingt.
- [ ] 5 Fehlversuche / 15 min sperren den Account temporär.
- [ ] Admin kann unter „Users" weitere Editor- und Viewer-User anlegen.
- [ ] Editor kann deployen und injecten, aber keine User verwalten (`/api/v1/users/*` → 403).
- [ ] Viewer sieht Flows und Context, aber Deploy- / Inject- / Delete-Buttons sind ausgeblendet; Server liefert für diese Aktionen `403`.
- [ ] Letzter aktiver Admin kann nicht deaktiviert / heruntergestuft werden (`409 Conflict`).
- [ ] WebSocket-Verbindungen ohne gültige Session werden abgelehnt; bei Session-Ablauf wird die offene Verbindung serverseitig geschlossen.
- [ ] Sessions überleben einen Server-Restart (KV-persistiert).

## Offene Fragen

1. **Master-Key-Beziehung**: Soll der Connector-Credentials-Master-Key (`loopze.key`) optional an den eingeloggten Admin gekoppelt werden (z. B. „Connector-Passwörter sind nur entschlüsselbar, wenn ein Admin angemeldet ist")? — **Vorschlag: Nein.** Der Server muss Flows auch ohne angemeldeten User ausführen (Boot-Time Deploy). Master-Key bleibt ein reines Server-Geheimnis. User-Auth schützt nur den UI-/API-Zugang.
2. **Passwort-Policy**: Mindestlänge / Komplexität? — **Vorschlag**: Nur Mindestlänge 8 Zeichen, keine Komplexitätsregeln (NIST 800-63B-konform).
3. **Session-Dauer**: 12 h sliding ist ein Start. Konfigurierbar? — **Vorschlag**: Vorerst hartcodiert, später Setting.
4. **HTTPS-Erzwingung**: Cookie ist `Secure` — funktioniert dann nicht über HTTP. Akzeptabel oder brauchen wir einen „Insecure-Dev-Modus"? — **Vorschlag**: Setting `auth.requireSecureCookies` (Default: an, abschaltbar nur per Flag/Env).
5. **Default-Login bei laufendem Dev-Server**: Soll es einen Dev-Modus geben, der die Auth komplett aushebelt (`LOOPZE_DISABLE_AUTH=1`)? — **Vorschlag**: Ja, aber mit dickem Warn-Log bei Start. Nützlich für lokale Entwicklung und Tests.

## Implementierungsplan (Skizze, vor Detailplan)

1. **Backend-Skeleton**: `internal/auth/` Package mit User-Store, Argon2-Hashing, Session-KV.
2. **API-Routen**: `/setup`, `/auth/login`, `/auth/logout`, `/auth/me`, `/users/*`. Middleware `RequireRole(...)` an die bestehenden Routen hängen.
3. **Frontend**: Auth-Pinia-Store, Setup-Modal, Login-Modal, „Users"-View, Rollen-Gating in vorhandenen Komponenten.
4. **WebSocket-Schutz**: Cookie-Check beim Upgrade, Verbindungs-Termination bei Session-Ablauf.
5. **Tests**: Setup-Flow, Login-Flow, Rollen-Enforcement (für jede geschützte Route ein 200/403-Test).

Nach Bestätigung dieses Konzepts → Detail-Plan in `docs/issues/USER_AUTH_PLAN.md`.
