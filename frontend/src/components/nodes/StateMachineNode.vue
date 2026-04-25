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

function machineId(config: Record<string, unknown> | undefined): string {
  if (!config?.machine) return ''
  try {
    const def = JSON.parse(config.machine as string)
    return def.id ?? ''
  } catch { return '' }
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="statemachine"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 2"
    :status="(props.data.status as any)"
    :disabled="props.data.disabled"
  >
    <template #body>
      <span class="truncate block">
        {{ machineId(props.data.config) || 'state machine' }}
      </span>
    </template>
  </BaseNode>
</template>
