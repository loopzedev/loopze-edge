<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const expression     = useNodeProperty<string>('expression', 'payload')
const outputProperty = useNodeProperty<string>('outputProperty', 'payload')
const passThrough    = useNodeProperty<boolean>('passThrough', false)
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Expression" class="flex-1 min-h-0">
      <template #action>
        <span class="text-[10px] text-terminal-text-dim font-mono">expr</span>
      </template>
      <CodeEditor
        v-model="expression"
        language="expr"
        placeholder="payload"
        min-height="240px"
      />
    </FormField>

    <FormField label="Output Property">
      <FormInput
        v-model="outputProperty"
        placeholder="payload"
      />
    </FormField>

    <FormField label="Pass Through">
      <AppSwitch
        v-model="passThrough"
        label="Keep original message fields, only overwrite output property"
      />
    </FormField>
  </div>
</template>
