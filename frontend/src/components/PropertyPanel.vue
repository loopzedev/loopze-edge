<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useUiStore } from '@/stores/uiStore'
import { useFlowStore } from '@/stores/flowStore'
import PanelHeader from '@/components/ui/PanelHeader.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import InjectConfig from '@/components/config/InjectConfig.vue'
import FunctionConfig from '@/components/config/FunctionConfig.vue'
import ContextWatchConfig from '@/components/config/ContextWatchConfig.vue'
import ChangeConfig from '@/components/config/ChangeConfig.vue'
import DebugConfig from '@/components/config/DebugConfig.vue'
import LinkConfig from '@/components/config/LinkConfig.vue'
import MqttNodeConfig from '@/components/config/MqttNodeConfig.vue'
import StateMachineConfig from '@/components/config/StateMachineConfig.vue'
import FlowProperties from '@/components/FlowProperties.vue'
import { getConfigEditor } from '@/components/config/configEditors'

const ui = useUiStore()
const flowStore = useFlowStore()

const selectedNode = computed(() => flowStore.selectedNode)
const nodeData = computed(() => selectedNode.value?.data ?? null)
const showFlowProperties = computed(() =>
  ui.propertiesContext?.type === 'flow-create' || ui.propertiesContext?.type === 'flow-edit',
)

const showConfigEditor = computed(() => ui.propertiesContext?.type === 'config-edit')
const configEditorComponent = computed(() => {
  const ctx = ui.propertiesContext
  if (ctx?.type !== 'config-edit') return null
  const loader = getConfigEditor(ctx.configType)
  if (!loader) return null
  return defineAsyncComponent(loader)
})
const configEditorId = computed(() => {
  const ctx = ui.propertiesContext
  return ctx?.type === 'config-edit' ? ctx.configId : undefined
})

const STATUS_COLORS: Record<string, string> = {
  green: '#4ade80', red: '#e24b4a', yellow: '#ef9f27',
  blue: '#60a5fa', grey: '#6b7280',
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'object') {
    try { return JSON.stringify(value, null, 2) } catch { return String(value) }
  }
  return String(value)
}

// ── Resize handle ────────────────────────────────────────────────
const panelWidth = computed({
  get: () => ui.propertiesPanelWidth,
  set: (v: number) => { ui.propertiesPanelWidth = v },
})
let resizing = false
let startX = 0
let startWidth = 0

function onResizeStart(e: MouseEvent) {
  resizing = true
  startX = e.clientX
  startWidth = panelWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function onResizeMove(e: MouseEvent) {
  if (!resizing) return
  const delta = startX - e.clientX
  panelWidth.value = Math.max(220, startWidth + delta)
}

function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}
</script>

