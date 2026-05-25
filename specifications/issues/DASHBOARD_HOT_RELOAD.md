# Issue: Dashboard Hot Reload

## Status: Proposed

Implementation plan for the `{type: "deploy"}` WebSocket frame that
already has its conceptual contract in
[DASHBOARD_NODES.md § Hot-deploy](./DASHBOARD_NODES.md). This issue
closes that open item from the dashboard project status.

---

## Locked decisions (resolved before implementation)

| # | Decision | Rationale |
|---|----------|-----------|
| L-1 | The deploy frame **carries the new layout inline** when `layoutChanged: true` | Atomic — no window where a `widget` frame arrives for a widget the client doesn't know yet, no second HTTP round-trip, no extra REST request to track in the network panel. The marginal WS-frame size cost (a few KB) is irrelevant on a local loopback. |
| L-2 | Layout-change detection via **stable hash of the serialized snapshot** | Cheap, deterministic, no false positives, no per-field comparison code to maintain. SHA-256 over the JSON bytes — collision probability is negligible for this use-case, and the cost is paid once per Deploy (not per message). |
| L-3 | **The client preserves state across reloads**: active page, sidebar collapsed, and widget values for IDs that still exist | A hot reload that wipes the operator's current view is worse than a full refresh — at least the latter is explicit. Vue's reactivity makes preservation the natural path: replace the layout ref, let the renderer diff. |
| L-4 | **Silent by default**, with a brief 600 ms pulse on the connection pill as the only visual cue | A toast on every save is noisy for the dev workflow (every Ctrl-S in the editor triggers one). The pulse is detectable when looked for but invisible during normal operation. |
| L-5 | **Fallback to REST refetch** when the WS deploy frame is malformed or the inline layout is absent | Robustness: the SPA stays consistent even if a new server version drops the inline layout, or a proxy strips it. Triggered when `frame.layoutChanged === true` but `frame.layout` is missing. |
| L-6 | Hot reload **does not reconnect the WebSocket** | The deploy event arrives over the existing socket; tearing it down would lose buffered widget frames. Only an actual disconnect triggers reconnect logic. |
| L-7 | The deploy frame is **always broadcast** after every successful Deploy, even when `layoutChanged: false` | Cheap diagnostic for "is the deploy hook wired up?" debugging. Clients ignore `layoutChanged: false` frames today; the field opens an upgrade path for flow-only signals later (e.g. "node X enabled/disabled") without protocol churn. |

---

## Context

The dashboard currently picks up layout changes only on a fresh WS
connect (the snapshot handshake replays the new layout). In practice
this means the operator has to manually reload the browser after every
editor Deploy that adds/moves/removes a group or widget — every iterative
cycle is a Ctrl-S in the editor followed by Ctrl-R in the dashboard.

The infrastructure for hot reload is already in place:

- `Engine.notifyDeploy` calls the registered `DeployListener` after every
  successful Deploy (`internal/flow/engine.go:815`).
- `Hub.RebuildLayout` already rebuilds the snapshot and prunes the cache
  (`internal/dashboard/hub.go:199`).
- The `msgTypeDeploy = "deploy"` constant exists with a `// PR 5`
  comment marker (`internal/dashboard/hub.go:33`).
- The dashboard SPA has a no-op `case 'deploy'` arm in `onFrame`
  awaiting wiring (`frontend/dashboard/src/App.vue`).

Only the broadcast + the client-side reload action are missing.

---

## Problem description

After every Deploy:

1. Connected dashboard clients see no notification.
2. Their layout snapshot is now stale.
3. Manual full-page reload is the only fix.

---

## Architecture overview

```
Editor → Engine.Deploy → Engine.notifyDeploy ──► Hub.RebuildLayout
                                                        │
                                                        │ (1) update snapshot
                                                        │ (2) cache.Retain(newWidgetIDs)
                                                        │ (3) hash new snapshot
                                                        │ (4) compare to lastHash
                                                        │ (5) broadcastDeploy(snap, changed)
                                                        ▼
                                              all connected clients
                                                        │
                                                        │ WS frame: {type:"deploy", layoutChanged, layout?}
                                                        ▼
                                              App.vue: onFrame('deploy')
                                                        │
                                                        ├─ layoutChanged=false → no-op
                                                        │
                                                        └─ layoutChanged=true:
                                                              ├─ frame.layout present → layout.value = frame.layout
                                                              └─ frame.layout absent → await fetchLayout()
                                                              ↓
                                                        Vue reactive re-render
                                                        ├─ surviving groups/widgets keep their values
                                                        ├─ active page preserved if still present (existing watch)
                                                        ├─ removed widgets purged from widgetValues
                                                        └─ 600 ms pulse on connection pill
```

