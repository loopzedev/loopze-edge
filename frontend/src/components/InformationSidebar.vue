<script setup lang="ts">
import { useUiStore } from '@/stores/uiStore'
import PanelHeader from '@/components/ui/PanelHeader.vue'
import DebugPanel from '@/components/DebugPanel.vue'

const ui = useUiStore()
</script>

<template>
  <div class="h-full w-[320px] flex-shrink-0 flex flex-col bg-terminal-surface border-l border-terminal-border select-none">
    <PanelHeader title="Information" closable @close="ui.closeInfoPanel()" />
    <div class="flex-1 overflow-hidden">
      <DebugPanel />
    </div>
  </div>
</template>
