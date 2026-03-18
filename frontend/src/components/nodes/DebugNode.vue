<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import BaseNode from '@/components/nodes/BaseNode.vue'
import NodeIcon from '@/components/nodes/NodeIcon.vue'
import { useFlowStore } from '@/stores/flowStore'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()
const flowStore = useFlowStore()

const label = computed(() => props.data?.label)

const enabled = computed(() => {
  const active = props.data?.config?.active
  return active !== false
})

const bodyText = computed(() => {
  const cfg = props.data?.config
  const output = (cfg?.output as string) ?? 'property'
  if (output === 'message') return 'complete msg \u2192 debug'
  if (output === 'gjson') return `gjson: ${(cfg?.property as string) ?? ''}`
  return `msg.${(cfg?.property as string) ?? 'payload'} \u2192 debug`
})

const messageCount = computed(() => {
  const count = props.data?.messageCount
  return typeof count === 'number' ? count : null
})

function handleToggle(value: boolean) {
  flowStore.updateNodeData(props.id, {
    config: { ...props.data?.config, active: value },
  })
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
      <span class="text-terminal-text-dim">{{ bodyText }}</span>
    </template>
  </BaseNode>
</template>
