# Issue: Debug Node – ON/OFF State Persistence & Backend Logic

## Status: Open

## Problem Description

The Debug node has a latching toggle button (ON/OFF) on its output side. Currently this state is only stored locally in the Vue component (`enabled = ref(true)`) and is lost when the browser is reloaded. The state must be persisted in `workspace.json` via the node's `config.active` field.

## Requirements

### 1. Persistence in workspace.json

- The ON/OFF state is mapped in the node config field `active` (boolean)
- Example workspace.json structure:
  ```json
  {
    "id": "node-uuid",
    "type": "debug",
    "config": {
      "property": "payload",
      "active": true
    }
  }
  ```
- Default value when the field is missing: `true` (ON)

### 2. Toggle → Dirty → Deploy Cycle

- When the toggle button is pressed, `config.active` changes in the frontend state
- The change marks the node as **dirty** (`flowStore.markNodeDirty(nodeId)`)
- The blue dirty indicator is shown on the node
- Only on **deploy** is the changed state written to `workspace.json`
- After a successful deploy the dirty state is reset

### 3. Initial State on Load

- When LOOPZE is opened in the browser, `config.active` is read from the loaded flow data
- The toggle button shows the persisted state correctly (ON/OFF)
- If never deployed, the default `active: true` applies

### 4. Frontend Filtering with Dirty State

- Filtering happens **on incoming** new messages in the frontend (`addMessage`), not retroactively on the existing list
- Messages that are already in the debug list **remain when the filter changes**
- When a Debug node is deactivated, only **new** incoming messages of this node are dropped
- When a Debug node is reactivated, new messages appear in the list again from that point on
- The **current toggle state of the frontend** applies, even if it is dirty (not yet deployed)
- The operator can activate/deactivate Debug nodes immediately without having to deploy first
- Only persisting the state requires a deploy – the input filtering takes effect immediately

### 5. Backend Logic: Debug Stream

- The Debug node **always** streams to the DEBUG stream – regardless of whether `active` is true or false
- The backend ignores the `active` state during message processing: every incoming message is published via `DebugFunc` to the NATS subject `debug.{flowId}.{nodeId}`
- **Filtering (show/suppress) happens exclusively in the frontend**, not in the backend

### 6. Architecture Decision: Always Stream vs. Adjust Subscription

**Two options were weighed:**

| | Option A: Always stream, frontend filters | Option B: Frontend adjusts subscriptions |
|---|---|---|
| **Principle** | Backend publishes all debug messages, frontend hides deactivated nodes | Frontend subscribes/unsubscribes per Debug node on toggle change |
| **Latency on toggle** | Immediate – pure UI filtering | Delay due to subscribe/unsubscribe roundtrip |
| **Dirty-state compatibility** | Trivial – frontend knows the local state and filters directly | Complex – subscription change without deploy requires a separate signal path to the WebSocket layer |
| **Message loss** | None – stream runs continuously | Possible – messages during the unsubscribe/subscribe transition are lost |
| **Traffic** | Slightly more – deactivated nodes also send | Less – only active nodes send |
| **Complexity** | Low – no subscription management | High – subscription state must be kept in sync with toggle state |

**Decision: Option A – always stream, frontend filters**

Rationale:
- Debug messages are small and typically low-volume
- The toggle must take effect **immediately**, even in the dirty state without deploy – this practically rules out subscription management
- No race conditions or message loss when toggling rapidly back and forth
- Significantly less complexity across the entire stack

### 7. Configuring the Debug Output

The operator can configure in the properties panel **which information** from the message is written into the debug, and **which content is shown as the node status**.

#### Debug Output (`output`)

Dropdown field **"Output"** with the following options:

| Value | Label | Description |
|---|---|---|
| `property` | `msg.` (+ input field) | Outputs a single property of the message (default: `payload`) |
| `message` | Complete message object | Outputs the entire `msg` as JSON |
| `gjson` | GJSON | Evaluates a GJSON path expression on the message |

