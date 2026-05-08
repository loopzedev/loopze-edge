import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  useApi,
  type StateMachineListEntry,
  type StateMachineSnapshot,
} from '@/composables/useApi'

export interface StateMachineFlowGroup {
  flowID: string
  flowLabel: string
  machines: StateMachineListEntry[]
}

export const useStateMachineStore = defineStore('stateMachine', () => {
  const api = useApi()

  const machines = ref<StateMachineListEntry[]>([])
  const selectedFlowId = ref<string | null>(null)
  const selectedNodeId = ref<string | null>(null)
  const snapshot = ref<StateMachineSnapshot | null>(null)
  const loadingList = ref(false)
  const loadingSnapshot = ref(false)
  const error = ref<string | null>(null)

  // Group the flat list by flow for the dropdown's <optgroup>s.
  const grouped = computed<StateMachineFlowGroup[]>(() => {
    const groups = new Map<string, StateMachineFlowGroup>()
    for (const m of machines.value) {
      let g = groups.get(m.flowID)
      if (!g) {
        g = { flowID: m.flowID, flowLabel: m.flowLabel, machines: [] }
        groups.set(m.flowID, g)
      }
      g.machines.push(m)
    }
    return Array.from(groups.values())
  })

  async function loadList() {
    error.value = null
    loadingList.value = true
    try {
      const res = await api.listAllStateMachines()
      machines.value = res.machines ?? []

      // Drop selection if it no longer exists.
      const stillExists = machines.value.some(
        m => m.flowID === selectedFlowId.value && m.nodeID === selectedNodeId.value,
      )
      if (!stillExists) {
        selectedFlowId.value = null
        selectedNodeId.value = null
        snapshot.value = null
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
    if (!selectedFlowId.value || !selectedNodeId.value) {
      snapshot.value = null
      return
    }
    loadingSnapshot.value = true
    try {
      const res = await api.getStateMachineSnapshot(selectedFlowId.value, selectedNodeId.value)
      snapshot.value = res.snapshot

      // Reflect current state in the dropdown label.
      const idx = machines.value.findIndex(
        m => m.flowID === selectedFlowId.value && m.nodeID === selectedNodeId.value,
      )
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

  async function selectMachine(flowId: string | null, nodeId: string | null) {
    selectedFlowId.value = flowId
    selectedNodeId.value = nodeId
    snapshot.value = null
    if (flowId && nodeId) {
      await loadSnapshot()
    }
  }

  async function refresh() {
    await loadList()
    if (selectedFlowId.value && selectedNodeId.value) {
      await loadSnapshot()
    }
  }

  return {
    machines,
    grouped,
    selectedFlowId,
    selectedNodeId,
    snapshot,
    loadingList,
    loadingSnapshot,
    error,
    loadList,
    loadSnapshot,
    selectMachine,
    refresh,
  }
})
