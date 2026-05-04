# Issue: Node Enable/Disable — disable individual nodes without deleting

## Status: Done

## Problem description

Currently, a node that should temporarily not run in a flow has to be **deleted** — alternatively the entire flow can be disabled (`Flow.Disabled` exists at the data level, but not as a UI toggle). Both are inappropriate for the common case:

- **Debugging**: temporarily silence a problematic MQTT-Out without losing the configuration
- **Selective testing**: in a flow with several parallel branches, isolate one branch
- **Maintenance**: pause an inject trigger while the backend target service is restarting
- **Stepwise enabling** when building a flow: create nodes disabled, enable them later

In Node-RED, `disable`/`enable` via properties panel and right-click is one of the most-used editor operations. For LOOPZE it is completely missing, even though the data structure and the engine skip path are already prepared.

## View / rationale

The groundwork already exists — both backend and frontend already carry the field, only it isn't operable in the UI:

- `internal/flow/types.go:80` — `Node.Disabled bool` with JSON tag, part of persistence
- `internal/flow/engine.go:465-468` — `instantiateNode` skips disabled nodes before Init/Start
- `frontend/src/types/flow.ts:84` — `disabled?: boolean` in the node type
- `frontend/src/components/nodes/BaseNode.vue:25,42` — prop `disabled?: boolean` declared but visually unused
- `internal/flow/diff.go` + `Engine.Deploy(..., DeployModifiedNodes)` — partial deploy stops/starts individual nodes without flow restart

This makes the engine path clean: toggling the `Disabled` flag turns the node into a "modified node" on the next partial deploy, `stopAndRemoveNodes` stops it, `instantiateNode` skips it. No new lifecycle branch needed.

What is missing is the **UI operation**, the **visual representation**, and the **clean routing behavior** (no wire warnings for predictably missing targets).

## Requirements

### 1. Toggle in the properties panel

In the properties panel of the selected node, a general "Node" area appears **above** the type-specific config section with a checkbox `Enabled` (default `true`, mirrored to `disabled === false`).

- Toggle persists directly to `node.disabled` via the flow store (`flowStore.updateNode`)
- Marks the node as `dirty` (existing mechanism `dirtyNodeIds`) so it is considered on the next deploy
- Existing fields like `name` and (prospectively) `info` also belong in this general properties segment — a single toggle does not justify its own section, but it is the right place as a hook for future general node properties

### 2. Visual representation on the node

A disabled node must be recognizable as such at a glance:

| Element | Active | Disabled |
|---|---|---|
| Body opacity | `1.0` | `0.4` |
| Border style | `solid` | `dashed` |
| Status dot | as before | **hidden** (a disabled node has no live status) |
| Action/toggle buttons | active | rendered, but `pointer-events: none` and also `opacity: 0.4` |

The values are consumed in `BaseNode.vue` via the already-existing `disabled` prop. Selection and highlight remain visible — a disabled node must remain selectable, otherwise it cannot be re-enabled.

### 3. Wire behavior

Incoming wires to a disabled node:
- Backend: messages are dropped **silently** in the engine routing (no `slog.Warn`). Currently `makeSendFunc` warns on unknown targets — that is noise for nodes the operator deliberately disabled.
- Frontend: wire stays visible, but dashed and with reduced opacity (analogous to the node), so the break in the flow is recognizable.

Outgoing wires of a disabled node are irrelevant — the node does not run, it produces no messages. No additional code.

### 4. Keyboard shortcut

`Ctrl+E` / `Cmd+E` toggles the `disabled` state of the currently selected node(s).

**Bulk semantics on multi-select:**
- If **all** selected nodes are active → disable all
- If **all** selected nodes are disabled → enable all
- **Mixed** → enable all (the least-surprising choice: you reliably get out of the mixed state by pressing twice)

The shortcut is registered in `FlowEditor.vue` analogously to the existing Ctrl+C/V/X/D bindings — same skip logic on input/textarea focus.

### 5. On deploy