<template>
  <div
    class="h-full flex-shrink-0 flex bg-terminal-surface border-l border-terminal-border select-none relative"
    :style="{ width: panelWidth + 'px' }"
  >
    <!-- Resize handle -->
    <div
      class="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize z-20 hover:bg-accent/30 active:bg-accent/50 transition-colors"
      @mousedown.prevent="onResizeStart"
    />

    <!-- Panel content -->
    <div class="flex-1 flex flex-col min-w-0">
      <PanelHeader title="Properties" closable @close="ui.closePropertiesPanel()" />

      <!-- Content -->
      <div class="flex-1 overflow-y-auto flex flex-col min-h-0">
        <!-- Config node editor (e.g. MQTT Broker) -->
        <component
          v-if="showConfigEditor && configEditorComponent"
          :is="configEditorComponent"
          :config-id="configEditorId"
        />

        <!-- Flow properties (create / edit) -->
        <FlowProperties v-else-if="showFlowProperties" />

        <!-- No node selected -->
        <div
          v-else-if="!selectedNode"
          class="flex flex-col items-center justify-center h-full px-6 text-center"
        >
          <div class="text-terminal-text-dim/20 mb-4">
            <svg xmlns="http://www.w3.org/2000/svg" class="w-12 h-12" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1">
              <path stroke-linecap="round" stroke-linejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
            </svg>
          </div>
          <p class="text-terminal-text-dim text-xs font-medium mb-1">No Node Selected</p>
          <p class="text-[11px] text-terminal-text-dim/60">Click a node to view its properties</p>
        </div>

        <!-- Node selected -->
        <div v-else class="flex flex-col flex-1 min-h-0">
          <!-- Node identity -->
          <div class="px-4 py-4 border-b border-terminal-border">
            <div class="flex items-center gap-2.5 mb-2">
              <span
                class="w-3 h-3 rounded-sm flex-shrink-0"
                :style="{ backgroundColor: '#58a6ff' }"
              />
              <span class="text-sm font-semibold text-terminal-text truncate">
                {{ nodeData?.label || selectedNode.type }}
              </span>
            </div>
            <div class="flex items-center gap-2 text-[10px] text-terminal-text-dim">
              <span class="terminal-badge">{{ selectedNode.type }}</span>
              <span class="font-mono text-terminal-text-dim/50">{{ selectedNode.id.slice(0, 8) }}</span>
            </div>
          </div>

          <!-- Properties -->
          <div class="px-4 py-3 border-b border-terminal-border">
            <SectionHeader title="Properties">
              <div class="flex flex-col gap-1.5">
                <FormLabel>Name</FormLabel>
                <FormInput
                  :model-value="(nodeData?.label as string) ?? ''"
                  placeholder="Node name"
                  @update:model-value="flowStore.updateNodeData(selectedNode!.id, { label: $event })"
                />
              </div>
            </SectionHeader>
          </div>

          <!-- Type-specific config -->
          <div class="px-4 py-3 border-b border-terminal-border flex-1 flex flex-col min-h-0">
            <!-- State Machine: skip collapsible SectionHeader to preserve flex chain -->
            <template v-if="selectedNode?.type === 'statemachine'">
              <p class="flex items-center gap-1.5 text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2.5 font-semibold">
                <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3 rotate-90 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                </svg>
                Configuration
              </p>
              <StateMachineConfig />
            </template>
            <SectionHeader v-else title="Configuration">
              <InjectConfig v-if="selectedNode?.type === 'inject'" />
              <FunctionConfig v-else-if="selectedNode?.type === 'function'" />
              <ContextWatchConfig v-else-if="selectedNode?.type === 'context-watch'" />
              <ChangeConfig v-else-if="selectedNode?.type === 'change'" />
              <DebugConfig v-else-if="selectedNode?.type === 'debug'" />
              <LinkConfig v-else-if="['link-in', 'link-out', 'link-call'].includes(selectedNode?.type ?? '')" />
              <MqttNodeConfig v-else-if="['mqtt-in', 'mqtt-out'].includes(selectedNode?.type ?? '')" />
              <template v-else>
                <div v-if="nodeData?.config && Object.keys(nodeData.config).length > 0" class="flex flex-col gap-3">
                  <div
                    v-for="(value, key) in (nodeData.config as Record<string, unknown>)"
                    :key="String(key)"
                    class="flex flex-col gap-1.5"
                  >
                    <FormLabel>{{ String(key) }}</FormLabel>
                    <div class="bg-terminal-bg border border-terminal-border rounded px-2.5 py-1.5 text-[11px] font-mono break-all whitespace-pre-wrap max-h-24 overflow-y-auto text-terminal-text-dim">
                      {{ formatValue(value) }}
                    </div>
                  </div>
                </div>
                <div v-else class="text-[11px] text-terminal-text-dim/60">No configuration available</div>
              </template>
            </SectionHeader>
          </div>

          <!-- Status -->
          <div class="px-4 py-3">
            <SectionHeader title="Status">
              <div v-if="nodeData?.status" class="flex items-center gap-2.5">
                <span
                  class="w-2 h-2 flex-shrink-0 rounded-full"
                  :style="{ background: STATUS_COLORS[nodeData.status.fill] ?? STATUS_COLORS.grey }"
                />
                <span class="text-[11px] text-terminal-text-dim">{{ nodeData.status.text ?? 'OK' }}</span>
              </div>
              <div v-else class="text-[11px] text-terminal-text-dim/60">No status</div>
            </SectionHeader>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
