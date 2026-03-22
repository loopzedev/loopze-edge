<script setup lang="ts">
import { ref } from 'vue'
import { CollapsibleRoot, CollapsibleTrigger, CollapsibleContent } from 'radix-vue'

withDefaults(defineProps<{
  title: string
  collapsible?: boolean
  defaultOpen?: boolean
}>(), {
  collapsible: true,
  defaultOpen: true,
})

const isOpen = ref(true)
</script>

<template>
  <CollapsibleRoot v-if="collapsible" v-model:open="isOpen">
    <CollapsibleTrigger
      class="flex items-center gap-1.5 text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2.5
             cursor-pointer hover:text-terminal-text transition-colors w-full text-left font-semibold"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        class="w-3 h-3 transition-transform duration-150 shrink-0"
        :class="isOpen ? 'rotate-90' : ''"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
      </svg>
      {{ title }}
    </CollapsibleTrigger>
    <CollapsibleContent>
      <slot />
    </CollapsibleContent>
  </CollapsibleRoot>

  <template v-else>
    <p class="flex items-center gap-1.5 text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2.5 font-semibold">
      <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3 rotate-90 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
      </svg>
      {{ title }}
    </p>
    <slot />
  </template>
</template>
