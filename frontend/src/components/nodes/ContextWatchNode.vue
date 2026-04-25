<script setup lang="ts">
import BaseNode from './BaseNode.vue'

interface Props {
  id: string
  data: {
    label?: string
    nodeType?: string
    config?: Record<string, any>
    status?: any
    inputs?: number
    outputs?: number
    disabled?: boolean
  }
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  selected: false,
})

defineOptions({ inheritAttrs: false })
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="context-watch"
    :inputs="props.data.inputs ?? 0"
    :outputs="props.data.outputs ?? 1"
    :selected="props.selected"
    :disabled="props.data.disabled"
    :status="props.data.status"
  >
    <template #body>
      <span class="uppercase tracking-wider text-[9px]">
        {{ props.data.config?.scope ?? 'global' }}:{{ props.data.config?.storage ?? 'memory' }}
      </span>
      <span class="ml-1">
        {{ props.data.config?.keyPattern ?? '>' }}
      </span>
    </template>
  </BaseNode>
</template>
