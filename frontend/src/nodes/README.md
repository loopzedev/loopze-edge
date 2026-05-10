# Frontend Node Groups

This directory mirrors the backend's `internal/nodes/` structure: every node
group has its own subfolder containing the Vue config editors, optional
group-specific enums, and a `index.ts` manifest.

## Layout

```
frontend/src/nodes/
├── core/      ← inject, debug, function, change, switch, statemachine, …
├── modbus/    ← read, write, parser, server config
├── mqtt/      ← in, out, request, broker config
├── network/   ← tcp / udp / http nodes
├── opcua/     ← read, write, subscribe, server config (+ shared/)
├── s7/        ← read, write, parser, plc config
├── types.ts   ← NodeGroupManifest type
└── index.ts   ← aggregator that builds the global registries
```

The `name` field of every group's manifest **must match** the backend group
registered in `internal/nodes/<group>/init.go` — that string is the single
handshake between the two trees. A typo means the editor silently falls back
to the generic key/value renderer.

## How node selection works

When a user clicks a node on the canvas, `PropertyPanel.vue` reads the node
type and looks it up via `getFlowNodeEditor(type)`:

1. The aggregator (`./index.ts`) walks every group's manifest at module load
   and builds three registries (flow editors, config editors, palette
   categories).
2. The lookup returns a lazy `() => Promise<Component>` plus a `fullHeight`
   flag (used by code editors that need the whole panel).
3. `defineAsyncComponent` mounts the editor on demand — the bundle for an
   unused group is never fetched.

## Adding a new node group

1. Create the backend group first: see `internal/nodes/README.md`.
2. Create `frontend/src/nodes/<group>/` and add:
   - One `<NodeType>Config.vue` per node type
   - One `<ConfigType>Config.vue` per config-node type
   - `enums.ts` if the group has its own option lists
   - `index.ts` exporting a `NodeGroupManifest`:
     ```ts
     import type { Component } from 'vue'
     import type { NodeGroupManifest } from '../types'

     export const manifest: NodeGroupManifest = {
       name: 'bacnet',          // must match backend group name
       category: 'industrial',  // palette colour key (see nodes/tokens.ts)
       flowEditors: {
         'bacnet-read':  () => import('./BACnetReadConfig.vue') as Promise<Component>,
         'bacnet-write': () => import('./BACnetWriteConfig.vue') as Promise<Component>,
       },
       configEditors: {
         'bacnet-device': () => import('./BACnetDeviceConfig.vue') as Promise<Component>,
       },
     }
     ```
3. Add a single import line to `frontend/src/nodes/index.ts` `GROUPS` array.
4. If the group introduces a new palette colour, add an entry to
   `frontend/src/components/nodes/tokens.ts` `TOKENS` map.

That is the full surface — `PropertyPanel.vue`, `tokens.ts`, the per-group
files. No other touchpoints.

## Manifest fields

```ts
interface NodeGroupManifest {
  name: string                                   // backend group name
  category?: string                              // default palette colour
  categories?: Record<string, string>            // per-type override
  flowEditors: Record<string, () => Promise<Component>>
  configEditors?: Record<string, () => Promise<Component>>
  fullHeightEditors?: string[]
}
```

- Use `category` when every node in the group shares one palette colour
  (e.g. all S7 nodes are "rust").
- Use `categories` when a group spans multiple visual families
  (e.g. network covers HTTP / TCP / UDP, each with its own colour).
- Use both: `categories` takes precedence over `category` for listed types.

## Group-specific enums vs. shared enums

| Where | What | Used by |
|---|---|---|
| `frontend/src/nodes/<group>/enums.ts` | Group-specific lists & validators | The group's editors only |
| `frontend/src/components/config/enums.ts` | Cross-group constants | Shared editors (MsgFieldEditor, ValueTypeInput, InjectConfig, ChangeConfig, …) |

Adding a new MQTT QoS level → `frontend/src/nodes/mqtt/enums.ts`. Adding a new
value-type for the global `msg.<...>` picker → the shared file. The shared
file must NOT grow with protocol-specific entries.
