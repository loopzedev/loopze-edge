<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import FormField from '@/components/ui/FormField.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const code = useNodeProperty<string>(
  'code',
  'package main\n\nfunc handle(payload any) any {\n    return payload\n}\n',
)
const outputs = useStructuralProperty<number>('outputs', 1, {
  port: 'outputs',
  clamp: { min: 1, max: 10 },
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
        min-height="400px"
      />
    </FormField>

    <FormField label="Outputs">
      <ToggleGroup v-model="outputs" :options="outputPresets" />
      <NumberInput v-model="outputs" :min="1" :max="10" :unit="outputs === 1 ? 'port' : 'ports'" />
    </FormField>
  </div>
</template>
