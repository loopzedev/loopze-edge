# LOOPZE – Project Decisions & Open Questions

> This document serves as the central decision log for the project.
> Each question is answered jointly and the decision is captured here.

---

## 1. Project Goal

| Question | Answer |
|---|---|
| What is the vision? | A Node-RED clone with a modern tech stack |
| Target audience? | ✅ Experienced technicians (not full-stack developers), automation engineers, PLC programmers with scripting experience |
| License model? | ✅ AGPL-3.0-or-later — copyleft, prevents embedding into proprietary products (switched from ELv2 on 2026-05-03) |
| Project name (edge app)? | ✅ **LOOPZE** |
| Project name (management platform)? | ✅ **LOOPZE Hub** |

### 1.1 Target Audience – Details & Design Consequences

**User profile:**
- Technicians, engineers, PLC programmers (e.g. IEC 61131-3: Ladder, FBD, ST)
- Scripting experience (Bash, Python, VBA, Structured Text) — but no full-stack background
- Thinks in **processes, signal flows, and functional blocks** (as in industrial automation)
- Used to: clear I/O definitions, deterministic behavior, fault diagnosis in the field

**Consequences for the UI/UX:**
| Aspect | Consequence |
|---|---|
| No complex IDE features | Editor must be intuitive, low learning curve |
| PLC mindset: function blocks | Nodes need clearly labeled inputs/outputs (like FBD blocks) |
| Scripting experience present | Function nodes with a simple scripting language (no full JS framework needed) |
| Field-oriented applications | Solid MQTT, Serial, Modbus, OPC-UA support is important |
| Fault diagnosis in operation | Debug output must be clear, live, and filterable |
| No package-manager knowledge | Installation must be simple → single binary preferred |
| Inline documentation | Nodes should have tooltips / built-in help |

**Consequences for the node system:**
| Aspect | Consequence |
|---|---|
| PLC: clearly typed signals | Optional typing of msg fields (string, number, bool, byte) is desirable |
| PLC: cyclic execution | Timer/Inject nodes must be precise and reliable |
| Scripting | Function-node language must be simple — JavaScript (Goja) or Lua preferred |
| Modbus / OPC-UA / Serial | These nodes have high priority (industrial environment) |
| Deterministic behavior | Clear statements about execution order and error behavior |

### 1.2 Business Model – Open Core ✅

Reference: GitLab · n8n · Grafana — free core, commercial enterprise extensions

╔══════════════════════════════════════════════════════════╗
║        TIER 2 – FLEET / ENTERPRISE  (Commercial)         ║
║                                                          ║
║  [Edge A] ◀──▶ [Management Platform] ◀──▶ [Edge B]      ║
║  [Edge C] ◀────────────┘              [Edge D]           ║
║                                                          ║
║  Fleet management · Message routing · Master-data sync   ║
╠══════════════════════════════════════════════════════════╣
║        TIER 1 – EDGE  (Open Source / AGPL-3.0)           ║
║                                                          ║
║  [Flow editor (Vue 3)] [Runtime (Go)] [Node library]     ║
║  Runs standalone · Single binary · No dependencies       ║
╚══════════════════════════════════════════════════════════╝

#### Tier 1 – Edge Node (Open Source)
| License        | ✅ AGPL-3.0-or-later                                          |
| Scope          | Full flow editor + runtime on a single device                |
| Standalone     | Runs autonomously, no network dependency                     |
| Goal           | Community, adoption, trust                                   |

#### Tier 2 – Fleet & Management Platform (Commercial)
| Fleet management   | Centrally register, monitor, and manage all edges        |
| Message routing    | Forward and transform messages between edges             |
| Master-data sync   | Distribute configurations / lookup tables to all nodes   |
| Management UI      | Central web interface for all nodes, status, flows       |
| Remote deploy      | Roll out flows centrally to single or all edges          |
| Audit log          | Who deployed/changed which flow, and when                |
| Hosting            | ✅ Self-hosted on-premise                                 |
| Pricing            | ✅ Per device/node — license key per installation         |

