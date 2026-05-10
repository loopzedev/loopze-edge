<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NodeIdInput from './shared/NodeIdInput.vue'
import OpcuaBrowser from './shared/OpcuaBrowser.vue'

const flowStore = useFlowStore()
const { options: serverOptions, openNewConfig, openEditConfig } = useConfigSelector('opcua-server')

const server = useNodeProperty<string>('server', '')
const mode = useStructuralProperty<string>('mode', 'triggered', {
  port: 'inputs',
  derive: (v) => (v === 'static' ? 0 : 1),
})
// nodeIds is the on-disk shape: each entry is { id, name? } where name is
// the user's optional override of the BrowseName for the by-name output
// shape. Legacy workspaces with string[] are migrated on first edit.
interface NodeIdEntry { id: string; name?: string }
const nodeIds = useNodeProperty<NodeIdEntry[]>('nodeIds', [])

// migrateLegacyEntries normalises a freshly-loaded list: if any element is
// a plain string (legacy), we wrap it as { id }. Runs lazily on every read
// of nodeIds.value so it self-heals after a workspace import.
function migrateLegacyEntries(raw: unknown): NodeIdEntry[] {
  if (!Array.isArray(raw)) return []
  return raw.map((e: unknown) => {
    if (typeof e === 'string') return { id: e }
    if (e && typeof e === 'object' && 'id' in e) return e as NodeIdEntry
    return { id: '' }
  })
}
const entries = computed<NodeIdEntry[]>({
  get: () => migrateLegacyEntries(nodeIds.value as unknown),
  set: (v) => { nodeIds.value = v },
})
const attribute = useNodeProperty<string>('attribute', 'Value')
const outputShape = useNodeProperty<string>('outputShape', 'array')
const includeMetadata = useNodeProperty<boolean>('includeMetadata', true)
const interval = useNodeProperty<number>('interval', 1000)
const startupRead = useNodeProperty<boolean>('startupRead', true)

const modePresets = [
  { label: 'Static',    value: 'static'    },
  { label: 'Triggered', value: 'triggered' },
  { label: 'Dynamic',   value: 'dynamic'   },
]

const ATTRIBUTES = [
  { value: 'Value', label: 'Value' },
  { value: 'Description', label: 'Description' },
  { value: 'DisplayName', label: 'Display Name' },
  { value: 'BrowseName', label: 'Browse Name' },
  { value: 'DataType', label: 'Data Type' },
]

const OUTPUT_SHAPES = [
  { value: 'per-item', label: 'Per-item (one msg per NodeID)' },
  { value: 'array',    label: 'Array (one msg with all values)' },
  { value: 'object',   label: 'Object (one msg, NodeID → value map)' },
  { value: 'by-name',  label: 'By name (one msg, name → value map)' },
  { value: 'single',   label: 'Single (1 NodeID only)' },
]

function addNodeId() {
  entries.value = [...entries.value, { id: '' }]
}

function removeNodeId(idx: number) {
  const next = [...entries.value]
  next.splice(idx, 1)
  entries.value = next
}

function updateNodeIdField(idx: number, patch: Partial<NodeIdEntry>) {
  const next = [...entries.value]
  next[idx] = { ...next[idx], ...patch }
  entries.value = next
}

const browserOpen = ref(false)
const selectedServerConfig = computed(() => {
  if (!server.value) return undefined
  const cfg = flowStore.configs.find((c) => c.id === server.value)
  return cfg?.config as Record<string, unknown> | undefined
})

function openBrowser() {
  if (!server.value) return
  browserOpen.value = true
}

function handleBrowserSelect(items: Array<{ nodeId: string; displayName: string }>) {
  if (items.length === 0) return
  const existing = new Set(entries.value.map((e) => e.id))
  const additions: NodeIdEntry[] = items
    .filter((i) => !existing.has(i.nodeId))
    .map((i) => ({ id: i.nodeId, name: i.displayName ?? '' }))
  entries.value = [...entries.value, ...additions]
}

const isDynamic = computed(() => mode.value === 'dynamic')
const isStatic = computed(() => mode.value === 'static')
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Server">
      <template #action>
        <button
          v-if="server"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openEditConfig(server)"
        >Edit</button>
        <button
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openNewConfig()"
        >+ New</button>
      </template>
      <FormSelect v-model="server" :options="serverOptions" placeholder="Select server..." />
    </FormField>

    <FormField label="Mode">
      <ToggleGroup v-model="mode" :options="modePresets" />
    </FormField>

    <FormField label="Node IDs">
      <div class="flex flex-col gap-1.5">
        <p
          v-if="isDynamic"
          class="text-[10px] text-terminal-text-dim leading-relaxed"
        >
          Dynamic mode reads NodeIDs from <code class="font-mono">msg.nodeIds</code> on every input.
          The list below is a fallback used when the message doesn't carry it.
        </p>
        <div
          v-for="(entry, idx) in entries"
          :key="idx"
          class="flex items-stretch gap-1"
        >
          <div class="flex-[2] min-w-0">
            <NodeIdInput
              :model-value="entry.id"
              @update:model-value="updateNodeIdField(idx, { id: $event })"
            />
          </div>
          <div class="flex-1 min-w-0">
            <FormInput
              :model-value="entry.name ?? ''"
              placeholder="name (optional)"
              @update:model-value="updateNodeIdField(idx, { name: $event })"
            />
          </div>
          <button
            class="px-2 text-[10px] text-terminal-text-dim border border-terminal-border
                   hover:text-status-error hover:border-status-error transition-colors"
            @click="removeNodeId(idx)"
          >×</button>
        </div>
        <div class="flex items-center gap-2">
          <button
            class="px-2 py-0.5 text-[10px] uppercase tracking-wider
                   border border-terminal-border text-terminal-text-dim
                   hover:text-accent hover:border-accent transition-colors"
            @click="addNodeId"
          >+ Add NodeID</button>
          <button
            class="px-2 py-0.5 text-[10px] uppercase tracking-wider
                   border border-terminal-border text-terminal-text-dim
                   hover:text-accent hover:border-accent transition-colors disabled:opacity-50"
            :disabled="!server"
            @click="openBrowser"
          >Browse server…</button>
        </div>
      </div>
    </FormField>

    <OpcuaBrowser
      :open="browserOpen"
      :server-id="server"
      :server-config="selectedServerConfig"
      :existing-node-ids="entries.map((e) => e.id)"
      :multi-select="true"
      title="Browse OPC UA Server (Read)"
      @close="browserOpen = false"
      @select="handleBrowserSelect"
    />

    <FormField label="Attribute">
      <FormSelect v-model="attribute" :options="ATTRIBUTES" />
    </FormField>

    <FormField label="Output Shape">
      <FormSelect v-model="outputShape" :options="OUTPUT_SHAPES" />
    </FormField>

    <FormCheckbox v-model="includeMetadata" label="Include metadata (statusCode, timestamps)" />

    <SectionHeader v-if="isStatic" title="Static Mode">
      <div class="flex flex-col gap-2">
        <FormField label="Interval">
          <NumberInput v-model="interval" :min="0" unit="ms" />
        </FormField>
        <FormCheckbox v-model="startupRead" label="Read at startup" />
      </div>
    </SectionHeader>
  </div>
</template>
