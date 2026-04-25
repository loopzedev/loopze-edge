<script setup lang="ts">
import { useSlots } from 'vue'

withDefaults(defineProps<{
  modelValue: string | number
  type?: string
  placeholder?: string
  mono?: boolean
  invalid?: boolean
}>(), {
  type: 'text',
  placeholder: '',
  mono: false,
  invalid: false,
})

defineEmits<{
  'update:modelValue': [value: string]
}>()

const slots = useSlots()
const hasPrefix = !!slots.prefix
const hasSuffix = !!slots.suffix
</script>

<template>
  <div
    v-if="hasPrefix || hasSuffix"
    class="flex items-stretch w-full bg-terminal-bg border text-terminal-text
           focus-within:border-accent transition-colors"
    :class="invalid ? 'border-status-error' : 'border-terminal-border'"
  >
    <span
      v-if="hasPrefix"
      class="flex items-center px-2 text-[10px] text-terminal-text-dim border-r border-terminal-border bg-terminal-surface-alt/30 shrink-0"
    >
      <slot name="prefix" />
    </span>

    <input
      :value="modelValue"
      :type="type"
      :placeholder="placeholder"
      class="flex-1 min-w-0 bg-transparent px-2 py-1 text-[10px] outline-none placeholder:text-terminal-text-dim"
      :class="{ 'font-mono': mono }"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />

    <span
      v-if="hasSuffix"
      class="flex items-center px-2 text-[10px] text-terminal-text-dim border-l border-terminal-border bg-terminal-surface-alt/30 shrink-0"
    >
      <slot name="suffix" />
    </span>
  </div>

  <input
    v-else
    :value="modelValue"
    :type="type"
    :placeholder="placeholder"
    class="bg-terminal-bg border text-terminal-text
           px-2 py-1 text-[10px] outline-none
           focus:border-accent focus:ring-0
           placeholder:text-terminal-text-dim w-full transition-colors"
    :class="[
      mono ? 'font-mono' : '',
      invalid ? 'border-status-error' : 'border-terminal-border',
    ]"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
