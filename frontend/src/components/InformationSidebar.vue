<script setup lang="ts">
import { computed } from 'vue'
import { useUiStore } from '@/stores/uiStore'
import type { InfoTab } from '@/stores/uiStore'
import PanelHeader from '@/components/ui/PanelHeader.vue'
import DebugPanel from '@/components/DebugPanel.vue'
import HelpPanel from '@/components/HelpPanel.vue'
import ConfigPanel from '@/components/ConfigPanel.vue'
import ContextPanel from '@/components/ContextPanel.vue'
import StateMachinePanel from '@/components/StateMachinePanel.vue'

const ui = useUiStore()

const tabs: { id: InfoTab; label: string }[] = [
  { id: 'help', label: 'Help' },
  { id: 'config', label: 'Config' },
  { id: 'context', label: 'Context' },
  { id: 'state-machines', label: 'SM' },
  { id: 'debug', label: 'Debug' },
]

// ── Resize handle ────────────────────────────────────────────────
const panelWidth = computed({
  get: () => ui.infoPanelWidth,
  set: (v: number) => { ui.infoPanelWidth = v },
})
let resizing = false
let startX = 0
let startWidth = 0

function onResizeStart(e: MouseEvent) {
  resizing = true
  startX = e.clientX
  startWidth = panelWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}

function onResizeMove(e: MouseEvent) {
  if (!resizing) return
  const delta = startX - e.clientX
  panelWidth.value = Math.max(220, Math.min(600, startWidth + delta))
}

function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
}
</script>

<template>
  <div
    class="h-full flex-shrink-0 flex flex-col bg-terminal-surface border-l border-terminal-border select-none relative"
    :style="{ width: panelWidth + 'px' }"
  >
    <!-- Resize handle -->
    <div
      class="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize z-20 hover:bg-accent/30 active:bg-accent/50 transition-colors"
      @mousedown.prevent="onResizeStart"
    />

    <PanelHeader title="Information" closable @close="ui.closeInfoPanel()" />

    <!-- Tab Bar -->
    <div class="flex border-b border-terminal-border shrink-0">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        class="flex-1 px-2 py-1.5 text-[10px] uppercase tracking-wider font-semibold transition-all duration-100 cursor-pointer"
        :class="ui.activeInfoTab === tab.id
          ? 'text-accent border-b-2 border-accent bg-accent/5'
          : 'text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt/50 border-b-2 border-transparent'"
        @click="ui.setInfoTab(tab.id)"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Tab Content -->
    <div class="flex-1 overflow-hidden">
      <HelpPanel v-if="ui.activeInfoTab === 'help'" />
      <ConfigPanel v-else-if="ui.activeInfoTab === 'config'" />
      <ContextPanel v-else-if="ui.activeInfoTab === 'context'" />
      <StateMachinePanel v-else-if="ui.activeInfoTab === 'state-machines'" />
      <DebugPanel v-else />
    </div>
  </div>
</template>
