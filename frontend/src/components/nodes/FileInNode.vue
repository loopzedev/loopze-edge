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
  const mode = (cfg.mode as string) || 'read'
  const incremental = Boolean(cfg.incremental)
  const path = (cfg.path as string) || '(no path)'
  const tag = incremental ? `${mode}+inc` : mode
  return `${tag} · ${path}`
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="file-in"
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
