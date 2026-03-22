<script setup lang="ts">
withDefaults(defineProps<{
  title: string
  closable?: boolean
}>(), {
  closable: false,
})

defineEmits<{
  close: []
}>()
</script>

<template>
  <div class="flex items-center justify-between px-4 py-2.5 border-b border-terminal-border shrink-0 bg-terminal-surface">
    <span class="text-xs font-semibold uppercase tracking-wider text-terminal-text">{{ title }}</span>
    <div class="flex items-center gap-1">
      <slot name="actions" />
      <button
        v-if="closable"
        class="w-6 h-6 flex items-center justify-center text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt rounded transition-all duration-100"
        title="Close panel"
        @click="$emit('close')"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  </div>
</template>
