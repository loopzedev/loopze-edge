<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const property = useNodeProperty<string>('property', 'payload')
const action = useNodeProperty<string>('action', 'auto')
const root = useNodeProperty<string>('root', 'root')
const indent = useNodeProperty<number>('indent', 0)
const declaration = useNodeProperty<boolean>('declaration', true)

const actions = [
  { value: 'auto',      label: 'auto (string ↔ object)' },
  { value: 'parse',     label: 'parse (string → object)' },
  { value: 'stringify', label: 'stringify (object → string)' },
]
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Property">
      <FormInput
        v-model="property"
        placeholder="payload"
        mono
        :invalid="!property"
      >
        <template #prefix>msg.</template>
      </FormInput>
      <div v-if="!property" class="text-[10px] text-status-error leading-tight">
        Property name required
      </div>
    </FormField>

    <FormField label="Action">
      <FormSelect
        :model-value="action"
        :options="actions"
        @update:model-value="action = String($event)"
      />
    </FormField>

    <FormField v-if="action !== 'parse'" label="Root element">
      <FormInput
        v-model="root"
        placeholder="root"
        mono
        :invalid="!root"
      />
      <div v-if="!root" class="text-[10px] text-status-error leading-tight">
        Root element name required
      </div>
    </FormField>

    <FormField v-if="action !== 'parse'" label="Indent">
      <NumberInput
        :model-value="indent"
        :min="0"
        :max="8"
        :unit="indent === 1 ? 'space' : 'spaces'"
        @update:model-value="indent = Number($event)"
      />
    </FormField>

    <FormField v-if="action !== 'parse'">
      <FormCheckbox v-model="declaration" label="Prepend XML declaration" />
    </FormField>
  </div>
</template>
