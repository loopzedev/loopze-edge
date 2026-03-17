<script setup lang="ts">
import { computed, watch } from 'vue'
import HeaderBar from '@/components/HeaderBar.vue'
import NodePalette from '@/components/NodePalette.vue'
import PropertyPanel from '@/components/PropertyPanel.vue'
import { useUiStore } from '@/stores/uiStore'
import { useWebSocket } from '@/composables/useWebSocket'

const ui = useUiStore()

// Connect WebSocket and sync status to uiStore.
const ws = useWebSocket()
watch(ws.status, (status) => {
  switch (status) {
    case 'connected':
      ui.setConnectionStatus('connected')
      break
    case 'connecting':
    case 'reconnecting':
      ui.setConnectionStatus('connecting')
      break
    case 'disconnected':
      ui.setConnectionStatus('disconnected')
      break
  }
}, { immediate: true })

const mainAreaStyle = computed(() => {
  const left = ui.leftPanelOpen ? '240px' : '0px'
  const right = ui.rightPanelOpen ? '320px' : '0px'
  return {
    marginLeft: left,
    marginRight: right,
  }
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col overflow-hidden bg-terminal-bg font-mono text-terminal-text">
    <!-- Top Header Bar -->
    <HeaderBar />

    <!-- Main Content Area -->
    <div class="flex flex-1 overflow-hidden relative" style="margin-top: 0">
      <!-- Left Sidebar: Node Palette -->
      <aside
        v-show="ui.leftPanelOpen"
        class="
          w-[240px] flex-shrink-0 overflow-y-auto
          bg-terminal-surface border-r border-terminal-border
          absolute top-0 left-0 bottom-0 z-10
        "
      >
        <NodePalette />
      </aside>

      <!-- Center: Flow Editor (router-view) -->
      <main
        class="flex-1 overflow-hidden transition-all duration-150"
        :style="mainAreaStyle"
      >
        <router-view />
      </main>

      <!-- Right Sidebar: Properties / Debug Panel -->
      <aside
        v-show="ui.rightPanelOpen"
        class="
          w-[320px] flex-shrink-0 overflow-y-auto
          bg-terminal-surface border-l border-terminal-border
          absolute top-0 right-0 bottom-0 z-10
        "
      >
        <PropertyPanel />
      </aside>
    </div>
  </div>
</template>

<style scoped>
/* Ensure no rounding leaks in from any library defaults */
aside,
main {
  border-radius: 0;
}
</style>
