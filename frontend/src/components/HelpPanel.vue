<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { getTokens } from '@/components/nodes/tokens'
import NodeIcon from '@/components/nodes/NodeIcon.vue'

const flow = useFlowStore()

const selectedNode = computed(() => flow.selectedNode)

const catalogEntry = computed(() => {
  if (!selectedNode.value) return null
  const nodeType = selectedNode.value.data?.nodeType
  if (!nodeType) return null
  return flow.nodeCatalog.get(nodeType) ?? null
})

const tokens = computed(() => {
  const nodeType = selectedNode.value?.data?.nodeType
  return nodeType ? getTokens(nodeType) : null
})
</script>

<template>
  <div class="flex flex-col h-full bg-terminal-bg text-xs">
    <!-- No node selected -->
    <div v-if="!selectedNode" class="flex items-center justify-center h-full">
      <div class="text-center">
        <div class="text-terminal-text-dim/40 mb-2">
          <svg xmlns="http://www.w3.org/2000/svg" class="w-8 h-8 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z" />
          </svg>
        </div>
        <span class="text-terminal-text-dim text-xs">Select a node to view help</span>
      </div>
    </div>

    <!-- Node info -->
    <div v-else class="flex-1 overflow-y-auto">
      <!-- Node header -->
      <div class="px-4 py-3 border-b border-terminal-border">
        <div class="flex items-center gap-2.5 mb-2">
          <div
            class="w-8 h-8 rounded flex items-center justify-center shrink-0"
            :style="{ background: tokens?.bgIcon, color: tokens?.accent }"
          >
            <NodeIcon :type="selectedNode.data?.nodeType" />
          </div>
          <div class="min-w-0">
            <div class="text-sm font-semibold text-terminal-text truncate">
              {{ catalogEntry?.label ?? selectedNode.data?.nodeType }}
            </div>
            <div class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              {{ catalogEntry?.category ?? 'unknown' }}
            </div>
          </div>
        </div>
      </div>

      <!-- Description -->
      <div v-if="catalogEntry?.description" class="px-4 py-3 border-b border-terminal-border">
        <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">Description</div>
        <p class="text-[11px] text-terminal-text leading-relaxed m-0">
          {{ catalogEntry.description }}
        </p>
      </div>

      <!-- Ports -->
      <div class="px-4 py-3 border-b border-terminal-border">
        <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">Ports</div>
        <div class="flex gap-4">
          <div class="flex items-center gap-1.5">
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5 text-terminal-text-dim" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
            </svg>
            <span class="text-[11px] text-terminal-text">{{ catalogEntry?.inputs ?? selectedNode.data?.inputs ?? 0 }} input{{ (catalogEntry?.inputs ?? selectedNode.data?.inputs ?? 0) !== 1 ? 's' : '' }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5 text-terminal-text-dim" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13 5l7 7-7 7M5 5l7 7-7 7" />
            </svg>
            <span class="text-[11px] text-terminal-text">{{ catalogEntry?.outputs ?? selectedNode.data?.outputs ?? 0 }} output{{ (catalogEntry?.outputs ?? selectedNode.data?.outputs ?? 0) !== 1 ? 's' : '' }}</span>
          </div>
        </div>
      </div>

      <!-- Properties (defaults) -->
      <div v-if="catalogEntry?.defaults && Object.keys(catalogEntry.defaults).length > 0" class="px-4 py-3">
        <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">Properties</div>
        <div class="flex flex-col gap-1">
          <div
            v-for="(val, key) in catalogEntry.defaults"
            :key="key"
            class="flex items-start gap-2 py-0.5"
          >
            <span class="text-[11px] font-mono text-accent shrink-0">{{ key }}</span>
            <span class="text-[11px] text-terminal-text-dim font-mono truncate">{{ JSON.stringify(val) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