---

## Wire protocol

### Updated `DeployFrame`

```typescript
// frontend/dashboard/src/types.ts
export interface DeployFrame {
  type: 'deploy'
  layoutChanged: boolean
  /** Present when layoutChanged is true. The client applies this
   *  directly instead of refetching via REST. */
  layout?: Snapshot
}
```

### Backend serialization

```go
// internal/dashboard/hub.go
type deployFrame struct {
    Type          string    `json:"type"`           // always "deploy"
    LayoutChanged bool      `json:"layoutChanged"`
    Layout        *Snapshot `json:"layout,omitempty"`
}
```

---

## Backend changes

### `internal/dashboard/hub.go`

1. **Add `lastLayoutHash` field to Hub:**

   ```go
   type Hub struct {
       // … existing fields …

       // lastLayoutHash is the hash of the most recently broadcast
       // layout snapshot. Used by RebuildLayout to suppress deploy
       // frames whose layout did not change. Guarded by snapMu since
       // it's only touched from RebuildLayout.
       lastLayoutHash [32]byte
       snapMu         sync.Mutex
   }
   ```

2. **Extend `Hub.RebuildLayout`:**

   ```go
   func (h *Hub) RebuildLayout(ws flow.Workspace) {
       snap := BuildLayout(ws)
       h.snapshot.Store(snap)
       h.cache.Retain(snap.WidgetIDs())

       h.snapMu.Lock()
       defer h.snapMu.Unlock()
       hash := snapshotHash(snap)
       changed := hash != h.lastLayoutHash
       h.lastLayoutHash = hash

       h.broadcastDeploy(snap, changed)
   }
   ```

3. **New `snapshotHash` helper:**

   ```go
   // snapshotHash returns a stable SHA-256 over the JSON-serialized
   // snapshot. Field order is deterministic because encoding/json
   // walks struct fields in declaration order; map keys are sorted.
   func snapshotHash(s *Snapshot) [32]byte {
       buf, _ := json.Marshal(s) // deterministic
       return sha256.Sum256(buf)
   }
   ```

4. **New `broadcastDeploy` method:**

   ```go
   func (h *Hub) broadcastDeploy(snap *Snapshot, changed bool) {
       frame := deployFrame{
           Type:          msgTypeDeploy,
           LayoutChanged: changed,
       }
       if changed {
           frame.Layout = snap
       }
       h.broadcastJSON(frame)
   }
   ```

### What does NOT change on the backend

- The snapshot handshake on fresh WS connect still replays the full
  current layout. Hot reload is an additive path on top of it.
- `Cache.Retain` already prunes purged-widget values — no changes
  needed for cache semantics.
- The `DeployListener` registration in `internal/server/server.go`
  already calls `Hub.RebuildLayout`. No engine-side change.

---

## Frontend changes

### `frontend/dashboard/src/types.ts`

Extend `DeployFrame` per the wire protocol section above.

### `frontend/dashboard/src/App.vue`

1. **Replace the no-op `case 'deploy'`** in `onFrame`:

   ```typescript
   case 'deploy':
     if (!frame.layoutChanged) break
     if (frame.layout) {
       layout.value = frame.layout
     } else {
       // Fallback: refetch when layout is missing (forward-compat
       // with future server versions, proxies that strip large
       // frames, etc.)
       fetchLayout().then((next) => { layout.value = next })
                    .catch((err) => console.warn('layout refetch failed', err))
     }
     pulseConnection()
     break
   ```

