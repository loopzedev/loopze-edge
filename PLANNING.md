# LOOPZE – Implementation Plan

> This plan describes the step-by-step path from the current state to a working MVP.
> Each phase builds on the previous one. Within a phase, the steps are sequential.

---

## Status Quo

| Layer | Status |
|---|---|
| **Infrastructure** (server, config, middleware, WebSocket hub) | ✅ Done |
| **Storage** (JSON files, credentials AES-256-GCM, atomic writes) | ✅ Done |
| **Embedded NATS** (server + JetStream + in-process client) | ✅ Done |
| **Frontend** (Vue 3 + Vue Flow, stores, palette, debug panel) | ✅ Done — backend wired up, deploy + debug working |
| **Flow engine** (types, registry, deploy, message routing) | ✅ Done — full lifecycle, goroutine-per-node |
| **API handlers** | ✅ Done — flows, nodes, inject, debug |
| **Auth** (first-run setup, Argon2id, sessions, roles, dev bypass) | ✅ Done — see `docs/issues/USER_AUTH.md` |
| **Node types** | 🟡 2 implemented (Inject, Debug) |

---

## Phase 1 – Foundation: Get the Engine Running

> **Goal:** An Inject node sends a message to a Debug node → message appears in the debug panel.

### 1.1 NATS Bootstrapping

The embedded NATS broker is already running. Now create the structures needed at runtime.

- [x] **Create debug stream** — JetStream stream `DEBUG` with subject `debug.>`, MaxMsgs 1000, memory storage
- [x] **Create context KV** — KV bucket `context-global` at server start (memory storage)
- [x] **Per-flow context KV** — KV bucket `context-flow-{flowID}` is created on deploy
- [x] Extend broker startup in `server.Start()` with debug-stream bootstrap

### 1.2 Message Routing in the Engine

The core: connect nodes and route messages between them.

- [x] **Implement node lifecycle** — `Engine.Deploy()` steps 1–7 implemented:
  1. Stop running nodes (if any)
  2. Validate node types against the registry
  3. Create a `NodeInstance` per node (via factory)
  4. Call `Init()` on every node
  5. Wire Go channels between connected nodes
  6. Start a goroutine per node (message loop)
  7. Save the active state for introspection
- [x] **`sync.WaitGroup`** for clean waiting on node goroutines on stop
- [x] **Error channel** for node errors → reported to the engine *(errors are logged, no dedicated channel)*

### 1.3 First Node Types

Minimum set to execute a flow.

- [x] **Inject node** — Timer / manual trigger, sends `msg.payload` + `msg.topic`
- [x] **Debug node** — Receives a message, publishes on NATS `debug.<flowID>.<nodeID>`, broadcasts via WebSocket
- [x] Register nodes in the engine registry (at server start)

### 1.4 Wire Up API Handlers

Replace the stubs with real implementations.

- [x] **`GET /api/v1/nodes`** — Return the node catalog from the registry (type, category, label, icon, defaults, ports)
- [x] **`POST /api/v1/flows`** — Parse the flow JSON, validate, hand off to `Engine.Deploy()`, persist in storage
- [x] **`GET /api/v1/flows`** — Load flows from storage and return them
- [x] **`GET /api/v1/flows/{id}`** — Return a single flow
- [x] **`POST /api/v1/inject/{id}`** — Trigger an Inject node manually (`Engine.TriggerNode()`)

### 1.5 Debug Pipeline

Pipe messages from the node all the way to the frontend.

- [x] Debug node → NATS publish on `debug.<flowID>.<nodeID>`
- [x] Subscriber in the server: NATS `debug.>` → WebSocket hub broadcast as `EventDebug`
- [x] **`GET /api/v1/debug/messages`** — Read the last N messages from the JetStream stream *(stub, history endpoint)*
- [x] Frontend: receive WebSocket `debug` events → debugStore → DebugPanel

### 1.6 Frontend Integration

- [x] Load the node palette from `/api/v1/nodes` instead of hardcoded *(still hardcoded)*
- [x] Deploy button: `POST /api/v1/flows` with the current flow state
- [x] Deploy feedback: receive WebSocket `deploy` event → UI indicator *(open)*
- [x] Inject button on the node: call `POST /api/v1/inject/{id}`
- [x] Display debug messages live (WebSocket → store → panel)
- [x] Connection status ONLINE/OFFLINE (WebSocket → uiStore → HeaderBar)
- [x] Load flows from the backend on page start (GET /flows → flowStore)
- [x] Node configuration in the property panel (InjectConfig editable)
- [x] Node click opens the property panel (@node-click event)

**Result of Phase 1:** ✅ Inject → Debug works end-to-end. Deploy a flow, trigger manually, see debug output.

---

## Phase 2 – Core Nodes: Data Processing

> **Goal:** Flows can transform, route, and delay data.

