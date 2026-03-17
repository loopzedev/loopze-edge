<script setup lang="ts">
import BaseNode from '@/components/nodes/BaseNode.vue'

interface Props {
  id: string
  data: {
    label?: string
    nodeType?: string
    config?: Record<string, unknown>
    status?: {
      fill?: string
      shape?: string
      text?: string
    } | null
    inputs?: number
    outputs?: number
    disabled?: boolean
  }
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  selected: false,
})
</script>

<template>
  <BaseNode
    :id="props.id"
    :data="props.data"
    :selected="props.selected"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    node-color="#665500"
  >
    <template #icon>
      <div class="w-6 h-6 flex items-center justify-center border border-terminal-border bg-terminal-bg text-amber text-sm font-bold">
        ƒ
      </div>
    </template>

    <template #body>
      <div class="px-2 py-1.5 text-[10px] text-terminal-text-dim">
        <div
          v-if="props.data.config?.func"
          class="truncate max-w-[120px]"
          :title="String(props.data.config.func)"
        >
          <span class="text-terminal-text-dim opacity-60">»</span>
          {{ String(props.data.config.func).split('\n')[0].slice(0, 30) }}
        </div>
        <div v-else class="italic opacity-50">
          // empty function
        </div>
      </div>
    </template>
  </BaseNode>
</template>
