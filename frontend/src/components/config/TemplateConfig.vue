<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { STORAGE_TYPES, isContextScope } from '@/components/config/enums'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const template = useNodeProperty<string>('template', '')
const field = useNodeProperty<string>('field', 'payload')
const fieldType = useNodeProperty<string>('fieldType', 'msg')
const fieldStorage = useNodeProperty<string>('fieldStorage', 'memory')
const format = useNodeProperty<string>('format', 'plain')
const syntax = useNodeProperty<string>('syntax', 'mustache')

const scopes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
]

const formats = [
  { value: 'plain', label: 'plain text' },
  { value: 'json', label: 'parsed JSON' },
]

const syntaxes = [
  { value: 'mustache', label: 'Mustache' },
  { value: 'plain', label: 'plain (no parsing)' },
]
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Template" class="flex-1 min-h-0">
      <template #action>
        <span class="text-[10px] text-terminal-text-dim font-mono">{{ syntax === 'plain' ? 'text' : 'mustache' }}</span>
      </template>
      <CodeEditor
        v-model="template"
        language="plaintext"
        placeholder="Hello {{ '{{' }}payload{{ '}}' }}!"
        min-height="180px"
      />
    </FormField>

    <FormField label="Output">
      <div class="flex items-center gap-1.5">
        <FormSelect
          :model-value="fieldType"
          :options="scopes"
          width="80px"
          @update:model-value="fieldType = String($event)"
        />
        <FormInput
          v-model="field"
          placeholder="property"
          mono
          class="flex-1 min-w-0"
          :invalid="!field"
        />
        <FormSelect
          v-if="isContextScope(fieldType)"
          :model-value="fieldStorage"
          :options="STORAGE_TYPES"
          width="88px"
          @update:model-value="fieldStorage = String($event)"
        />
      </div>
      <div v-if="!field" class="text-[10px] text-status-error leading-tight">
        Property name required
      </div>
    </FormField>

    <FormField label="Format">
      <FormSelect
        :model-value="format"
        :options="formats"
        @update:model-value="format = String($event)"
      />
    </FormField>

    <FormField label="Syntax">
      <FormSelect
        :model-value="syntax"
        :options="syntaxes"
        @update:model-value="syntax = String($event)"
      />
    </FormField>
  </div>
</template>
