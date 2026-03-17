<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'

export interface BaseNodeProps {
  id: string
  label?: string
  nodeType?: string
  inputs?: number
  outputs?: number
  selected?: boolean
  disabled?: boolean
  accentColor?: string
  status?: {
    fill?: 'red' | 'green' | 'yellow' | 'blue' | 'grey'
    shape?: 'ring' | 'dot'
    text?: string
  } | null
}

const props = withDefaults(defineProps<BaseNodeProps>(), {
  label: '',
  nodeType: 'unknown',
  inputs: 0,
  outputs: 0,
  selected: false,
  disabled: false,
  accentColor: '#6b7280',
  status: null,
})

const displayLabel = computed(() => props.label || props.nodeType)

const statusColor = computed(() => {
  const colors: Record<string, string> = {
    red: '#ef4444', green: '#4ade80', yellow: '#facc15',
    blue: '#60a5fa', grey: '#6b7280',
  }
  return colors[props.status?.fill ?? ''] ?? '#6b7280'
})

const inputHandles = computed(() => {
  return Array.from({ length: props.inputs }, (_, i) => ({
    id: `input-${i}`,
    style: { top: props.inputs === 1 ? '50%' : `${20 + (60 / Math.max(props.inputs - 1, 1)) * i}%` },
  }))
})

const outputHandles = computed(() => {
  return Array.from({ length: props.outputs }, (_, i) => ({
    id: `output-${i}`,
    style: { top: props.outputs === 1 ? '50%' : `${20 + (60 / Math.max(props.outputs - 1, 1)) * i}%` },
  }))
})
</script>

<template>
  <div
    class="flint-node w-[180px] relative bg-terminal-surface font-mono select-none"
    :class="{ selected: props.selected, 'opacity-40': props.disabled }"
    :style="{
      border: '1px solid #3a3a28',
      borderLeft: `3px solid ${props.accentColor}`,
    }"
  >
    <!-- Input Handles -->
    <Handle
      v-for="h in inputHandles"
      :key="h.id"
      :id="h.id"
      type="target"
      :position="Position.Left"
      :style="h.style"
      class="!w-2.5 !h-2.5 !rounded-full !bg-terminal-surface !border-2 hover:!bg-amber transition-colors"
      :class="selected ? '!border-amber' : '!border-terminal-text-dim'"
    />

    <!-- Output Handles -->
    <Handle
      v-for="h in outputHandles"
      :key="h.id"
      :id="h.id"
      type="source"
      :position="Position.Right"
      :style="h.style"
      class="!w-2.5 !h-2.5 !rounded-full !bg-terminal-surface !border-2 hover:!bg-amber transition-colors"
      :class="selected ? '!border-amber' : '!border-terminal-text-dim'"
    />

    <!-- Header -->
    <div class="flex items-center gap-2 px-2.5 py-1.5">
      <span class="shrink-0 leading-none" :style="{ color: props.accentColor }">
        <slot name="icon">●</slot>
      </span>
      <span class="text-xs text-terminal-text font-medium truncate flex-1">
        {{ displayLabel }}
      </span>
      <slot name="badge" />
    </div>

    <!-- Body (optional) -->
    <div
      v-if="$slots.body"
      class="px-2.5 py-1.5 text-[10px] text-terminal-text-dim"
      style="border-top: 1px solid #3a3a2855"
    >
      <slot name="body" />
    </div>

    <!-- Actions (optional, e.g. buttons) -->
    <div
      v-if="$slots.actions"
      style="border-top: 1px solid #3a3a2855"
    >
      <slot name="actions" />
    </div>

    <!-- Status bar -->
    <div
      v-if="props.status"
      class="flex items-center gap-1.5 px-2.5 py-1 text-[10px] text-terminal-text-dim"
      style="border-top: 1px solid #3a3a2855"
    >
      <span
        class="w-1.5 h-1.5 shrink-0 rounded-full"
        :style="{ background: statusColor }"
      />
      <span class="truncate">{{ props.status.text ?? '' }}</span>
    </div>

    <!-- Disabled overlay -->
    <div
      v-if="props.disabled"
      class="absolute inset-0 bg-terminal-bg/50 flex items-center justify-center"
    >
      <span class="text-[9px] text-terminal-text-dim uppercase tracking-widest">disabled</span>
    </div>
  </div>
</template>

<style scoped>
.flint-node {
  transition: box-shadow 0.1s ease;
}
.flint-node.selected {
  box-shadow: 0 0 0 1px #FFBF00, 0 0 12px #ffbf0033;
}
.flint-node:hover:not(.selected) {
  box-shadow: 0 0 0 1px #998a00;
}
</style>
