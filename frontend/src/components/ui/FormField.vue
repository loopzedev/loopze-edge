<script setup lang="ts">
import { useSlots } from 'vue'

withDefaults(defineProps<{
  label?: string
  hint?: string
  error?: string
  /** Renders label & control on a single row (e.g. for checkbox-style fields). */
  inline?: boolean
}>(), {
  label: '',
  hint: '',
  error: '',
  inline: false,
})

const slots = useSlots()
const hasAction = !!slots.action
</script>

<template>
  <div class="flex flex-col gap-1" :class="inline ? 'flex-row items-center' : ''">
    <div v-if="label || hasAction" class="flex items-center justify-between gap-2">
      <span
        v-if="label"
        class="text-[10px] text-terminal-text-dim uppercase tracking-wider font-semibold"
      >
        {{ label }}
      </span>
      <span v-if="hasAction" class="flex items-center gap-1">
        <slot name="action" />
      </span>
    </div>

    <slot />

    <div v-if="error" class="text-[10px] text-status-error leading-tight">
      {{ error }}
    </div>
    <div v-else-if="hint" class="text-[10px] text-terminal-text-dim/80 leading-tight">
      {{ hint }}
    </div>
  </div>
</template>
