<script setup lang="ts">
withDefaults(defineProps<{
  title?: string
  variant?: 'ghost' | 'danger'
}>(), {
  title: '',
  variant: 'ghost',
})
</script>

<template>
  <button
    class="w-6 h-6 flex items-center justify-center transition-colors duration-100 shrink-0"
    :class="{
      'text-terminal-text-dim hover:text-terminal-text': variant === 'ghost',
      'text-terminal-text-dim hover:text-red-400': variant === 'danger',
    }"
    :title="title"
  >
    <slot />
  </button>
</template>
