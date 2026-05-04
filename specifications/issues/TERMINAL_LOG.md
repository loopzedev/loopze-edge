# Issue: Terminal Log — Make application log visible in the frontend

## Status: Proposed

## Problem description

The application log currently runs exclusively on **stdout** of the LOOPZE process (`cmd/loopze/main.go`, `slog.NewTextHandler(os.Stdout, …)`). To see the log you need access to the terminal in which LOOPZE was started — for deployed instances that means SSH, `journalctl`, Docker logs, etc.

For the operator in the browser, the log is therefore invisible. Typical questions like *"why did my deploy fail?"*, *"is my MQTT connect coming through?"* or *"why is the server logging so much right now?"* require switching to the server console every time.

Goal: Make the running log directly visible in the frontend — as a switchable full-screen window over the flow editor, with colored level indication and live streaming.

## View / rationale

The groundwork already exists:

- **WebSocket hub** (`internal/ws/hub.go`) with generic `Broadcast(eventType, payload)` — new event types can be added without architectural changes
- **REST pattern with limit param** (`GET /api/v1/debug/messages?limit=N` in `internal/api/handlers.go`) — exactly the pattern we need for `/api/v1/logs`
- **Standardized logger**: everything goes through `slog.Default()` — a single custom `slog.Handler` is enough to capture all log calls
- **Header icons + uiStore toggles** (`HeaderBar.vue`, `uiStore.ts`) — the pattern for a new toggle icon is established

Architecture decision: **Ring buffer in the backend** (no NATS stream). Logs are short-lived observation, not a persistent event log. On restart they are gone — that is accepted and matches today's stdout behavior.

## Requirements

### 1. Header icon

In `HeaderBar.vue`, to the right of the Info/Debug icon, a new "Terminal Log" icon (terminal/console symbol, matching the Lucide/Heroicon set currently used).

- Click toggles `uiStore.logsPanelOpen` (`true`/`false`)
- Active state: icon visually highlighted (same logic as Properties/Info icon)
- Tooltip: "Terminal Log" / "Show application log"

### 2. Full-screen overlay

Unlike the Properties and Info panels (which collapse into the right sidebar), the Terminal Log is an **overlay over the entire flow area** — as the operator knows it from the console window and as the user request explicitly describes: *"a window over the entire flow"*.

Layout:
- Position: absolute, covers the entire canvas area (HeaderBar stays visible, sidebars optionally covered — pragmatically: `inset-0` below the HeaderBar, z-index above the sidebars)
- Background: `bg-terminal-bg` with slight opacity gradation if needed, so the "floating window" character remains recognizable
- Close: X button at the top right of the panel + ESC key

Deliberately **not** a tab in the existing `InformationSidebar` — the log is not an inspect tool for a single element but an overview view and needs corresponding space.

### 3. Toolbar in the panel

At the top of the panel:
- **Limit dropdown**: `100 / 200 / 500 / 1000` lines. Default: `200`. Selection persists in `uiStore.logsLimit` (in `localStorage`)
- **Level filter** (optional Phase 1, presumably useful): multi-select `DEBUG / INFO / WARN / ERROR`. Default: all. Acts only client-side on the already loaded / streamed entries — no additional backend round-trip
- **CLR button**: clears the display (not the backend buffer). Same UX as DebugPanel
- **Auto-scroll toggle**: on = new entries scroll along. Switched off automatically on manual scroll-up, switched on again when the user scrolls to the end (pattern from `DebugPanel.vue`)

### 4. Lazy loading + streaming

**On opening** the panel:
1. `GET /api/v1/logs?limit=<dropdown-value>` fetches the last N entries from the ring buffer and populates the list
2. Frontend registers a WebSocket listener for `EventLog` and appends each incoming entry to the end of the list

**On closing**:
- WebSocket listener is deregistered (no state update, no re-renders while the panel is closed)
- The last displayed list is discarded — load fresh on next open

**On limit change** in the open panel:
- A fresh `GET /api/v1/logs?limit=N` replaces the list
- Streaming continues

This behavior fulfills the user requirement: *"The log should only be fetched when I open the log page, and new logs should stream in while the window is open."*

