# Flint – Implementierungsplan

> Dieser Plan beschreibt die schrittweise Umsetzung vom aktuellen Stand zum lauffähigen MVP.
> Jede Phase baut auf der vorherigen auf. Innerhalb einer Phase sind die Schritte sequenziell.

---

## Status Quo

| Schicht | Status |
|---|---|
| **Infrastruktur** (Server, Config, Middleware, WebSocket Hub) | ✅ Fertig |
| **Storage** (JSON-Files, Credentials AES-256-GCM, Atomic Writes) | ✅ Fertig |
| **Embedded NATS** (Server + JetStream + In-Process Client) | ✅ Fertig |
| **Frontend** (Vue 3 + Vue Flow, Stores, Palette, Debug Panel) | ✅ Fertig – Backend angebunden, Deploy + Debug funktionieren |
| **Flow Engine** (Types, Registry, Deploy, Message-Routing) | ✅ Fertig – Full Lifecycle, Goroutine-per-Node |
| **API Handler** | ✅ Fertig – Flows, Nodes, Inject, Debug |
| **Node-Typen** | 🟡 2 implementiert (Inject, Debug) |

---

## Phase 1 – Foundation: Engine zum Laufen bringen

> **Ziel:** Ein Inject-Node sendet eine Message an einen Debug-Node → Message erscheint im Debug-Panel.

### 1.1 NATS Bootstrapping

Der embedded NATS-Broker läuft bereits. Jetzt die Strukturen anlegen, die zur Laufzeit gebraucht werden.

- [x] **Debug-Stream anlegen** – JetStream Stream `DEBUG` mit Subject `debug.>`, MaxMsgs 1000, Memory Storage
- [x] **Context-KV anlegen** – KV-Bucket `context-global` beim Server-Start (Memory Storage)
- [x] **Context-KV pro Flow** – KV-Bucket `context-flow-{flowID}` wird bei Deploy angelegt
- [x] Broker-Startup in `server.Start()` um Debug-Stream-Bootstrap erweitern

### 1.2 Message-Routing im Engine

Der Kern: Nodes verbinden und Messages zwischen ihnen routen.

- [x] **Node-Lifecycle implementieren** – `Engine.Deploy()` Steps 1–7 umgesetzt:
  1. Laufende Nodes stoppen (falls vorhanden)
  2. Node-Typen gegen Registry validieren
  3. `NodeInstance` pro Node erzeugen (via Factory)
  4. `Init()` auf jedem Node aufrufen
  5. Go-Channels zwischen verbundenen Nodes verdrahten
  6. Goroutine pro Node starten (Message-Loop)
  7. Aktiven State für Introspection speichern
- [x] **sync.WaitGroup** für sauberes Warten auf Node-Goroutines beim Stop
- [x] **Error-Channel** für Node-Fehler → an Engine melden *(Fehler werden geloggt, kein dedizierter Channel)*

### 1.3 Erste Node-Typen

Minimalsatz um einen Flow auszuführen.

- [x] **Inject Node** – Timer/Manueller Trigger, sendet `msg.payload` + `msg.topic`
- [x] **Debug Node** – Empfängt Message, publiziert auf NATS `debug.<flowID>.<nodeID>`, broadcastet via WebSocket
- [x] Nodes im Engine-Registry registrieren (beim Server-Start)

### 1.4 API Handler verdrahten

Die Stubs mit echten Implementierungen ersetzen.

- [x] **`GET /api/v1/nodes`** – Node-Katalog aus Registry liefern (Typ, Kategorie, Label, Icon, Defaults, Ports)
- [x] **`POST /api/v1/flows`** – Flow-JSON parsen, validieren, an `Engine.Deploy()` übergeben, in Storage persistieren
- [x] **`GET /api/v1/flows`** – Flows aus Storage laden und zurückgeben
- [x] **`GET /api/v1/flows/{id}`** – Einzelnen Flow zurückgeben
- [x] **`POST /api/v1/inject/{id}`** – Inject-Node manuell triggern (`Engine.TriggerNode()`)

### 1.5 Debug-Pipeline

Messages vom Node bis ins Frontend durchschleusen.

