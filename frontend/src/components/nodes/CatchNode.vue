<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import BaseNode from '@/components/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()

const label = computed(() => props.data?.label)

const scopeLabel = computed(() => {
  const scope = (props.data?.config?.scope as string | undefined) ?? 'flow'
  if (scope === 'all') return 'all flows'
  if (scope === 'selected') {
    const targets = (props.data?.config?.targetNodes as string[] | undefined) ?? []
    return targets.length === 1 ? '1 node' : `${targets.length} nodes`
  }
  return 'this flow'
})
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="label"
    node-type="catch"
    :selected="props.selected"
    :inputs="0"
    :outputs="1"
    :status="props.data?.status"
    :disabled="props.data?.disabled"
  >
    <template #body>
      <div class="flex items-center justify-between gap-1">
        <span class="uppercase tracking-wider">scope</span>
        <span class="truncate">{{ scopeLabel }}</span>
      </div>
    </template>
  </BaseNode>
</template>