#### Technical Consequences
| Clean module separation     | Edge core ≠ management code — separated from day one  |
| Agent concept in the edge   | Optional "fleet agent" — activatable via license key   |
| No enterprise code in OSS   | Management features are NOT in the open-source code    |
| Offline capability          | Edge keeps running autonomously when management is unreachable |
| Edge identity               | Every edge needs UUID + cryptographic credentials      |
| Secure communication        | Edge ↔ management requires TLS / mTLS                  |

---

## 2. Tech Stack

### 2.1 Backend

| Question | Options | Decision |
|---|---|---|
| Backend language | **Go** (fast, single binary, goroutines for concurrency) | ✅ Go |
| HTTP framework | `net/http` (stdlib), `Fiber`, `Echo`, `Chi`, `Gin` | ✅ Chi |
| WebSocket library | `gorilla/websocket`, `nhooyr/websocket`, `coder/websocket` | ✅ gorilla/websocket |

### 2.2 Frontend

| Question | Options | Decision |
|---|---|---|
| Frontend framework | **Vue 3** (with Vue Flow), React (with xyflow), Svelte | ✅ Vue 3 |
| Flow/canvas library | **Vue Flow** (`@vue-flow/core`) — 6.4k stars, MIT, used by n8n | ✅ Vue Flow |
| UI component library | Vuetify, PrimeVue, Naive UI, Headless UI, custom | ✅ Tailwind CSS + Radix Vue (custom components) |
| State management | Pinia, Vuex, or just composables | ✅ Pinia |
| CSS / styling | Tailwind CSS, UnoCSS, plain CSS/SCSS | ✅ Tailwind CSS |
| Build tool | Vite | ✅ Vite (standard for Vue 3) |

### 2.3 Deployment & Build

| Question | Options | Decision |
|---|---|---|
| Frontend embedded in Go binary? | Yes (`go:embed`) → single binary like Node-RED | yes |
| Deployment target | Single binary, Docker, both? | both |
| Target platforms | Linux, macOS, Windows, ARM (Raspberry Pi)? | all |

### 2.4 Storage / Persistence

| Question | Options | Decision |
|---|---|---|
| Flow storage | JSON files (like Node-RED), SQLite, PostgreSQL | JSON file |
| Credentials/secrets | Encrypted in a separate file (tier 1), centralized in management (tier 2) | ✅ Separate encrypted credentials file (AES-256-GCM) + auto-generated keyfile (`goflux.key`) |
| Node configurations | Embedded in the flow JSON (like Node-RED) | yes |


---

## 2.6 NATS – Architecture & Scope ✅

### NATS is used for:

| Use | Mechanism | Detail |
|---|---|---|
| **LOOPZE ↔ LOOPZE Hub** | LeafNode connection | Communication between edge and management platform |
| **Context store** | KV store | Flow context and global context for function nodes |
| **Master-data sync** | KV store mirror | LOOPZE Hub distributes master data to all edges |
| **Debug messages** | JetStream (ring buffer) | Last N debug messages persisted, retrievable live |

### NATS is NOT used for:

| What | Instead |
|---|---|
| **Node ↔ node within a flow** | Go channels — direct, no overhead |
| **Flow ↔ flow communication** | Go-internal |
| **Node state** | Go memory |
| **Internal message handling** | Go channels |

### Rationale for the hybrid model:

| Aspect | Detail |
|---|---|
| **Performance** | Node-to-node within a flow over Go channels — nanosecond latency |
| **Transparency** | Debug messages in JetStream — technician can inspect live with the NATS CLI |
| **Offline capability** | NATS embedded — edge runs fully without a hub connection |
| **Simplicity** | Clean separation: Go = internal, NATS = persistence + fleet |

---

## 3. Architecture & Design

### 3.1 Node System

