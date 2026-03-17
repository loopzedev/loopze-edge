<script setup lang="ts">
import { useUiStore } from '@/stores/uiStore'
import DebugPanel from '@/components/DebugPanel.vue'

const ui = useUiStore()
</script>

<template>
  <div class="h-full w-[320px] flex-shrink-0 flex flex-col bg-terminal-surface border-l border-terminal-border font-mono select-none">
    <!-- Header -->
    <div class="flex items-center justify-between px-3 py-2 border-b border-terminal-border shrink-0">
      <span class="text-xs uppercase tracking-widest text-terminal-text-dim">Debug</span>
      <button
        class="w-6 h-6 flex items-center justify-center text-terminal-text-dim hover:text-accent hover:bg-terminal-border transition-colors duration-100"
        title="Close panel"
        @click="ui.closeDebugPanel()"
      >
        ✕
      </button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-hidden">
      <DebugPanel />
    </div>
  </div>
</template>