- For `property`: additional text field for the property path (e.g. `payload`, `topic`, `payload.temperature`)
- For `gjson`: additional text field for the GJSON expression (e.g. `payload.items.#`, `payload.items.0.name`)
- Default: `property` with value `payload`

#### Node Status (`statusOutput`)

Checkbox **"Node status"** (max. 32 chars) with associated dropdown:

| Value | Label | Description |
|---|---|---|
| `same` | Identical to debug output | Shows the same content as the debug output as status |
| `property` | `msg.` (+ input field) | Shows a specific property as status |
| `gjson` | GJSON | Evaluates a GJSON path expression and shows the result as status |
| `count` | message count | Shows the number of received messages as status |

- Node status is optional (checkbox enables/disables the display)
- The status text is truncated to **max. 32 chars**
- Default: disabled

#### Config Structure in workspace.json

```json
{
  "id": "node-uuid",
  "type": "debug",
  "config": {
    "active": true,
    "output": "property",
    "property": "payload",
    "statusEnabled": false,
    "statusOutput": "same",
    "statusProperty": ""
  }
}
```

## Affected Files

### Frontend
- `frontend/src/components/nodes/DebugNode.vue` – read toggle state from config, on toggle change config + mark dirty
- `frontend/src/stores/flowStore.ts` – node config update and dirty tracking
- `frontend/src/stores/debugStore.ts` – filter the debug messages based on the current toggle state (incl. dirty)

### Backend
- `internal/nodes/debug.go` – read `active` field from config, but **always** stream messages
- `internal/flow/registry.go` – `DebugMessage` struct (no `active` field needed since the backend always streams)

---

## Extension: Interactive JSON Tree View in the Debug Sidebar

### Status: Open

### Background

Currently every debug payload in `frontend/src/components/DebugPanel.vue` (lines 59-69, 182-189) is rendered as plain, static text via `JSON.stringify(payload, null, 2)` in a `<pre>` block. For nested objects and arrays this means:

- No way to collapse uninteresting subtrees
- Paths must be derived visually from indentation
- Values or paths must be selected and copied manually – error-prone for long strings

### Goal

The payload is rendered as an interactive JSON tree, similar to browser DevTools or Node-RED. The operator can expand subtrees, copy paths and values with one click, and thus work with debug messages much faster.

### Requirements

#### 1. Collapsible Tree

- Objects and arrays are rendered as collapsible nodes with a twisty icon (`▶` / `▼`)
- Primitive values (string, number, boolean, null, undefined) are rendered inline without a toggle
- Initial expand state: **top level expanded, child nodes collapsed** (depth 1 visible)
- A preview is shown for collapsed nodes:
  - Object: `{ … } (5 keys)`
  - Array: `[ … ] (12 items)`
- The expand state is held **per message locally** and survives re-renders, but not closing the panel (in-memory, not persisted)

#### 2. Syntax Coloring

Values are colored by type, matching the existing terminal theme:

| Type | Style |
|---|---|
| `string` | green/teal, in quotes |
| `number` | orange/yellow |
| `boolean` | violet |
| `null` / `undefined` | dimmed gray, italic |
| Object/Array header | terminal-text in normal color |
| Keys | terminal-text-dim |

Existing status colors (`error`, `warn`) remain for the entire message card – the tree colors are only used in the default status (`debug`); on error/warn the tree is rendered in the respective status color (as today).

#### 3. Copy Actions per Node

When hovering over a tree row, two small icon buttons appear on the right:

- **Copy Path** – copies the property path relative to the root of the message payload
  - Notation: JavaScript property style with `[idx]` for arrays, e.g. `payload.items[0].name` or `temperature`
  - If the payload itself is not the entire `msg` object (i.e. `msg.property !== 'payload'` or a configured property path/GJSON), the path begins **relative to the displayed payload** – the sidebar shows what it shows, the copied path applies in the same frame of reference
