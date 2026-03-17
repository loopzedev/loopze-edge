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
      inputs: node.inputs,
      outputs: node.outputs,
    })
  )
  event.dataTransfer.effectAllowed = 'move'
}
</script>

<template>
  <aside class="h-full bg-terminal-surface border-r border-terminal-border flex flex-col overflow-hidden select-none">
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

      <!-- Empty -->
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

          <!-- Nodes — rendered like canvas nodes -->
          <div v-show="expandedCategories.has(cat.name) || filterText" class="px-2 pb-2 flex flex-col gap-1.5">
            <div
              v-for="node in cat.nodes"
              :key="node.type"
              draggable="true"
              class="cursor-grab active:cursor-grabbing"
              :title="node.description"
              @dragstart="onDragStart($event, node)"
            >
              <!-- Mini node preview — same visual as BaseNode on canvas -->
              <div
                class="flex font-mono text-xs"
                :style="{
                  border: `1px solid ${getTokens(node.type).border}`,
                  borderLeft: `3px solid ${getTokens(node.type).accent}`,
                  borderRadius: '2px',
                }"
              >
                <!-- Icon column -->
                <div
                  class="w-10 shrink-0 flex items-center justify-center"
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
    <div class="px-3 py-1.5 border-t border-terminal-border text-[10px] text-terminal-text-dim shrink-0">
      {{ nodes.length }} node{{ nodes.length !== 1 ? 's' : '' }} registered
    </div>
  </aside>
</template>
