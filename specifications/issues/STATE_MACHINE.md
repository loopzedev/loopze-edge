# State Machine Node — Example

## Scenario: Door Lock with PIN Code

A door has 3 states: `locked`, `unlocked`, `alarm`.
- Unlock only with the correct PIN (guard)
- After 3 failed attempts -> alarm (action counts attempts)
- After 10 seconds of unlock -> automatically locked again (delayed transition)

---

## Machine Definition (JSON)

```json
{
  "id": "doorLock",
  "initial": "locked",
  "context": {
    "failedAttempts": 0,
    "maxAttempts": 3
  },
  "states": {
    "locked": {
      "onEntry": ["logLocked"],
      "on": {
        "UNLOCK": { "target": "unlocked", "guard": "pinCorrect" },
        "WRONG_PIN": { "target": "locked", "actions": ["countFailure"] },
        "TRIGGER_ALARM": "alarm"
      }
    },
    "unlocked": {
      "onEntry": ["resetFailures", "notifyOpen"],
      "after": {
        "10000": "locked"
      },
      "on": {
        "LOCK": "locked"
      }
    },
    "alarm": {
      "onEntry": ["triggerAlarm"],
      "on": {
        "RESET": { "target": "locked", "actions": ["resetFailures"] }
      }
    }
  }
}
```

## Guards & Actions (JavaScript)

```javascript
return {
  guards: {
    pinCorrect: function(ctx, event) {
      return event.pin === "1234";
    }
  },
  actions: {
    countFailure: function(ctx, event) {
      ctx.failedAttempts = (ctx.failedAttempts || 0) + 1;
      node.log("Failed attempt #" + ctx.failedAttempts);

      // After maxAttempts -> trigger alarm
      if (ctx.failedAttempts >= ctx.maxAttempts) {
        node.send({ topic: "TRIGGER_ALARM", payload: {} });
      }
    },
    resetFailures: function(ctx, event) {
      ctx.failedAttempts = 0;
    },
    notifyOpen: function(ctx, event) {
      // Send message on port 1 (e.g. to MQTT)
      node.send({ topic: "door/status", payload: { open: true } });
    },
    triggerAlarm: function(ctx, event) {
      node.warn("ALARM: too many failed attempts!");
      node.send({ topic: "alarm/door", payload: { reason: "too_many_attempts", attempts: ctx.failedAttempts } });
    },
    logLocked: function(ctx, event) {
      node.log("Door locked");
    }
  }
}
```

---

## Flow Example

```
[Inject: topic="UNLOCK", payload={"pin":"0000"}]  ->  [State Machine]  ->  Port 0: [Debug: State Changes]
[Inject: topic="LOCK"]                             ->        v
                                                      Port 1: [MQTT Out: Action Messages]
```

### Test Sequence

| # | Input (msg.topic / payload)             | Expected State   | Port 0 Output                        | Port 1 Output                    |
|---|----------------------------------------|------------------|--------------------------------------|----------------------------------|
| 1 | `UNLOCK` / `{"pin":"0000"}`            | `locked`         | `changed: false` (guard blocks)      | -                                |
| 2 | `WRONG_PIN` / `{}`                     | `locked`         | `changed: false`, failedAttempts=1   | -                                |
| 3 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=2                     | -                                |
| 4 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=3                     | `TRIGGER_ALARM` (from action)    |
| 5 | `TRIGGER_ALARM`                        | `alarm`          | `locked->alarm`                      | `alarm/door` message             |
| 6 | `RESET` / `{}`                         | `locked`         | `alarm->locked`                      | -                                |
| 7 | `UNLOCK` / `{"pin":"1234"}`            | `unlocked`       | `locked->unlocked`                   | `door/status: {open: true}`      |
| 8 | *(wait 10s)*                           | `locked`         | `unlocked->locked` (auto-timer)      | -                                |

---

## What This Demonstrates

- **Guards:** `pinCorrect` checks the PIN before the transition is allowed
- **Actions with context:** `countFailure` increments failed attempts in the machine context
- **Entry actions:** `notifyOpen` and `triggerAlarm` send messages when entering a state
- **node.send():** Actions can emit messages on port 1 (e.g. to MQTT, Debug, etc.)
- **node.log/warn():** Diagnostic output in the debug panel
- **Delayed transitions:** `after: {"10000": "locked"}` — automatic locking after 10s
- **Self-transitions:** `WRONG_PIN` stays in `locked` but executes the action

---

## State Machine Inspector (Right Sidebar)

### Context

Once running, a state machine node carries two pieces of live runtime information that are otherwise invisible to the user:

1. **Current state** — which step the machine is in (e.g. `locked`, `unlocked`, `alarm`).
2. **Machine context** — the mutable JSON object actions read/write (e.g. `failedAttempts`, `maxAttempts`).

Today this can only be observed via Debug nodes downstream of the state machine, which is cumbersome — you cannot see the *current* state at rest, only transitions you happened to log. The Context Viewer (see `CONTEXT_VIEWER.md`) does not help here, because the machine context lives in-memory inside the node, not in the four NATS KV buckets.

### Requirement

A new tab **"State Machines"** in the Information Panel (right sidebar) — analogous to the existing `Context` tab — that lets the user inspect every state machine node in the active flow at runtime.

#### Functionality

