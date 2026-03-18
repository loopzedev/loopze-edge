<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const flowStore = useFlowStore()
const { updateNodeInternals } = useVueFlow('flint-flow-editor')

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const funcCode = computed({
  get: () => (config.value.func as string) ?? 'return msg;',
  set: (v: string) => update('func', v),
})

const outputs = computed({
  get: () => (config.value.outputs as number) ?? 1,
  set: (v: number) => {
    if (!node.value) return
    const clamped = Math.max(1, Math.min(10, v))
    const nodeId = node.value.id
    flowStore.updateNodeData(nodeId, {
      config: { ...config.value, outputs: clamped },
      outputs: clamped,
    })
    nextTick(() => updateNodeInternals(nodeId))
  },
})

const outputPresets = [
  { label: '1', value: 1 },
  { label: '2', value: 2 },
  { label: '3', value: 3 },
]
</script>

<template>
  <div class="flex flex-col gap-2 flex-1 min-h-0">
    <!-- Code editor -->
    <div class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between">
        <FormLabel>Function Body</FormLabel>
        <span class="text-[10px] text-terminal-text-dim">JavaScript</span>
      </div>
      <CodeEditor
        v-model="funcCode"
        placeholder="return msg;"
        min-height="180px"
      />
    </div>

    <!-- Outputs -->
    <div class="flex flex-col gap-1">
      <FormLabel>Outputs</FormLabel>
      <div class="flex items-center gap-2">
        <FormInput
          :model-value="String(outputs)"
          type="number"
          placeholder="1"
          @update:model-value="outputs = Number($event)"
        />
        <span class="text-[10px] text-terminal-text-dim">port{{ outputs !== 1 ? 's' : '' }}</span>
      </div>
      <ToggleGroup v-model="outputs" :options="outputPresets" />
    </div>
  </div>
</template>