| Question | Options | Decision |
|---|---|---|
| Node-RED compatible? | Yes (same JSON format) vs. no (own format, only UX-inspired) | no |
| Scripting in function nodes | JavaScript (via Goja), expr-lang, Lua, WASM | ✅ **Dual-mode**: JavaScript (via Goja) + expr-lang |
| Node extensibility | Go plugins, WASM modules, scripted nodes, gRPC | ✅ Go plugins |
| Node categories | Common, Function, Network, Sequence, Parser, Storage, Industrial | ✅ Common · Function · Network · Industrial · Storage · Parser |

### 3.2 Flow Runtime

| Question | Options | Decision |
|---|---|---|
| Message model | Struct with fixed fields vs. free `map[string]any` like Node-RED | ✅ Free `map[string]any` — all fields equal, `_id` immutable (see 5.1.5) |
| Concurrency model | Goroutine per flow, per node, or per message | ✅ Goroutine per node |
| Error handling | Catch nodes (like Node-RED), global error handler, both | ✅ Catch nodes + global error handler |
| Flow types | Normal flows, subflows (reusable), config nodes | ✅ Normal flows + subflows + config nodes |

### 3.3 Frontend ↔ Backend Communication

| Question | Options | Decision |
|---|---|---|
| API style | REST + WebSocket, GraphQL + WebSocket, gRPC-Web | ✅ REST + WebSocket |
| Real-time events | WebSocket for debug output, flow status, deploy events | ✅ WebSocket |
| Auth/security | None initially, basic auth, JWT, OAuth | ✅ MVP: none — tier 2: JWT |

---

## 4. MVP – Minimum Viable Product

### 4.1 Which features must be in the MVP?

| Feature | Priority | In MVP? |
|---|---|---|
| Flow editor canvas (place, connect, delete nodes) | Must | ✅ Yes |
| Node palette / sidebar (add nodes via drag & drop) | Must | ✅ Yes |
| Node configuration dialog (double-click → settings) | Must | ✅ Yes |
| Deploy button (activate flow) | Must | ✅ Yes |
| Debug sidebar (live messages via WebSocket) | Must | ✅ Yes |
| Import/export of flows (JSON) | Should | ✅ Yes |
| Undo/redo | Should | ✅ Yes |
| Tabs (multiple flows simultaneously) | Should | ✅ Yes |
| Subflows | Could | ✅ Yes |
| Flow variables (flow/global context) | Could | ✅ Yes |
| Dashboard / UI nodes | Won't (MVP) | ❌ No |

### 4.2 Which nodes must be in the MVP?

| Node | Type | Description | Priority (target audience) |
|---|---|---|---|
| **Inject** | Input | Timer/trigger, sends messages | 🔴 Must |
| **Debug** | Output | Shows messages in the sidebar | 🔴 Must |
| **Function** | Processing | Run custom script | 🔴 Must |
| **Change** | Processing | Set / change / delete msg properties | 🔴 Must |
| **Switch** | Routing | Route messages based on rules | 🔴 Must |
| **HTTP In** | Input | Create an HTTP endpoint | 🟡 Should |
| **HTTP Response** | Output | Send an HTTP response | 🟡 Should |
| **HTTP Request** | Processing | Make HTTP calls | 🟡 Should |
| **Template** | Processing | Render text templates | 🟡 Should |
| **Delay** | Processing | Delay / rate-limit messages | 🟡 Should |
| **MQTT In/Out** | I/O | MQTT pub/sub — highly relevant industrially | 🔴 Must (target audience!) |
| **Modbus TCP/RTU** | I/O | PLC communication — essential for PLC programmers | 🟡 Should |
| **OPC-UA** | I/O | Industry standard for machine communication | 🟢 Could |
| **Serial/COM** | I/O | RS232/RS485 — common in industry | 🟢 Could |
| **WebSocket In/Out** | I/O | WebSocket communication | 🟢 Could |

---

## 5. Differentiation & Unique Selling Points