- **Copy Value** – copies the value of the node
  - Primitives: raw value (string without quotes, number as string, etc.)
  - Objects/arrays: compact JSON without indentation (`JSON.stringify(value)`)

Visual feedback after the click:
- Icon switches to a checkmark for ~1 second
- No toast, no modal – consistent with the lean terminal style

#### 4. Behavior for Non-JSON Payloads

- `string`, `number`, `boolean` are still rendered inline without a tree, but with a **Copy Value** button at the line edge (hover)
- `null` / `undefined`: only display, no copy needed
- `format: 'buffer'`: out of scope – rendered as a string as today
- If `JSON.stringify` fails (e.g. circular reference), fall back to the current `String(payload)` rendering without a tree

#### 5. Performance

- Per message the recursive component only iterates over the **expanded** paths – children of collapsed nodes are not rendered into the DOM
- On the initial render of the list, only the top-level properties of each message are materialized
- Long string values (> 200 chars) are truncated with `…`; an **expand** click shows the full string (within the same message)

#### 6. Toolbar Extension per Message (Phase 2, optional)

In the header row of each message (right next to the format badge in `DebugPanel.vue:176-178`), two additional icon buttons on hover:

- **Expand All** – expands all nodes of this message
- **Collapse All** – collapses all but the top level

Phase 1 scope: per-node toggle only. Implement phase 2 only if real-world use shows the need.

### Implementation Plan

#### New Component: `JsonTreeView.vue`

Path: `frontend/src/components/debug/JsonTreeView.vue` (new subfolder for debug-specific UI building blocks)

Props:
```ts
interface Props {
  data: unknown
  path?: string                // current path (default '')
  rootKey?: string             // display name of the root node (e.g. 'payload' or property name)
  depth?: number               // current recursion depth (default 0)
  defaultExpandDepth?: number  // up to which depth initially expanded (default 1)
}
```

Behavior:
- Recursive self-reference for children
- Local `expanded` state per node (`ref<boolean>`), initialized via `depth < defaultExpandDepth`
- Path construction:
  - Object key: `path === '' ? key : ${path}.${key}`
  - Array index: `${path}[${idx}]`
  - Keys with special characters (dot, bracket, whitespace): bracket notation `${path}["weird.key"]`
- Copy function via `navigator.clipboard.writeText()` with a brief `copied` flag per button for the icon feedback

#### Integration in `DebugPanel.vue`

Replace the `<pre>` block (lines 182-189) with:

```vue
<JsonTreeView
  :data="msg.payload"
  :root-key="msg.property || 'payload'"
  :class="{
    'text-red-300': msg.status === 'error',
    'text-yellow-200': msg.status === 'warn',
  }"
/>
```

The `formatPayload` function (lines 59-69) becomes obsolete for objects/arrays. For primitive top-level values the `JsonTreeView` component internally renders the inline display directly with a copy button.

#### No New Dependency

Since the stack relies on TailwindCSS + Radix Vue with a deliberately minimal footprint and the styling is heavily terminal-themed, **no external JSON viewer lib** (vue-json-pretty etc.) is intentionally introduced. A custom recursive component in ~150–200 LOC fits seamlessly into the existing theme.

### Affected Files

#### New
- `frontend/src/components/debug/JsonTreeView.vue` – recursive tree component with collapse + copy

#### Changed
- `frontend/src/components/DebugPanel.vue` – replace `<pre>` block with `<JsonTreeView>`, remove `formatPayload` if no longer needed

#### Optional
- `frontend/src/utils/clipboard.ts` (if no central copy helper exists yet) – thin wrapper around `navigator.clipboard.writeText` with fallback

### Out of Scope

- Persisting the expand state across session boundaries
- Search/filter function within a single JSON tree
- Diff view between consecutive debug messages of the same node
- Edit mode for debug values (this is an inspector, not an editor)

---

## Extension: Hover Highlight in the Flow

### Status: Open

### Background

With many debug messages in the sidebar it is hard to tell which node in the flow a particular message originates from. The node name in the header row is visible, but with several nodes of the same or similar name the operator has to search in the flow.

