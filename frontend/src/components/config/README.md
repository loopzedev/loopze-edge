# Adding a Node Config Editor — Vue Frontend

This document explains how to wire up a new node type in the frontend editor.

## How node editors are loaded

`PropertyPanel.vue` uses two lazy registries:

| Registry | File | Used for |
|---|---|---|
| `NODE_EDITORS` | `nodeEditors.ts` | Regular flow nodes (inject, mqtt-in, s7-read, …) |
| `CONFIG_EDITORS` | `configEditors.ts` | Config nodes (mqtt-broker, s7-plc, …) |

Both map a node type string → `() => Promise<Component>`. The component is loaded on demand — no static imports, no switch/case needed in `PropertyPanel.vue`.

## Step-by-step: adding a new flow node editor

### 1. Create the Vue component

Create `frontend/src/components/config/<NodeType>Config.vue`.

Minimal skeleton:

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'

const flowStore = useFlowStore()
const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(patch: Record<string, unknown>) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, { config: { ...config.value, ...patch } })
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-col gap-1">
      <FormLabel>My Field</FormLabel>
      <FormInput
        :model-value="(config.myField as string) ?? ''"
        placeholder="..."
        @update:model-value="update({ myField: $event })"
      />
    </div>
  </div>
</template>
```

Look at `InjectConfig.vue`, `MqttNodeConfig.vue`, or `S7NodeConfig.vue` for real-world examples.

### 2. Register in nodeEditors.ts

Open `nodeEditors.ts` and add one line:

```ts
'bacnet-read': { loader: () => import('./BACnetReadConfig.vue') },
```

If multiple type strings share the same component (e.g. read + write), add one entry per type:

```ts
'bacnet-read':  { loader: () => import('./BACnetNodeConfig.vue') },
'bacnet-write': { loader: () => import('./BACnetNodeConfig.vue') },
```

If the editor needs the full panel height (code editors like `function`, `statemachine`), set `fullHeight: true`:

```ts
'bacnet-script': { loader: () => import('./BACnetScriptConfig.vue'), fullHeight: true },
```

That is the only change needed in existing files — `PropertyPanel.vue` picks it up automatically.

### 3. Add shared enums/constants (optional)

If your node has its own option lists (data types, function codes, address patterns), add them to `enums.ts` with a comment block header, e.g.:

```ts
// ── BACnet ─────────────────────────────────────────────────────────────────

export const BACNET_OBJECT_TYPES: OptionEntry<number>[] = [
  { value: 0, label: 'Analog Input' },
  { value: 1, label: 'Analog Output' },
  ...
]
```

Keep these in sync with the backend (`internal/nodes/bacnet_*.go`).

## Step-by-step: adding a new config node editor

Config nodes (shared connections like MQTT brokers, PLC connections) use `configEditors.ts` instead.

Open `configEditors.ts` and add one line:

```ts
'bacnet-device': () => import('./BACnetDeviceConfig.vue') as Promise<Component>,
```

The component receives a `configId` prop (the ID of the config node being edited) and manages its own save/cancel footer.
See `MqttBrokerConfig.vue` or `S7PlcConfig.vue` for complete examples.

## Node color palette

Node colors are defined in `nodes/tokens.ts`. To assign a color category to your new node type, add entries to the `TYPE_CATEGORY` map:

```ts
'bacnet-read':  'bacnet',  // reuse an existing palette, or…
```

To define a new palette, add an entry to the `TOKENS` map:

```ts
bacnet: {
  accent:     '#4f9',
  accentDim:  '#4f991a',
  // … see existing entries for the full set of required keys
},
```

## Node summary line (optional)

The properties panel shows a one-line summary below the node title. To add one, update `frontend/src/components/help/index.ts`:

```ts
case 'bacnet-read':
  return `Reading ${config?.objectType ?? '?'} from ${config?.deviceId ?? '?'}`
```

## Verify

```bash
cd frontend
pnpm type-check
pnpm lint
```