| Question | Answer |
|---|---|
| What should LOOPZE do **better** than Node-RED? | ✅ Performance (Go instead of Node.js), single binary, no npm/Node.js needed, industrial protocols first-class (Modbus, OPC-UA), expr-lang for fast expressions |
| What should LOOPZE do **differently** from Node-RED? | ✅ Open core with fleet management (LOOPZE Hub), master-data sync, retro-industrial UI, dual-mode scripting, AGPL-3.0 license |
| What should LOOPZE deliberately **not** have? | ✅ No npm package ecosystem, no dashboard / UI-node system (MVP), no cloud dependency |

### 5.1 UX Improvements over Node-RED ✅

> Concrete usability and concept decisions that LOOPZE deliberately makes differently (better) than Node-RED.

#### 5.1.1 Node Ports: Single Input, Multiple Outputs ✅

| Aspect | Decision |
|---|---|
| Inputs per node | **Exactly 1** (or 0 for pure source nodes like Inject) |
| Outputs per node | **1 to N** (depending on node type, e.g. Switch has multiple outputs) |
| Rationale | Clear, deterministic signal-flow model — like in the PLC world (FBD). One input = one trigger. Multiple inputs introduce ambiguity over execution order and merge behavior. Anyone wanting to combine signals uses an explicit Join/Merge node. |
| Node-RED comparison | Node-RED allows multiple inputs — in practice this leads to confusing flows where it's unclear which input triggers execution. |

```
  ┌──────────┐     ┌──────────────┐     ┌──────────┐
  │  Inject  ├────▶│   Function   ├──┬─▶│  Debug   │
  └──────────┘     └──────────────┘  │  └──────────┘
                        1 input      │  ┌──────────┐
                        2 outputs    └─▶│  MQTT Out│
                                        └──────────┘
```

#### 5.1.2 Universal Node Debugging ✅

| Aspect | Decision |
|---|---|
| Debug node | Still exists as a dedicated node (like Node-RED) |
| **NEW: per-node debug** | **Any node** can be put into debug mode individually |
| Activation | Small debug icon (🔍) directly on the node in the flow editor — toggle on/off with a click |
| Display | The debug panel shows **all incoming AND outgoing messages** of the node |
| Presentation | Incoming messages with `→ IN` prefix, outgoing with `OUT →` prefix, each with a timestamp |
| Benefit | No manual insertion of debug nodes between wires needed — saves clicks, keeps the flow clean |
| Performance | Debug tap is only active when enabled — no overhead in normal operation |
| Node-RED comparison | Node-RED requires a separate debug node for each debug spot → flow quickly becomes cluttered |

```
  ┌──────────┐         ┌──────────────┐         ┌──────────┐
  │  Inject  ├────────▶│  Function 🔍 ├────────▶│  MQTT Out│
  └──────────┘         └──────┬───────┘         └──────────┘
                              │
                   ┌──────────▼──────────┐
                   │   Debug panel:      │
                   │   09:14:01 → IN     │
                   │     { payload: 42 } │
                   │   09:14:01 OUT →    │
                   │     { payload: 84 } │
                   └─────────────────────┘
```

#### 5.1.3 Wire Debugging (Signal Lines) ✅

| Aspect | Decision |
|---|---|
| **NEW: per-wire debug** | Individual **wires** (connections) can also be put into debug mode |
| Activation | Click on a wire → context menu or small debug icon on the wire |
| Display | The debug panel shows all messages flowing over this specific wire |
| Presentation | Wire debug shows: source node → target node, timestamp, message content |
| Visualization | Active debug wires are visually highlighted in the flow editor (e.g. pulsing amber animation) |
| Benefit | Precise fault-finding in complex flows with many branches — you see exactly what flows on which path |
| Node-RED comparison | Node-RED has no wire debugging — you must place debug nodes between every connection |