2. **Add `pulseConnection`** — a 600 ms class toggle on the connection
   pill to give a subtle visual cue:

   ```typescript
   const connPulse = ref(false)
   let pulseTimer: ReturnType<typeof setTimeout> | null = null
   function pulseConnection() {
     connPulse.value = true
     if (pulseTimer) clearTimeout(pulseTimer)
     pulseTimer = setTimeout(() => { connPulse.value = false }, 600)
   }
   ```

   ```html
   <span
     class="dash-pill"
     :class="{ pulse: connPulse }"
     :style="{ '--pill-color': connColor }"
   >
   ```

   ```css
   .dash-pill.pulse {
     animation: pill-pulse 600ms ease-out;
   }
   @keyframes pill-pulse {
     0%   { box-shadow: 0 0 0 0 var(--pill-color); }
     50%  { box-shadow: 0 0 0 6px color-mix(in srgb, var(--pill-color) 30%, transparent); }
     100% { box-shadow: 0 0 0 0 transparent; }
   }
   ```

### What does NOT change on the frontend

- Widget value cache (`widgetValues` ref) is **not** touched on reload.
  Vue's keyed `v-for` removes DOM nodes for vanished widgets; the
  values for IDs that no longer exist become unreachable garbage and
  are cleaned up next time the user navigates away or on the next
  fresh snapshot handshake. (A future PR can add explicit pruning, but
  it's not necessary for correctness.)
- `activePageId` watch already falls back to the first page when the
  selected page disappears — no change needed.
- Sidebar collapsed state already persists in `localStorage` — no
  change needed.

---

## Widget state preservation across hot reload

A key non-obvious property: **a chart widget with accumulated history
keeps all its data when an unrelated part of the flow is redeployed**.
This works because three independent layers all key on the widget's
stable node ID:

1. **Server-side cache**: `Cache.Retain(snap.WidgetIDs())` drops only
   entries for widgets that no longer exist. The chart's windowed
   history survives any deploy where the chart ID survives.
2. **Client-side value cache** (`widgetValues` ref): never mutated by
   the hot-reload path (decision L-3). The chart's last-known value
   prop stays referenced.
3. **DOM diff**: the group renderer uses `:key="widget.id"` on the
   widget `v-for`. Vue's keyed diff reuses the existing component
   instance for surviving IDs — **no remount, no internal-state loss**.
   Any in-component state (echarts instance, animation timers, scroll
   position) is preserved.

Even in the change-elsewhere case — where the snapshot hash changes
because of an unrelated widget update — the chart instance is reused
and its data buffer is untouched. The chart only resets if its ID
changes (deletion + recreation in the editor) or if Vue's `:key` is
ever changed to include mutable fields (do not).

This property is the reason for L-3 (preserve `widgetValues`) and for
the testing requirements T-11 (server cache) and T-13 (component
identity, see below).

---

## Edge cases

| # | Case | Behaviour |
|---|------|-----------|
| E-1 | Layout unchanged | Server sends `{layoutChanged:false}`, client ignores. No visual cue. |
| E-2 | Layout changed, all groups/widgets renamed but IDs preserved | Widget values survive (cache keyed by ID). Headers/labels update via reactivity. |
| E-3 | Active page deleted | Existing `pagesSorted` watch falls back to first page. |
| E-4 | Widget the operator is currently clicking gets removed | Vue removes the DOM node; the click handler is detached cleanly. No crash. |
| E-5 | Mid-deploy WS disconnect | Reconnect handshake delivers fresh snapshot. Same end state. |
| E-6 | Deploy frame received but `layout` field is missing despite `layoutChanged:true` | Client falls back to `fetchLayout()` via REST. |
| E-7 | Two deploys happen within 50 ms (rapid editor save) | Each triggers `RebuildLayout`; the hash check suppresses the second deploy frame if its layout is identical to the first. If both changed, both frames are broadcast — last write wins on the client. |
| E-8 | Hash collision (two different snapshots → same SHA-256) | Cryptographically negligible; if it ever occurred the client would miss one reload — recoverable via manual refresh. Not worth defending against. |
| E-9 | Client never received the initial snapshot (deploy arrives first) | The snapshot handshake on connect always wins because `RebuildLayout` writes to `Hub.snapshot` before broadcasting; the WS write goroutine guarantees ordering. |
| E-10 | Pulse animation interrupted by a second deploy within 600 ms | `pulseTimer` is cleared and restarted — animation restarts from frame 0. |

---

## Tests

| # | Test | Verification |
|---|------|-------------|
| T-1 | `RebuildLayout` with unchanged layout broadcasts `{layoutChanged:false}` and no layout | Capture broadcast bytes, decode, assert. |
| T-2 | `RebuildLayout` with changed layout broadcasts `{layoutChanged:true}` with full layout | Capture, decode, assert layout deep-equals. |
| T-3 | `snapshotHash` is stable across runs for the same input | Two calls return identical hashes. |
| T-4 | `snapshotHash` differs when a widget's `x` changes | Two snapshots differing only in `widgets[0].x` produce different hashes. |
| T-5 | Repeated `RebuildLayout` with the same workspace broadcasts only the first frame, then suppresses | (Actually: each call still broadcasts — see L-7 — but `layoutChanged` is `true` then `false`.) |
| T-6 | Two deploys in quick succession with truly different layouts: both broadcasts have `layoutChanged:true` | Hash-based diff, not time-based throttle. |
| T-7 | Frontend `onFrame('deploy', changed:true, layout:X)` replaces `layout.value` with `X` | Unit test the handler in isolation. |
| T-8 | Frontend `onFrame('deploy', changed:true, layout:undefined)` calls `fetchLayout()` fallback | Mock the API; assert called once. |
| T-9 | After hot reload, `activePageId` survives if the page still exists | Set active page, deploy with the same page present, assert unchanged. |
| T-10 | After hot reload, `activePageId` falls back to the first page if the active page was deleted | Set active page, deploy without it, assert fallback. |
| T-11 | After hot reload, widget values for surviving widget IDs are preserved | Push widget value, deploy, assert `widgetValues[id]` still set. |
| T-12 | Pulse class toggles on, then clears after 600 ms | Use fake timers; assert sequence. |
| T-13 | Widget component instance is reused (not remounted) when its ID survives a layout change | Mount, capture component instance ref, hot-reload with the same widget ID present, assert ref is the same instance. Guards against accidental `:key` regressions. |
| T-14 | Flow-only change (no widget edit) produces `{layoutChanged:false}` | Add a non-widget node to the flow, deploy, assert hash unchanged and chart cache survives unchanged on server. |

---

## Affected files

### Backend

| File | Change |
|------|--------|
| `internal/dashboard/hub.go` | Add `lastLayoutHash` field, extend `RebuildLayout`, new `snapshotHash` helper, new `broadcastDeploy`, new `deployFrame` struct. |
| `internal/dashboard/hub_test.go` | Add T-1, T-2, T-3, T-4, T-5, T-6. |

### Frontend

| File | Change |
|------|--------|
| `frontend/dashboard/src/types.ts` | Add `layout?: Snapshot` to `DeployFrame`. |
| `frontend/dashboard/src/App.vue` | Wire `case 'deploy'`, add `pulseConnection` + `connPulse` ref + CSS animation. |

No new dependencies. No protocol break — existing clients ignore the new
optional `layout` field gracefully (it's only consumed when present).

---

## Out of scope (deferred)

- Toast / status-bar notification on every reload (noisy in dev,
  noisier in production).
- Explicit cache pruning of `widgetValues` in the client (the dead
  entries are unreachable and small; cleanup adds complexity for no
  visible benefit).
- Optimistic per-widget partial updates (only resize/move/rename one
  group without touching the rest). The full-snapshot replace + Vue's
  keyed diff already gives sub-frame re-render performance.
- Reload throttling/debouncing. Hash-based suppression already covers
  the only spammy case (rapid identical-snapshot deploys).
- Disabling hot reload via a config flag. If someone needs that we'll
  add it then — until proven necessary, simpler is better.

---

## Open questions

| # | Question | Suggestion |
|---|----------|------------|
| Q-1 | Should the pulse use the *current* connection pill color (green) or the *accent* color? | Current pill color is more semantic ("the live channel just delivered something"). Accent could be confused with a connection-state change. Recommend current pill color. |
| Q-2 | Do we want a separate frame type for "flow-only redeploy" (e.g. for future telemetry like "node X started failing") instead of overloading the deploy frame's `layoutChanged:false` path? | Keep `deploy` as the one frame for now; revisit when there's a concrete second signal to send. |
| Q-3 | Should we expose the per-deploy hash on the snapshot endpoint too so a stale client can verify it has the right version before resyncing? | YAGNI for now. The atomic snapshot + WS-broadcast covers the use-case without needing client-side reconciliation. |