### 5. Level colors

Each line shows the level in square brackets and in color:

| Level | Color |
|---|---|
| `DEBUG` | gray (`text-zinc-500`) |
| `INFO`  | blue / standard foreground (`text-blue-400` or theme default) |
| `WARN`  | yellow (`text-yellow-400`) |
| `ERROR` | red (`text-red-400`) |

At a minimum, the **level tag** is colored; the message stays in the standard foreground. Full-line coloring (background tints for ERROR lines) is Phase 2 if desired.

### 6. Display format per line

A log line renders as monospace text:

```
10:23:14.428 [INFO ] starting LOOPZE version=0.1.0 commit=abc123 log_level=info
10:23:14.512 [WARN ] websocket broadcast channel full type=debug
10:23:15.001 [ERROR] failed to connect mqtt broker error="connection refused"
```

- Timestamp: local time, format `HH:mm:ss.SSS` (seconds + milliseconds — concise and sufficient for sequence analysis)
- Level right-aligned in a 5-character wide field (`INFO ` with space, so column alignment fits)
- Message + attributes rendered contiguously: `key=value` for each attr entry, values containing whitespace quoted with `"…"` (matches slog TextHandler output, which the operator is used to from the terminal)

## Technical sketch

### Backend — Ring buffer + slog.Handler

New package `internal/logbuffer` with:

```go
type LogEntry struct {
    Seq     uint64         `json:"seq"`     // monotonically increasing, assigned by the buffer
    Time    time.Time      `json:"time"`
    Level   string         `json:"level"`   // "DEBUG" | "INFO" | "WARN" | "ERROR"
    Message string         `json:"message"`
    Attrs   map[string]any `json:"attrs,omitempty"`
}

type Buffer struct {
    mu      sync.RWMutex
    entries []LogEntry // ring; size = capacity
    head    int
    full    bool
    cap     int
    seq     uint64     // incremented in Add() and set in entry.Seq
}

func New(capacity int) *Buffer
func (b *Buffer) Add(e LogEntry) LogEntry // returns entry with assigned Seq (for notify)
func (b *Buffer) Last(n int) []LogEntry   // oldest-first slice of the last min(n, len) entries
```

Capacity: configurable via `cfg.LogBufferSize` with default `1000`. The value is passed to `logbuffer.New` at startup in `cmd/loopze/main.go`. The UI maximum stays at `1000` (the dropdown cap); the backend capacity may be larger if e.g. an external API later wants to fetch more. Validation in `config`: minimum `1`, no maximum enforced.

The `Seq` ID is assigned by the buffer in `Add` and both written into the buffer and forwarded in the returned entry to the notify callback — so REST response and WebSocket stream have **the same** `Seq` values for the same entry.

New `slog.Handler` wrapper in `internal/logbuffer`:

```go
type Handler struct {
    inner  slog.Handler          // delegate for stdout (TextHandler)
    buf    *Buffer
    notify func(LogEntry)         // optional: callback for live broadcast
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    // 1) forward to inner (stdout behavior unchanged)
    if err := h.inner.Handle(ctx, r); err != nil {
        return err
    }
    // 2) convert to LogEntry, write to ring, notify
    entry := toEntry(r)
    h.buf.Add(entry)
    if h.notify != nil {
        h.notify(entry)
    }
    return nil
}

// Enabled, WithAttrs, WithGroup forwarded to inner.
```

Important: **stdout behavior remains 1:1 preserved** — anyone working with `journalctl` today will notice nothing. The wrapper is additive.

### Backend — Wiring in `cmd/loopze/main.go`

```go
buf := logbuffer.New(cfg.LogBufferSize) // default 1000, configurable
inner := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})

// Hub must exist before the logger so that notify can call it.
// Currently the hub is created in the server — either move hub creation into main,
// or set notify via two-stage hookup (see below).

handler := logbuffer.NewHandler(inner, buf, nil) // notify initially nil
slog.SetDefault(slog.New(handler))

srv, err := server.New(cfg, buf) // server gets buffer for REST endpoint
// After srv.New: hub exists. Set notify afterwards.
handler.SetNotify(func(e logbuffer.LogEntry) {
    srv.Hub().Broadcast(ws.EventLog, e)
})
```