```
  ┌──────────┐    ⚡DebugWire     ┌──────────┐
  │  Switch  ├═══════════════════▶│  HTTP Out│
  └────┬─────┘                    └──────────┘
       │            ┌──────────┐
       └───────────▶│  MQTT Out│
                    └──────────┘

  Debug panel:
  ═══ Switch → HTTP Out ═══
  09:14:01  { payload: "ok", topic: "status" }
  09:14:02  { payload: "ok", topic: "status" }
```

#### Summary: Debug Concept in LOOPZE

| Feature | Node-RED | LOOPZE |
|---|---|---|
| Debug node | ✅ Yes | ✅ Yes (kept) |
| Per-node debug | ❌ No | ✅ **Any node can be debugged** |
| Debug shows IN + OUT | ❌ No (input only) | ✅ **Incoming + outgoing messages** |
| Per-wire debug | ❌ No | ✅ **Wires individually debuggable** |
| Debug overhead in normal operation | Always active (debug node) | ✅ **Only when enabled — zero-cost when off** |
| Flow cleanliness | Debug nodes everywhere → cluttered | ✅ **Flow stays clean, debug is overlay** |

#### 5.1.4 Naming: Workspace → Flow ✅

| Aspect | Decision |
|---|---|
| Top-level container | **Workspace** — contains all flows, configuration, credentials |
| Single tab | **Flow** — an independent dataflow with nodes and wires |
| Rationale | In Node-RED, "flow" means both the entire configuration and a single tab — that's confusing. LOOPZE separates clearly: a **workspace** has multiple **flows**. |
| Node-RED comparison | Node-RED: "flows.json" = everything, "flow" = tab → ambiguous. LOOPZE: workspace = container, flow = tab → unambiguous. |

```
  ┌─────────────────────────────────────────────┐
  │  Workspace                                  │
  │  ┌─────────┐  ┌─────────┐  ┌─────────┐     │
  │  │ Flow 1  │  │ Flow 2  │  │ Flow 3  │     │
  │  │ (Tab)   │  │ (Tab)   │  │ (Tab)   │     │
  │  └─────────┘  └─────────┘  └─────────┘     │
  └─────────────────────────────────────────────┘
```

#### 5.1.5 Message Model: Free Map with Immutable ID ✅

| Aspect | Decision |
|---|---|
| Data structure | **`map[string]any`** — all fields are equal, no struct with fixed fields |
| `_id` | Assigned automatically on creation (16 bytes random hex), **immutable** — cannot be overwritten via Set/Delete |
| Field access | Uniform via `Get("path.to.field")` / `Set("path.to.field", val)` — no difference between `payload`, `topic`, and arbitrary custom fields |
| JSON serialization | Flat JSON object: `{"_id":"abc","payload":42,"topic":"x","myField":true}` |
| Clone | Produces a deep copy with a **new** `_id` |
| Rationale | In Node-RED, `msg` is a free JS object — fields can be copied, moved, added arbitrarily. That's part of its success. Fixed Go structs would break this flexibility (first-class vs. second-class fields). |
| Node-RED comparison | Node-RED: `msg.payload = msg.topic` works immediately. LOOPZE: identical via `msg.Set("payload", msg.Get("topic"))`. |

```
  Protected fields (runtime-internal):
  ┌─────────────────────────────────────┐
  │  _id: "a1b2c3..."  ← IMMUTABLE      │
  └─────────────────────────────────────┘

  Free fields (user/node):
  ┌─────────────────────────────────────┐
  │  payload: { id: 42, name: "S1" }    │
  │  topic: "sensors/temp"              │
  │  original_payload: { ... }          │
  │  myCustomField: true                │
  │  _timestamp: "2026-03-17T..."       │
  └─────────────────────────────────────┘
```

---

## 6. Decision Log

> Decisions made are documented chronologically here.

