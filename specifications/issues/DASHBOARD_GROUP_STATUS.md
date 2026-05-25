# Issue: Dashboard Group Status

## Status: Proposed

---

## Locked decisions (resolved before implementation)

| # | Decision | Rationale |
|---|----------|-----------|
| L-1 | Status is a **runtime value**, not a static config | Groups are config nodes with no inputs. Status comes from the flow at runtime — it must go through the hub, not the workspace config. |
| L-2 | A dedicated `ui-group-status` flow node feeds the group | Analogous to how display widgets connect to their group: a new node type is registered in the flow, references a group by ID, and any message arriving at its input becomes the group's current status. This is clean, composable, and avoids polluting the group config schema. |
| L-3 | Status is a discrete enum, not a free string | The dashboard renders semantic colors and badge labels from a closed set: `running`, `idle`, `warning`, `fault`, `ok`, `off`. Free strings would break visual consistency and CSS theming. |
| L-4 | Status resolves from `msg.status` first, then `msg.payload` | `msg.status` is the canonical field. `msg.payload` is a fallback when the value is one of the valid enum members. This allows the node to be driven directly by, e.g., a Switch node without a Change node in between. |
| L-5 | The left border and the status tag are both controlled by the same status value | They are two visual representations of the same semantic state — they never diverge. One data field drives both. |
| L-6 | Group header is **required** when a status is active | A status tag without a visible header makes no sense. If `showHeader` is false but a `ui-group-status` node references the group, the group header is rendered anyway (header auto-enables for the badge). |
| L-7 | Status is not persisted to `workspace.json` | Status is ephemeral runtime data. The hub caches the current value per group and replays it to fresh dashboard clients on connect. On server restart the status is cleared (groups appear without a status badge). |

---

## Context

The dashboard supports grouping widgets into named panels. Currently, all groups look identical — they have no visual differentiation beyond their title. In production use-cases (manufacturing, operations, monitoring) it is essential to see the **health or operational state** of a logical area at a glance, without reading individual widget values.

The screenshot below shows the target appearance: each group panel has a colored **left border** and a **status badge** in the top-right corner. The badge and border color both reflect the group's current runtime state.

Target states from the reference design:

| Status | Color | Use case example |
|--------|-------|-----------------|
| `running` | Green `#3fb950` | Machine is actively processing |
| `idle` | Amber `#d29922` | Machine is powered but not producing |
| `warning` | Orange `#f0883e` | Elevated but non-critical condition |
| `fault` | Red `#f85149` | Active fault / interlock tripped |
| `ok` | Teal `#39d3b0` | Explicit "all clear" signal |
| `off` | Muted `#4a5568` | Machine is switched off / disabled |

---

## Problem description

There is currently no mechanism to:

1. Push a semantic state from the flow into a dashboard group.
2. Render a per-group status indicator (colored border + badge) in the dashboard SPA.

Without this feature, operators must read individual widget values to determine the state of a production area. The goal is to provide an instant visual overview at panel level.

---

## View / rationale

The design follows the same pattern as widget value updates:

- A dedicated flow node (`ui-group-status`) is the **sole writer** of a group's status.
- It has one input port and no output ports (fire-and-forget).
- The hub stores one `GroupStatus` entry per group ID and broadcasts changes via the existing WebSocket channel.
- The dashboard SPA subscribes to status updates alongside widget value updates and applies them to the group component.

This approach is consistent with how `ui-text`, `ui-chart`, and other display widgets work. No new persistence layer is needed.

---

## Functional scope reference

| Feature | Scope |
|---------|-------|
| `ui-group-status` flow node | In scope |
| Left colored border on group panel | In scope |
| Status badge (top-right of group header) | In scope |
| Six-state enum: running / idle / warning / fault / ok / off | In scope |
| WebSocket push for group status changes | In scope |
| Hub replay on fresh client connect | In scope |
| `msg.status` + `msg.payload` fallback resolution | In scope |
| Group header auto-enable when status node is active | In scope |
| Status reset on server restart | In scope (by design — ephemeral) |
| Free-text / custom status labels | Out of scope (Phase 2) |
| Per-group status history / log | Out of scope |
| Status written directly from `ui-group` config (static) | Out of scope |

---

## Architecture overview

```
Flow
  [any node] ──msg──▶ [ui-group-status] ──hub.PushGroupStatus(groupID, status)──▶ Hub
                                                                                     │
                                                              ┌──────────────────────┘
                                                              │  cache: groupStatuses map[string]GroupStatus
                                                              │  broadcast: WS message {type:"group-status", id, status, ts}
                                                              ▼
                                                       Dashboard SPA
                                                       layoutStore.setGroupStatus(id, status)
                                                       ▼
                                              <GroupPanel :status="group.status">
                                                  left border color = statusColor[status]
                                                  badge label = statusLabel[status]
```

### Message resolution

```
msg arrives at ui-group-status input
  │
  ├─ msg.status is valid enum member?  ──yes──▶ use msg.status
  │
  └─ no ──▶ msg.payload is valid enum member?  ──yes──▶ use msg.payload
                │
                └─ no ──▶ log warning, discard message
```

