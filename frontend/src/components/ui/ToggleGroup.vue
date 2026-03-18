<script setup lang="ts">
import { ToggleGroupRoot, ToggleGroupItem } from 'radix-vue'

defineProps<{
  modelValue: string | number
  options: { value: string | number; label: string }[]
}>()

defineEmits<{
  'update:modelValue': [value: string | number]
}>()
</script>

<template>
  <ToggleGroupRoot
    type="single"
    :model-value="String(modelValue)"
    class="flex flex-wrap gap-1"
    @update:model-value="(v: any) => { if (typeof v === 'string') $emit('update:modelValue', v) }"
  >
    <ToggleGroupItem
      v-for="opt in options"
      :key="String(opt.value)"
      :value="String(opt.value)"
      class="px-1.5 py-0.5 text-[10px] border border-terminal-border
             transition-colors duration-75 cursor-pointer
             bg-terminal-bg text-terminal-text-dim
             hover:text-terminal-text hover:border-terminal-text
             data-[state=on]:bg-accent data-[state=on]:text-terminal-bg data-[state=on]:border-accent"
    >
      {{ opt.label }}
    </ToggleGroupItem>
  </ToggleGroupRoot>
</template>
