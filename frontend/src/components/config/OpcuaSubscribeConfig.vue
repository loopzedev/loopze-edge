<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NodeIdInput from './shared/NodeIdInput.vue'
import OpcuaBrowser from './shared/OpcuaBrowser.vue'

const flowStore = useFlowStore()
const { options: serverOptions, openNewConfig, openEditConfig } = useConfigSelector('opcua-server')

interface MonitoredItem {
  nodeId: string
  samplingInterval: number
  queueSize: number
  discardOldest: boolean
  deadband: { type: 'none' | 'absolute' | 'percent'; value: number }
}

const server = useNodeProperty<string>('server', '')
const mode = useStructuralProperty<string>('mode', 'static', {
  port: 'inputs',
  derive: (v) => (v === 'dynamic' ? 1 : 0),
})
const monitoredItems = useNodeProperty<MonitoredItem[]>('monitoredItems', [])
const publishingInterval = useNodeProperty<number>('publishingInterval', 500)
const lifetimeCount = useNodeProperty<number>('lifetimeCount', 60)
const keepAliveCount = useNodeProperty<number>('keepAliveCount', 10)
const priority = useNodeProperty<number>('priority', 0)
const outputShape = useNodeProperty<string>('outputShape', 'per-item')

const modePresets = [
  { label: 'Static',  value: 'static'  },
  { label: 'Dynamic', value: 'dynamic' },
]

const DEADBAND_TYPES = [
  { value: 'none',     label: 'None' },
  { value: 'absolute', label: 'Absolute' },
  { value: 'percent',  label: 'Percent' },
]

const OUTPUT_SHAPES = [
  { value: 'per-item', label: 'Per-item (one msg per change)' },
  { value: 'batch',    label: 'Batch (one msg per publish)' },
]

function blankItem(): MonitoredItem {
  return {
    nodeId: '',
    samplingInterval: 1000,
    queueSize: 1,
    discardOldest: true,
    deadband: { type: 'none', value: 0 },
  }
}

function addItem() {
  monitoredItems.value = [...(monitoredItems.value ?? []), blankItem()]
}

function removeItem(idx: number) {
  const next = [...(monitoredItems.value ?? [])]
  next.splice(idx, 1)
  monitoredItems.value = next
}

function updateItem(idx: number, patch: Partial<MonitoredItem>) {
  const next = [...(monitoredItems.value ?? [])]
  next[idx] = { ...next[idx], ...patch }
  monitoredItems.value = next
}

function updateDeadband(idx: number, patch: Partial<MonitoredItem['deadband']>) {
  const items = monitoredItems.value ?? []
  if (!items[idx]) return
  const next = [...items]
  next[idx] = { ...next[idx], deadband: { ...next[idx].deadband, ...patch } }
  monitoredItems.value = next
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
  const existing = new Set((monitoredItems.value ?? []).map((m) => m.nodeId))
  const fresh: MonitoredItem[] = items
    .filter((i) => !existing.has(i.nodeId))
    .map((i) => ({ ...blankItem(), nodeId: i.nodeId }))
  monitoredItems.value = [...(monitoredItems.value ?? []), ...fresh]
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
      Dynamic mode reshapes the subscription on every input. Send
      <code class="font-mono">msg.action = "subscribe"</code> with
      <code class="font-mono">msg.payload</code> as a NodeID string, an array of
      strings, or an array of <code class="font-mono">{nodeId, samplingInterval, queueSize, deadband}</code>.
      Use <code class="font-mono">"unsubscribe"</code> or <code class="font-mono">"clear"</code> to remove items.
    </p>

    <template v-if="isStatic">
      <FormField label="Monitored Items">
        <div class="flex flex-col gap-2">
          <div
            v-for="(item, idx) in (monitoredItems ?? [])"
            :key="idx"
            class="flex flex-col gap-1.5 p-2 border border-terminal-border bg-terminal-surface/40"
          >
            <div class="flex items-stretch gap-1">
              <div class="flex-1 min-w-0">
                <NodeIdInput
                  :model-value="item.nodeId"
                  @update:model-value="updateItem(idx, { nodeId: $event })"
                />
              </div>
              <button
                class="px-2 text-[10px] text-terminal-text-dim border border-terminal-border
                       hover:text-status-error hover:border-status-error transition-colors"
                @click="removeItem(idx)"
              >×</button>
            </div>
            <div class="grid grid-cols-2 gap-1.5">
              <FormField label="Sampling">
                <NumberInput
                  :model-value="item.samplingInterval"
                  :min="-1"
                  unit="ms"
                  @update:model-value="updateItem(idx, { samplingInterval: $event })"
                />
              </FormField>
              <FormField label="Queue Size">
                <NumberInput
                  :model-value="item.queueSize"
                  :min="1"
                  @update:model-value="updateItem(idx, { queueSize: $event })"
                />
              </FormField>
            </div>
            <FormCheckbox
              :model-value="item.discardOldest"
              label="Discard oldest on overflow"
              @update:model-value="updateItem(idx, { discardOldest: $event })"
            />
            <div class="grid grid-cols-2 gap-1.5">
              <FormField label="Deadband Type">
                <FormSelect
                  :model-value="item.deadband.type"
                  :options="DEADBAND_TYPES"
                  @update:model-value="updateDeadband(idx, { type: String($event) as 'none' | 'absolute' | 'percent' })"
                />
              </FormField>
              <FormField v-if="item.deadband.type !== 'none'" label="Deadband Value">
                <NumberInput
                  :model-value="item.deadband.value"
                  :min="0"
                  :step="0.1"
                  @update:model-value="updateDeadband(idx, { value: $event })"
                />
              </FormField>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="px-2 py-0.5 text-[10px] uppercase tracking-wider
                     border border-terminal-border text-terminal-text-dim
                     hover:text-accent hover:border-accent transition-colors"
              @click="addItem"
            >+ Add MonitoredItem</button>
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
      :existing-node-ids="(monitoredItems ?? []).map((m) => m.nodeId)"
      :multi-select="true"
      title="Browse OPC UA Server (Subscribe)"
      @close="browserOpen = false"
      @select="handleBrowserSelect"
    />

    <SectionHeader title="Subscription Defaults">
      <div class="flex flex-col gap-2">
        <FormField label="Publishing Interval">
          <NumberInput v-model="publishingInterval" :min="50" unit="ms" />
        </FormField>
        <FormField label="Lifetime Count">
          <NumberInput v-model="lifetimeCount" :min="1" />
        </FormField>
        <FormField label="KeepAlive Count">
          <NumberInput v-model="keepAliveCount" :min="1" />
        </FormField>
        <FormField label="Priority">
          <NumberInput v-model="priority" :min="0" :max="255" />
        </FormField>
      </div>
    </SectionHeader>

    <FormField label="Output Shape">
      <FormSelect v-model="outputShape" :options="OUTPUT_SHAPES" />
    </FormField>
  </div>
</template>
