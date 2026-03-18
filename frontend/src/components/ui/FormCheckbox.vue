<script setup lang="ts">
import { CheckboxRoot, CheckboxIndicator } from 'radix-vue'

defineProps<{
  modelValue: boolean
  label: string
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<template>
  <label class="flex items-center gap-2 cursor-pointer group">
    <CheckboxRoot
      :checked="modelValue"
      class="w-3.5 h-3.5 flex items-center justify-center
             bg-terminal-bg border border-terminal-border rounded-sm
             hover:border-terminal-text
             data-[state=checked]:bg-accent data-[state=checked]:border-accent
             transition-colors duration-75"
      @update:checked="$emit('update:modelValue', $event as boolean)"
    >
      <CheckboxIndicator class="text-white">
        <svg class="w-2.5 h-2.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M2 6l3 3 5-5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </CheckboxIndicator>
    </CheckboxRoot>
    <span class="text-xs text-terminal-text group-hover:text-terminal-text-bright transition-colors">
      {{ label }}
    </span>
  </label>
</template>
