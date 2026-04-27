<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useAuthStore } from '@/stores/authStore'
import { useContextStore, type ContextView, type TaggedContextEntry } from '@/stores/contextStore'
import JsonTreeView from '@/components/JsonTreeView.vue'

const flow = useFlowStore()
const ui = useUiStore()
const auth = useAuthStore()
const ctx = useContextStore()

interface ViewOption {
  key: ContextView
  label: string
}

const viewOptions: ViewOption[] = [
  { key: 'global', label: 'Global' },
  { key: 'flow',   label: 'Flow'   },
]

const isFlowView = computed(() => ctx.view === 'flow')

if (!ctx.flowId) {
  ctx.setFlowId(flow.activeFlowId ?? null)
}

const expanded = ref<Record<string, boolean>>({})

function entryKey(e: TaggedContextEntry): string {
  return `${e.storage}::${e.key}`
}

function toggleExpand(e: TaggedContextEntry) {
  const k = entryKey(e)
  expanded.value[k] = !expanded.value[k]
}

function isExpanded(e: TaggedContextEntry): boolean {
  return !!expanded.value[entryKey(e)]
}

function handleSelect(key: ContextView) {
  ctx.selectView(key)
}

function handleFlowChange(id: string) {
  ctx.setFlowId(id || null)
}

async function handleRefreshAll() {
  await ctx.loadAll()
}

async function handleRefreshKey(e: TaggedContextEntry) {
  await ctx.loadKey(e)
}

async function handleDeleteKey(e: TaggedContextEntry) {
  if (!window.confirm(`Delete key "${e.key}" (${e.storage})?`)) return
  await ctx.deleteKey(e)
}

async function handleClearAll() {
  const label = viewOptions.find(o => o.key === ctx.view)?.label ?? ctx.view
  if (!window.confirm(`Delete ALL keys in "${label}"? This cannot be undone.`)) return
  await ctx.clearAll()
}

function toggleAutoRefresh() {
  ui.setContextAutoRefresh(!ui.contextAutoRefresh)
}

watch(
  () => [ctx.view, ctx.flowId] as const,
  () => { ctx.loadAll() },
)

