<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const path = useNodeProperty<string>('path', '')
const mode = useNodeProperty<string>('mode', 'overwrite')
const encoding = useNodeProperty<string>('encoding', 'auto')
const createDirs = useNodeProperty<boolean>('createDirs', false)
const appendNewline = useNodeProperty<boolean>('appendNewline', false)
const rootJail = useNodeProperty<string>('rootJail', '')

const modes = [
  { value: 'overwrite', label: 'Overwrite (replace existing)' },
  { value: 'append',    label: 'Append (add to existing)' },
  { value: 'create',    label: 'Create only (fail if exists)' },
]

const encodings = [
  { value: 'auto',   label: 'Auto (by extension)' },
  { value: 'utf-8',  label: 'UTF-8 (text)' },
  { value: 'binary', label: 'Binary (bytes)' },
]
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Path">
      <FormInput
        v-model="path"
        placeholder="/var/log/events.log"
        mono
        :invalid="!path"
      />
      <div v-if="!path" class="text-[10px] text-status-error leading-tight">
        Absolute file path required
      </div>
      <div v-else class="text-[10px] text-text-muted leading-tight">
        {{ '{{mustache}}' }} supported; msg.filename overrides this path
      </div>
    </FormField>

    <FormField label="Mode">
      <FormSelect
        :model-value="mode"
        :options="modes"
        @update:model-value="mode = String($event)"
      />
    </FormField>

    <FormField label="Encoding">
      <FormSelect
        :model-value="encoding"
        :options="encodings"
        @update:model-value="encoding = String($event)"
      />
    </FormField>

    <FormCheckbox v-model="appendNewline" label="Append newline after each write" />
    <FormCheckbox v-model="createDirs" label="Create parent directories if missing" />

    <FormField label="Root jail (optional)">
      <FormInput
        v-model="rootJail"
        placeholder="/var/log"
        mono
      />
      <div class="text-[10px] text-text-muted leading-tight">
        Resolved paths must stay inside this directory. Empty disables the check.
      </div>
    </FormField>
  </div>
</template>
