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

function bodyText(): string {
  const cfg = props.data.config ?? {}
  const mode = (cfg.mode as string) || 'append'
  const path = (cfg.path as string) || '(no path)'
  // Show only the file name in the body to keep the node body compact.
  const tail = path.split('/').pop() || path
  return `${mode} · ${tail}`
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="csv-out"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #body>
      <span class="truncate block">{{ bodyText() }}</span>
    </template>
  </BaseNode>
</template>
