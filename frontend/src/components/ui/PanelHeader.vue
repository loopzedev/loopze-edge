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
  <div class="flex items-center justify-between px-3 py-2 border-b border-terminal-border shrink-0">
    <span class="text-xs uppercase tracking-widest text-terminal-text-dim">{{ title }}</span>
    <slot name="actions" />
    <button
      v-if="closable"
      class="w-6 h-6 flex items-center justify-center text-terminal-text-dim hover:text-accent hover:bg-terminal-border transition-colors duration-100"
      title="Close panel"
      @click="$emit('close')"
    >
      &#x2715;
    </button>
  </div>
</template>
