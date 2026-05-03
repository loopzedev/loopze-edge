# Information Panel: Tab System with Help, Config, and Debug

## Context

The information panel (right sidebar, `InformationSidebar.vue`) currently shows only the debug panel as its sole content. A tab navigation and additional important panels for the daily workflow are missing.

## Requirement

The information panel is extended with a **tab system** containing three tabs:

### Tab 1: Help

- Shows the **help/documentation** for the currently selected node type
- Reacts to node clicks in the editor (`flowStore.selectedNode`)
- If no node is selected: placeholder text ("Click a node to view its help")
- Content per node type:
  - Description / purpose of the node
  - Explanation of input/output ports
  - Description of configurable properties
  - Examples / usage notes
- Help texts can initially come from `NodeTypeInfo` (backend: `Description` field) and later be extended with more detailed Markdown documentation

### Tab 2: Config

- Lists all **config nodes** of the workspace (e.g. MQTT broker connections, DB connections)
- Data source: `flowStore.configs` (array of `ConfigNode`)
- Each entry shows: name, type, status (connected/disconnected if available)
- Clicking a config entry opens the config editor in the property panel (`ui.openConfigEditor(type, id)`)
- Button to create new config nodes

### Tab 3: Debug (existing)

- Already implemented as `DebugPanel.vue`
- Adopted 1:1 into the tab
- Existing behavior remains unchanged (filter, ON/OFF, CLR, auto-scroll, message display)

## Technical Details

### Affected Files

| File | Change |
|-------|-----------|
| `frontend/src/components/InformationSidebar.vue` | Add tab navigation, render tabs |
| `frontend/src/components/HelpPanel.vue` | **New** — help display for selected node |
| `frontend/src/components/ConfigPanel.vue` | **New** — config node list |
| `frontend/src/components/DebugPanel.vue` | No change, embedded as a tab |
| `frontend/src/stores/uiStore.ts` | Add `activeInfoTab` state |

### State

```typescript
// uiStore
const activeInfoTab = ref<'help' | 'config' | 'debug'>('debug')
```

### Existing Infrastructure

- **Node selection**: `flowStore.selectedNode` — reactive, already implemented
- **Config nodes**: `flowStore.configs` — already loaded and available
- **Config editor**: `ui.openConfigEditor(type, id)` — already implemented
- **Node type info**: comes from the backend via registry, contains `Description`, `Category`, `Inputs`, `Outputs`

## UI Mockup

```
+- Information --------------------+
|  [Help]  [Config]  [Debug]       |  <- Tab bar
+----------------------------------+
|                                  |
|  Tab content                     |
|                                  |
|                                  |
+----------------------------------+
```

## Out of Scope

- The detailed Markdown help per node type is NOT part of this issue — initially the `Description` from `NodeTypeInfo` is sufficient
- Config node status display (connected/disconnected) is optional and can be added later