Alternative: pull the hub out of the server and instantiate it in `main.go` — cleaner, but a larger refactor. **Pragmatic**: two-stage hookup with `SetNotify` (a single atomic pointer field is enough).

### Backend — New event type

`internal/ws/hub.go`:

```go
const (
    EventDebug        = "debug"
    EventStatus       = "status"
    EventDeploy       = "deploy"
    EventNotification = "notification"
    EventLog          = "log" // NEW
)
```

### Backend — REST endpoint

`internal/api/routes.go` registers `r.Get("/logs", h.GetLogs)`.

`internal/api/handlers.go`:

```go
func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
    limit := parseIntQuery(r, "limit", 200)
    if limit < 1 { limit = 1 }
    if limit > 1000 { limit = 1000 } // hard cap = buffer capacity
    entries := h.logBuffer.Last(limit)
    writeJSON(w, http.StatusOK, entries)
}
```

Pattern and param validation exactly like `GetDebugMessages`.

### Backend — Avoid self-reference

The WebSocket hub itself logs (`slog.Info("websocket client connected", …)`). These logs end up in the buffer via the wrapper and are then broadcast again via the hub to all clients — no loop, because the clients only receive the events, they don't send them back. Still, **caution** with future changes: If a hub broadcast ever logs itself (e.g. error path in `Broadcast`), an infinite loop arises. Protection: `Broadcast` may only log internally on "channel full" (currently the case), via `slog.Warn` — as long as warn logs are tied to the "channel full" condition, they are self-throttling.

### Frontend — `uiStore.ts`

Analogous to `propertiesPanelOpen` / `infoPanelOpen`:

```ts
const logsPanelOpen = ref(false)
const logsLimit = ref<100|200|500|1000>(200)
function toggleLogsPanel() { logsPanelOpen.value = !logsPanelOpen.value }
function openLogsPanel()   { logsPanelOpen.value = true }
function closeLogsPanel()  { logsPanelOpen.value = false }
```

`logsLimit` is persisted in `localStorage` (same pattern as `propertiesPanelWidth`).

### Frontend — `HeaderBar.vue`

New button block after the Debug/Info icon (HeaderBar.vue ~line 153):

```vue
<button
  @click="ui.toggleLogsPanel()"
  :class="['icon-btn', { active: ui.logsPanelOpen }]"
  title="Terminal Log"
>
  <TerminalIcon class="w-5 h-5" />
</button>
```

### Frontend — `TerminalLogPanel.vue` (new)

Component with the following structure:

```vue
<template v-if="ui.logsPanelOpen">
  <div class="absolute inset-0 z-40 bg-terminal-bg flex flex-col">
    <header>
      <select v-model="ui.logsLimit" @change="reload">
        <option :value="100">100 lines</option>
        <option :value="200">200 lines</option>
        <option :value="500">500 lines</option>
        <option :value="1000">1000 lines</option>
      </select>
      <LevelFilterChips v-model="levelFilter" />
      <button @click="entries = []">CLR</button>
      <button @click="ui.closeLogsPanel()">✕</button>
    </header>
    <div ref="scrollEl" class="flex-1 overflow-auto font-mono text-xs">
      <LogLine v-for="e in visibleEntries" :key="e.idx" :entry="e" />
    </div>
  </div>
</template>
```

`onMounted` (more precisely: `watch(() => ui.logsPanelOpen, …, { immediate: true })`):
- When opened:
  1. `unsubscribe = ws.onLog(stagedAdd)` register **first** — streaming entries land in a staging array from now on
  2. `const initial = await api.getLogs(limit)` — fetch REST response (comes with `Seq` IDs)
  3. `entries.value = initial; lastSeqFromHttp = initial.at(-1)?.seq ?? 0`
  4. Flush staging array into `entries.value`, discarding all entries with `seq <= lastSeqFromHttp` (= duplicates from the HTTP set)
  5. Switch `stagedAdd` to direct append
