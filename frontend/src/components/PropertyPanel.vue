<script setup lang="ts">
import { computed } from 'vue'
import { useUiStore } from '@/stores/uiStore'
import { useFlowStore } from '@/stores/flowStore'
import InjectConfig from '@/components/config/InjectConfig.vue'
import FunctionConfig from '@/components/config/FunctionConfig.vue'

const ui = useUiStore()
const flowStore = useFlowStore()

const selectedNode = computed(() => flowStore.selectedNode)
const nodeData = computed(() => selectedNode.value?.data ?? null)

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'object') {
    try { return JSON.stringify(value, null, 2) } catch { return String(value) }
  }
  return String(value)
}
</script>

<template>
  <div class="h-full w-[280px] flex-shrink-0 flex flex-col bg-terminal-surface border-l border-terminal-border font-mono select-none">
    <!-- Header -->
    <div class="flex items-center justify-between px-3 py-2 border-b border-terminal-border shrink-0">
      <span class="text-xs uppercase tracking-widest text-terminal-text-dim">Properties</span>
      <button
        class="w-6 h-6 flex items-center justify-center text-terminal-text-dim hover:text-amber hover:bg-terminal-border transition-colors duration-100"
        title="Close panel"
        @click="ui.closePropertiesPanel()"
      >
        ✕
      </button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <!-- No node selected -->
      <div
        v-if="!selectedNode"
        class="flex flex-col items-center justify-center h-full px-4 text-center"
      >
        <div class="text-terminal-text-dim text-2xl mb-3">⬡</div>
        <p class="text-terminal-text-dim text-xs uppercase tracking-wider mb-1">No Node Selected</p>
        <p class="text-terminal-text-dim text-[10px]">Click a node on the canvas to view its properties</p>
      </div>

      <!-- Node selected -->
      <div v-else class="flex flex-col">
        <!-- Node identity -->
        <div class="px-3 py-3 border-b border-terminal-border">
          <div class="flex items-center gap-2 mb-2">
            <span class="w-3 h-3 bg-amber flex-shrink-0"></span>
            <span class="text-amber text-sm font-bold uppercase tracking-wider truncate">
              {{ nodeData?.label ?? selectedNode.type }}
            </span>
          </div>
          <div class="flex items-center gap-2 text-[10px] text-terminal-text-dim">
            <span class="terminal-badge">{{ selectedNode.type }}</span>
            <span class="opacity-60">{{ selectedNode.id.slice(0, 12) }}…</span>
          </div>
        </div>

        <!-- Properties table -->
        <div class="px-3 py-2 border-b border-terminal-border">
          <p class="text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2">▸ Properties</p>
          <div class="flex flex-col gap-0.5">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">Name</label>
            <input
              type="text"
              :value="nodeData?.label ?? ''"
              class="terminal-input w-full text-xs"
              placeholder="Node name"
              @change="flowStore.updateNodeData(selectedNode.id, { label: ($event.target as HTMLInputElement).value })"
            />
          </div>
        </div>

        <!-- Type-specific config -->
        <div class="px-3 py-2 border-b border-terminal-border">
          <p class="text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2">▸ Configuration</p>
          <InjectConfig v-if="selectedNode?.type === 'inject'" />
          <FunctionConfig v-else-if="selectedNode?.type === 'function'" />
          <template v-else>
            <div v-if="nodeData?.config && Object.keys(nodeData.config).length > 0" class="space-y-1.5">
              <div
                v-for="(value, key) in (nodeData.config as Record<string, unknown>)"
                :key="String(key)"
                class="flex flex-col gap-0.5"
              >
                <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">{{ String(key) }}</label>
                <div class="terminal-input w-full text-xs break-all whitespace-pre-wrap max-h-20 overflow-y-auto">
                  {{ formatValue(value) }}
                </div>
              </div>
            </div>
            <div v-else class="text-[10px] text-terminal-text-dim italic">No configuration available</div>
          </template>
        </div>

        <!-- Status -->
        <div class="px-3 py-2">
          <p class="text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2">▸ Status</p>
          <div v-if="nodeData?.status" class="flex items-center gap-2">
            <span
              class="w-2 h-2 flex-shrink-0"
              :class="{
                'bg-green-500': nodeData.status.fill === 'green',
                'bg-red-500': nodeData.status.fill === 'red',
                'bg-yellow-500': nodeData.status.fill === 'yellow',
                'bg-blue-500': nodeData.status.fill === 'blue',
                'bg-gray-500': nodeData.status.fill === 'grey' || !nodeData.status.fill,
              }"
            ></span>
            <span class="text-xs text-terminal-text-dim">{{ nodeData.status.text ?? 'OK' }}</span>
          </div>
          <div v-else class="text-[10px] text-terminal-text-dim italic">No status</div>
        </div>
      </div>
    </div>
  </div>
</template>