| Date | Decision | Rationale |
|---|---|---|
| 2026-03-16 | Backend: **Go** | Performance, single binary, goroutines for parallel flow execution |
| 2026-03-16 | Frontend: **Vue 3 + Vue Flow** | Vue Flow is mature, MIT-licensed, used by n8n, ideal for a node editor |
| 2026-03-16 | Build tool: **Vite** | Standard for Vue 3, fast, simple |
| 2026-03-16 | Target audience: **technicians & PLC programmers** | Focus on industrial protocols, simple UX, script-based function nodes |
| 2026-03-16 | License tier 1: **Elastic License 2.0 (ELv2)** | Free to use, no embedding, no modification, no competing clone |
| 2026-03-16 | HTTP framework: **Chi** | 100% net/http compatible, minimal, Go-idiomatic, URL parameters + middleware |
| 2026-03-16 | WebSocket library: **gorilla/websocket** | De-facto standard, battle-tested, compatible with Chi |
| 2026-03-16 | UI styling: **Tailwind CSS + Radix Vue** | Custom components, maximum control, no off-the-shelf framework look |
| 2026-03-16 | Design: **Amber terminal retro look** | Consolas everywhere, square components, dark theme, amber accent — fits the technician audience |
| 2026-03-16 | Credentials: **separate encrypted file** (tier 1) + **central vault** in management (tier 2) | Clear separation, passwords never in flow JSON, tier 2 as a differentiator |
| 2026-03-16 | State management: **Pinia** | Official Vue 3 standard, TypeScript-native, minimal boilerplate |
| 2026-03-16 | Encryption key: **auto-generated keyfile** (`goflux.key`) | Simplest UX for technicians, key separate from credentials, AES-256-GCM |
| 2026-03-16 | Function-node scripting: **dual-mode** — JavaScript (Goja) + expr-lang | JS for complex logic, expr-lang for fast expressions with runtime compilation |
| 2026-03-16 | Project name: **LOOPZE** (edge app) + **LOOPZE Hub** (management platform) | Short, memorable, industrial — flintstone as a metaphor for small, hard, reliable |
| 2026-03-16 | Frontend embedded: **go:embed** | Single binary, no dependencies, maximum simplicity for technicians |
| 2026-03-16 | Storage: **JSON files** | Simple, readable, no setup — like Node-RED |
| 2026-03-16 | Flow format: **own LOOPZE format** | Cleanly designed, typed, extensible, no compromises from Node-RED compatibility |
| 2026-03-16 | Target platforms: **all** | Linux x64/ARM64/ARM32, Windows x64, macOS — Go cross-compile out of the box |
| 2026-03-16 | License LOOPZE Hub: **proprietary** | Closed source, full control, clear commercial model |
| 2026-03-16 | NATS hybrid model | LeafNode + KV + JetStream for hub/context/debug — Go channels for internal node/flow handling |
| 2026-03-17 | Node ports: **single input, multiple outputs** | Clear signal-flow model like in PLC/FBD — no merge ambiguity |
| 2026-03-17 | **Universal node debugging** | Any node can be put into debug mode via icon — shows IN + OUT messages |
| 2026-03-17 | **Wire debugging** | Individual signal wires can also be debugged — precise fault-finding without extra nodes |
| 2026-03-17 | **Naming: workspace → flow** | A **workspace** is the top-level container (replaces the ambiguous "flows" from Node-RED). A workspace contains multiple **flows** (tabs). Clear hierarchy without confusion. |
| 2026-03-17 | **Message: free `map[string]any` + immutable `_id`** | All fields equal as in Node-RED. `_id` is set on creation and protected against Set/Delete. Clone produces a new `_id`. |
| 2026-05-03 | **License switch: ELv2 → AGPL-3.0-or-later** | Real open-source license instead of source-available; strong copyleft + § 13 (SaaS) prevents embedding into proprietary products. Copyright on Dennis Bleul personally (was NiceClouds GmbH). Commercial re-licensing remains possible for AGPL-incompatible use cases. |

---

## Next Step

**→ All decisions made! ✅**
**→ Phase 1 can begin: set up the project skeleton.**
