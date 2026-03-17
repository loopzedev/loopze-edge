<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'

const props = defineProps<NodeProps>()

const label = computed(() => props.data?.label ?? 'Debug')

const statusFill = computed(() => {
  const fill = props.data?.status?.fill
  switch (fill) {
    case 'green':
      return '#4ade80'
    case 'red':
      return '#ef4444'
    case 'yellow':
      return '#facc15'
    case 'blue':
      return '#60a5fa'
    case 'grey':
      return '#6b7280'
    default:
      return null
  }
})

const statusText = computed(() => props.data?.status?.text ?? '')

const isDisabled = computed(() => props.data?.disabled === true)

const messageCount = computed(() => {
  const count = props.data?.messageCount
  return typeof count === 'number' ? count : null
})
</script>

<template>
  <div
    class="flint-node"
    :class="{ 'selected': props.selected, 'opacity-50': isDisabled }"
  >
    <!-- Input Handle -->
    <Handle
      id="input-0"
      type="target"
      :position="Position.Left"
    />

    <!-- Node Header -->
    <div class="flint-node-header">
      <!-- Debug icon: eye / bug -->
      <span class="w-4 h-4 flex items-center justify-center text-[11px] text-green-400 shrink-0">
        ⬤
      </span>
      <span class="truncate flex-1">{{ label }}</span>
      <!-- Message count badge -->
      <span
        v-if="messageCount !== null"
        class="ml-auto text-[9px] text-terminal-text-dim bg-terminal-bg px-1 py-0 border border-terminal-border"
      >
        {{ messageCount }}
      </span>
    </div>

    <!-- Node Body -->
    <div class="flint-node-body flex items-center gap-1.5">
      <span class="text-[10px] text-terminal-text-dim">msg.payload</span>
      <span class="ml-auto text-[10px] text-terminal-text-dim">→ debug</span>
    </div>

    <!-- Status Bar -->
    <div class="flint-node-status">
      <span
        v-if="statusFill"
        class="w-[6px] h-[6px] shrink-0"
        :style="{ backgroundColor: statusFill }"
      />
      <span
        v-else
        class="w-[6px] h-[6px] shrink-0 border border-terminal-border bg-transparent"
      />
      <span class="truncate">{{ statusText || 'idle' }}</span>
    </div>
  </div>
</template>

<style scoped>
.flint-node {
  min-width: 140px;
  max-width: 200px;
  background-color: #252518;
  border: 1px solid #3a3a28;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  color: #FFBF00;
  font-size: 12px;
  user-select: none;
  border-radius: 0;
}

.flint-node.selected {
  border-color: #FFBF00;
  box-shadow: 0 0 10px #ffbf0033;
}

.flint-node-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-bottom: 1px solid #3a3a28;
  font-size: 11px;
  font-weight: bold;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.flint-node-body {
  padding: 4px 8px;
  color: #998a00;
  font-size: 10px;
}

.flint-node-status {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-top: 1px solid #3a3a28;
  color: #998a00;
  font-size: 10px;
}
</style>