- [x] Debug Node → NATS Publish auf `debug.<flowID>.<nodeID>`
- [x] Subscriber im Server: NATS `debug.>` → WebSocket Hub Broadcast als `EventDebug`
- [x] **`GET /api/v1/debug/messages`** – Letzte N Messages aus JetStream Stream lesen *(Stub, History-Endpoint)*
- [x] Frontend: WebSocket `debug` Events empfangen → debugStore → DebugPanel

### 1.6 Frontend-Anbindung

- [x] Node-Palette aus `/api/v1/nodes` laden statt hardcoded *(noch hardcoded)*
- [x] Deploy-Button: `POST /api/v1/flows` mit aktuellem Flow-State
- [x] Deploy-Feedback: WebSocket `deploy` Event empfangen → UI-Indikator *(offen)*
- [x] Inject-Button im Node: `POST /api/v1/inject/{id}` aufrufen
- [x] Debug-Messages live anzeigen (WebSocket → Store → Panel)
- [x] Connection-Status ONLINE/OFFLINE (WebSocket → uiStore → HeaderBar)
- [x] Flows beim Seitenstart vom Backend laden (GET /flows → flowStore)
- [x] Node-Konfiguration im PropertyPanel (InjectConfig editierbar)
- [x] Node-Klick öffnet PropertyPanel (@node-click Event)

**Ergebnis Phase 1:** ✅ Inject → Debug funktioniert End-to-End. Flow deployen, manuell triggern, Debug-Output sehen.

---

## Phase 2 – Core Nodes: Datenverarbeitung

> **Ziel:** Flows können Daten transformieren, routen und verzögern.

### 2.1 Processing Nodes

- [ ] **Function Node** – JavaScript via Goja ausführen, `msg` rein → `msg` raus
- [ ] **Change Node** – `msg`-Properties setzen, ändern, löschen, verschieben (Rules-basiert)
- [ ] **Switch Node** – Messages anhand von Regeln auf verschiedene Outputs routen
- [ ] **Template Node** – Go `text/template` für String-Rendering mit `msg`-Daten
- [ ] **Delay Node** – Messages verzögern oder Rate-Limiten

### 2.2 Context-System (NATS KV)

- [x] **Global Context** – `global.get(key)` / `global.set(key, value)` via NATS KV `context-global`
- [x] **Flow Context** – `flow.get(key)` / `flow.set(key, value)` via NATS KV `context-flow-{id}`
- [x] Context-API für Function Nodes bereitstellen (Goja-Bindings)
- [x] Context Watch Node - Watch for a KV Key in the global or flow context

### 2.3 Universelles Node-Debugging

Das Alleinstellungsmerkmal von Flint (siehe DECISIONS.md §5.1.2 / §5.1.3).

- [ ] **Debug-Tap pro Node** – Wenn aktiviert: IN + OUT Messages auf `debug.{nodeID}` publizieren
- [ ] **Debug-Tap pro Wire** – Wenn aktiviert: Messages auf `debug.wire.{sourceID}.{targetID}` publizieren
- [ ] Zero-Cost wenn deaktiviert (kein Publish, kein Overhead)
- [ ] Frontend: Debug-Icon am Node (an/aus toggle)
- [ ] Frontend: Debug-Icon an Wire (Klick → Kontextmenü)
- [ ] Debug-Panel: Filter nach Node-ID, Wire, Flow

### 2.4 Error Handling

- [ ] **Catch Node** – Fängt Fehler von Nodes im selben Flow
- [ ] **Status Node** – Meldet Node-Status-Änderungen (connected, disconnected, error)
- [ ] Globaler Error-Handler im Engine (unhandled errors → Log + WebSocket Notification)

**Ergebnis Phase 2:** Vollständige Datenverarbeitung. Function-Nodes mit Context, Routing, Error Handling.

---

## Phase 3 – Netzwerk & Industrie: Die Außenwelt anbinden

> **Ziel:** Flint kann mit externen Systemen kommunizieren.

### 3.1 HTTP Nodes

- [ ] **HTTP In Node** – HTTP-Endpunkt erstellen (GET/POST/PUT/DELETE), Request als `msg`
- [ ] **HTTP Response Node** – Antwort an den HTTP-Client senden
- [ ] **HTTP Request Node** – Ausgehende HTTP-Aufrufe, Response als `msg`

### 3.2 MQTT Nodes