---

## Requirements

### 1. `ui-group-status` node

#### 1.1 Registration

- Node type: `"ui-group-status"`
- Label: `"Group Status"`
- Category: `"dashboard"` (same palette section as other dashboard nodes)
- Inputs: 1
- Outputs: 0

#### 1.2 Config fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `group` | `string` (config ID) | `""` | The `ui-group` instance this node writes status to. Required. |
| `name` | `string` | `""` | Optional label shown in the flow editor. |

#### 1.3 Runtime behaviour

- On each incoming message, resolve the status value using the rule in §Architecture.
- Call `hub.PushGroupStatus(groupID, status, timestamp)`.
- The node itself does not pass the message downstream (no output port).
- If `group` is not configured or the referenced group does not exist, log a warning once (not per message).

#### 1.4 Editor appearance

- Same style as other `ui-*` nodes: dark teal/green left stripe, dashboard icon.
- Config panel fields: `Name` (text), `Group` (dropdown, lists all `ui-group` configs on the current tab/page or all pages).
- A short help text below the group selector: *"Send a message with `msg.status` set to one of: running, idle, warning, fault, ok, off."*

### 2. Hub changes

#### 2.1 New method

```go
// PushGroupStatus stores and broadcasts the current status for a dashboard group.
func (h *Hub) PushGroupStatus(groupID string, status GroupStatusValue, ts time.Time)
```

#### 2.2 GroupStatus cache

```go
type GroupStatusValue string

const (
    GroupStatusRunning GroupStatusValue = "running"
    GroupStatusIdle    GroupStatusValue = "idle"
    GroupStatusWarning GroupStatusValue = "warning"
    GroupStatusFault   GroupStatusValue = "fault"
    GroupStatusOk      GroupStatusValue = "ok"
    GroupStatusOff     GroupStatusValue = "off"
)

type GroupStatus struct {
    GroupID string           `json:"id"`
    Status  GroupStatusValue `json:"status"`
    Ts      time.Time        `json:"ts"`
}
```

- The hub maintains `groupStatuses map[string]GroupStatus` (keyed by group ID).
- On `PushGroupStatus`: update the map, broadcast the WS message.
- On fresh client connect: replay the entire `groupStatuses` map as individual `group-status` messages before the `ready` message.

#### 2.3 WebSocket message format

```json
{
  "type": "group-status",
  "id": "<ui-group config ID>",
  "status": "running",
  "ts": "2026-05-23T14:23:07.000Z"
}
```

This is broadcast on the same WebSocket channel as `widget` messages.

### 3. LayoutGroup type extension

Add `status` as an optional runtime field in the dashboard SPA types. This field is **not** part of the `GET /api/dashboard/layout` response (layout is static config). It is applied in the Pinia store at runtime.

```typescript
// types.ts — add to LayoutGroup
status?: GroupStatusValue   // set at runtime via WS, absent = no indicator shown

type GroupStatusValue = 'running' | 'idle' | 'warning' | 'fault' | 'ok' | 'off'
```

### 4. Dashboard SPA — layout store

```typescript
// In layout store action:
function applyGroupStatus(id: string, status: GroupStatusValue) {
  const group = groups.value.find(g => g.id === id)
  if (group) group.status = status
}
```

WebSocket handler adds a case for `type === 'group-status'` alongside the existing `type === 'widget'` case.

### 5. Dashboard SPA — GroupPanel component

The group panel component (wherever groups are rendered in `App.vue` / the layout renderer) must:

#### 5.1 Left border

- When `group.status` is set: render a 3–4 px left border in the status color.
- When `group.status` is absent: no left border (current behaviour preserved).
- The border replaces or overrides any existing left border styling.
- Border is rendered as a CSS `border-left` or an absolutely positioned `::before` pseudo-element — implementation choice.

**Status → color mapping:**

```typescript
const STATUS_COLORS: Record<GroupStatusValue, string> = {
  running: '#3fb950',
  idle:    '#d29922',
  warning: '#f0883e',
  fault:   '#f85149',
  ok:      '#39d3b0',
  off:     '#4a5568',
}
```

#### 5.2 Status badge

- Rendered in the top-right of the group header row, beside or replacing the existing right-side header area.
- Badge is only shown when `group.status` is set.
- Badge content: uppercase label (see below) with a small filled circle to the left.
- Badge background: `STATUS_COLORS[status]` at ~15% opacity; badge text: `STATUS_COLORS[status]`.

**Status → label mapping:**

```typescript
const STATUS_LABELS: Record<GroupStatusValue, string> = {
  running: 'RUNNING',
  idle:    'IDLE',
  warning: 'WARNING',
  fault:   'FAULT',
  ok:      'OK',
  off:     'OFF',
}
```

#### 5.3 Header auto-enable

If `group.status` is set and `group.showHeader` is `false`, the group renders its header row anyway (to display the badge). The name in the header may be empty/hidden if `showHeader` is false, but the badge row is always present.

---

## Technical sketch

### Backend

**New file:** `internal/nodes/dashboard/group_status.go`

