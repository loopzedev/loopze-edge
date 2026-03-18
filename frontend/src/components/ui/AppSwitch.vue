<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'radix-vue'

withDefaults(defineProps<{
  modelValue: boolean
  label?: string
}>(), {
  label: '',
})

defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<template>
  <label class="flex items-center gap-2 cursor-pointer group">
    <SwitchRoot
      :checked="modelValue"
      class="w-7 h-4 rounded-full relative transition-colors duration-100
             bg-terminal-border
             data-[state=checked]:bg-accent"
      @update:checked="$emit('update:modelValue', $event)"
    >
      <SwitchThumb
        class="block w-3 h-3 rounded-full bg-white shadow transition-transform duration-100
               translate-x-0.5 data-[state=checked]:translate-x-3.5"
      />
    </SwitchRoot>
    <span v-if="label" class="text-xs text-terminal-text group-hover:text-terminal-text-bright transition-colors">
      {{ label }}
    </span>
  </label>
</template>
