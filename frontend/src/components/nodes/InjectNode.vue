<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { NodeProps } from '@vue-flow/core'
import { useApi } from '@/composables/useApi'

const props = defineProps<NodeProps>()

const api = useApi()

const label = computed(() => props.data?.label ?? 'Inject')
const statusText = computed(() => props.data?.status?.text ?? '')
const statusFill = computed(() => props.data?.status?.fill ?? '')
const isDisabled = computed(() => props.data?.disabled ?? false)

const intervalLabel = computed(() => {
  const cfg = props.data?.config ?? {}
  const interval = cfg.interval as number | undefined
  const once = cfg.once as boolean | undefined

  const parts: string[] = []
  if (once) parts.push('once')
  if (interval && interval > 0) {
    if (interval >= 60000) parts.push(`${interval / 60000}min`)
    else if (interval >= 1000) parts.push(`${interval / 1000}s`)
    else parts.push(`${interval}ms`)
  }
  return parts.length > 0 ? parts.join(' + ') : 'manual'
})

const statusColorClass = computed(() => {
  switch (statusFill.value) {
    case 'green':
      return 'bg-green-500'
    case 'red':
      return 'bg-red-500'
    case 'yellow':
      return 'bg-yellow-500'
    case 'blue':
      return 'bg-blue-500'
    default:
      return 'bg-gray-500'
  }
})

async function handleTrigger(): Promise<void> {
  try {
    await api.triggerInject(props.id)
  } catch (err) {
    console.error('[InjectNode] Trigger failed:', err)
  }
}
</script>

<template>
  <div
    class="flint-node min-w-[160px]"
    :class="{
      'selected': props.selected,
      'opacity-40': isDisabled,
    }"
  >
    <!-- Node Header -->
    <div class="flint-node-header" style="border-left: 3px solid #FFBF00;">
      <!-- Trigger button -->
      <button
        class="w-5 h-5 flex items-center justify-center border border-terminal-border bg-terminal-bg text-terminal-text hover:bg-amber hover:text-terminal-bg active:bg-amber-dim transition-colors duration-75 shrink-0"
        title="Trigger inject"
        @click.stop="handleTrigger"
        @mousedown.stop
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="w-3 h-3"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="square" stroke-linejoin="miter" d="M5 3l14 9-14 9V3z" />
        </svg>
      </button>

      <!-- Node type icon -->
      <span class="text-[10px] text-terminal-text-dim">⏱</span>

      <!-- Label -->
      <span class="truncate text-amber text-xs font-bold uppercase tracking-wide">
        {{ label }}
      </span>
    </div>

    <!-- Node Body -->
    <div class="flint-node-body">
      <div class="flex items-center gap-1.5 text-[10px] text-terminal-text-dim">
        <span class="uppercase tracking-wider">Trigger</span>
        <span class="text-terminal-text">
          {{ intervalLabel }}
        </span>
      </div>
      <div class="flex items-center gap-1.5 text-[10px] text-terminal-text-dim mt-0.5">
        <span class="uppercase tracking-wider">Payload</span>
        <span class="text-terminal-text truncate max-w-[100px]">
          {{ props.data?.config?.payloadType ?? 'timestamp' }}
        </span>
      </div>
    </div>

    <!-- Node Status -->
    <div v-if="statusText" class="flint-node-status">
      <span class="w-1.5 h-1.5 shrink-0" :class="statusColorClass"></span>
      <span class="truncate">{{ statusText }}</span>
    </div>

    <!-- Output Handle (right side) -->
    <Handle
      type="source"
      :position="Position.Right"
      id="output-0"
      class="!w-[10px] !h-[10px] !rounded-none !bg-amber-dim !border !border-amber hover:!bg-amber"
    />
  </div>
</template>

<style scoped>
.flint-node {
  background-color: #252518;
  border: 1px solid #3a3a28;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  color: #FFBF00;
  font-size: 12px;
  user-select: none;
}

.flint-node.selected {
  border-color: #FFBF00;
  box-shadow: 0 0 10px #ffbf0033;
}
</style>
