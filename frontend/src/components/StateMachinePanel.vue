<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watchEffect } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useStateMachineStore } from '@/stores/stateMachineStore'
import JsonTreeView from '@/components/JsonTreeView.vue'

const flow = useFlowStore()
const ui = useUiStore()
const sm = useStateMachineStore()

// Encode/decode flowID + nodeID into a single dropdown value so a single
// <select> can drive a cross-flow selection.
function encodeKey(flowID: string, nodeID: string): string {
  return `${flowID}::${nodeID}`
}

function decodeKey(value: string): { flowID: string; nodeID: string } | null {
  const idx = value.indexOf('::')
  if (idx < 0) return null
  return { flowID: value.slice(0, idx), nodeID: value.slice(idx + 2) }
}

const selectedKey = computed(() =>
  sm.selectedFlowId && sm.selectedNodeId
    ? encodeKey(sm.selectedFlowId, sm.selectedNodeId)
    : '',
)

onMounted(async () => {
  // Restore the last selection from uiStore before the first load.
  if (ui.selectedStateMachineFlowID && ui.selectedStateMachineNodeID) {
    sm.selectedFlowId = ui.selectedStateMachineFlowID
    sm.selectedNodeId = ui.selectedStateMachineNodeID
  }

  await sm.loadList()

  // If no selection survived, default to a machine in the active flow if any,
  // otherwise the first machine overall.
  if (!sm.selectedFlowId || !sm.selectedNodeId) {
    const preferred = flow.activeFlowId
      ? sm.machines.find(m => m.flowID === flow.activeFlowId)
      : undefined
    const pick = preferred ?? sm.machines[0]
    if (pick) {
      sm.selectedFlowId = pick.flowID
      sm.selectedNodeId = pick.nodeID
    }
  }

  ui.setSelectedStateMachine(sm.selectedFlowId, sm.selectedNodeId)
  if (sm.selectedFlowId && sm.selectedNodeId) {
    await sm.loadSnapshot()
  }
})

async function handleSelectChange(value: string) {
  const decoded = decodeKey(value)
  if (decoded) {
    ui.setSelectedStateMachine(decoded.flowID, decoded.nodeID)
    await sm.selectMachine(decoded.flowID, decoded.nodeID)
  } else {
    ui.setSelectedStateMachine(null, null)
    await sm.selectMachine(null, null)
  }
}

async function handleRefresh() {
  await sm.refresh()
}

function toggleAutoRefresh() {
  ui.setStateMachineAutoRefresh(!ui.stateMachineAutoRefresh)
}

let timer: ReturnType<typeof setInterval> | null = null
watchEffect(() => {
  const active =
    ui.stateMachineAutoRefresh &&
    ui.activeInfoTab === 'state-machines' &&
    ui.infoPanelOpen
  if (active && !timer) {
    timer = setInterval(() => { void sm.refresh() }, 1000)
  } else if (!active && timer) {
    clearInterval(timer)
    timer = null
  }
})

onBeforeUnmount(() => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
})

function shortTime(ts: string): string {
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ts
  return d.toLocaleTimeString(undefined, { hour12: false })
}
</script>

