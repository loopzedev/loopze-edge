import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  useApi,
  type StateMachineListItem,
  type StateMachineSnapshot,
} from '@/composables/useApi'

export const useStateMachineStore = defineStore('stateMachine', () => {
  const api = useApi()

  const flowId = ref<string | null>(null)
  const machines = ref<StateMachineListItem[]>([])
  const selectedNodeId = ref<string | null>(null)
  const snapshot = ref<StateMachineSnapshot | null>(null)
  const loadingList = ref(false)
  const loadingSnapshot = ref(false)
  const error = ref<string | null>(null)

  function setFlowId(id: string | null) {
    if (flowId.value === id) return
    flowId.value = id
    machines.value = []
    snapshot.value = null
    selectedNodeId.value = null
  }

  async function loadList() {
    error.value = null
    if (!flowId.value) {
      machines.value = []
      return
    }
    loadingList.value = true
    try {
      const res = await api.listStateMachines(flowId.value)
      machines.value = res.machines ?? []

      // Keep selection if still present, otherwise pick the first.
      if (selectedNodeId.value && !machines.value.some(m => m.nodeID === selectedNodeId.value)) {
        selectedNodeId.value = null
        snapshot.value = null
      }
      if (!selectedNodeId.value && machines.value.length > 0) {
        selectedNodeId.value = machines.value[0].nodeID
      }
    } catch (err) {
      error.value = api.isApiError(err) ? err.message : 'Failed to load state machines'
      machines.value = []
    } finally {
      loadingList.value = false
    }
  }

  async function loadSnapshot() {
    error.value = null
    if (!flowId.value || !selectedNodeId.value) {
      snapshot.value = null
      return
    }
    loadingSnapshot.value = true
    try {
      const res = await api.getStateMachineSnapshot(flowId.value, selectedNodeId.value)
      snapshot.value = res.snapshot

      // Reflect current state in the dropdown label.
      const idx = machines.value.findIndex(m => m.nodeID === selectedNodeId.value)
      if (idx >= 0 && machines.value[idx].currentState !== res.snapshot.currentState) {
        machines.value[idx] = {
          ...machines.value[idx],
          currentState: res.snapshot.currentState,
        }
      }
    } catch (err) {
      if (api.isApiError(err) && err.status === 404) {
        snapshot.value = null
        return
      }
      error.value = api.isApiError(err) ? err.message : 'Failed to load snapshot'
    } finally {
      loadingSnapshot.value = false
    }
  }

  async function selectMachine(nodeId: string | null) {
    selectedNodeId.value = nodeId
    snapshot.value = null
    if (nodeId) {
      await loadSnapshot()
    }
  }

  async function refresh() {
    await loadList()
    if (selectedNodeId.value) {
      await loadSnapshot()
    }
  }

  return {
    flowId,
    machines,
    selectedNodeId,
    snapshot,
    loadingList,
    loadingSnapshot,
    error,
    setFlowId,
    loadList,
    loadSnapshot,
    selectMachine,
    refresh,
  }
})
