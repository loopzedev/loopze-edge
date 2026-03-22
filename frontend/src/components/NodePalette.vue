<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useApi } from '@/composables/useApi'
import type { NodeCatalogEntry } from '@/types/flow'
import NodeIcon from '@/components/nodes/NodeIcon.vue'
import { getTokens } from '@/components/nodes/tokens'

const api = useApi()

const nodes = ref<NodeCatalogEntry[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const filterText = ref('')
const expandedCategories = ref<Set<string>>(new Set(['common']))

onMounted(async () => {
  try {
    nodes.value = await api.getNodes()
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Failed to load nodes'
  } finally {
    loading.value = false
  }
})

const filteredNodes = computed(() => {
  const q = filterText.value.trim().toLowerCase()
  if (!q) return nodes.value
  return nodes.value.filter(n =>
    n.label.toLowerCase().includes(q) || n.type.toLowerCase().includes(q)
  )
})

const CATEGORY_ORDER = ['common', 'network', 'industrial', 'function', 'parser', 'storage', 'other']

const categories = computed(() => {
  const map = new Map<string, NodeCatalogEntry[]>()
  for (const node of filteredNodes.value) {
    const cat = node.category ?? 'other'
    if (!map.has(cat)) map.set(cat, [])
    map.get(cat)!.push(node)
  }
  for (const [, catNodes] of map) {
    catNodes.sort((a, b) => a.label.localeCompare(b.label))
  }
  const ordered: { name: string; nodes: NodeCatalogEntry[] }[] = []
  for (const cat of CATEGORY_ORDER) {
    if (map.has(cat)) {
      ordered.push({ name: cat, nodes: map.get(cat)! })
      map.delete(cat)
    }
  }
  for (const [name, catNodes] of map) {
    ordered.push({ name, nodes: catNodes })
  }
  return ordered
})

function toggleCategory(name: string): void {
  if (expandedCategories.value.has(name)) {
    expandedCategories.value.delete(name)
  } else {
    expandedCategories.value.add(name)
  }
}

function onDragStart(event: DragEvent, node: NodeCatalogEntry): void {
  if (!event.dataTransfer) return
  event.dataTransfer.setData(
    'application/flint-node',
    JSON.stringify({
      type: node.type,
      inputs: node.inputs,
      outputs: node.outputs,
      defaults: node.defaults,
    })
  )
  event.dataTransfer.effectAllowed = 'move'
}
</script>

<template>
  <aside class="h-full bg-terminal-surface border-r border-terminal-border flex flex-col overflow-hidden select-none">
    <!-- Panel Header -->
    <div class="flex items-center justify-between px-4 py-2.5 border-b border-terminal-border shrink-0">
      <span class="text-xs font-semibold uppercase tracking-wider text-terminal-text">Nodes</span>
      <span class="text-[10px] text-terminal-text-dim font-mono tabular-nums">{{ nodes.length }}</span>
    </div>

    <!-- Search -->
    <div class="px-3 py-2 border-b border-terminal-border shrink-0">
      <div class="relative">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-terminal-text-dim/50" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          v-model="filterText"
          type="text"
          placeholder="Search nodes..."
          class="terminal-input w-full text-[11px] py-1.5 pl-7"
        />
      </div>
    </div>

    <!-- Body -->
    <div class="flex-1 overflow-y-auto">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center h-16 text-terminal-text-dim text-xs">
        loading...
      </div>

      <!-- Error -->
      <div v-else-if="error" class="px-3 py-3 text-xs text-status-error">
        {{ error }}
      </div>

      <!-- Empty -->
      <div v-else-if="categories.length === 0" class="px-3 py-3 text-xs text-terminal-text-dim">
        No nodes found.
      </div>

      <!-- Categories -->
      <template v-else>
        <div
          v-for="cat in categories"
          :key="cat.name"
          class="border-b border-terminal-border"
        >
          <!-- Category Header -->
          <button
            class="w-full flex items-center justify-between px-3 py-2 text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt/50 transition-colors cursor-pointer"
            @click="toggleCategory(cat.name)"
          >
            <div class="flex items-center gap-1.5">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="w-3 h-3 transition-transform duration-150 shrink-0"
                :class="expandedCategories.has(cat.name) || filterText ? 'rotate-90' : ''"
                fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
              </svg>
              <span>{{ cat.name }}</span>
            </div>
            <span class="text-[9px] font-mono text-terminal-text-dim/50 tabular-nums">
              {{ cat.nodes.length }}
            </span>
          </button>

          <!-- Nodes -->
          <div v-show="expandedCategories.has(cat.name) || filterText" class="px-2.5 pb-2.5 flex flex-col gap-1.5">
            <div
              v-for="node in cat.nodes"
              :key="node.type"
              draggable="true"
              class="cursor-grab active:cursor-grabbing group"
              :title="node.description"
              @dragstart="onDragStart($event, node)"
            >
              <!-- Mini node preview -->
              <div
                class="flex text-xs transition-all duration-100 group-hover:translate-x-0.5"
                :style="{
                  border: `1px solid ${getTokens(node.type).border}`,
                  borderLeft: `3px solid ${getTokens(node.type).accent}`,
                  borderRadius: '3px',
                }"
              >
                <!-- Icon column -->
                <div
                  class="w-9 shrink-0 flex items-center justify-center"
                  :style="{ background: getTokens(node.type).bgIcon, color: getTokens(node.type).accent }"
                >
                  <NodeIcon :type="node.type" />
                </div>
                <!-- Label -->
                <div
                  class="flex-1 min-w-0 flex items-center px-2 py-1.5"
                  :style="{ background: getTokens(node.type).bgHdr }"
                >
                  <span
                    class="text-[11px] font-medium tracking-wide truncate"
                    :style="{ color: getTokens(node.type).accent }"
                  >
                    {{ node.label }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- Footer -->
    <div class="px-3 py-2 border-t border-terminal-border text-[10px] text-terminal-text-dim/60 shrink-0">
      {{ nodes.length }} node{{ nodes.length !== 1 ? 's' : '' }} registered
    </div>
  </aside>
</template>
