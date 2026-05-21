<script setup lang="ts">
import BaseNode from '@/components/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

interface Props {
  id: string
  data: {
    label?: string
    config?: Record<string, unknown>
    status?: { fill?: string; shape?: string; text?: string } | null
    disabled?: boolean
  }
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), { selected: false })

function bodyText(): string {
  const cfg = props.data.config ?? {}
  const buttonLabel = (cfg.label as string) || 'Click me'
  const topic = cfg.topic ? ` · ${cfg.topic as string}` : ''
  return `${buttonLabel}${topic}`
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="ui-button"
    :selected="props.selected"
    :inputs="0"
    :outputs="1"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #body>
      <span class="truncate block">{{ bodyText() }}</span>
    </template>
  </BaseNode>
</template>
