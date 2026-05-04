# Issue: Flow management – tabs, create, delete, sort

## Status: Open

## Problem description

A workspace can contain multiple flows. The infrastructure for this already exists in the backend (`workspace.json` stores an array of flows) and in the FlowStore (`flows[]`, `activeFlowId`, `setActiveFlow()`). However, the **UI for managing multiple flows** is missing — currently only the first flow is loaded and there is no way to switch between flows, create new ones, or delete existing ones.

## Requirements

### 1. Flow tab bar

- Horizontal bar **above the FlowEditor**, below the HeaderBar
- Shows all flows of the workspace as tabs
- The active flow tab is visually highlighted
- Clicking a tab switches the active flow (`flowStore.setActiveFlow(flowId)`)
- At the **right end** of the tab bar there is a tab with a **plus icon** (+) to create a new flow

### 2. Create new flow

- Clicking the plus tab (+) opens the **Properties Sidebar** with a flow creation form
- The form contains:
  - **Name**: text field for the flow name (required)
  - **Create button**: creates the flow and activates it
  - **Cancel button**: closes the form without changes
- After creation:
  - The new flow is added to `flowStore.flows` (`flowStore.addFlow(label)`)
  - The new flow is automatically set as the active flow
  - The workspace is marked as **dirty**
  - The new flow tab appears in the tab bar
  - The FlowEditor shows an empty canvas

### 3. Edit flow (properties)

- **Double-click** on an existing flow tab opens the Properties Sidebar with the flow settings
- The flow properties show:
  - **Name**: text field for renaming the flow
  - **Enabled/Disabled**: toggle to disable the entire flow
  - **Delete button**: deletes the flow after confirmation
- Changes to name or enabled status mark the workspace as **dirty**
- A disabled flow is transmitted to the backend on deploy, but the engine starts no nodes for that flow

### 4. Disable flow

- A disabled flow gets `disabled: true` in the flow configuration
- The tab of a disabled flow is rendered visually weakened (e.g. reduced opacity, struck-through name, or grayed out)
- The operator can still open and edit a disabled flow
- Only after deploy is the disabled flow ignored by the backend
- Reactivation sets `disabled: false` and marks as dirty

### 5. Delete flow

- The delete button in the flow properties opens a **confirmation dialog**
- After confirmation:
  - The flow is removed from `flowStore.flows` (`flowStore.removeFlow(flowId)`)
  - If the deleted flow is the active flow, automatically switch to the next (or previous) flow
  - The workspace is marked as dirty
  - The last remaining flow **cannot** be deleted (button disabled or show a hint)

### 6. Sort tabs by drag & drop

- Flow tabs can be reordered by **drag & drop** within the tab bar
- During drag, the tab is visually lifted and a drop indicator shows the target position
- After drop, the order in `flowStore.flows` is updated
- The new order is persisted in `workspace.json` on the next deploy
- The reorder marks the workspace as dirty

## UI concept

```
┌─────────────────────────────────────────────────────────────────────┐
│ HeaderBar                                                           │
├────────────┬────────────┬────────────┬─────┬────────────────────────┤
│ ▪ Flow 1   │  Flow 2    │  Flow 3    │  +  │                        │  ← Flow tab bar
├────────────┴────────────┴────────────┴─────┴────────────────────────┤
│                                                                     │
│                         FlowEditor canvas                           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Tab states:**
- **Active**: highlighted background, bolder font
- **Inactive**: normal background
- **Disabled**: grayed out / reduced opacity
- **Dirty**: blue dot (●) like on nodes
- **Dragging**: slight elevation / shadow

**Properties Sidebar – New flow:**
```
┌──────────────────────┐
│  New flow            │
├──────────────────────┤
│  Name                │
│  ┌────────────────┐  │
│  │                │  │
│  └────────────────┘  │
│                      │
│  [Create] [Cancel]   │
└──────────────────────┘
```

**Properties Sidebar – Edit flow:**
```
┌──────────────────────┐
│  Flow settings       │
├──────────────────────┤
│  Name                │
│  ┌────────────────┐  │
│  │ Flow 1         │  │
│  └────────────────┘  │
│                      │
│  Status              │
│  [●] Enabled         │
│                      │
│  ──────────────────  │
│  [Delete flow]       │
└──────────────────────┘
```

## Data structure

### workspace.json (existing, no change)

```json
[
  {
    "id": "flow-uuid-1",
    "type": "tab",
    "label": "Flow 1",
    "disabled": false,
    "nodes": [...],
    "wires": [...]
  },
  {
    "id": "flow-uuid-2",
    "type": "tab",
    "label": "Flow 2",
    "disabled": true,
    "nodes": [],
    "wires": []
  }
]
```

The flow order matches the array order — drag & drop changes the array position.

## Affected files

### New files

- `frontend/src/components/FlowTabBar.vue` — horizontal tab bar with drag & drop
- `frontend/src/components/FlowProperties.vue` — flow form for Properties Sidebar (create/edit)

### Existing files (changes)

- `frontend/src/App.vue` — embed FlowTabBar between HeaderBar and FlowEditor
- `frontend/src/stores/flowStore.ts` — new actions: `reorderFlows(fromIndex, toIndex)`, `updateFlowLabel(flowId, label)`, `toggleFlowDisabled(flowId)`
- `frontend/src/components/PropertyPanel.vue` — render flow properties when no node but a flow is selected
- `frontend/src/stores/uiStore.ts` — new state: `selectedFlowForProperties: string | null` to distinguish between node properties and flow properties
- `frontend/src/views/FlowEditor.vue` — remove flow name from HeaderBar (now shown in the tab), keep initial loading of all flows

### Backend

- No backend changes needed — the existing API (`GET/POST /api/v1/flows`) and the engine already support multiple flows with `disabled` field

## Technical notes

### Drag & drop sorting

For tab sorting, native HTML5 drag & drop or a lightweight library like `vuedraggable` is recommended. The implementation should:
- Handle `dragstart`, `dragover`, `drop` events on the tabs
- Show a visual indicator at the drop position during drag
- Call `flowStore.reorderFlows(fromIndex, toIndex)` after drop

### Properties Sidebar context

The Properties Sidebar must distinguish between two modes:
1. **Node properties** (existing): when a node is selected on the canvas
2. **Flow properties** (new): when a flow tab is double-clicked or the plus tab is clicked

Priority rule: flow properties temporarily override node properties. On close or create/cancel, the sidebar returns to the previous node context.

### Flow name validation

- The flow name must not be empty
- Duplicates are allowed (flows are identified by their ID, not by name)
- Maximum length: 50 characters (for tab display)

## Dependencies

- No blocking dependencies — can be implemented independently of other issues
- The existing `flowStore.addFlow()` and `flowStore.removeFlow()` methods form the basis
- The `disabled` field already exists in the flow type (`types.go`) and in `workspace.json`
