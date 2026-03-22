<script setup lang="ts">
import { computed, watch } from 'vue'
import HeaderBar from '@/components/HeaderBar.vue'
import FlowTabBar from '@/components/FlowTabBar.vue'
import NodePalette from '@/components/NodePalette.vue'
import PropertyPanel from '@/components/PropertyPanel.vue'
import DebugSidebar from '@/components/DebugSidebar.vue'
import { useUiStore } from '@/stores/uiStore'
import { useFlowStore } from '@/stores/flowStore'
import { useDebugStore } from '@/stores/debugStore'
import { useWebSocket } from '@/composables/useWebSocket'

const ui = useUiStore()
const flowStore = useFlowStore()
const debugStore = useDebugStore()

const ws = useWebSocket()

ws.onDebug((msg) => {
  debugStore.addMessage(msg)
})

ws.onStatus((event) => {
  flowStore.updateNodeStatus(event.nodeId, event.status)
})

ws.onDeploy((event) => {
  switch (event.action) {
    case 'deploying': ui.setDeployStatus('deploying'); break
    case 'deployed':  ui.setDeployStatus('deployed');  break
    case 'failed':    ui.setDeployStatus('failed');    break
  }
})

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
  const rightWidth = (ui.propertiesPanelOpen ? ui.propertiesPanelWidth : 0) + (ui.debugPanelOpen ? 320 : 0)
  return {
    marginLeft: left,
    marginRight: rightWidth + 'px',
  }
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col overflow-hidden bg-terminal-bg text-terminal-text">
    <!-- Top Header Bar -->
    <HeaderBar />

    <!-- Flow Tab Bar -->
    <FlowTabBar />

    <!-- Main Content Area -->
    <div class="flex flex-1 overflow-hidden relative">
      <!-- Left Sidebar: Node Palette -->
      <aside
        v-show="ui.leftPanelOpen"
        class="w-[240px] flex-shrink-0 absolute top-0 left-0 bottom-0 z-10 bg-terminal-surface border-r border-terminal-border"
      >
        <NodePalette />
      </aside>

      <!-- Center: Flow Editor -->
      <main
        class="flex-1 overflow-hidden transition-all duration-150"
        :style="mainAreaStyle"
      >
        <router-view />
      </main>

      <!-- Right Sidebars: Properties | Debug (side by side) -->
      <div class="absolute top-0 right-0 bottom-0 z-10 flex">
        <aside v-show="ui.propertiesPanelOpen">
          <PropertyPanel />
        </aside>
        <aside v-show="ui.debugPanelOpen">
          <DebugSidebar />
        </aside>
      </div>
    </div>
  </div>
</template>

<style scoped>
aside, main { border-radius: 0; }
</style>
