import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export type RightPanelTab = 'properties' | 'debug'
export type ConnectionStatus = 'connected' | 'disconnected' | 'connecting'
export type DeployStatus = 'idle' | 'deploying' | 'deployed' | 'failed'

export const useUiStore = defineStore('ui', () => {
  // ── State ──────────────────────────────────────────────────────────
  const leftPanelOpen = ref<boolean>(true)
  const rightPanelOpen = ref<boolean>(false)
  const rightPanelTab = ref<RightPanelTab>('properties')
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const deployStatus = ref<DeployStatus>('idle')
  let deployResetTimer: ReturnType<typeof setTimeout> | null = null

  // ── Getters ────────────────────────────────────────────────────────
  const isConnected = computed(() => connectionStatus.value === 'connected')
  const isConnecting = computed(() => connectionStatus.value === 'connecting')

  const connectionStatusColor = computed(() => {
    switch (connectionStatus.value) {
      case 'connected':
        return '#4ade80' // green
      case 'connecting':
        return '#FFBF00' // amber
      case 'disconnected':
        return '#ef4444' // red
    }
  })

  // ── Actions ────────────────────────────────────────────────────────
  function toggleLeftPanel() {
    leftPanelOpen.value = !leftPanelOpen.value
  }

  function toggleRightPanel() {
    rightPanelOpen.value = !rightPanelOpen.value
  }

  function openRightPanel(tab?: RightPanelTab) {
    rightPanelOpen.value = true
    if (tab) {
      rightPanelTab.value = tab
    }
  }

  function closeRightPanel() {
    rightPanelOpen.value = false
  }

  function setRightPanelTab(tab: RightPanelTab) {
    rightPanelTab.value = tab
    if (!rightPanelOpen.value) {
      rightPanelOpen.value = true
    }
  }

  function setConnectionStatus(status: ConnectionStatus) {
    connectionStatus.value = status
  }

  function setDeployStatus(status: DeployStatus) {
    if (deployResetTimer !== null) {
      clearTimeout(deployResetTimer)
      deployResetTimer = null
    }
    deployStatus.value = status
    if (status === 'deployed') {
      deployResetTimer = setTimeout(() => {
        deployStatus.value = 'idle'
        deployResetTimer = null
      }, 3000)
    }
  }

  return {
    // state
    leftPanelOpen,
    rightPanelOpen,
    rightPanelTab,
    connectionStatus,
    deployStatus,

    // getters
    isConnected,
    isConnecting,
    connectionStatusColor,

    // actions
    toggleLeftPanel,
    toggleRightPanel,
    openRightPanel,
    closeRightPanel,
    setRightPanelTab,
    setConnectionStatus,
    setDeployStatus,
  }
})
