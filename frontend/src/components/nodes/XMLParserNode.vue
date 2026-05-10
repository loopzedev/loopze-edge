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
  const property = (cfg.property as string) || 'payload'
  const action = (cfg.action as string) || 'auto'
  const indent = Number(cfg.indent ?? 0)
  if (action === 'stringify' && indent > 0) return `${property} · stringify (${indent})`
  return `${property} · ${action}`
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="xml"
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
