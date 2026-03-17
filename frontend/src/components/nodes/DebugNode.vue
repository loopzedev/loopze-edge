<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import BaseNode from '@/components/nodes/BaseNode.vue'
import NodeIcon from '@/components/nodes/NodeIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()

const label = computed(() => props.data?.label)

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
    :selected="props.selected"
    :inputs="1"
    :outputs="0"
    :status="props.data?.status"
    :disabled="props.data?.disabled"
  >
    <template #icon><NodeIcon type="debug" /></template>

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
