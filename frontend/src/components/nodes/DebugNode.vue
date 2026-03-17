<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import BaseNode from '@/components/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()

const label = computed(() => props.data?.label ?? 'Debug')

const messageCount = computed(() => {
  const count = props.data?.messageCount
  return typeof count === 'number' ? count : null
})
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="label"
    node-type="debug"
    accent-color="#4ade80"
    :selected="props.selected"
    :inputs="1"
    :outputs="0"
    :status="props.data?.status"
    :disabled="props.data?.disabled"
  >
    <template #icon>
      <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
        <path stroke-linecap="square" d="M12 8v4m0 4h.01" />
        <circle cx="12" cy="12" r="9" />
      </svg>
    </template>

    <template #badge>
      <span
        v-if="messageCount !== null"
        class="text-[9px] px-1 border border-terminal-border text-terminal-text-dim bg-terminal-bg"
      >
        {{ messageCount }}
      </span>
    </template>

    <template #body>
      <span class="opacity-60">msg.payload → debug</span>
    </template>
  </BaseNode>
</template>
