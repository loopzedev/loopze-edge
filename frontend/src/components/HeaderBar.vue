<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import type { DeployModeType } from '@/types/flow'

const flowStore = useFlowStore()
const uiStore = useUiStore()

const connectionDotColor = computed(() => uiStore.connectionStatusColor)

const connectionLabel = computed(() => {
  switch (uiStore.connectionStatus) {
    case 'connected':   return 'ONLINE'
    case 'connecting':  return 'CONNECTING'
    case 'disconnected': return 'OFFLINE'
  }
})

const deployModeLabel: Record<DeployModeType, string> = {
  nodes: 'DEPLOY',
  flows: 'DEPLOY FLOWS',
  full: 'FULL DEPLOY',
  restart: 'RESTART',
}

const deployLabel = computed(() => {
  switch (uiStore.deployStatus) {
    case 'deploying': return 'DEPLOYING'
    case 'deployed':  return 'DEPLOYED'
    case 'failed':    return 'FAILED'
    default:          return deployModeLabel[flowStore.deployMode]
  }
})

const deployDisabled = computed(() => uiStore.deployStatus === 'deploying')

const showDeployMenu = ref(false)

const deployModes: { value: DeployModeType; label: string; description: string }[] = [
  { value: 'nodes', label: 'Modified Nodes', description: 'Only changed nodes' },
  { value: 'flows', label: 'Modified Flows', description: 'Entire changed flows' },
  { value: 'full',  label: 'Full Deploy',    description: 'Restart all nodes' },
  { value: 'restart', label: 'Restart',      description: 'Full engine restart' },
]

function handleClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.deploy-menu-container')) {
    showDeployMenu.value = false
  }
}

onMounted(() => document.addEventListener('click', handleClickOutside))
onUnmounted(() => document.removeEventListener('click', handleClickOutside))

async function handleDeploy(): Promise<void> {
  showDeployMenu.value = false
  await flowStore.deploy()
}

function selectMode(mode: DeployModeType): void {
  flowStore.setDeployMode(mode)
  showDeployMenu.value = false
}
</script>

