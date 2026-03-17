<script setup lang="ts">
import { computed } from 'vue'
import type { NodeProps } from '@vue-flow/core'
import { useApi } from '@/composables/useApi'
import BaseNode from '@/components/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<NodeProps>()
const api = useApi()

const label = computed(() => props.data?.label ?? 'Inject')

const intervalLabel = computed(() => {
  const cfg = props.data?.config ?? {}
  const interval = cfg.interval as number | undefined
  const once = cfg.once as boolean | undefined
  const parts: string[] = []
  if (once) parts.push('once')
  if (interval && interval > 0) {
    if (interval >= 60000) parts.push(`${interval / 60000}min`)
    else if (interval >= 1000) parts.push(`${interval / 1000}s`)
    else parts.push(`${interval}ms`)
  }
  return parts.length > 0 ? parts.join(' + ') : 'manual'
})

async function handleTrigger(): Promise<void> {
  try {
    await api.triggerInject(props.id)
  } catch (err) {
    console.error('[InjectNode] Trigger failed:', err)
  }
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="label"
    node-type="inject"
    accent-color="#7fa8c9"
    :selected="props.selected"
    :inputs="0"
    :outputs="1"
    :status="props.data?.status"
    :disabled="props.data?.disabled"
  >
    <template #icon>
      <button
        class="w-4 h-4 flex items-center justify-center hover:text-accent active:opacity-60 transition-colors"
        style="color: #7fa8c9"
        title="Trigger inject"
        @click.stop="handleTrigger"
        @mousedown.stop
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
          <path d="M5 3l14 9-14 9V3z" />
        </svg>
      </button>
    </template>

    <template #body>
      <div class="flex items-center justify-between gap-1">
        <span class="uppercase tracking-wider opacity-60">{{ intervalLabel }}</span>
        <span class="opacity-60 truncate">{{ props.data?.config?.payloadType ?? 'timestamp' }}</span>
      </div>
    </template>
  </BaseNode>
</template>
