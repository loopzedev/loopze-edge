<script setup lang="ts">
import { computed, ref } from 'vue'
import { useConfigSelector } from '@/composables/useConfigSelector'
import { useNodeProperty } from '@/composables/useNodeProperty'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NodeIdInput from './shared/NodeIdInput.vue'
import DataTypeSelect from './shared/DataTypeSelect.vue'
import OpcuaBrowser from './shared/OpcuaBrowser.vue'
import { useFlowStore } from '@/stores/flowStore'

const flowStore = useFlowStore()
const { options: serverOptions, openNewConfig, openEditConfig } = useConfigSelector('opcua-server')

interface WriteEntry {
  nodeId: string
  dataType: string
  structureType?: string
  valueSource: string // "msg" | "static"
  valuePath: string
  value: string
}

const server = useNodeProperty<string>('server', '')
const rawMode = useNodeProperty<string>('mode', 'static')
const writes = useNodeProperty<WriteEntry[]>('writes', [])
const passthrough = useNodeProperty<boolean>('passthrough', false)
const disableTypeCache = useNodeProperty<boolean>('disableTypeCache', false)

const mode = computed({
  get: () => rawMode.value,
  set: (v: string) => {
    if (v === 'static' || v === 'dynamic') rawMode.value = v
  },
})

const modePresets = [
  { label: 'Static',  value: 'static'  },
  { label: 'Dynamic', value: 'dynamic' },
]

const VALUE_SOURCES = [
  { value: 'msg',    label: 'msg.<path>' },
  { value: 'static', label: 'static value' },
]

function blankEntry(): WriteEntry {
  return { nodeId: '', dataType: 'Double', structureType: '', valueSource: 'msg', valuePath: 'payload', value: '' }
}

function addEntry() {
  writes.value = [...(writes.value ?? []), blankEntry()]
}

function removeEntry(idx: number) {
  const next = [...(writes.value ?? [])]
  next.splice(idx, 1)
  writes.value = next
}

function updateEntry(idx: number, patch: Partial<WriteEntry>) {
  const next = [...(writes.value ?? [])]
  next[idx] = { ...next[idx], ...patch }
  writes.value = next
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

interface BrowsedItem {
  nodeId: string
  displayName: string
  dataType?: string
  structureType?: string
}

function handleBrowserSelect(items: BrowsedItem[]) {
  if (items.length === 0) return
  const item = items[0]
  // Single-select wiring: dump straight into the next/blank entry, or add
  // a fresh row when nothing is empty.
  const list = [...(writes.value ?? [])]
  let target = list.findIndex((e) => !e.nodeId)
  if (target === -1) {
    target = list.length
    list.push(blankEntry())
  }
  list[target] = {
    ...list[target],
    nodeId: item.nodeId,
    dataType: item.dataType ?? list[target].dataType,
    structureType: item.structureType ?? '',
  }
  writes.value = list
}

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

    <p
      v-if="!isStatic"
      class="text-[10px] text-terminal-text-dim leading-relaxed"
    >
      Dynamic mode reads writes from <code class="font-mono">msg.writes</code> (array of
      <code class="font-mono">{nodeId, dataType?, value}</code>) or from a single
      <code class="font-mono">msg.nodeId + msg.payload</code> pair.
    </p>

    <template v-if="isStatic">
      <FormField label="Writes">
        <div class="flex flex-col gap-2">
          <div
            v-for="(entry, idx) in (writes ?? [])"
            :key="idx"
            class="flex flex-col gap-1.5 p-2 border border-terminal-border bg-terminal-surface/40"
          >
            <div class="flex items-stretch gap-1">
              <div class="flex-1 min-w-0">
                <NodeIdInput
                  :model-value="entry.nodeId"
                  @update:model-value="updateEntry(idx, { nodeId: $event })"
                />
              </div>
              <button
                class="px-2 text-[10px] text-terminal-text-dim border border-terminal-border
                       hover:text-status-error hover:border-status-error transition-colors"
                @click="removeEntry(idx)"
              >×</button>
            </div>
            <div class="grid grid-cols-2 gap-1.5">
              <DataTypeSelect
                :model-value="entry.dataType"
                @update:model-value="updateEntry(idx, { dataType: String($event) })"
              />
              <FormSelect
                :model-value="entry.valueSource"
                :options="VALUE_SOURCES"
                @update:model-value="updateEntry(idx, { valueSource: String($event) })"
              />
            </div>
            <div v-if="entry.dataType === 'ExtensionObject'" class="flex flex-col gap-1">
              <span class="text-[9px] uppercase tracking-wider text-terminal-text-dim">Structure Type (DataType NodeID)</span>
              <NodeIdInput
                :model-value="entry.structureType ?? ''"
                @update:model-value="updateEntry(idx, { structureType: $event })"
              />
            </div>
            <FormInput
              v-if="entry.valueSource === 'msg'"
              :model-value="entry.valuePath"
              placeholder="payload"
              mono
              @update:model-value="updateEntry(idx, { valuePath: $event })"
            />
            <FormInput
              v-else
              :model-value="entry.value"
              placeholder="static value"
              mono
              @update:model-value="updateEntry(idx, { value: $event })"
            />
          </div>
          <div class="flex items-center gap-2">
            <button
              class="px-2 py-0.5 text-[10px] uppercase tracking-wider
                     border border-terminal-border text-terminal-text-dim
                     hover:text-accent hover:border-accent transition-colors"
              @click="addEntry"
            >+ Add Write</button>
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
    </template>

    <OpcuaBrowser
      :open="browserOpen"
      :server-id="server"
      :server-config="selectedServerConfig"
      :existing-node-ids="(writes ?? []).map((w) => w.nodeId)"
      :multi-select="false"
      title="Browse OPC UA Server (Write)"
      @close="browserOpen = false"
      @select="handleBrowserSelect"
    />

    <FormCheckbox v-model="passthrough" label="Pass message through with writeResult" />
    <FormCheckbox v-model="disableTypeCache" label="Disable DataType cache (always require explicit dataType)" />
  </div>
</template>