### Goal

When hovering over a debug message row in the sidebar, the originating node in the flow editor is highlighted with a **dashed border**. When leaving the row the indicator disappears again. This makes it possible at a glance to associate which node produced which message.

### Requirements

#### 1. Hover Behavior

- Mouse enter on a debug row → node with matching `nodeId` gets a dashed border
- Mouse leave → highlight is removed
- Quick switching between rows: the highlight moves immediately, without flicker or afterglow
- No click required, purely hover-based (click stays reserved for selection)

#### 2. Visual Appearance

- **Border style**: `dashed`
- **Border color**: `accent` (`#58a6ff` from the theme) — clearly distinct from the gray default border
- **Border width**: 1px — identical to the normal border, so there is no layout shift of the nodes
- **No** box-shadow / glow — deliberately more subtle than the `selected` state, so the two states stay visually distinct
- If the node is already `selected`: `selected` styling takes precedence (glow + solid border remain), no overlay

#### 3. Edge Cases

- Hovered node does not exist in the currently visible flow (multi-flow / changed tab): highlight simply does not apply — no error
- Debug message without `nodeId` (should not happen anymore after the ID fix): no highlight
- Sidebar is closed while hover is active: highlight must be reset (via `onBeforeUnmount` of the sidebar or via a mouse-leave event)

### Implementation Plan

#### State

In `frontend/src/stores/flowStore.ts` new:

```ts
const hoveredDebugNodeId = ref<string | null>(null)

function setHoveredDebugNodeId(id: string | null) {
  hoveredDebugNodeId.value = id
}
```

Export both in the store return. Deliberately in `flowStore` (not `uiStore`), because semantically it is a node state and `BaseNode.vue` already accesses `flowStore`.

#### Sidebar (`DebugPanel.vue`)

Per message row:

```vue
<div
  v-for="msg in messages"
  :key="msg.id"
  v-memo="[msg.id]"
  @mouseenter="flowStore.setHoveredDebugNodeId(msg.nodeId)"
  @mouseleave="flowStore.setHoveredDebugNodeId(null)"
  ...
>
```

Note: listeners are registered at mount and are static — compatible with `v-memo`.

Additionally `onBeforeUnmount` in the `<script>` block: call `flowStore.setHoveredDebugNodeId(null)` in case the sidebar is closed while a hover is active.

#### Node (`BaseNode.vue`)

Computed:

```ts
const isDebugHovered = computed(
  () => flowStore.hoveredDebugNodeId === props.id,
)
```

Extend class binding:

```vue
:class="{ selected: props.selected, 'debug-hovered': isDebugHovered, ... }"
```

CSS rule in the `<style>` block (after `.selected`, so `.selected` takes precedence, or via a more specific selector):

```css
.loopze-node.debug-hovered:not(.selected) {
  border-style: dashed;
  border-color: var(--color-accent, #58a6ff);
}
```

### Out of Scope for Phase 1

- **LinkNode highlight**: `LinkNode.vue` has its own wrapper styling without the `.loopze-node` class — can be retrofitted, but is not critical for the diagnostic UX (link nodes typically do not produce debug messages)
- **Auto-pan/scroll** in the flow to the hovered node when it is outside the viewport
- **Bidirectionality** (hover over node → highlight all messages of this node in the sidebar)
- **Animation** / transition when the highlight changes

### Affected Files

#### Changed
- `frontend/src/stores/flowStore.ts` — new `hoveredDebugNodeId` ref + setter
- `frontend/src/components/DebugPanel.vue` — hover listener per row, cleanup on unmount
- `frontend/src/components/nodes/BaseNode.vue` — `isDebugHovered` computed, class binding, CSS rule

---

## Extension: Click-to-Jump to the Source Node

### Status: Open

### Background

The Debug sidebar shows the node name (or the first 8 characters of the node ID as a fallback) per message as a plain text label. With large flows or multiple tabs it can be tedious to find the source node manually — especially when it is outside the viewport or in a different flow tab.

