<script setup lang="ts">
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import FormField from '@/components/ui/FormField.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const scope = useNodeProperty<string>('scope', 'global')
const storage = useNodeProperty<string>('storage', 'memory')
const keyPattern = useNodeProperty<string>('keyPattern', '>')
const emitDeletes = useNodeProperty<boolean>('emitDeletes', false)

const scopeOptions = [
  { label: 'global', value: 'global' },
  { label: 'flow', value: 'flow' },
]

const storageOptions = [
  { label: 'memory', value: 'memory' },
  { label: 'persistent', value: 'persistent' },
]
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Scope">
      <ToggleGroup v-model="scope" :options="scopeOptions" />
    </FormField>

    <FormField label="Storage">
      <ToggleGroup v-model="storage" :options="storageOptions" />
    </FormField>

    <FormField label="Key Pattern" hint="Use > for all keys, or a specific key name">
      <FormInput v-model="keyPattern" placeholder=">" mono />
    </FormField>

    <FormCheckbox v-model="emitDeletes" label="Emit delete operations" />
  </div>
</template>
