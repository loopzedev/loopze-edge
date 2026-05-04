# Context Viewer: display values and delete manually

## Context

LOOPZE stores context data in four NATS JetStream KV buckets (see `internal/nats/context_store.go` and `internal/nats/broker.go`):

- **Global Memory** — `context-global-memory` (volatile)
- **Global Persistent** — `context-global-persistent` (file-backed)
- **Flow Memory** — `context-flow-{flowID}-memory` (volatile, per flow)
- **Flow Persistent** — `context-flow-{flowID}-persistent` (file-backed, per flow)

Function nodes read/write via `node.*`, `global.*` and `flow.*` (`internal/nodes/function.go`). The Context Watch node observes changes.

**Problem:** Currently there is neither a REST/WS interface nor a UI to inspect the stored context values or delete them selectively. When debugging flows, you have to rely on watch nodes and debug output, which is cumbersome.

## Requirement

A new tab **"Context"** in the Information Panel (right sidebar) that displays the context stores and allows targeted deletion of individual keys (as well as all keys of a store).

### Functionality

- **Browse**: display all four store variants (Global Memory, Global Persistent, Flow Memory, Flow Persistent)
- **Flow selection**: for flow stores, the currently active flow (or a selection from `flowStore.flows`) is used as scope
- **Key list**: for each store, display all keys with their current value (JSON-formatted)
- **Delete single**: delete a single key with one click (with confirmation)
- **Delete all**: delete all keys of a store (with confirmation — destructive)
- **Auto-refresh toggle**: user can turn the per-second polling on/off (default: **off** — user activates deliberately, state persisted in `uiStore`)
- **Manual refresh (full)**: button reloads all keys+values of the currently selected store — always works, regardless of auto-refresh
- **Manual refresh (single)**: a refresh icon per key row that reloads only that single key

### Update strategy

**Frontend polling instead of WebSocket push** — at one-second intervals (`setInterval(load, 1000)`), only active when all conditions are met:

- auto-refresh toggle is on (`ui.contextAutoRefresh === true`)
- the Context tab in the Information Panel is open (`ui.activeInfoTab === 'context'`)
- the Information Panel is not closed (`ui.infoPanelOpen`)

If auto-refresh is disabled, the user only has the manual refresh buttons (entire store / single key) for updates.

**Rationale:**

- A WebSocket watch (backend pushes every KV change) can easily produce hundreds of updates per second on "hot" counter keys — we don't want to route that through the WS
- Polling is trivial, has no backend state, stops automatically on tab change or panel close
- 1× HTTP GET/second per open panel is negligible; with n open browser tabs, at most n requests/second
- When leaving the tab / closing the panel, the interval is cleared

**Optional (later):**

- Edit values directly in the UI (set)
- Filter / search across keys
- If polling load does become a problem: WS push with server-side throttling (max 1 frame/second, aggregated)

## Backend

### New REST endpoints

Mounted under `/api/v1/context` (in `internal/api/routes.go`):

| Method | Path | Purpose |
|---------|------|-------|
| `GET` | `/context/global/{storage}` | All keys+values of a global store (`storage` ∈ `memory`, `persistent`) |
| `GET` | `/context/global/{storage}/{key}` | Load single key in global store (for single-key refresh) |
| `GET` | `/context/flow/{flowID}/{storage}` | All keys+values of a flow store |
| `GET` | `/context/flow/{flowID}/{storage}/{key}` | Load single key in flow store |
| `DELETE` | `/context/global/{storage}/{key}` | Delete single key in global store |
| `DELETE` | `/context/flow/{flowID}/{storage}/{key}` | Delete single key in flow store |
| `DELETE` | `/context/global/{storage}` | Delete all keys in global store |
| `DELETE` | `/context/flow/{flowID}/{storage}` | Delete all keys in flow store |

**GET response format:**

```json
{
  "scope": "global",
  "storage": "memory",
  "entries": [
    { "key": "counter", "value": 42 },
    { "key": "lastRun", "value": "2026-04-25T08:30:00Z" }
  ]
}
```

### Implementation

- New handler in `internal/api/handlers.go` (e.g. `handleGetContext`, `handleDeleteContextKey`, `handleClearContext`)
- Access to the KV stores via the existing `ContextProvider` from `internal/flow/context.go`
- `KVContextStore` already has `Keys()`, `Get(key)`, `Delete(key)` — no backend change to the storage layer needed
- "Delete All" iterates over `Keys()` and calls `Delete()` per key (alternatively purge the KV bucket if JetStream provides this easily)
- Error handling: unknown flow → 404, unknown storage name → 400