```go
package dashboard

import (
    "loopze/internal/flow"
    "loopze/internal/dashboard"
)

type UIGroupStatus struct{}

func (n *UIGroupStatus) TypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:     "ui-group-status",
        Label:    "Group Status",
        Inputs:   1,
        Outputs:  0,
        Category: "dashboard",
    }
}

func (n *UIGroupStatus) Process(ctx flow.NodeContext, msg flow.Message) {
    groupID := ctx.Config().String("group", "")
    if groupID == "" {
        return
    }
    status := resolveStatus(msg)
    if status == "" {
        ctx.Warn("ui-group-status: msg.status / msg.payload is not a valid status value")
        return
    }
    dashboard.GlobalHub.PushGroupStatus(groupID, dashboard.GroupStatusValue(status), msg.Timestamp())
}

func resolveStatus(msg flow.Message) string {
    valid := map[string]bool{
        "running": true, "idle": true, "warning": true,
        "fault": true, "ok": true, "off": true,
    }
    if s, ok := msg.Get("status").(string); ok && valid[s] { return s }
    if s, ok := msg.Payload().(string); ok && valid[s] { return s }
    return ""
}
```

**Modify:** `internal/dashboard/hub.go` — add `groupStatuses` map, `PushGroupStatus` method, and replay logic.

**Modify:** `internal/nodes/dashboard/register.go` — register `UIGroupStatus` node type.

### Frontend (editor)

**New file:** `frontend/src/nodes/dashboard/UIGroupStatusConfig.vue`

- Simple form: `name` text field, `group` dropdown (same pattern as other `ui-*` config panels).
- Node appearance: same teal-ish stripe as other dashboard nodes.

**Modify:** `frontend/src/nodes/dashboard/index.ts` — register the new node type.

### Frontend (dashboard SPA)

**Modify:** `frontend/dashboard/src/types.ts` — add `status?` to `LayoutGroup`, add `GroupStatusValue` type.

**Modify:** `frontend/dashboard/src/stores/layout.ts` (or equivalent) — add `applyGroupStatus` action, handle `group-status` WS message type.

**Modify:** `frontend/dashboard/src/App.vue` (or group rendering component) — pass `status` prop to group panel, render left border and badge.

---

## Affected files

### Backend

| File | Change |
|------|--------|
| `internal/nodes/dashboard/group_status.go` | **New** — `UIGroupStatus` node implementation |
| `internal/nodes/dashboard/register.go` | Register `ui-group-status` |
| `internal/dashboard/hub.go` | Add `groupStatuses` cache, `PushGroupStatus`, replay on connect |

### Frontend — editor

| File | Change |
|------|--------|
| `frontend/src/nodes/dashboard/UIGroupStatusConfig.vue` | **New** — editor config panel |
| `frontend/src/nodes/dashboard/index.ts` | Register new node |

### Frontend — dashboard SPA

| File | Change |
|------|--------|
| `frontend/dashboard/src/types.ts` | Add `status?` field to `LayoutGroup`, add `GroupStatusValue` type |
| `frontend/dashboard/src/stores/layout.ts` | Add `applyGroupStatus`, handle WS message |
| `frontend/dashboard/src/App.vue` | Render left border + badge per group status |

---

## Tests

| # | Test | Verification |
|---|------|-------------|
| T-1 | Send `{status:"running"}` to `ui-group-status` | Dashboard group shows green left border and "RUNNING" badge |
| T-2 | Send `{payload:"fault"}` (no `msg.status`) | Fallback works — red border and "FAULT" badge |
| T-3 | Send `{status:"unknown"}` | Warning logged, no status change on dashboard |
| T-4 | Send `{status:"idle"}` then `{status:"running"}` | Status updates correctly — badge and border change |
| T-5 | Fresh dashboard client connects while status is active | Replay delivers current group status — badge visible immediately |
| T-6 | Server restart | Group status cleared — no badge on reconnect |
| T-7 | `showHeader: false` group receives status | Header row appears with badge; group name may be hidden |
| T-8 | `group` field not configured on node | Warning logged once; no crash |
| T-9 | Multiple `ui-group-status` nodes referencing same group | Last received message wins (last-write-wins) |
| T-10 | Six states sequentially | Each state shows correct color and label per the mapping table |

---

## Dependencies

No new external dependencies. Uses existing hub WebSocket broadcast infrastructure.

---

## Out of scope for Phase 1

- Custom status labels or colors defined by the user.
- Status history / timeline panel.
- Status written from a static config field on the group (without a `ui-group-status` node).
- Multiple simultaneous statuses per group (e.g., stacked badges).
- Transition animations on status change.

---

## Open questions

| # | Question | Suggestion |
|---|----------|-----------|
| Q-1 | Should `off` status suppress/dim the entire group card? | Probably yes — low opacity on the group body (not just the border) would make it immediately clear that the area is powered off. Leave for Phase 2. |
| Q-2 | Should the status badge be clickable (e.g., link to an alarm list)? | Potentially useful in production. Out of scope for Phase 1; could be added as an `href` config on the node. |
| Q-3 | Blinking border on `fault`? | CSS `animation: pulse` on the border for `fault` state would add urgency. Low implementation cost. Decision pending. |