### 2.1 Processing Nodes

- [x] **Function node** — Run JavaScript via Goja, `msg` in → `msg` out
- [x] **Change node** — Set, change, delete, move `msg` properties (rule-based)
- [x] **Switch node** — Route messages to different outputs based on rules
- [ ] **Template node** — Go `text/template` for string rendering with `msg` data
- [x] **Delay node** — Delay messages, rate-limit, or random jitter (3 modes); override via `msg.delay`/`msg.flush`/`msg.reset`

### 2.2 Context System (NATS KV)

- [x] **Global context** — `global.get(key)` / `global.set(key, value)` via NATS KV `context-global`
- [x] **Flow context** — `flow.get(key)` / `flow.set(key, value)` via NATS KV `context-flow-{id}`
- [x] Provide a context API for function nodes (Goja bindings)
- [x] Context Watch node — watch for a KV key in the global or flow context

### 2.3 Universal Node Debugging

The unique selling point of LOOPZE (see DECISIONS.md §5.1.2 / §5.1.3).

- [ ] **Debug tap per node** — When enabled: publish IN + OUT messages on `debug.{nodeID}`
- [ ] **Debug tap per wire** — When enabled: publish messages on `debug.wire.{sourceID}.{targetID}`
- [ ] Zero-cost when disabled (no publish, no overhead)
- [ ] Frontend: debug icon on the node (on/off toggle)
- [ ] Frontend: debug icon on the wire (click → context menu)
- [ ] Debug panel: filter by node ID, wire, flow

### 2.4 Error Handling

- [x] **Catch node** — Catches errors from nodes in the same flow
- [x] **Status node** — Reports node status changes (connected, disconnected, error)
- [ ] Global error handler in the engine (unhandled errors → log + WebSocket notification)

**Result of Phase 2:** Complete data processing. Function nodes with context, routing, error handling.

---

## Phase 3 – Network & Industry: Connect to the Outside World

> **Goal:** LOOPZE can communicate with external systems.

### 3.1 HTTP Nodes

- [ ] **HTTP In node** — Create an HTTP endpoint (GET/POST/PUT/DELETE), request as `msg`
- [ ] **HTTP Response node** — Send the response to the HTTP client
- [ ] **HTTP Request node** — Outgoing HTTP calls, response as `msg`

### 3.2 MQTT Nodes

- [ ] **MQTT broker config node** — Connection data (host, port, TLS, credentials)
- [ ] **MQTT In node** — Subscribe to a topic, receive messages
- [ ] **MQTT Out node** — Publish messages on a topic
- [ ] Load credentials from CredentialManager (decrypt AES-256-GCM)

### 3.3 Industrial Protocols

- [ ] **Modbus TCP read/write** — Read/write registers (holding, input, coil, discrete)
- [ ] **Modbus RTU read/write** — Over a serial port
- [ ] **Serial/COM node** — RS232/RS485 read/write
- [ ] **OPC-UA node** — Browse, read, write, subscribe (gopcua library)

### 3.4 Other Network Nodes

- [ ] **TCP In/Out** — Raw TCP sockets
- [ ] **WebSocket In/Out** — WebSocket client/server
- [ ] **UDP In/Out** — UDP datagrams

**Result of Phase 3:** LOOPZE speaks HTTP, MQTT, Modbus, Serial — industrial-grade.

---

## Phase 4 – Editor Polish: Professional UX

> **Goal:** The flow editor feels finished.

### 4.1 Flow Management

- [ ] **Tabs** — Multiple flows open at the same time, switch between flows
- [ ] **Import/export** — Export and import flows as JSON
- [ ] **Undo/redo** — History stack for flow changes (Vue Flow provides the basis)

### 4.2 Subflows

- [ ] **Create subflow** — Select nodes → bundle them into a subflow
- [ ] **Subflow node** — Show the subflow as a reusable node in the palette
- [ ] **Subflow editor** — Dedicated tab for editing the subflow contents

### 4.3 Editor Features

- [ ] **Search** — Search nodes and flows
- [ ] **Minimap** — Overview map of the flow
- [ ] **Keyboard shortcuts** — Standard shortcuts (Ctrl+Z, Ctrl+C/V, Del, etc.)
- [ ] **Node tooltips** — Inline help per node type
- [ ] **Validation** — Visually mark invalid configurations
- [ ] **Connection status** — Live indicator: WebSocket connected/disconnected

### 4.4 Settings & Configuration

- [ ] **Settings UI** — Change backend settings from the frontend
- [ ] **Settings API** — Implement `GET/POST /api/v1/settings`
- [ ] **Origin validation** — WebSocket origin check for production

**Result of Phase 4:** Professional editor with all the UX features from DECISIONS.md.

---

## Phase 5 – Production Readiness

