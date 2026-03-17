<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import NodeIcon from '@/components/nodes/NodeIcon.vue'
import { getTokens } from '@/components/nodes/tokens'
import { useFlowStore } from '@/stores/flowStore'

export interface BaseNodeProps {
  id: string
  label?: string
  nodeType?: string
  inputs?: number
  outputs?: number
  selected?: boolean
  disabled?: boolean
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
  status: null,
})

const flowStore = useFlowStore()

const t = computed(() => getTokens(props.nodeType))
const displayLabel = computed(() => props.label || props.nodeType)
const isDirty = computed(() => flowStore.isNodeDirty(props.id))

const statusColor = computed(() => {
  const colors: Record<string, string> = {
    red: '#e24b4a', green: '#4ade80', yellow: '#ef9f27',
    blue: '#60a5fa', grey: '#6b7280',
  }
  return colors[props.status?.fill ?? ''] ?? '#3a3a3a'
})

const inputHandles = computed(() =>
  Array.from({ length: props.inputs }, (_, i) => ({
    id: `input-${i}`,
    style: { top: props.inputs === 1 ? '50%' : `${20 + (60 / Math.max(props.inputs - 1, 1)) * i}%` },
  }))
)

const outputHandles = computed(() =>
  Array.from({ length: props.outputs }, (_, i) => ({
    id: `output-${i}`,
    style: { top: props.outputs === 1 ? '50%' : `${20 + (60 / Math.max(props.outputs - 1, 1)) * i}%` },
  }))
)
</script>

<template>
  <div
    class="flint-node min-w-[196px] w-max relative font-mono select-none flex"
    :class="{ selected: props.selected, 'opacity-40': props.disabled }"
    :style="{
      border: `1px solid ${props.selected ? t.accent : t.border}`,
      borderLeft: `3px solid ${t.accent}`,
      boxShadow: props.selected
        ? `0 0 0 1px ${t.accentBdr}, 0 4px 20px ${t.accentGlow}`
        : 'none',
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
      class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
      :class="selected
        ? '!border-accent !bg-accent/20'
        : '!border-terminal-text-dim !bg-terminal-surface'"
    />

    <!-- Output Handles -->
    <Handle
      v-for="h in outputHandles"
      :key="h.id"
      :id="h.id"
      type="source"
      :position="Position.Right"
      :style="h.style"
      class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
      :class="selected
        ? '!border-accent !bg-accent/20'
        : '!border-terminal-text-dim !bg-terminal-surface'"
    />

    <!-- Dirty indicator (undeployed changes) -->
    <span
      v-if="isDirty"
      class="absolute -top-1 -right-1 w-2.5 h-2.5 rounded-full z-10"
      style="background: #58a6ff; box-shadow: 0 0 4px #58a6ff80"
      title="Undeployed changes"
    />

    <!-- Left icon column -->
    <div
      class="w-10 shrink-0 flex items-center justify-center"
      :style="{ background: t.bgIcon, color: t.accent }"
    >
      <slot name="icon"><NodeIcon :type="props.nodeType" /></slot>
    </div>

    <!-- Right content area -->
    <div class="flex-1 min-w-0 flex flex-col">
      <!-- Header -->
      <div
        class="flex items-center gap-1.5 px-2 py-1.5"
        :style="{ background: t.bgHdr, borderBottom: `1px solid ${t.border}` }"
      >
        <span
          class="text-[11px] font-medium tracking-wide truncate flex-1"
          :style="{ color: t.accent }"
        >
          {{ displayLabel }}
        </span>
        <slot name="badge" />
      </div>

      <!-- Body (optional) -->
      <div
        v-if="$slots.body"
        class="px-2 py-1 text-[10px]"
        :style="{ background: t.bg, color: t.textSub }"
      >
        <slot name="body" />
      </div>

      <!-- Actions (optional) -->
      <div
        v-if="$slots.actions"
        :style="{ background: t.bg, borderTop: `1px solid ${t.border}` }"
      >
        <slot name="actions" />
      </div>

      <!-- Status bar -->
      <div
        v-if="props.status"
        class="flex items-center gap-1.5 px-2 py-1 text-[10px]"
        :style="{ background: t.bg, borderTop: `1px solid ${t.border}`, color: t.textSub }"
      >
        <span
          class="w-1.5 h-1.5 shrink-0 rounded-full"
          :style="{ background: statusColor }"
        />
        <span class="truncate">{{ props.status.text ?? '' }}</span>
      </div>
    </div>

    <!-- Disabled overlay -->
    <div
      v-if="props.disabled"
      class="absolute inset-0 bg-terminal-bg/60 flex items-center justify-center"
    >
      <span class="text-[9px] text-terminal-text-dim uppercase tracking-widest">disabled</span>
    </div>
  </div>
</template>

<style scoped>
.flint-node {
  transition: box-shadow .15s;
  border-radius: 2px;
}
</style>