- When closed: `unsubscribe?.()`; `entries.value = []`

The order "WS subscribe → HTTP fetch → merge → switch" guarantees that no entry is lost between the REST response and the WebSocket subscribe (otherwise it would in the gap), and that duplicates are reliably detected (same `Seq` from REST and WS).

Auto-scroll: after each `addLog`, `nextTick` → if `scrollEl` was previously at the end (`scrollHeight - scrollTop - clientHeight < 4`), scroll to the end again. User scroll-up interrupts auto-scroll, scroll-to-bottom reactivates.

### Frontend — `useWebSocket.ts`

New dispatcher `onLog(cb: (e: LogEntry) => void): () => void` analogous to `onDebug`/`onStatus`. Since the panel is the only place that consumes log events, there is no global store for it — the listener registers only while the panel is open (see Lazy loading).

### Frontend — `useApi.ts`

New method:

```ts
async function getLogs(limit: number): Promise<LogEntry[]> {
  return request<LogEntry[]>(`/logs?limit=${limit}`)
}
```

### Frontend — `LogLine.vue` (new)

Small helper component for a single line:
- Time formatting
- Level class from mapping
- Attrs joined as `key=value`, values with whitespace quoted

Pure presentation, no own state.

## Affected files

### Backend
- `internal/logbuffer/buffer.go` (new) — ring buffer + `LogEntry` with `Seq` assignment
- `internal/logbuffer/handler.go` (new) — `slog.Handler` wrapper with `SetNotify`
- `internal/logbuffer/buffer_test.go` (new) — wraparound, Last(n), seq monotonicity, concurrency
- `internal/config/config.go` — new field `LogBufferSize int` (default `1000`, min validation), flag `--log-buffer-size`, env `LOOPZE_LOG_BUFFER_SIZE`
- `internal/ws/hub.go` — add `EventLog = "log"` constant
- `internal/server/server.go` — `New(cfg, buf)` signature, `Hub()` getter, REST routing call passes the buffer through to `api.NewHandler`
- `internal/api/handlers.go` — `GetLogs` handler, `logBuffer` field in handler struct
- `internal/api/routes.go` — `r.Get("/logs", h.GetLogs)`
- `cmd/loopze/main.go` — instantiate buffer with `cfg.LogBufferSize` + wrapper handler, after `server.New` wire `SetNotify` with hub broadcast

### Frontend
- `frontend/src/stores/uiStore.ts` — `logsPanelOpen`, `logsLimit`, toggle actions, localStorage persistence for `logsLimit`
- `frontend/src/components/HeaderBar.vue` — new icon button next to Debug/Info
- `frontend/src/components/TerminalLogPanel.vue` (new) — full-screen overlay
- `frontend/src/components/LogLine.vue` (new) — a log line with level color
- `frontend/src/views/FlowEditor.vue` — mount `<TerminalLogPanel />` (position: above the canvas)
- `frontend/src/composables/useWebSocket.ts` — `onLog` dispatcher
- `frontend/src/composables/useApi.ts` — `getLogs(limit)`

## Dependencies

None external. Uses:
- `log/slog` (standard library) — already in use
- existing WebSocket hub
- existing REST pattern (`/debug/messages?limit=N`)
- existing header icon / uiStore pattern

## Out of scope for Phase 1

- **Persistent log across restart** — buffer is in-memory, that is intentional (matches stdout behavior)
- **Backend-side filter / search** — client-side level filtering is plenty for 1000 entries
- **Full-text search in the frontend** — can be added later as a browser-typical Ctrl+F or search input field
- **Download/export** of the current log content — conceivable as a "Copy as text" button in Phase 2
- **Source filter** (only logs from certain packages) — slog records don't carry a reliable source without `AddSource`, we'll push for it only when the need arises
- **Multiple servers / cluster logs** — LOOPZE is single-instance, each browser sees the log of its connected server
- **Translate ANSI color codes from stdout into HTML** — slog produces no ANSI codes, so not relevant
- **Background tint for ERROR lines** — if the wish concretely arises, easily added

## Open questions

None.
