<script setup lang="ts">
defineProps<{
  modelValue: string
  tabs: { id: string; label: string; badge?: string | number }[]
}>()

defineEmits<{
  'update:modelValue': [value: string]
}>()
</script>

<template>
  <div class="flex border-b border-terminal-border" role="tablist">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      type="button"
      role="tab"
      :aria-selected="modelValue === tab.id"
      class="px-3 py-1.5 text-[10px] uppercase tracking-wider font-semibold transition-all duration-100 cursor-pointer flex items-center gap-1.5 border-b-2 -mb-px"
      :class="modelValue === tab.id
        ? 'text-accent border-accent bg-accent/5'
        : 'text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt/50 border-transparent'"
      @click="$emit('update:modelValue', tab.id)"
    >
      {{ tab.label }}
      <span
        v-if="tab.badge !== undefined && tab.badge !== ''"
        class="text-[9px] px-1 py-px rounded bg-terminal-surface-alt text-terminal-text-dim font-mono"
      >{{ tab.badge }}</span>
    </button>
  </div>
</template>
