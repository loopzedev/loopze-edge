<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useApi } from '@/composables/useApi'
import type { NodeCatalogEntry } from '@/types/flow'

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

const categories = computed(() => {
  const map = new Map<string, NodeCatalogEntry[]>()
  for (const node of filteredNodes.value) {
    const cat = node.category ?? 'other'
    if (!map.has(cat)) map.set(cat, [])
    map.get(cat)!.push(node)
  }
  return Array.from(map.entries()).map(([name, catNodes]) => ({ name, nodes: catNodes }))
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
      label: node.label,
      inputs: node.inputs,
      outputs: node.outputs,
    })
  )
  event.dataTransfer.effectAllowed = 'move'
}

const NODE_ICONS: Record<string, string> = {
  inject: '⏱',
  debug: '⬤',
  comment: '✎',
  'link-in': '←',
  'link-out': '→',
  catch: '⚡',
  status: '◈',
  function: 'ƒ',
  change: '⇄',
  switch: '⑂',
  template: '▤',
  delay: '⏲',
  filter: '⧩',
  'http-in': '▶',
  'http-response': '◀',
  'http-request': '⇆',
  'mqtt-in': '▼',
  'mqtt-out': '▲',
  'tcp-in': '⇊',
  'tcp-out': '⇈',
  'modbus-read': '⎍',
  'modbus-write': '⎌',
  'opc-ua': '⚙',
  'file-in': '⤓',
  'file-out': '⤒',
  json: '{}',
  xml: '⟨⟩',
  csv: '⊞',
}

function getNodeIcon(node: NodeCatalogEntry): string {
  if (node.icon) return node.icon
  return NODE_ICONS[node.type] ?? '●'
}
</script>

<template>
  <aside class="h-full w-[220px] bg-terminal-surface border-r border-terminal-border flex flex-col overflow-hidden select-none">
    <!-- Panel Header -->
    <div class="flex items-center px-3 py-2 border-b border-terminal-border shrink-0">
      <span class="text-terminal-text text-xs font-bold uppercase tracking-widest">Nodes</span>
    </div>

    <!-- Search -->
    <div class="px-2 py-1.5 border-b border-terminal-border shrink-0">
      <input
        v-model="filterText"
        type="text"
        placeholder="Filter nodes..."
        class="terminal-input w-full text-xs py-1"
      />
    </div>

    <!-- Body -->
    <div class="flex-1 overflow-y-auto">

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center h-16 text-terminal-text-dim text-xs">
        loading…
      </div>

      <!-- Error -->
      <div v-else-if="error" class="px-3 py-2 text-xs text-red-400">
        {{ error }}
      </div>

      <!-- Empty (after filter) -->
      <div v-else-if="categories.length === 0" class="px-3 py-2 text-xs text-terminal-text-dim">
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
            class="w-full flex items-center justify-between px-3 py-1.5 text-xs font-bold uppercase tracking-wider text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-border/30 transition-colors cursor-pointer"
            @click="toggleCategory(cat.name)"
          >
            <span>{{ cat.name }}</span>
            <span
              class="text-[10px] transition-transform duration-150"
              :class="expandedCategories.has(cat.name) ? 'rotate-0' : '-rotate-90'"
            >
              ▼
            </span>
          </button>

          <!-- Nodes -->
          <div v-show="expandedCategories.has(cat.name) || filterText" class="pb-1">
            <div
              v-for="node in cat.nodes"
              :key="node.type"
              draggable="true"
              class="flex items-center gap-2 mx-1 px-2 py-1 cursor-grab text-xs text-terminal-text hover:bg-terminal-border/40 active:cursor-grabbing transition-colors group"
              :title="node.description"
              @dragstart="onDragStart($event, node)"
            >
              <!-- Icon -->
              <span class="w-5 h-5 flex items-center justify-center border border-terminal-border bg-terminal-bg text-[10px] text-terminal-text-dim group-hover:border-amber group-hover:text-terminal-text shrink-0 transition-colors">
                {{ getNodeIcon(node) }}
              </span>

              <!-- Label -->
              <span class="truncate">{{ node.label }}</span>

              <!-- Port Indicators -->
              <span class="ml-auto flex items-center gap-0.5 text-[9px] text-terminal-text-dim shrink-0">
                <span v-if="node.inputs > 0" title="inputs">▸{{ node.inputs }}</span>
                <span v-if="node.outputs > 0" title="outputs">{{ node.outputs }}▸</span>
              </span>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- Footer -->
    <div class="px-3 py-1.5 border-t border-terminal-border text-[10px] text-terminal-text-dim shrink-0">
      {{ nodes.length }} node{{ nodes.length !== 1 ? 's' : '' }} registered
    </div>
  </aside>
</template>
