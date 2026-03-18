<script setup lang="ts">
withDefaults(defineProps<{
  modelValue: string | number
  type?: string
  placeholder?: string
  mono?: boolean
}>(), {
  type: 'text',
  placeholder: '',
  mono: false,
})

defineEmits<{
  'update:modelValue': [value: string]
}>()
</script>

<template>
  <input
    :value="modelValue"
    :type="type"
    :placeholder="placeholder"
    class="bg-terminal-bg border border-terminal-border text-terminal-text
           px-2 py-1 text-[10px] outline-none
           focus:border-accent focus:ring-0
           placeholder:text-terminal-text-dim w-full"
    :class="{ 'font-mono': mono }"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