### Goal

Clicking the node identification in a debug row jumps directly to the source node:
1. Switches the active flow tab if needed (`flowId` of the message)
2. Selects the node (properties panel opens)
3. Pans/zooms the Vue Flow canvas so the node is centered in the viewport

### Requirements

#### 1. Click Target

- The clickable area is the **node-name span** in the header row of each debug row (`DebugPanel.vue:156-165`) — i.e. exactly the area that already shows `nodeName` or the truncated `nodeId` fallback today
- Marked visually as interactive: `cursor-pointer`, subtle hover effect (e.g. underline or slight color shift)
- Tooltip on hover: `Jump to <nodeName> (<nodeId>)` — gives the operator clarity that this is an action
- No conflicts with the hover highlight (separate feature): hover **highlights** the node, click **jumps to it** and selects

#### 2. Jump Behavior

On click:

1. **Flow switch** (if needed): if `msg.flowId !== flowStore.activeFlowId`, then call `flowStore.setActiveFlow(msg.flowId)`. Vue Flow then renders the other flow.
2. **Selection**: `flowStore.selectNode(msg.nodeId)` — properties panel opens, `selected` state activated
3. **Pan/zoom to node**: Vue Flow's `setCenter(x, y, { zoom })` retrieves the node position from the loaded nodes and centers it. Current zoom is preserved (where sensible), alternatively a moderate default zoom (e.g. 1.0) — to be decided at implementation time based on look and feel

#### 3. Edge Cases

- **Node no longer exists** (deleted since the message): flow switch happens if needed, selection fails → no crash, optional brief notification ("Node no longer in flow")
- **Flow no longer exists**: no tab switch, no crash, optional notification
- **Node is outside the current zoom**: `setCenter` centers it, zoom remains unchanged — the node is guaranteed to be visible
- **Click during a running auto-scroll burst**: no impact — sidebar scroll behavior remains independent

### Implementation Plan

#### Cross-Component Bridge: Focus Request via Store

Since `useVueFlow('loopze-flow-editor')` is callable from anywhere but the pan call must happen **only after the render** of the new flow following any required flow switch, the trigger goes via a store state that is watched by the `FlowEditor`:

In `flowStore.ts`:

```ts
const focusRequest = ref<{ nodeId: string; flowId: string; ts: number } | null>(null)

function focusNode(nodeId: string, flowId: string) {
  if (flowId !== activeFlowId.value) {
    setActiveFlow(flowId)
  }
  selectNode(nodeId)
  // ts forces reactivity even on repeated clicks on the same ID
  focusRequest.value = { nodeId, flowId, ts: performance.now() }
}
```

In `FlowEditor.vue`:

```ts
const { setCenter, getNode } = useVueFlow('loopze-flow-editor')

watch(
  () => flowStore.focusRequest,
  async (req) => {
    if (!req) return
    await nextTick() // ensure that nodes are rendered after a flow switch
    const node = getNode.value(req.nodeId)
    if (!node) return
    const x = node.position.x + (node.dimensions?.width ?? 0) / 2
    const y = node.position.y + (node.dimensions?.height ?? 0) / 2
    setCenter(x, y, { duration: 300 })
  },
)
```

#### Sidebar (`DebugPanel.vue`)

Convert the node-name span into a `<button>` (or `<span role="button" tabindex="0">`), click handler:

```vue
<button
  class="text-[11px] font-medium truncate cursor-pointer hover:underline ..."
  :title="`Jump to ${msg.nodeName || msg.nodeId} (${msg.nodeId})`"
  @click="flowStore.focusNode(msg.nodeId, msg.flowId)"
>
  {{ msg.nodeName || msg.nodeId?.slice(0, 8) }}
</button>
```

`v-memo` compatibility: the click handler is a static property reference on the store — Vue mounts it once, no diff needed.

### Out of Scope for Phase 1

