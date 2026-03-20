<script setup lang="ts">
import { ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

const flowStore = useFlowStore()
const ui = useUiStore()

function switchFlow(flowId: string) {
  if (flowId === flowStore.activeFlowId) return
  flowStore.syncCanvasToActiveFlow()
  flowStore.setActiveFlow(flowId)
  ui.clearFlowProperties()
}

function handleDblClick(flowId: string) {
  ui.openFlowEditProperties(flowId)
}

function handleAddClick() {
  ui.openFlowCreateProperties()
}

// ── Drag & Drop ──────────────────────────────────────────────────
const dragIdx = ref<number | null>(null)
const dropIdx = ref<number | null>(null)

function onDragStart(idx: number, e: DragEvent) {
  dragIdx.value = idx
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(idx))
  }
}

function onDragOver(idx: number, e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropIdx.value = idx
}

function onDrop(idx: number) {
  if (dragIdx.value !== null && dragIdx.value !== idx) {
    flowStore.reorderFlows(dragIdx.value, idx)
  }
  dragIdx.value = null
  dropIdx.value = null
}

function onDragEnd() {
  dragIdx.value = null
  dropIdx.value = null
}
</script>

<template>
  <div class="flex items-center h-8 bg-terminal-surface border-b border-terminal-border font-mono select-none shrink-0 overflow-x-auto">
    <!-- Flow Tabs -->
    <button
      v-for="(flow, idx) in flowStore.flows"
      :key="flow.id"
      draggable="true"
      class="relative flex items-center gap-1.5 h-full px-3 text-xs whitespace-nowrap border-r border-terminal-border transition-colors duration-100 cursor-pointer"
      :class="[
        flow.id === flowStore.activeFlowId
          ? 'bg-terminal-bg text-terminal-text'
          : 'text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-bg/50',
        flow.disabled ? 'opacity-40' : '',
        dragIdx === idx ? 'opacity-30' : '',
        dropIdx === idx && dragIdx !== idx ? 'border-l-2 border-l-accent' : '',
      ]"
      @click="switchFlow(flow.id)"
      @dblclick="handleDblClick(flow.id)"
      @dragstart="onDragStart(idx, $event)"
      @dragover="onDragOver(idx, $event)"
      @drop="onDrop(idx)"
      @dragend="onDragEnd"
    >
      <span class="truncate max-w-[120px]">{{ flow.label }}</span>
      <span
        v-if="flowStore.dirty && flow.id === flowStore.activeFlowId"
        class="text-accent text-[10px] leading-none"
      >●</span>
    </button>

    <!-- Add Flow Button -->
    <button
      class="flex items-center justify-center h-full px-3 text-terminal-text-dim hover:text-accent hover:bg-terminal-bg/50 transition-colors duration-100"
      title="New flow"
      @click="handleAddClick"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        class="w-3.5 h-3.5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2.5"
      >
        <path stroke-linecap="square" stroke-linejoin="miter" d="M12 5v14m-7-7h14" />
      </svg>
    </button>
  </div>
</template>
