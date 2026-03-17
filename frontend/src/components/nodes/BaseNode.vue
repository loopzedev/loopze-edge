<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'

export interface BaseNodeProps {
  id: string
  label?: string
  nodeType?: string
  icon?: string
  inputs?: number
  outputs?: number
  selected?: boolean
  disabled?: boolean
  status?: {
    fill?: 'red' | 'green' | 'yellow' | 'blue' | 'grey'
    shape?: 'ring' | 'dot'
    text?: string
  } | null
  headerColor?: string
}

const props = withDefaults(defineProps<BaseNodeProps>(), {
  label: 'Node',
  nodeType: 'unknown',
  icon: '●',
  inputs: 0,
  outputs: 0,
  selected: false,
  disabled: false,
  status: null,
  headerColor: '',
})

defineEmits<{
  (e: 'action', payload: { nodeId: string; action: string }): void
}>()

const displayLabel = computed(() => props.label || props.nodeType)

const statusDotColor = computed(() => {
  if (!props.status?.fill) return ''
  const colors: Record<string, string> = {
    red: '#ef4444',
    green: '#4ade80',
    yellow: '#facc15',
    blue: '#60a5fa',
    grey: '#9ca3af',
  }
  return colors[props.status.fill] ?? '#9ca3af'
})

const statusIsRing = computed(() => props.status?.shape === 'ring')

const inputHandles = computed(() => {
  const handles = []
  for (let i = 0; i < props.inputs; i++) {
    const offset = props.inputs === 1 ? 50 : 20 + (60 / Math.max(props.inputs - 1, 1)) * i
    handles.push({
      id: `input-${i}`,
      position: Position.Left,
      style: { top: `${offset}%` },
    })
  }
  return handles
})

const outputHandles = computed(() => {
  const handles = []
  for (let i = 0; i < props.outputs; i++) {
    const offset = props.outputs === 1 ? 50 : 20 + (60 / Math.max(props.outputs - 1, 1)) * i
    handles.push({
      id: `output-${i}`,
      position: Position.Right,
      style: { top: `${offset}%` },
    })
  }
  return handles
})
</script>

<template>
  <div
    class="flint-node relative"
    :class="{
      selected: props.selected,
      'opacity-40': props.disabled,
    }"
  >
    <!-- Input Handles -->
    <Handle
      v-for="handle in inputHandles"
      :key="handle.id"
      :id="handle.id"
      type="target"
      :position="handle.position"
      :style="handle.style"
      class="!w-[10px] !h-[10px] !rounded-none !bg-terminal-text-dim !border !border-amber hover:!bg-amber"
    />

    <!-- Output Handles -->
    <Handle
      v-for="handle in outputHandles"
      :key="handle.id"
      :id="handle.id"
      type="source"
      :position="handle.position"
      :style="handle.style"
      class="!w-[10px] !h-[10px] !rounded-none !bg-terminal-text-dim !border !border-amber hover:!bg-amber"
    />

    <!-- Header -->
    <div
      class="flint-node-header"
      :style="props.headerColor ? { borderBottomColor: props.headerColor } : {}"
    >
      <!-- Icon -->
      <span class="node-icon w-4 h-4 flex items-center justify-center text-[10px] text-amber shrink-0">
        <slot name="icon">{{ props.icon }}</slot>
      </span>

      <!-- Label -->
      <span class="truncate flex-1 text-terminal-text">
        {{ displayLabel }}
      </span>

      <!-- Type badge -->
      <span class="text-[8px] text-terminal-text-dim uppercase tracking-widest shrink-0 opacity-70">
        {{ props.nodeType }}
      </span>
    </div>

    <!-- Body -->
    <div class="flint-node-body">
      <slot>
        <span class="text-terminal-text-dim italic text-[10px]">no config</span>
      </slot>
    </div>

    <!-- Action Slot (optional buttons, triggers, etc.) -->
    <div v-if="$slots.actions" class="px-2 py-1 border-t border-terminal-border">
      <slot name="actions" />
    </div>

    <!-- Status Bar -->
    <div
      v-if="props.status"
      class="flint-node-status"
    >
      <!-- Status dot/ring -->
      <span
        class="w-[6px] h-[6px] shrink-0"
        :style="{
          backgroundColor: statusIsRing ? 'transparent' : statusDotColor,
          border: statusIsRing ? `1.5px solid ${statusDotColor}` : 'none',
        }"
      />

      <!-- Status text -->
      <span class="truncate">
        {{ props.status.text ?? '' }}
      </span>
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
/* Selected glow effect */
.flint-node.selected {
  border-color: #FFBF00;
  box-shadow: 0 0 12px rgba(255, 191, 0, 0.25), inset 0 0 2px rgba(255, 191, 0, 0.1);
}

.flint-node.selected .flint-node-header {
  border-bottom-color: #FFBF00;
}

/* Hover effect for interactive feel */
.flint-node:hover:not(.selected) {
  border-color: #998a00;
}

/* Transition for border and shadow */
.flint-node {
  transition: border-color 0.1s ease, box-shadow 0.15s ease;
}
</style>