- [ ] **MQTT Broker Config-Node** – Verbindungsdaten (Host, Port, TLS, Credentials)
- [ ] **MQTT In Node** – Topic subscriben, Messages empfangen
- [ ] **MQTT Out Node** – Messages auf Topic publishen
- [ ] Credentials aus CredentialManager laden (AES-256-GCM entschlüsseln)

### 3.3 Industrie-Protokolle

- [ ] **Modbus TCP Read/Write** – Register lesen/schreiben (Holding, Input, Coil, Discrete)
- [ ] **Modbus RTU Read/Write** – Über Serial Port
- [ ] **Serial/COM Node** – RS232/RS485 Lesen/Schreiben
- [ ] **OPC-UA Node** – Browse, Read, Write, Subscribe (gopcua Library)

### 3.4 Weitere Netzwerk-Nodes

- [ ] **TCP In/Out** – Raw TCP Sockets
- [ ] **WebSocket In/Out** – WebSocket Client/Server
- [ ] **UDP In/Out** – UDP Datagramme

**Ergebnis Phase 3:** Flint spricht HTTP, MQTT, Modbus, Serial – industrietauglich.

---

## Phase 4 – Editor-Polish: Professionelle UX

> **Ziel:** Der Flow-Editor fühlt sich fertig an.

### 4.1 Flow-Management

- [ ] **Tabs** – Mehrere Flows gleichzeitig offen, zwischen Flows wechseln
- [ ] **Import/Export** – Flows als JSON exportieren und importieren
- [ ] **Undo/Redo** – History-Stack für Flow-Änderungen (Vue Flow bietet Basis)

### 4.2 Subflows

- [ ] **Subflow erstellen** – Auswahl von Nodes → zu Subflow zusammenfassen
- [ ] **Subflow-Node** – Subflow als wiederverwendbaren Node im Palette anzeigen
- [ ] **Subflow-Editor** – Eigener Tab zum Editieren des Subflow-Inhalts

### 4.3 Editor-Features

- [ ] **Suche** – Nodes und Flows durchsuchen
- [ ] **Minimap** – Übersichtskarte des Flows
- [ ] **Keyboard Shortcuts** – Standardkürzel (Ctrl+Z, Ctrl+C/V, Del, etc.)
- [ ] **Node-Tooltips** – Inline-Hilfe pro Node-Typ
- [ ] **Validierung** – Fehlerhafte Konfigurationen visuell markieren
- [ ] **Connection Status** – Live-Indikator WebSocket verbunden/getrennt

### 4.4 Settings & Konfiguration

- [ ] **Settings UI** – Backend-Einstellungen im Frontend ändern
- [ ] **Settings API** – `GET/POST /api/v1/settings` implementieren
- [ ] **Origin Validation** – WebSocket Origin-Check für Production

**Ergebnis Phase 4:** Professioneller Editor mit allen UX-Features aus DECISIONS.md.

---

## Phase 5 – Production Readiness

> **Ziel:** Flint ist stabil, sicher und deploybar.

### 5.1 Sicherheit

- [ ] **WebSocket Origin-Restriction** – Nur eigener Host erlaubt
- [ ] **Rate Limiting** – API-Endpunkte gegen Missbrauch schützen
- [ ] **Input Validation** – Alle API-Inputs validieren und sanitizen
- [ ] **CORS** – Konfigurierbare CORS-Policy

### 5.2 Observability

- [ ] **Structured Logging** – Log-Level konfigurierbar (Debug/Info/Warn/Error)
- [ ] **Health-Check erweitern** – `/health` mit NATS-Status, Engine-Status, Uptime
- [ ] **Metrics** – Optional Prometheus-Endpoint (`/metrics`)

### 5.3 Packaging

- [ ] **Docker Image** – Multi-Stage Build, minimal Image
- [ ] **systemd Unit** – Service-File für Linux
- [ ] **Cross-Compile** – Alle Plattformen testen (bereits im Makefile)
- [ ] **Release Automation** – GitHub Actions für Build + Release

### 5.4 Testing

- [ ] **Engine Tests** – Deploy, Message-Routing, Node-Lifecycle
- [ ] **Node Tests** – Jeder Node-Typ mit Unit-Tests
- [ ] **API Tests** – HTTP-Handler Integration Tests
- [ ] **Frontend Tests** – Component Tests für kritische UI-Teile
- [ ] **E2E Test** – Inject → Function → Debug als Smoke Test

