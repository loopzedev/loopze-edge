<script setup lang="ts">
import { defineAsyncComponent, nextTick, computed } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'
import NumberInput from '@/components/ui/NumberInput.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import FormField from '@/components/ui/FormField.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const flowStore = useFlowStore()
const { updateNodeInternals } = useVueFlow('flint-flow-editor')

const code = useNodeProperty<string>(
  'code',
  'package main\n\nfunc handle(payload any) any {\n    return payload\n}\n',
)
const rawOutputs = useNodeProperty<number>('outputs', 1)

// Outputs is a structural property — mirror onto node.outputs and
// trigger updateNodeInternals so the handles re-render.
const outputs = computed({
  get: () => rawOutputs.value,
  set: (v: number) => {
    const clamped = Math.max(1, Math.min(10, v))
    const node = flowStore.selectedNode
    if (!node) return
    const cfg = (node.data?.config ?? {}) as Record<string, unknown>
    flowStore.updateNodeData(node.id, {
      config: { ...cfg, outputs: clamped },
      outputs: clamped,
    })
    nextTick(() => updateNodeInternals([node.id]))
  },
})

const outputPresets = [
  { label: '1', value: 1 },
  { label: '2', value: 2 },
  { label: '3', value: 3 },
]
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Code" class="flex-1 min-h-0">
      <template #action>
        <span class="text-[10px] text-terminal-text-dim font-mono">Go</span>
      </template>
      <CodeEditor
        v-model="code"
        language="go"
        min-height="220px"
      />
    </FormField>

    <FormField label="Outputs">
      <ToggleGroup v-model="outputs" :options="outputPresets" />
      <NumberInput v-model="outputs" :min="1" :max="10" :unit="outputs === 1 ? 'port' : 'ports'" />
    </FormField>
  </div>
</template>
