<script setup lang="ts">
defineProps<{
  isDragging?: boolean
  isDropTarget?: boolean
  isInvalid?: boolean
}>()

defineEmits<{
  remove: []
  'handle-mousedown': []
}>()
</script>

<template>
  <div
    class="group relative bg-terminal-surface-alt/40 border-y border-r border-l-2 border-y-terminal-border border-r-terminal-border p-2 flex flex-col gap-1.5 transition-all duration-100"
    :class="[
      isDragging ? 'opacity-40' : '',
      isDropTarget && !isDragging ? 'border-l-accent' : isInvalid ? 'border-l-status-error' : 'border-l-transparent group-hover:border-l-accent/40 hover:border-l-accent/40',
    ]"
  >
    <div class="flex items-start gap-2">
      <!-- 6-dot drag handle -->
      <button
        type="button"
        class="shrink-0 cursor-grab active:cursor-grabbing opacity-30 group-hover:opacity-100 hover:text-terminal-text text-terminal-text-dim transition-opacity p-0.5 -ml-0.5 mt-1"
        title="Drag to reorder"
        aria-label="Drag to reorder"
        @mousedown="$emit('handle-mousedown')"
      >
        <span class="grid grid-cols-2 gap-[2px] w-[6px]">
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
          <span class="w-[2px] h-[2px] rounded-full bg-current" />
        </span>
      </button>

      <div class="flex-1 min-w-0 flex flex-col gap-1.5">
        <slot />
      </div>

      <button
        type="button"
        class="shrink-0 text-terminal-text-dim hover:text-status-error text-xs leading-none p-1 -mr-0.5 -mt-0.5 transition-colors"
        title="Remove"
        aria-label="Remove"
        @click="$emit('remove')"
      >&#x2715;</button>
    </div>
  </div>
</template>