- **List**: enumerate all `state-machine` nodes in the currently active flow, by node label / ID.
- **Selection**: clicking a node selects it and shows its detail view; persist the last selection in `uiStore` for convenience.
- **Current state**: prominently display the current state name.
- **State diagram (optional, v2)**: render the states declared in the machine definition and highlight the active one. v1 may show just the state name.
- **Available transitions**: list the events accepted in the current state (keys of `states[current].on` plus any `after` timer), so the user knows which events would do something right now.
- **Context browser**: display the machine context as a JSON tree (reuse `JsonTreeView.vue`); collapsed by default, expandable per key.
- **Transition history (optional, v2)**: rolling buffer of the last N transitions (`from -> to` plus event), useful for debugging.
- **Manual refresh**: button reloads state + context for the selected machine.
- **Auto-refresh toggle**: per-second polling, default **off**, persisted in `uiStore`. Same gating as the Context tab — only polls while the tab is open and the panel is not collapsed.
- **Read-only** in v1: no manual state injection or context editing from the UI (avoids racing with running flow logic). A "send event" textbox is a v2 candidate.

### Backend

#### New REST endpoints

Mounted under `/api/v1/state-machines` (analogous to `/api/v1/context`):

| Method | Path | Purpose |
|--------|------|---------|
| `GET`  | `/state-machines/flow/{flowID}` | List all state machine nodes in a flow with their current state |
| `GET`  | `/state-machines/flow/{flowID}/{nodeID}` | Full snapshot for a node: definition states, current state, context, available events, optional history |

**List response:**

```json
{
  "flowID": "f1",
  "machines": [
    { "nodeID": "n7", "label": "Door Lock", "currentState": "locked" },
    { "nodeID": "n9", "label": "Conveyor",  "currentState": "running" }
  ]
}
```

**Snapshot response:**

```json
{
  "flowID": "f1",
  "nodeID": "n7",
  "label": "Door Lock",
  "definition": { "id": "doorLock", "states": ["locked", "unlocked", "alarm"] },
  "currentState": "locked",
  "context": { "failedAttempts": 2, "maxAttempts": 3 },
  "availableEvents": ["UNLOCK", "WRONG_PIN", "TRIGGER_ALARM"],
  "history": [
    { "ts": "2026-05-08T08:30:01Z", "from": "alarm",    "to": "locked", "event": "RESET" },
    { "ts": "2026-05-08T08:30:05Z", "from": "locked",   "to": "unlocked", "event": "UNLOCK" }
  ]
}
```

#### Implementation notes

- The state machine runtime already holds `currentState` and `context` per node instance — expose a read accessor on the node (`Snapshot()` returning the data above).
- Registry lookup: handler resolves `flowID` + `nodeID` against the running flow registry, returns 404 on unknown flow / node, 400 if the node is not a `state-machine` type.
- History buffer: keep a small ring buffer (e.g. last 20 transitions) on the node instance. Cheap, bounded, no persistence.
- No new storage layer — everything lives in node memory; this endpoint is a pure read of in-process state.

### Frontend

#### Affected files

| File | Change |
|------|--------|
| `frontend/src/components/InformationSidebar.vue` | Add `'state-machines'` to the tab list |
| `frontend/src/stores/uiStore.ts` | Extend `InfoTab` with `'state-machines'`; add `stateMachineAutoRefresh`, `selectedStateMachineNodeID` |
| `frontend/src/components/StateMachinePanel.vue` | **New** — tab content |
| `frontend/src/stores/stateMachineStore.ts` | **New** — Pinia store for machine list + snapshot |

#### UI structure

```
┌─ Information ─────────────────────────────────────┐
│  [Help] [Config] [Context] [State Machines] [Debug]│
├───────────────────────────────────────────────────┤
│ Machine: [Door Lock (locked) ▼]                   │
│ [↻ Refresh]   Auto-refresh 1s: [ ☐ ]              │
├───────────────────────────────────────────────────┤
│ Current state:  locked                            │
│ Available:      UNLOCK, WRONG_PIN, TRIGGER_ALARM  │
├───────────────────────────────────────────────────┤
│ Context                                           │
│ ▸ failedAttempts   2                              │
│ ▸ maxAttempts      3                              │
├───────────────────────────────────────────────────┤
│ History (last 20)                                 │
│ 08:30:01  alarm   -> locked    (RESET)            │
│ 08:30:05  locked  -> unlocked  (UNLOCK)           │
└───────────────────────────────────────────────────┘
```

- Dropdown lists every state machine node in the active flow, label suffixed with current state for at-a-glance visibility.
- Refresh button reloads the snapshot for the selected machine.
- Auto-refresh checkbox toggles per-second polling (default: off), persisted in `uiStore.stateMachineAutoRefresh`.
- Polling gate identical to the Context tab: `stateMachineAutoRefresh && activeInfoTab === 'state-machines' && infoPanelOpen`.
- Empty state: when the active flow has no state machine nodes, the panel shows a hint instead of an empty dropdown.

### Acceptance Criteria

- [ ] Backend exposes `GET /api/v1/state-machines/flow/{flowID}` and `GET /api/v1/state-machines/flow/{flowID}/{nodeID}`
- [ ] New "State Machines" tab visible in the Information Panel
- [ ] Dropdown lists all state machine nodes in the currently active flow
- [ ] Detail view shows current state, available events, and machine context as a JSON tree
- [ ] Manual refresh button reloads the snapshot
- [ ] Auto-refresh toggle polls once per second while the tab+panel are open; stops on tab change or panel close
- [ ] Selecting a different machine in the dropdown updates the detail view without page reload
- [ ] Flows without any state machine nodes render an empty-state hint, not an error

### Out of scope (v1)

- **No state injection / context editing** from the UI (read-only).
- **No graphical state-diagram rendering** — current state name is sufficient initially; a visual diagram is a v2 candidate.
- **No event sending** ("fire UNLOCK from the panel") — would race with live flow inputs; v2 candidate.
- **No persistence of history across restarts** — history is in-memory only.