let timer: ReturnType<typeof setInterval> | null = null
watchEffect(() => {
  const active =
    ui.contextAutoRefresh &&
    ui.activeInfoTab === 'context' &&
    ui.infoPanelOpen
  if (active && !timer) {
    timer = setInterval(() => { ctx.loadAll() }, 1000)
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

onMounted(() => {
  ctx.loadAll()
})

function formatPreview(value: unknown): string {
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return JSON.stringify(value)
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return `Array(${value.length})`
  if (typeof value === 'object') return '{…}'
  return String(value)
}

function storageTagLabel(storage: string): string {
  return storage === 'memory' ? 'MEM' : 'PERS'
}
</script>

<template>
  <div class="flex flex-col h-full bg-terminal-bg text-xs">
    <!-- Toolbar -->
    <div class="px-3 py-2 border-b border-terminal-border space-y-2 shrink-0">
      <!-- View selector -->
      <div class="grid grid-cols-2 gap-1">
        <button
          v-for="opt in viewOptions"
          :key="opt.key"
          class="px-1.5 py-1 text-[10px] uppercase tracking-wider font-semibold rounded border transition-colors cursor-pointer"
          :class="ctx.view === opt.key
            ? 'bg-accent/20 border-accent text-accent'
            : 'bg-terminal-surface border-terminal-border text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text-dim'"
          @click="handleSelect(opt.key)"
        >
          {{ opt.label }}
        </button>
      </div>

      <!-- Flow dropdown (only for flow views) -->
      <div v-if="isFlowView" class="flex items-center gap-2">
        <label class="text-[10px] uppercase tracking-wider text-terminal-text-dim shrink-0">Flow</label>
        <select
          :value="ctx.flowId ?? ''"
          class="flex-1 bg-terminal-surface border border-terminal-border rounded px-2 py-1 text-[11px] text-terminal-text"
          @change="handleFlowChange(($event.target as HTMLSelectElement).value)"
        >
          <option value="">— select a flow —</option>
          <option
            v-for="f in flow.flows"
            :key="f.id"
            :value="f.id"
          >
            {{ f.label || f.id.slice(0, 8) }}
          </option>
        </select>
      </div>

      <!-- Refresh + auto-refresh toggle -->
      <div class="flex items-center justify-between gap-2">
        <button
          class="flex items-center gap-1 px-2 py-1 text-[10px] uppercase tracking-wider font-semibold rounded border border-terminal-border bg-terminal-surface text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text-dim transition-colors cursor-pointer"
          :disabled="ctx.loading"
          @click="handleRefreshAll"
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
            :checked="ui.contextAutoRefresh"
            @change="toggleAutoRefresh"
          />
          <span class="text-[10px] uppercase tracking-wider text-terminal-text-dim">Auto-Refresh 1s</span>
        </label>
      </div>
    </div>

    <!-- Error -->
    <div v-if="ctx.error" class="px-3 py-2 bg-red-900/20 border-b border-red-800/40 text-[11px] text-red-400">
      {{ ctx.error }}
    </div>

    <!-- Empty state: flow view but no flow selected -->
    <div
      v-if="isFlowView && !ctx.flowId"
      class="flex-1 flex items-center justify-center"
    >
      <span class="text-terminal-text-dim text-[11px]">Select a flow to view its context.</span>
    </div>

    <!-- Empty state: no entries -->
    <div
      v-else-if="ctx.entries.length === 0 && !ctx.loading"
      class="flex-1 flex items-center justify-center"
    >
      <span class="text-terminal-text-dim text-[11px]">No keys in this context store.</span>
    </div>

    <!-- Entries -->
    <div v-else class="flex-1 overflow-y-auto">
      <div
        v-for="entry in ctx.entries"
        :key="entryKey(entry)"
        class="border-b border-terminal-border/40"
      >
        <!-- Row header -->
        <div
          class="flex items-center gap-2 px-3 py-1.5 hover:bg-terminal-surface-alt/30 transition-colors group"
        >
          <button
            class="shrink-0 text-terminal-text-dim hover:text-terminal-text cursor-pointer"
            @click="toggleExpand(entry)"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="w-3 h-3 transition-transform"
              :class="{ 'rotate-90': isExpanded(entry) }"
              fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
            >
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </button>

          <div class="flex-1 flex items-center gap-1.5 min-w-0">
            <button
              class="font-mono text-[11px] text-terminal-text truncate min-w-0 text-left cursor-pointer"
              @click="toggleExpand(entry)"
            >
              {{ entry.key }}
            </button>

            <!-- Storage tag (memory vs persistent) — directly after the key -->
            <span
              class="shrink-0 px-1 py-0.5 text-[9px] uppercase tracking-wider font-semibold rounded border"
              :class="entry.storage === 'persistent'
                ? 'bg-amber-900/30 border-amber-700/40 text-amber-400'
                : 'bg-sky-900/30 border-sky-700/40 text-sky-400'"
            >
              {{ storageTagLabel(entry.storage) }}
            </span>
          </div>

          <span
            v-if="!isExpanded(entry)"
            class="font-mono text-[10px] text-terminal-text-dim truncate max-w-[140px]"
          >
            {{ formatPreview(entry.value) }}
          </span>

          <button
            class="shrink-0 p-1 text-terminal-text-dim hover:text-terminal-text opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
            title="Refresh this key"
            @click="handleRefreshKey(entry)"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>

          <button
            v-if="auth.can('mutateContext')"
            class="shrink-0 p-1 text-terminal-text-dim hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
            title="Delete this key"
            @click="handleDeleteKey(entry)"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
            </svg>
          </button>
        </div>

        <!-- Expanded value -->
        <div v-if="isExpanded(entry)" class="px-3 pb-2 pl-7">
          <JsonTreeView :data="entry.value" :default-expand-depth="2" />
        </div>
      </div>
    </div>

    <!-- Footer: Clear All -->
    <div
      v-if="ctx.entries.length > 0 && auth.can('mutateContext')"
      class="px-3 py-2 border-t border-terminal-border shrink-0 flex justify-end"
    >
      <button
        class="px-2 py-1 text-[10px] uppercase tracking-wider font-semibold rounded border border-red-800/60 bg-red-900/20 text-red-400 hover:bg-red-900/40 hover:border-red-700 transition-colors cursor-pointer"
        @click="handleClearAll"
      >
        Clear All
      </button>
    </div>
  </div>
</template>
