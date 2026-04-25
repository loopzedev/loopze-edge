<script setup lang="ts">
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const output = useNodeProperty<string>('output', 'property')
const property = useNodeProperty<string>('property', 'payload')
const statusEnabled = useNodeProperty<boolean>('statusEnabled', false)
const statusOutput = useNodeProperty<string>('statusOutput', 'same')
const statusProperty = useNodeProperty<string>('statusProperty', '')

const outputOptions = [
  { label: 'msg. property', value: 'property' },
  { label: 'Complete message', value: 'message' },
  { label: 'GJSON expression', value: 'gjson' },
]

const statusOutputOptions = [
  { label: 'Same as debug output', value: 'same' },
  { label: 'msg. property', value: 'property' },
  { label: 'GJSON expression', value: 'gjson' },
  { label: 'Message count', value: 'count' },
]
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Output">
      <FormSelect v-model="output" :options="outputOptions" />
    </FormField>

    <FormField v-if="output === 'property'" label="Property">
      <FormInput v-model="property" placeholder="payload" mono>
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField v-if="output === 'gjson'" label="GJSON Path">
      <FormInput v-model="property" placeholder="payload.items.#" mono />
    </FormField>

    <FormCheckbox v-model="statusEnabled" label="Show node status (max. 32 chars)" />

    <template v-if="statusEnabled">
      <FormField label="Status Source">
        <FormSelect v-model="statusOutput" :options="statusOutputOptions" />
      </FormField>

      <FormField v-if="statusOutput === 'property'" label="Status Property">
        <FormInput v-model="statusProperty" placeholder="payload" mono>
          <template #prefix>msg.</template>
        </FormInput>
      </FormField>

      <FormField v-if="statusOutput === 'gjson'" label="Status GJSON Path">
        <FormInput v-model="statusProperty" placeholder="payload.items.#" mono />
      </FormField>
    </template>
  </div>
</template>
