<script setup lang="ts">
import { computed, ref } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import BaseNode from '@/components/nodes/BaseNode.vue'
import NodeIcon from '@/components/nodes/NodeIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()

const label = computed(() => props.data?.label)
const enabled = ref(true)

const messageCount = computed(() => {
  const count = props.data?.messageCount
  return typeof count === 'number' ? count : null
})

function handleToggle(value: boolean) {
  enabled.value = value
}
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
    :toggle-button="true"
    :toggle-state="enabled"
    @toggle="handleToggle"
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
      <span class="text-terminal-text-dim">msg.payload → debug</span>
    </template>
  </BaseNode>
</template>
