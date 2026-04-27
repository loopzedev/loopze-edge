<script setup lang="ts">
import { computed, onMounted, provide, watch } from 'vue'
import HeaderBar from '@/components/HeaderBar.vue'
import FlowTabBar from '@/components/FlowTabBar.vue'
import NodePalette from '@/components/NodePalette.vue'
import PropertyPanel from '@/components/PropertyPanel.vue'
import InformationSidebar from '@/components/InformationSidebar.vue'
import TerminalLogPanel from '@/components/TerminalLogPanel.vue'
import SetupModal from '@/components/auth/SetupModal.vue'
import LoginModal from '@/components/auth/LoginModal.vue'
import { useUiStore } from '@/stores/uiStore'
import { useFlowStore } from '@/stores/flowStore'
import { useDebugStore } from '@/stores/debugStore'
import { useAuthStore } from '@/stores/authStore'
import { useWebSocket } from '@/composables/useWebSocket'

const ui = useUiStore()
const flowStore = useFlowStore()
const debugStore = useDebugStore()
const auth = useAuthStore()

onMounted(() => {
  auth.init()
})

// Hold the WebSocket until the user is authenticated. Connecting earlier
// would just rack up 401-then-reconnect noise, and any server-driven
// teardown (logout, disable, password reset) should also tear down the
// browser-side socket.
const ws = useWebSocket({ autoConnect: false })

watch(
  () => auth.isAuthenticated,
  (authed) => {
    if (authed) ws.connect()
    else ws.disconnect()
  },
  { immediate: true },
)

// TerminalLogPanel mounts/unmounts dynamically; pass the existing onLog
// dispatcher through provide so it does not spawn a second WebSocket.
provide('onLog', ws.onLog)

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
  const rightWidth = (ui.propertiesPanelOpen ? ui.propertiesPanelWidth : 0) + (ui.infoPanelOpen ? ui.infoPanelWidth : 0)
  return {
    marginLeft: left,
    marginRight: rightWidth + 'px',
  }
})
</script>

<template>
  <div class="h-screen w-screen flex flex-col overflow-hidden bg-terminal-bg text-terminal-text">
    <!-- Auth gate. The Setup and Login modals replace the editor entirely
         until the user is authenticated. Schritt 8/9 will swap these
         placeholders for real modal components. -->
    <template v-if="auth.loading">
      <div class="flex flex-1 items-center justify-center text-terminal-muted text-sm">
        Loading…
      </div>
    </template>
    <template v-else-if="auth.needsSetup">
      <SetupModal />
    </template>
    <template v-else-if="!auth.isAuthenticated">
      <LoginModal />
    </template>
    <template v-else>
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
          class="flex-1 overflow-hidden transition-all duration-150 relative"
          :style="mainAreaStyle"
        >
          <router-view />
          <!-- Terminal Log overlay: covers the canvas, leaves sidebars visible -->
          <TerminalLogPanel />
        </main>

        <!-- Right Sidebars: Properties | Debug (side by side) -->
        <div class="absolute top-0 right-0 bottom-0 z-10 flex">
          <aside v-show="ui.propertiesPanelOpen" class="h-full">
            <PropertyPanel />
          </aside>
          <aside v-show="ui.infoPanelOpen" class="h-full">
            <InformationSidebar />
          </aside>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
aside, main { border-radius: 0; }
</style>