<template>
  <header
    class="flex items-center justify-between h-11 px-4 bg-terminal-surface border-b border-terminal-border select-none shrink-0"
  >
    <!-- Left: Logo / App Name -->
    <div class="flex items-center gap-3">
      <button
        class="text-terminal-text-dim hover:text-terminal-text transition-colors duration-100 p-1 -ml-1 rounded hover:bg-terminal-surface-alt"
        title="Toggle node palette"
        @click="uiStore.toggleLeftPanel()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      <div class="flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4 text-accent" viewBox="0 0 24 24" fill="currentColor">
          <path d="M13 2L3 14h8l-1 8 10-12h-8l1-8z" />
        </svg>
        <span class="text-accent text-sm font-bold tracking-widest font-mono">FLINT</span>
      </div>

      <span class="text-terminal-text-dim text-[10px] tracking-wider hidden sm:inline font-medium uppercase">
        Flow Automation
      </span>
    </div>

    <!-- Right: Actions & Status -->
    <div class="flex items-center gap-2">
      <!-- Connection status -->
      <div
        class="flex items-center gap-1.5 px-2 py-1 rounded"
        :title="`Status: ${connectionLabel}`"
      >
        <span
          class="block w-2 h-2 rounded-full"
          :style="{ backgroundColor: connectionDotColor }"
        />
        <span class="text-[10px] text-terminal-text-dim tracking-wider hidden sm:inline font-medium">
          {{ connectionLabel }}
        </span>
      </div>

      <!-- Separator -->
      <div class="w-px h-5 bg-terminal-border mx-1" />

      <!-- Settings link -->
      <router-link
        to="/settings"
        class="p-1.5 rounded text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt transition-all duration-100"
        title="Settings"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.75">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 15a3 3 0 100-6 3 3 0 000 6z" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 01-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
        </svg>
      </router-link>

      <!-- Toggle Properties panel -->
      <button
        class="p-1.5 rounded transition-all duration-100"
        :class="uiStore.propertiesPanelOpen
          ? 'text-accent bg-accent/10'
          : 'text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt'"
        title="Toggle properties"
        @click="uiStore.togglePropertiesPanel()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.75">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
        </svg>
      </button>

      <!-- Toggle Debug panel -->
      <button
        class="p-1.5 rounded transition-all duration-100"
        :class="uiStore.infoPanelOpen
          ? 'text-accent bg-accent/10'
          : 'text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt'"
        title="Toggle information"
        @click="uiStore.toggleInfoPanel()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.75">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      </button>

      <!-- Separator -->
      <div class="w-px h-5 bg-terminal-border mx-1" />

      <!-- Deploy split-button -->
      <div class="deploy-menu-container relative">
        <div class="flex items-center">
          <!-- Main deploy button -->
          <button
            class="flex items-center gap-2 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider transition-all duration-150 rounded-l"
            :disabled="deployDisabled"
            :class="{
              'opacity-50 cursor-not-allowed': deployDisabled,
              'bg-status-success/15 border border-status-success/40 text-status-success': uiStore.deployStatus === 'deployed',
              'bg-status-error/15 border border-status-error/40 text-status-error': uiStore.deployStatus === 'failed',
              'bg-accent text-white border border-accent hover:bg-accent-dim': uiStore.deployStatus !== 'deployed' && uiStore.deployStatus !== 'failed',
            }"
            title="Deploy flows"
            @click="handleDeploy"
          >
            <!-- Spinning icon while deploying -->
            <svg
              v-if="uiStore.deployStatus === 'deploying'"
              xmlns="http://www.w3.org/2000/svg"
              class="w-3.5 h-3.5 animate-spin"
              fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
            >
              <path stroke-linecap="round" d="M12 2v4m0 12v4m-7-7H3m18 0h-2M6.34 6.34L4.93 4.93m12.73 12.73l1.41 1.41M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
            </svg>
            <!-- Checkmark when deployed -->
            <svg
              v-else-if="uiStore.deployStatus === 'deployed'"
              xmlns="http://www.w3.org/2000/svg"
              class="w-3.5 h-3.5"
              fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M5 12l5 5L20 7" />
            </svg>
            <!-- X when failed -->
            <svg
              v-else-if="uiStore.deployStatus === 'failed'"
              xmlns="http://www.w3.org/2000/svg"
              class="w-3.5 h-3.5"
              fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
            <!-- Upload/deploy icon (idle) -->
            <svg
              v-else
              xmlns="http://www.w3.org/2000/svg"
              class="w-3.5 h-3.5"
              fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M7 16l-4-4m0 0l4-4m-4 4h18M17 8l4 4m0 0l-4 4" />
            </svg>
            <span>{{ deployLabel }}</span>
          </button>

          <!-- Dropdown toggle -->
          <button
            class="flex items-center px-1.5 py-1.5 transition-all duration-150 rounded-r border-l-0"
            :disabled="deployDisabled"
            :class="{
              'opacity-50 cursor-not-allowed': deployDisabled,
              'bg-status-success/15 border border-status-success/40 text-status-success': uiStore.deployStatus === 'deployed',
              'bg-status-error/15 border border-status-error/40 text-status-error': uiStore.deployStatus === 'failed',
              'bg-accent text-white border border-accent hover:bg-accent-dim': uiStore.deployStatus !== 'deployed' && uiStore.deployStatus !== 'failed',
            }"
            title="Select deploy mode"
            @click.stop="showDeployMenu = !showDeployMenu"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
        </div>

        <!-- Dropdown menu -->
        <div
          v-if="showDeployMenu"
          class="absolute right-0 top-full mt-1.5 w-56 bg-terminal-surface border border-terminal-border rounded-md shadow-xl shadow-black/40 z-50 overflow-hidden"
        >
          <button
            v-for="mode in deployModes"
            :key="mode.value"
            class="w-full flex items-center gap-2.5 px-3 py-2.5 text-left text-xs hover:bg-terminal-surface-alt transition-colors duration-100"
            @click="selectMode(mode.value)"
          >
            <span
              class="w-2 h-2 rounded-full border border-terminal-text-dim shrink-0 transition-colors"
              :class="{ 'bg-accent border-accent': flowStore.deployMode === mode.value }"
            />
            <div>
              <div class="text-terminal-text font-medium">{{ mode.label }}</div>
              <div class="text-terminal-text-dim text-[10px] mt-0.5">{{ mode.description }}</div>
            </div>
          </button>
        </div>
      </div>
    </div>
  </header>
</template>
