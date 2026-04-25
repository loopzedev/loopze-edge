<script setup lang="ts">
import { computed } from 'vue'
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

const props = withDefaults(defineProps<Props>(), { selected: false })

defineOptions({ inheritAttrs: false })

const ruleCount = computed(() => {
  const rules = props.data.config?.rules
  return Array.isArray(rules) ? rules.length : 0
})
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="change"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :selected="props.selected"
    :disabled="props.data.disabled"
    :status="props.data.status"
  >
    <template #body>
      <span>{{ ruleCount }} rule{{ ruleCount !== 1 ? 's' : '' }}</span>
    </template>
  </BaseNode>
</template>
