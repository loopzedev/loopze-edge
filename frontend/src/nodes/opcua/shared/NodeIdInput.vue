<script setup lang="ts">
import { computed } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
  }>(),
  {
    placeholder: 'ns=2;s=Demo.Variable',
  },
)

defineEmits<{
  'update:modelValue': [value: string]
}>()

// Mirror the backend regex in opcua_types.go:nodeIDPattern so users get an
// inline warning before deploy fails. Empty values are not flagged here —
// callers display "required" through their own validation logic.
const pattern = /^(?:ns=\d+;)?[isgb]=.+$/
const isValid = computed(() => props.modelValue === '' || pattern.test(props.modelValue))
</script>

<template>
  <FormInput
    :model-value="modelValue"
    :placeholder="placeholder"
    mono
    :invalid="!isValid"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>