- **Jump into a sub-flow** (e.g. when sub-flows are introduced in the future)
- **Animation path** through the flow to the node (e.g. animated pan over intermediate nodes)
- **Highlight after the jump** (pulse / glow as confirmation) — the `selected` styling and the existing hover highlight are sufficient for now
- **Keyboard navigation** (tab through messages, Enter to jump) — useful, but a separate feature
- **Notifications** for missing node/flow — nice to have, fail silently for now

### Affected Files

#### Changed
- `frontend/src/stores/flowStore.ts` — `focusRequest` ref + `focusNode` action
- `frontend/src/components/DebugPanel.vue` — node-name span → clickable button
- `frontend/src/views/FlowEditor.vue` — watcher on `focusRequest`, call to `setCenter`

---

## Extension: Pin-Path — Highlight Attribute and Auto-Expand in All Messages

### Status: Open

### Background

The JSON tree view allows expanding individual nodes, but with many consecutive debug messages from the same node, each message has to be expanded manually to see a particular attribute. When an operator wants to e.g. observe `payload.temperature` over time, this is tedious.

### Goal

Clicking a node in the JSON tree **pins** the path. Consequence:
1. The pinned node is visually highlighted
2. **All messages of the same node** (past **and** future) auto-expand their tree so this path is visible — provided the attribute exists
3. Click again → pin removed, auto-expand disappears, trees fall back to the manual/default state

The operator can thus "follow" a property without touching every message.

### Requirements

#### 1. Pin Action

- New third hover button on the right per tree row, next to the existing `path` / `val` buttons in `JsonTreeView.vue:147-156`. Label e.g. `pin` (uppercase, same style)
- Click on `pin`:
  - If the path is not yet pinned → pin
  - If it is pinned → unpin (toggle)
- Pinning is always **per `nodeId`**: the same path in messages of another node is unaffected
- **Multiple pins per node** are allowed (set-based) — e.g. pin `payload.temperature` AND `payload.humidity` in parallel
- A pin exists only in memory; not persisted across session boundaries (out of scope for phase 1)

#### 2. Visual Highlighting

- **Pinned node**: subtly tinted background (`bg-accent/10` or similar) plus a 2px left marker in `accent` color, so it is findable at a glance
- The `pin` button itself, in the pinned state, shows a filled pin symbol or `★` instead of the word `pin` — clearly distinguishable from the default state
- **Path ancestors are NOT additionally highlighted** — otherwise the tree quickly looks cluttered. Only the leaf node is marked.

#### 3. Auto-Expand Logic

- A pinned path forces all **container nodes on its path** into the expanded state
- Example: pin on `payload.items[0].name` → forces `root`, `payload`, `payload.items`, `payload.items[0]` as expanded; `name` itself is a leaf, no toggle needed
- The pinned path **dominates** the manual toggle state: as long as the pin is active, an ancestor container cannot be collapsed (toggle is ignored or visually disabled). This is the clear rule "pinned = always visible"
- If the attribute **does not exist** in a particular message (e.g. the payload has `payload.foo` but no `payload.items`), the affected message remains in the default state. No error, no force-expand on something that is not there.

#### 4. Scope

- Pin applies to **messages of the same node** (`nodeId` match)
- Other nodes in the list are unaffected
- Filter (`debugStore.filter`) and suppression (`active === false`) remain in effect — pin does not override them

#### 5. Cleanup

- When a Debug node is deleted from the flow or its `active === false` is set: existing pins remain stored (they do no harm), could optionally be cleaned up — out of scope for phase 1
- "Clear all messages" (CLR button) clears the message buffer but **keeps pins** — pins follow nodes, not messages

### Implementation Plan

#### State (`debugStore.ts`)

