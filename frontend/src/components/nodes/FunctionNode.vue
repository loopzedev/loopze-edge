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

function firstLine(code: unknown): string {
  if (!code) return ''
  return String(code).split('\n').find(l => l.trim()) ?? ''
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="function"
    accent-color="#FFBF00"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #icon>ƒ</template>

    <template #body>
      <span class="truncate block" :title="String(props.data.config?.func ?? '')">
        {{ firstLine(props.data.config?.func) || '// empty' }}
      </span>
    </template>
  </BaseNode>
</template>
