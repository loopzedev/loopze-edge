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
      class="flex items-center gap-1 text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2
             cursor-pointer hover:text-terminal-text transition-colors w-full text-left"
    >
      <span class="transition-transform duration-100" :class="isOpen ? 'rotate-90' : ''">&#x25B8;</span>
      {{ title }}
    </CollapsibleTrigger>
    <CollapsibleContent>
      <slot />
    </CollapsibleContent>
  </CollapsibleRoot>

  <template v-else>
    <p class="text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2">
      &#x25B8; {{ title }}
    </p>
    <slot />
  </template>
</template>