```ts
// nodeId → set of pinned paths (relative to message payload root)
const pinnedPaths = ref<Map<string, Set<string>>>(new Map())

// Bumped on every pin change. Used as v-memo dependency in DebugPanel
// to force re-render of message rows so the new pin state takes effect.
const pinnedPathsVersion = ref(0)

function togglePinnedPath(nodeId: string, path: string): void {
  const map = new Map(pinnedPaths.value)
  const existing = map.get(nodeId)
  if (existing && existing.has(path)) {
    existing.delete(path)
    if (existing.size === 0) map.delete(nodeId)
    else map.set(nodeId, new Set(existing))
  } else {
    const next = new Set(existing ?? [])
    next.add(path)
    map.set(nodeId, next)
  }
  pinnedPaths.value = map
  pinnedPathsVersion.value++
}

function pinnedPathsForNode(nodeId: string): Set<string> {
  return pinnedPaths.value.get(nodeId) ?? EMPTY_SET
}
```

In the return: export `pinnedPaths`, `pinnedPathsVersion`, `togglePinnedPath`, `pinnedPathsForNode`.

#### Tree Component (`JsonTreeView.vue`)

- New prop: `nodeId?: string` — passed in by `DebugPanel` at the top-level call, forwarded in recursive calls
- Computeds:
  ```ts
  const pinnedPaths = computed(() =>
    props.nodeId ? debugStore.pinnedPathsForNode(props.nodeId) : EMPTY_SET,
  )

  const isPinned = computed(() => pinnedPaths.value.has(props.path))

  const isOnPinnedPath = computed(() => {
    for (const p of pinnedPaths.value) {
      if (p === props.path) return true
      if (p.startsWith(props.path + '.')) return true
      if (p.startsWith(props.path + '[')) return true
    }
    return false
  })
  ```
- The previous `expanded` ref remains (local manual), but the template/toggle behavior uses a computed wrapper:
  ```ts
  const effectiveExpanded = computed(() => isOnPinnedPath.value || expanded.value)

  function toggle() {
    if (!isContainer.value) return
    if (isOnPinnedPath.value) return // pin dominates manual toggle
    expanded.value = !expanded.value
  }
  ```
- Pin button: `v-if="props.nodeId"` (only sensible in the sidebar), clicks `debugStore.togglePinnedPath(props.nodeId, props.path)`
- Visual highlight of the pinned node: additional class on the header row, e.g. `bg-accent/10 border-l-2 border-accent -ml-1 pl-0.5`

#### `DebugPanel.vue`

- `<JsonTreeView :node-id="msg.nodeId" ... />`
- Extend the `v-memo` of the message row to `[msg.id, debugStore.pinnedPathsVersion]`. A pin toggle thus invalidates all items — functionally correct because pins are toggled rarely (not in the 100/s range)

### Edge Cases

- **Path with special characters**: `JsonTreeView.buildChildPath` already produces correct bracket notation for keys with dots/brackets. The pinned `props.path` thus matches exactly.
- **Pin at root** (`path === ''`): theoretically possible, but visually meaningless (root is expanded anyway). The `pin` button can be hidden at the root (it already uses `v-if="path"` for `path` copy today — analogous logic).
- **Identical `nodeId` across flows**: should not occur (UUIDs), but if it does: pin would apply "across" flow boundaries — acceptable, because `nodeId` is deterministically unique.

### Out of Scope for Phase 1

- **Persistence** of pins across session boundaries (LocalStorage)
- **Cross-node pins** (track the same path in messages of all nodes)
- **Inline value overview** in a separate "Pinned Values" bar at the top of the sidebar (interesting phase 2 feature)
- **Cleanup** of pins on deleting/deactivating a node
- **Pin management** (list of all active pins, remove individually)

### Affected Files

#### Changed
- `frontend/src/stores/debugStore.ts` — `pinnedPaths` map, `pinnedPathsVersion` counter, `togglePinnedPath`/`pinnedPathsForNode`
- `frontend/src/components/JsonTreeView.vue` — `nodeId` prop, pin button, `effectiveExpanded` computed, toggle block, highlight styling
- `frontend/src/components/DebugPanel.vue` — `nodeId` prop on `JsonTreeView`, extend `v-memo` with version counter
