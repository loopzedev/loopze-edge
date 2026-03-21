import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export type ConnectionStatus = 'connected' | 'disconnected' | 'connecting'
export type DeployStatus = 'idle' | 'deploying' | 'deployed' | 'failed'
export type PropertiesContext =
  | { type: 'node' }
  | { type: 'flow-create' }
  | { type: 'flow-edit'; flowId: string }
  | { type: 'config-edit'; configType: string; configId?: string }
  | null

export const useUiStore = defineStore('ui', () => {
  // ── State ──────────────────────────────────────────────────────────
  const leftPanelOpen = ref<boolean>(true)
  const propertiesPanelOpen = ref<boolean>(false)
  const propertiesPanelWidth = ref<number>(280)
  const debugPanelOpen = ref<boolean>(false)
  const connectionStatus = ref<ConnectionStatus>('disconnected')
  const deployStatus = ref<DeployStatus>('idle')
  const propertiesContext = ref<PropertiesContext>(null)
  let deployResetTimer: ReturnType<typeof setTimeout> | null = null

  // ── Getters ────────────────────────────────────────────────────────
  const isConnected = computed(() => connectionStatus.value === 'connected')
  const isConnecting = computed(() => connectionStatus.value === 'connecting')

  const connectionStatusColor = computed(() => {
    switch (connectionStatus.value) {
      case 'connected':    return '#4ade80'
      case 'connecting':   return '#FFBF00'
      case 'disconnected': return '#ef4444'
    }
  })

  // ── Actions ────────────────────────────────────────────────────────
  function toggleLeftPanel() {
    leftPanelOpen.value = !leftPanelOpen.value
  }

  function togglePropertiesPanel() {
    propertiesPanelOpen.value = !propertiesPanelOpen.value
  }

  function openPropertiesPanel() {
    propertiesPanelOpen.value = true
  }

  function closePropertiesPanel() {
    propertiesPanelOpen.value = false
    propertiesContext.value = null
  }

  function openFlowCreateProperties() {
    propertiesContext.value = { type: 'flow-create' }
    propertiesPanelOpen.value = true
  }

  function openFlowEditProperties(flowId: string) {
    propertiesContext.value = { type: 'flow-edit', flowId }
    propertiesPanelOpen.value = true
  }

  function clearFlowProperties() {
    propertiesContext.value = null
  }

  function openConfigEditor(configType: string, configId?: string) {
    propertiesContext.value = { type: 'config-edit', configType, configId }
    propertiesPanelOpen.value = true
  }

  function clearConfigEditor() {
    propertiesContext.value = null
  }

  function toggleDebugPanel() {
    debugPanelOpen.value = !debugPanelOpen.value
  }

  function openDebugPanel() {
    debugPanelOpen.value = true
  }

  function closeDebugPanel() {
    debugPanelOpen.value = false
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
    propertiesPanelOpen,
    propertiesPanelWidth,
    propertiesContext,
    debugPanelOpen,
    connectionStatus,
    deployStatus,

    // getters
    isConnected,
    isConnecting,
    connectionStatusColor,

    // actions
    toggleLeftPanel,
    togglePropertiesPanel,
    openPropertiesPanel,
    closePropertiesPanel,
    openFlowCreateProperties,
    openFlowEditProperties,
    clearFlowProperties,
    openConfigEditor,
    clearConfigEditor,
    toggleDebugPanel,
    openDebugPanel,
    closeDebugPanel,
    setConnectionStatus,
    setDeployStatus,
  }
})