> **Goal:** LOOPZE is stable, secure, and deployable.

### 5.1 Security

- [ ] **WebSocket origin restriction** — Only the own host is allowed
- [ ] **Rate limiting** — Protect API endpoints against abuse
- [ ] **Input validation** — Validate and sanitize all API inputs
- [ ] **CORS** — Configurable CORS policy

### 5.2 Observability

- [ ] **Structured logging** — Configurable log levels (debug/info/warn/error)
- [ ] **Extend health check** — `/health` with NATS status, engine status, uptime
- [ ] **Metrics** — Optional Prometheus endpoint (`/metrics`)

### 5.3 Packaging

- [ ] **Docker image** — Multi-stage build, minimal image
- [ ] **systemd unit** — Service file for Linux
- [ ] **Cross-compile** — Test all platforms (already in the Makefile)
- [ ] **Release automation** — GitHub Actions for build + release

### 5.4 Testing

- [ ] **Engine tests** — Deploy, message routing, node lifecycle
- [ ] **Node tests** — Every node type with unit tests
- [ ] **API tests** — HTTP-handler integration tests
- [ ] **Frontend tests** — Component tests for critical UI parts
- [ ] **E2E test** — Inject → Function → Debug as a smoke test

**Result of Phase 5:** LOOPZE is production-ready, tested, and packaged.

---

## Dependencies Between Phases

```
Phase 1 ──────► Phase 2 ──────► Phase 3
 (Engine)       (Core Nodes)    (Network)
    │                │
    │                ▼
    │           Phase 4
    │           (UX Polish)
    │                │
    ▼                ▼
              Phase 5
           (Production)
```

- **Phase 1 is a prerequisite for everything** — without the engine no flow runs
- **Phases 2 and 3** can be worked on partially in parallel
- **Phase 4** can be started from Phase 2 onward (independent of network nodes)
- **Phase 5** ideally runs through all phases (tests per feature)

---

## Next Step

**→ Phase 1 done! ✅**
**→ Phase 2 can begin: core processing nodes (Function, Change, Switch, Template, Delay).**

---

## Frontend Bugs & Open Items

> Result of the code analysis from 2026-03-17.

### CRITICAL

| # | Problem | File | Status |
|---|---------|------|--------|
| F1 | **Node drag does not mark the flow dirty** — `updateNodePosition()` does not call `markDirty()`. Changes can be lost. | `flowStore.ts:171` | [x] |
| F2 | **Deploy status only via WebSocket** — `flowStore.deploy()` does not update `uiStore.deployStatus` directly. Without WS no feedback. | `flowStore.ts:235` | [ ] |
| F3 | **Canvas bounds not dynamic** — `translate-extent` hardcoded to `[[0,0],[10000,10000]]`, no `nodeExtent`, MiniMap viewport doesn't change on zoom. | `FlowEditor.vue:154` | [ ] |

### HIGH

| # | Problem | File | Status |
|---|---------|------|--------|
| F4 | **DebugNode `messageCount` is never updated** — Badge shows `props.data?.messageCount`, but no code sets the value. | `DebugNode.vue:13` | [ ] |
| F6 | **Missing `terminal-checkbox` CSS class** — InjectConfig uses an undefined class. | `InjectConfig.vue:73` | [ ] |
| F7 | **No error feedback on deploy failure** — Errors only in `console.error`, no toast/notification. | `flowStore.ts:266` | [ ] |

### MEDIUM

| # | Problem | File | Status |
|---|---------|------|--------|
| F8 | **Max zoom limited to 1.0** — Cannot zoom in to see details. | `FlowEditor.vue:152` | [ ] |
| F9 | **Left sidebar overlays the canvas** — `position: absolute` instead of flexbox, hides nodes. | `App.vue:61` | [ ] |
| F10 | **No validation of node connections** — Incompatible ports can be connected. | `FlowEditor.vue` | [ ] |
| F11 | **Multi-select ignored** — On multi-selection only the first node is saved. | `FlowEditor.vue:54` | [ ] |
| F12 | **Status colors deviate from the design system** — Hardcoded hex instead of token colors. | `BaseNode.vue:36` | [ ] |

### LOW (Polish)

| # | Problem | File | Status |
|---|---------|------|--------|
| F13 | **Palette search filter not persisted** — Resets when closed. | `NodePalette.vue` | [ ] |
| F14 | **Hardcoded values** — MAX_MESSAGES=1000, deploy reset=3s, grid=16px. | various | [ ] |
| F15 | **Duplicate deploy API path** — `flowStore.deploy()` uses `fetch` directly, HeaderBar has `useApi().deployFlows()`. | `flowStore.ts` / `HeaderBar.vue` | [ ] |
| F16 | **Settings page is a dummy** — `handleSave()` and `handleReset()` are empty, no backend. | `SettingsView.vue` | [ ] |
