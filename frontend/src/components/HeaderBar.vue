<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

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

const deployLabel = computed(() => {
  switch (uiStore.deployStatus) {
    case 'deploying': return 'DEPLOYING'
    case 'deployed':  return 'DEPLOYED'
    case 'failed':    return 'FAILED'
    default:          return 'DEPLOY'
  }
})

const deployDisabled = computed(() => uiStore.deployStatus === 'deploying')

async function handleDeploy(): Promise<void> {
  await flowStore.deploy()
}
</script>

<template>
  <header
    class="flex items-center justify-between h-10 px-3 bg-terminal-surface border-b border-terminal-border font-mono select-none shrink-0"
  >
    <!-- Left: Logo / App Name -->
    <div class="flex items-center gap-3">
      <button
        class="text-terminal-text-dim hover:text-terminal-text transition-colors duration-100 px-1"
        title="Toggle node palette"
        @click="uiStore.toggleLeftPanel()"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path stroke-linecap="square" stroke-linejoin="miter" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      <div class="flex items-center gap-2">
        <!-- Flint icon: stylised lightning / spark -->
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="w-4 h-4 text-accent"
          viewBox="0 0 24 24"
          fill="currentColor"
        >
          <path d="M13 2L3 14h8l-1 8 10-12h-8l1-8z" />
        </svg>

        <span class="text-accent text-sm font-bold tracking-widest ">FLINT</span>
      </div>

      <span class="text-terminal-text-dim text-[10px] tracking-wide hidden sm:inline">
        FLOW AUTOMATION
      </span>
    </div>

    <!-- Center (optional): active flow name -->
    <div class="hidden md:flex items-center gap-2 text-xs text-terminal-text-dim">
      <span v-if="flowStore.activeFlow">
        {{ flowStore.activeFlow.label }}
      </span>
      <span
        v-if="flowStore.dirty"
        class="text-accent text-[10px]"
        title="Unsaved changes"
      >
        ●
      </span>
    </div>

    <!-- Right: Actions & Status -->
    <div class="flex items-center gap-3">
      <!-- Settings link -->
      <router-link
        to="/settings"
        class="text-terminal-text-dim hover:text-terminal-text text-xs transition-colors duration-100"
        title="Settings"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path
            stroke-linecap="square"
            stroke-linejoin="miter"
            d="M12 15a3 3 0 100-6 3 3 0 000 6z"
          />
          <path
            stroke-linecap="square"
            stroke-linejoin="miter"
            d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 01-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z"
          />
        </svg>
      </router-link>

      <!-- Connection status -->
      <div class="flex items-center gap-1.5" :title="`Status: ${connectionLabel}`">
        <span
          class="block w-2 h-2"
          :style="{ backgroundColor: connectionDotColor }"
        />
        <span class="text-[10px] text-terminal-text-dim tracking-wider hidden sm:inline">
          {{ connectionLabel }}
        </span>
      </div>

      <!-- Toggle Properties panel -->
      <button
        class="text-terminal-text-dim hover:text-terminal-text transition-colors duration-100 px-1"
        :class="{ 'text-accent': uiStore.propertiesPanelOpen }"
        title="Toggle properties"
        @click="uiStore.togglePropertiesPanel()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="square" stroke-linejoin="miter" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
        </svg>
      </button>

      <!-- Toggle Debug panel -->
      <button
        class="text-terminal-text-dim hover:text-terminal-text transition-colors duration-100 px-1"
        :class="{ 'text-accent': uiStore.debugPanelOpen }"
        title="Toggle debug"
        @click="uiStore.toggleDebugPanel()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="square" stroke-linejoin="miter" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      </button>

      <!-- Deploy button -->
      <button
        class="terminal-btn-primary flex items-center gap-1.5 text-xs uppercase tracking-wider transition-colors duration-150"
        :disabled="deployDisabled"
        :class="{
          'opacity-50 cursor-not-allowed': deployDisabled,
          '!border-green-500 !text-green-400': uiStore.deployStatus === 'deployed',
          '!border-red-500 !text-red-400': uiStore.deployStatus === 'failed',
        }"
        title="Deploy flows"
        @click="handleDeploy"
      >
        <!-- Spinning icon while deploying -->
        <svg
          v-if="uiStore.deployStatus === 'deploying'"
          xmlns="http://www.w3.org/2000/svg"
          class="w-3.5 h-3.5 animate-spin"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="square" d="M12 2v4m0 12v4m-7-7H3m18 0h-2M6.34 6.34L4.93 4.93m12.73 12.73l1.41 1.41M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
        </svg>
        <!-- Checkmark when deployed -->
        <svg
          v-else-if="uiStore.deployStatus === 'deployed'"
          xmlns="http://www.w3.org/2000/svg"
          class="w-3.5 h-3.5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="square" stroke-linejoin="miter" d="M5 12l5 5L20 7" />
        </svg>
        <!-- X when failed -->
        <svg
          v-else-if="uiStore.deployStatus === 'failed'"
          xmlns="http://www.w3.org/2000/svg"
          class="w-3.5 h-3.5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="square" stroke-linejoin="miter" d="M6 18L18 6M6 6l12 12" />
        </svg>
        <!-- Default deploy icon -->
        <svg
          v-else
          xmlns="http://www.w3.org/2000/svg"
          class="w-3.5 h-3.5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="square" stroke-linejoin="miter" d="M5 12l5 5L20 7" />
        </svg>
        <span>{{ deployLabel }}</span>
      </button>
    </div>
  </header>
</template>