<template>
  <div class="flex flex-col h-full bg-terminal-bg text-xs">
    <!-- Toolbar -->
    <div class="px-3 py-2 border-b border-terminal-border space-y-2 shrink-0">
      <!-- Machine dropdown — grouped by flow so selection works across flows -->
      <div class="flex items-center gap-2">
        <label class="text-[10px] uppercase tracking-wider text-terminal-text-dim shrink-0">Machine</label>
        <select
          :value="selectedKey"
          class="flex-1 bg-terminal-surface border border-terminal-border rounded px-2 py-1 text-[11px] text-terminal-text"
          :disabled="sm.machines.length === 0"
          @change="handleSelectChange(($event.target as HTMLSelectElement).value)"
        >
          <option v-if="sm.machines.length === 0" value="">— no state machines deployed —</option>
          <optgroup
            v-for="g in sm.grouped"
            :key="g.flowID"
            :label="g.flowLabel"
          >
            <option
              v-for="m in g.machines"
              :key="m.nodeID"
              :value="encodeKey(m.flowID, m.nodeID)"
            >
              {{ m.label }} ({{ m.currentState }})
            </option>
          </optgroup>
        </select>
      </div>

      <!-- Refresh + auto-refresh toggle -->
      <div class="flex items-center justify-between gap-2">
        <button
          class="flex items-center gap-1 px-2 py-1 text-[10px] uppercase tracking-wider font-semibold rounded border border-terminal-border bg-terminal-surface text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text-dim transition-colors cursor-pointer"
          :disabled="sm.loadingList || sm.loadingSnapshot"
          @click="handleRefresh"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh
        </button>

        <label class="flex items-center gap-1.5 cursor-pointer select-none">
          <input
            type="checkbox"
            class="accent-accent cursor-pointer"
            :checked="ui.stateMachineAutoRefresh"
            @change="toggleAutoRefresh"
          />
          <span class="text-[10px] uppercase tracking-wider text-terminal-text-dim">Auto-Refresh 1s</span>
        </label>
      </div>
    </div>

    <!-- Error -->
    <div v-if="sm.error" class="px-3 py-2 bg-red-900/20 border-b border-red-800/40 text-[11px] text-red-400">
      {{ sm.error }}
    </div>

    <!-- Empty state -->
    <div
      v-if="sm.machines.length === 0 && !sm.loadingList"
      class="flex-1 flex items-center justify-center px-4 text-center"
    >
      <span class="text-terminal-text-dim text-[11px]">No deployed flow has a state machine node.</span>
    </div>

    <!-- Detail -->
    <div v-else-if="sm.snapshot" class="flex-1 overflow-y-auto">
      <!-- Current state -->
      <div class="px-3 py-2 border-b border-terminal-border/40">
        <div class="text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Current state</div>
        <div class="font-mono text-sm text-accent">{{ sm.snapshot.currentState }}</div>
      </div>

      <!-- Available events -->
      <div class="px-3 py-2 border-b border-terminal-border/40">
        <div class="text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Available events</div>
        <div v-if="sm.snapshot.availableEvents.length === 0" class="text-[11px] text-terminal-text-dim italic">
          (no transitions defined for this state)
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <span
            v-for="ev in sm.snapshot.availableEvents"
            :key="ev"
            class="px-1.5 py-0.5 text-[10px] font-mono rounded border border-terminal-border bg-terminal-surface text-terminal-text"
          >
            {{ ev }}
          </span>
        </div>
      </div>

      <!-- Context -->
      <div class="px-3 py-2 border-b border-terminal-border/40">
        <div class="text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Context</div>
        <div v-if="!sm.snapshot.context || Object.keys(sm.snapshot.context).length === 0" class="text-[11px] text-terminal-text-dim italic">
          (empty)
        </div>
        <JsonTreeView v-else :data="sm.snapshot.context" :default-expand-depth="2" />
      </div>

      <!-- History -->
      <div class="px-3 py-2">
        <div class="text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">
          History ({{ sm.snapshot.history.length }})
        </div>
        <div v-if="sm.snapshot.history.length === 0" class="text-[11px] text-terminal-text-dim italic">
          (no transitions yet)
        </div>
        <div v-else class="space-y-1">
          <div
            v-for="(h, idx) in sm.snapshot.history.slice().reverse()"
            :key="idx"
            class="flex items-center gap-2 font-mono text-[10px]"
          >
            <span class="text-terminal-text-dim shrink-0">{{ shortTime(h.ts) }}</span>
            <span class="text-terminal-text">{{ h.from }}</span>
            <span class="text-terminal-text-dim">→</span>
            <span class="text-accent">{{ h.to }}</span>
            <span class="text-terminal-text-dim ml-auto truncate">({{ h.event }})</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Loading placeholder -->
    <div v-else-if="sm.loadingSnapshot" class="flex-1 flex items-center justify-center">
      <span class="text-terminal-text-dim text-[11px]">Loading snapshot…</span>
    </div>
  </div>
</template>
