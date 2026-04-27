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

function handleSignature(code: unknown): string {
  if (!code) return ''
  const lines = String(code).split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (trimmed.startsWith('func handle')) return trimmed
  }
  return lines.find(l => l.trim()) ?? ''
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="function-go"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #body>
      <span class="truncate block" :title="String(props.data.config?.code ?? '')">
        {{ handleSignature(props.data.config?.code) || '// empty' }}
      </span>
    </template>
  </BaseNode>
</template>