## Frontend

### Affected files

| File | Change |
|-------|----------|
| `frontend/src/components/InformationSidebar.vue` | Add new "Context" tab to tab list |
| `frontend/src/stores/uiStore.ts` | Extend `InfoTab` with `'context'` |
| `frontend/src/components/ContextPanel.vue` | **New** — tab content |
| `frontend/src/stores/contextStore.ts` | **New** — Pinia store for context data |

### UI structure

```
┌─ Information ────────────────────────────────┐
│  [Help] [Config] [Context] [Debug]           │
├──────────────────────────────────────────────┤
│ [Global Mem][Global Pers][Flow Mem][Flow Pers]│
│ Flow: [Current flow ▼]   (only for Flow-*)   │
│ [↻ Refresh]    Auto-refresh 1s: [ ☐ ]        │
├──────────────────────────────────────────────┤
│ ▸ counter      42              [↻] [✕]      │
│ ▸ lastRun      "2026-04-25..." [↻] [✕]      │
│ ▸ user         {…}             [↻] [✕]      │
├──────────────────────────────────────────────┤
│                           [Clear All]        │
└──────────────────────────────────────────────┘
```

- **4-button toggle** for store selection (Global Mem / Global Pers / Flow Mem / Flow Pers) — single state, fewer clicks than two dropdowns
- Flow dropdown only appears for flow scope
- Header refresh button (`↻ Refresh`) reloads the entire store
- A small refresh icon (`↻`) per row reloads only that key — useful when auto-refresh is off
- Auto-refresh checkbox toggles the per-second polling (**default: off**), state persisted in `uiStore.contextAutoRefresh`

- Values are rendered as a collapsed row (short preview), expandable on click as a JSON tree (reuse existing `JsonTreeView.vue`)
- Delete button with confirmation dialog (follow existing UI convention)
- "Clear All" marked red/destructive, with additional confirmation

### Pinia store (sketch)

```typescript
// contextStore.ts
const entries = ref<ContextEntry[]>([])
const scope = ref<'global' | 'flow'>('global')
const storage = ref<'memory' | 'persistent'>('memory')
const flowId = ref<string | null>(null)

async function loadAll() { /* GET /context/.../{storage} */ }
async function loadKey(key: string) { /* GET /context/.../{storage}/{key} → update entries[key] */ }
async function deleteKey(key: string) { /* DELETE … */ }
async function clearAll() { /* DELETE … */ }

// in ContextPanel.vue: poll only when auto-refresh + tab + panel open
let timer: ReturnType<typeof setInterval> | null = null
watchEffect(() => {
  const active =
    ui.contextAutoRefresh &&
    ui.activeInfoTab === 'context' &&
    ui.infoPanelOpen
  if (active && !timer) timer = setInterval(loadAll, 1000)
  if (!active && timer) { clearInterval(timer); timer = null }
})
```

**uiStore extension:**

```typescript
const activeInfoTab = ref<InfoTab>('debug')   // extended with 'context'
const contextAutoRefresh = ref<boolean>(true) // toggle, persisted (localStorage)
```

## Acceptance Criteria

- [ ] Backend returns keys + values for all four store variants via `GET /api/v1/context/...`
- [ ] Backend deletes individual keys and entire stores via `DELETE /api/v1/context/...`
- [ ] New "Context" tab visible in the Information Panel
- [ ] Scope/storage/flow switchable in the panel
- [ ] Keys are displayed with JSON value, individual keys deletable per button
- [ ] "Clear All" with confirmation dialog works
- [ ] Auto-refresh toggle updates values per second as long as Context tab+panel are open
- [ ] Auto-refresh can be disabled at any time, after which polling stops
- [ ] Manual refresh button reloads the entire store (even when auto-refresh is off)
- [ ] Per key row, the single-key refresh reloads only that one key
- [ ] Polling stops on tab change or closing the Information Panel

## Out of scope

- **No live update** — manual refresh is sufficient initially
- **No editing** of values (read + delete only)
- **No `node.*` scope** — that one is in-memory per node in the Function Node and not reachable via the KV stores
- **No filter/search** — added later if needed