No special case:
- Mode `nodes` (default): a changed `disabled` flag turns the node into a modified node, the existing `deployModifiedNodes` path handles stop or start.
- Mode `flows` / `full`: everything is recreated anyway — disabled nodes are skipped on re-instantiation.

There is **no** "live disable" path that disables the node without a deploy — consistency with the rest of the system: changes only take effect after deploy, the `dirty` indicator shows this.

### 6. Persistence and workspace diff

- Toggle creates a `dirty` state in the frontend (`flowStore.markNodeDirty`)
- `WorkspaceDiff.ModifiedNodes` contains the node as soon as `disabled` differs between the last deployed and current state — this falls out automatically through the existing diff mechanism, since `Disabled` is part of the node hash, provided it works on the full structure. If the diff currently only hashes `Config`, `Disabled` must be added here — see Technical sketch.

## Technical sketch

### Backend — `internal/flow/engine.go`

`makeSendFunc` (l. 855-876): handle the `slog.Warn` line on `targetNode == nil` differently. Variant: before logging, check whether the target belongs to the engine wires map, but is known there as `disabled`. Cleaner: in `wireAllNodes`, populate the `targets` map only with actively running nodes (which it de-facto already is, since disabled nodes don't end up in `e.nodes`), and **lower the warning to debug level** — because the absence of a target is a normal case with the disable feature enabled, not warning-worthy:

```go
for _, targetID := range wires[port] {
    targetNode := targets[targetID]
    if targetNode == nil {
        slog.Debug("wire target not active (disabled or unknown)",
            "source", sourceID, "target", targetID, "port", port)
        continue
    }
    ...
}
```

If actual "wire into nothingness" (frontend bug, corrupt flow) should be logged separately later, a second set `e.knownNodeIDs` (all IDs from the flow, including disabled) can help — but that is out of scope.

### Backend — `internal/flow/diff.go`

Ensure that `Disabled` is part of the node comparison logic. If the existing diff compares nodes via JSON roundtrip or struct equality, `Disabled` is automatically included (it is an exported field). If it compares only `Config`, add it.

### Backend — Tests (`internal/flow/engine_test.go`)

| Test | Verifies |
|---|---|
| `TestDisabledNodeNotInstantiated` | Flow with `Node.Disabled = true` → node not in `e.nodes` (probably already exists implicitly) |
| `TestWireToDisabledTargetDropsSilently` | Active source → disabled target: no panic, no warning, message dropped |
| `TestPartialDeployDisableStopsRunningNode` | Node runs → deploy with `Disabled: true` (`DeployModifiedNodes`) → node goroutine ends |
| `TestPartialDeployEnableStartsNode` | Node is disabled, deploy with `Disabled: false` → node runs, receives messages |
| `TestDisableMidFlowDoesNotKillUpstream` | Disable middle node of a 3-hop flow → first node still runs, third gets no messages |

### Frontend — `frontend/src/components/PropertyPanel.vue`

New section above the type-specific editor:

```
┌────────────────────────────────┐
│ Node                           │
│ Name  [_____________________]  │
│ ☑ Enabled                      │
└────────────────────────────────┘
```

`Name` probably already exists as an edit field somewhere — if so, it stays there and only the checkbox is added. The checkbox binds to `!nodeData.disabled` and on toggle calls `flowStore.updateNode(nodeId, { disabled: <bool> })`.

### Frontend — `frontend/src/components/nodes/BaseNode.vue`

Existing `disabled` prop (l. 25, 42) is used visually:

```vue
<div
  class="node-body"
  :class="{ 'node-body--disabled': disabled }"
  :style="{ minHeight: nodeMinHeight }"
>
```

In the style block:

```css
.node-body--disabled {
  opacity: 0.4;
  border-style: dashed;
}
.node-body--disabled .status-dot { display: none; }
.node-body--disabled .action-btn,
.node-body--disabled .toggle-btn { pointer-events: none; }
```

The prop is passed through in the concrete node components (`MqttInNode.vue`, `InjectNode.vue`, etc.) — for most this happens implicitly via `v-bind="$props"` to `<BaseNode>`. Where it is missing, add it.

### Frontend — wire styling

In the Vue Flow edge configuration (presumably `FlowEditor.vue` or a `customEdges` file), render edges whose source **or** target is a disabled node with `stroke-dasharray: 4 4` and reduced opacity. Computed via `flowStore.activeNodes` as a lookup.

### Frontend — keyboard shortcut (`frontend/src/views/FlowEditor.vue`)

Add to the existing `keydown` handler (l. 134-172):

```ts
case 'e': {
  event.preventDefault()
  const ids = flowStore.selectedNodeIds
  if (ids.length === 0) return
  const nodes = ids.map(id => flowStore.getNode(id)).filter(Boolean)
  const allDisabled = nodes.every(n => n.disabled)
  const target = !allDisabled ? true : false
  // if all equal: flip; if mixed: to "false" (enable all) — see requirement
  const next = nodes.every(n => !!n.disabled === !!nodes[0].disabled)
    ? !nodes[0].disabled
    : false
  for (const n of nodes) flowStore.updateNode(n.id, { disabled: next })
  break
}
```

### Frontend — store helper (`frontend/src/stores/flowStore.ts`)

If `updateNode` doesn't exist yet or is too generic, a concrete method:

```ts
function setNodeDisabled(nodeId: string, disabled: boolean) {
  const node = getNode(nodeId)
  if (!node) return
  if (!!node.disabled === disabled) return
  node.disabled = disabled
  markNodeDirty(nodeId)
}
```

## Affected files

### Backend
- `internal/flow/engine.go` — `makeSendFunc`: warning to debug level for non-active targets
- `internal/flow/diff.go` — ensure `Disabled` is part of the node diff (adjust if needed)
- `internal/flow/engine_test.go` — new tests (see table above)

### Frontend
- `frontend/src/components/PropertyPanel.vue` — new "Node" section with Enabled checkbox
- `frontend/src/components/nodes/BaseNode.vue` — visual styling for `disabled`
- `frontend/src/components/nodes/*.vue` — ensure `disabled` is passed through to `BaseNode` (if not via `$props`)
- `frontend/src/views/FlowEditor.vue` — Ctrl+E shortcut, edge styling for disabled endpoints
- `frontend/src/stores/flowStore.ts` — `setNodeDisabled` helper, if not solvable via generic `updateNode`
- `frontend/src/components/help/docs.ts` — short hint in the general editor help entry that `Ctrl+E` toggles nodes

### Docs
- `docs/MISSING_FUNCTIONALITY.md` — set "Node Enable/Disable" entry to done, link to this issue

## Dependencies

- **Partial deploy** (`DeployModifiedNodes`) must work — done (`internal/flow/diff.go`, l. 369ff.)
- **Multi-select** in the frontend — present (`flowStore.selectedNodeIds`)
- **Dirty tracking** — present (`dirtyNodeIds`)
- No new external dependencies

## Out of scope for phase 1

- **Context menu (right-click)** with "Disable selected" / "Enable selected" — no context menu system exists in the editor. Own issue, because own UX subsystem (menu component, positioning, close behavior, additional entries like Copy/Paste/Delete).
- **Bypass mode** (passing messages *through* a disabled node instead of dropping) — semantically controversial (output port mapping with multiple inputs/outputs?). Implement only when a real use case appears.
- **Disabled for individual wires** — Node-RED doesn't have it, neither do we.
- **Disabled status in the status cache** — a disabled node has no live status; the last status known before disable is discarded on stop anyway, that is consistent.
- **Flow-wide toggle in the tab bar** — `Flow.Disabled` exists at the data level; the UI for it is its own thing (tab context menu etc.).
- **Per-subflow-instance toggle** — subflows don't exist yet.

## Open questions

- **Should the `dirty` state on toggle also be visually marked** (e.g. blinking dot), or is the existing indicator sufficient? → existing indicator is sufficient, no special handling.
- **Should disabled nodes be ignored by status aggregations** (Status node, Catch node)? → yes, automatically — they don't run and produce no status/error events. No additional code needed.