**Ergebnis Phase 5:** Flint ist production-ready, getestet und paketiert.

---

## Abhängigkeiten zwischen Phasen

```
Phase 1 ──────► Phase 2 ──────► Phase 3
 (Engine)       (Core Nodes)    (Netzwerk)
    │                │
    │                ▼
    │           Phase 4
    │           (UX Polish)
    │                │
    ▼                ▼
              Phase 5
           (Production)
```

- **Phase 1 ist Voraussetzung für alles** – ohne Engine läuft kein Flow
- **Phase 2 und 3** können teilweise parallel bearbeitet werden
- **Phase 4** kann ab Phase 2 begonnen werden (unabhängig von Netzwerk-Nodes)
- **Phase 5** zieht sich idealerweise durch alle Phasen (Tests pro Feature)

---

## Nächster Schritt

**→ Phase 1 abgeschlossen! ✅**
**→ Phase 2 kann beginnen: Core Processing Nodes (Function, Change, Switch, Template, Delay).**

---

## Frontend Bugs & Offene Punkte

> Ergebnis der Code-Analyse vom 2026-03-17.

### CRITICAL

| # | Problem | Datei | Status |
|---|---------|-------|--------|
| F1 | **Node-Drag markiert Flow nicht als dirty** — `updateNodePosition()` ruft `markDirty()` nicht auf. Änderungen können verloren gehen. | `flowStore.ts:171` | [x] |
| F2 | **Deploy-Status nur über WebSocket** — `flowStore.deploy()` aktualisiert `uiStore.deployStatus` nicht direkt. Ohne WS kein Feedback. | `flowStore.ts:235` | [ ] |
| F3 | **Canvas-Bounds nicht dynamisch** — `translate-extent` hardcoded `[[0,0],[10000,10000]]`, kein `nodeExtent`, MiniMap-Viewport ändert sich nicht beim Zoomen. | `FlowEditor.vue:154` | [ ] |

### HIGH

| # | Problem | Datei | Status |
|---|---------|-------|--------|
| F4 | **DebugNode `messageCount` wird nie aktualisiert** — Badge zeigt `props.data?.messageCount`, aber kein Code setzt den Wert. | `DebugNode.vue:13` | [ ] |
| F6 | **Fehlende `terminal-checkbox` CSS-Klasse** — InjectConfig nutzt eine undefinierte Klasse. | `InjectConfig.vue:73` | [ ] |
| F7 | **Kein Error-Feedback bei Deploy-Fehler** — Fehler nur in `console.error`, kein Toast/Notification. | `flowStore.ts:266` | [ ] |

### MEDIUM

| # | Problem | Datei | Status |
|---|---------|-------|--------|
| F8 | **Max-Zoom auf 1.0 begrenzt** — Kann nicht reinzoomen um Details zu sehen. | `FlowEditor.vue:152` | [ ] |
| F9 | **Linke Sidebar überlagert Canvas** — `position: absolute` statt Flexbox, verdeckt Nodes. | `App.vue:61` | [ ] |
| F10 | **Keine Validierung von Node-Verbindungen** — Inkompatible Ports können verbunden werden. | `FlowEditor.vue` | [ ] |
| F11 | **Multi-Select ignoriert** — Bei Mehrfachauswahl wird nur der erste Node gespeichert. | `FlowEditor.vue:54` | [ ] |
| F12 | **Status-Farben weichen vom Design-System ab** — Hardcoded hex statt Token-Farben. | `BaseNode.vue:36` | [ ] |

### LOW (Polish)

| # | Problem | Datei | Status |
|---|---------|-------|--------|
| F13 | **Palette-Suchfilter nicht persistiert** — Reset beim Schließen. | `NodePalette.vue` | [ ] |
| F14 | **Hardcoded Werte** — MAX_MESSAGES=1000, Deploy-Reset=3s, Grid=16px. | diverse | [ ] |
| F15 | **Doppelter Deploy-API-Pfad** — `flowStore.deploy()` nutzt `fetch` direkt, HeaderBar hat `useApi().deployFlows()`. | `flowStore.ts` / `HeaderBar.vue` | [ ] |
| F16 | **Settings-Page ist ein Dummy** — `handleSave()` und `handleReset()` sind leer, kein Backend. | `SettingsView.vue` | [ ] |
