<script setup lang="ts">
import BaseNode from '@/components/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

interface Props {
  id: string
  data: {
    label?: string
    config?: Record<string, unknown>
    status?: { fill?: string; shape?: string; text?: string } | null
    inputs?: number
    outputs?: number
    disabled?: boolean
  }
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), { selected: false })

function firstLine(text: unknown): string {
  if (!text) return ''
  return String(text).split('\n').find(l => l.trim()) ?? ''
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="template"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #body>
      <span class="truncate block" :title="String(props.data.config?.template ?? '')">
        {{ firstLine(props.data.config?.template) || '(empty)' }}
      </span>
    </template>
  </BaseNode>
</template>
