<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useApi, type OpcuaBrowseChild, type OpcuaReadResult } from '@/composables/useApi'
import OpcuaBrowserTreeNode from './OpcuaBrowserTreeNode.vue'

interface SelectedItem {
  nodeId: string
  displayName: string
  dataType?: string
  structureType?: string
}

const props = withDefaults(
  defineProps<{
    open: boolean
    serverId?: string
    serverConfig?: Record<string, unknown>
    existingNodeIds?: string[]
    multiSelect?: boolean
    title?: string
  }>(),
  {
    multiSelect: true,
    existingNodeIds: () => [],
    title: 'Browse OPC UA Server',
  },
)

const emit = defineEmits<{
  close: []
  select: [items: SelectedItem[]]
}>()

const api = useApi()

interface TreeNode {
  child: OpcuaBrowseChild
  children?: TreeNode[]
  loading: boolean
  error?: string
  expanded: boolean
}

const root = ref<TreeNode | null>(null)
const focused = ref<TreeNode | null>(null)
const selectedIds = ref<Set<string>>(new Set())
const selectedMeta = ref<Map<string, SelectedItem>>(new Map())
const search = ref('')
const nodeClassFilter = ref<'all' | 'variables' | 'methods'>('variables')
const initError = ref<string | null>(null)
const initialising = ref(false)

// Read-now state for the focused variable. Cleared whenever the focus
// changes so a stale value never lingers under a different node.
const readResult = ref<OpcuaReadResult | null>(null)
const readError = ref<string | null>(null)
const reading = ref(false)

const existingSet = computed(() => new Set(props.existingNodeIds ?? []))

// Promises started before unmount can still resolve; the alive flag lets
// async handlers bail out instead of writing into a torn-down component.
let alive = true
onBeforeUnmount(() => {
  alive = false
})

function rebuildSelection() {
  selectedIds.value = new Set()
  selectedMeta.value = new Map()
}

watch(
  () => props.open,
  (now) => {
    if (now) {
      rebuildSelection()
      void loadRoot()
    }
  },
  { immediate: true },
)

async function loadRoot() {
  initialising.value = true
  initError.value = null
  try {
    const result = await fetchChildren('') // empty → Objects folder
    if (!alive) return
    root.value = {
      child: result.parent ?? {
        nodeId: 'i=85',
        browseName: 'Objects',
        displayName: 'Objects',
        nodeClass: 'Object',
        hasChildren: true,
      },
      children: (result.children ?? []).map(toTreeNode),
      loading: false,
      expanded: true,
    }
    focused.value = root.value
  } catch (err) {
    if (!alive) return
    initError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (alive) initialising.value = false
  }
}

function toTreeNode(c: OpcuaBrowseChild): TreeNode {
  return { child: c, loading: false, expanded: false }
}

async function fetchChildren(nodeId: string) {
  const res = await api.browseOpcua({
    serverId: props.serverId,
    config: props.serverConfig,
    nodeId,
  })
  if (!res.ok) {
    throw new Error(res.error ?? 'browse failed')
  }
  return res.result ?? { children: [] }
}

async function toggleNode(node: TreeNode) {
  if (!node.child.hasChildren) return
  if (node.children) {
    node.expanded = !node.expanded
    return
  }
  node.loading = true
  node.error = undefined
  try {
    const result = await fetchChildren(node.child.nodeId)
    if (!alive) return
    node.children = (result.children ?? []).map(toTreeNode)
    node.expanded = true
  } catch (err) {
    if (!alive) return
    node.error = err instanceof Error ? err.message : String(err)
  } finally {
    if (alive) node.loading = false
  }
}

function focusNode(node: TreeNode) {
  focused.value = node
  readResult.value = null
  readError.value = null
}

async function readNow() {
  if (!focused.value) return
  reading.value = true
  readError.value = null
  readResult.value = null
  try {
    const res = await api.readOpcua({
      serverId: props.serverId,
      config: props.serverConfig,
      nodeId: focused.value.child.nodeId,
    })
    if (!alive) return
    if (res.ok && res.result) {
      readResult.value = res.result
    } else {
      readError.value = res.error ?? 'read failed'
    }
  } catch (err) {
    if (!alive) return
    readError.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (alive) reading.value = false
  }
}

function formatReadValue(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'object') return JSON.stringify(v, null, 2)
  return String(v)
}

function isSelectable(child: OpcuaBrowseChild): boolean {
  // Synthetic struct fields are selectable as a UX convenience: the
  // selection translates to the parent variable in buildSelectedItem,
  // because OPC UA only allows subscribing to the whole structure value.
  if (child.synthetic) return true
  if (nodeClassFilter.value === 'variables') {
    return child.nodeClass === 'Variable'
  }
  if (nodeClassFilter.value === 'methods') {
    return child.nodeClass === 'Method'
  }
  return child.nodeClass === 'Variable' || child.nodeClass === 'Method' || child.nodeClass === 'Object'
}

function isVisible(child: OpcuaBrowseChild): boolean {
  // Always show containers that may host matches further down so the user can
  // navigate to filtered targets. Variables / methods themselves obey the filter.
  if (child.nodeClass === 'Object' || child.nodeClass === 'View') return true
  if (nodeClassFilter.value === 'all') return true
  if (nodeClassFilter.value === 'variables') return child.nodeClass === 'Variable'
  if (nodeClassFilter.value === 'methods') return child.nodeClass === 'Method'
  return false
}

function matchesSearch(child: OpcuaBrowseChild): boolean {
  const q = search.value.trim().toLowerCase()
  if (!q) return true
  return (
    child.browseName.toLowerCase().includes(q) ||
    child.displayName.toLowerCase().includes(q) ||
    child.nodeId.toLowerCase().includes(q)
  )
}

function toggleSelection(node: TreeNode) {
  // Synthetic struct fields use a different "id key" for the selection set
  // (the full $field= form) so multiple fields of one struct can each be
  // checked independently in the tree, even though they all collapse to the
  // same parent NodeID in the final SelectedItem map.
  const treeId = node.child.nodeId
  const item = buildSelectedItem(node.child)
  if (existingSet.value.has(item.nodeId)) return
  if (!isSelectable(node.child)) return

  if (!props.multiSelect) {
    selectedIds.value = new Set([treeId])
    selectedMeta.value = new Map([[item.nodeId, item]])
    return
  }
  const next = new Set(selectedIds.value)
  const meta = new Map(selectedMeta.value)
  if (next.has(treeId)) {
    next.delete(treeId)
    // Only drop the parent SelectedItem when no other field of the same
    // parent is still checked.
    const stillReferenced = Array.from(next).some((id) => {
      if (id === treeId) return false
      return parentNodeID(id) === item.nodeId
    })
    if (!stillReferenced) meta.delete(item.nodeId)
  } else {
    next.add(treeId)
    meta.set(item.nodeId, item)
  }
  selectedIds.value = next
  selectedMeta.value = meta
}

// parentNodeID extracts the addressable parent from any tree-level NodeID
// (synthetic field IDs include the "$field=" suffix; everything else maps
// to itself).
function parentNodeID(treeNodeID: string): string {
  return treeNodeID.split('$field=')[0] ?? treeNodeID
}

function buildSelectedItem(child: OpcuaBrowseChild): SelectedItem {
  // Synthetic struct field: the on-the-wire NodeID is the encoded
  // "$field=…" form. We translate that to the parent variable's NodeID so
  // the actual subscription/read works — the field path goes into the
  // display name as a hint for the user.
  if (child.synthetic) {
    const parentId = child.nodeId.split('$field=')[0] ?? child.nodeId
    const fieldPath = child.nodeId.split('$field=')[1] ?? child.browseName
    return {
      nodeId: parentId,
      displayName: `${shortenNodeID(parentId)}.${fieldPath}`,
    }
  }
  const item: SelectedItem = {
    nodeId: child.nodeId,
    displayName: child.displayName || child.browseName || child.nodeId,
  }
  if (child.dataType) {
    item.dataType = child.dataType.name
    if (child.dataType.isStructure) {
      item.structureType = child.dataType.nodeId
    }
  }
  return item
}

// shortenNodeID extracts a human-friendly name from a NodeID. For string
// identifiers we use everything after the last "." (the leaf name); for
// numeric identifiers we just return the raw NodeID.
function shortenNodeID(nodeId: string): string {
  const stringIdMatch = /;s=(.+)$/.exec(nodeId)
  if (stringIdMatch) {
    const id = stringIdMatch[1]
    const lastDot = id.lastIndexOf('.')
    return lastDot >= 0 ? id.slice(lastDot + 1) : id
  }
  return nodeId
}

function commit() {
  emit('select', Array.from(selectedMeta.value.values()))
  emit('close')
}

function close() {
  emit('close')
}

const selectedCount = computed(() => selectedIds.value.size)
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
    @click.self="close"
  >
    <div
      class="bg-terminal-bg border border-terminal-border w-[min(80vw,900px)] h-[min(85vh,700px)] flex flex-col"
    >
      <!-- Header -->
      <div class="shrink-0 flex items-center justify-between px-4 py-2 border-b border-terminal-border">
        <span class="text-accent text-sm font-bold uppercase tracking-wider">{{ title }}</span>
        <button
          class="text-[10px] uppercase tracking-wider text-terminal-text-dim hover:text-terminal-text"
          @click="close"
        >Close ✕</button>
      </div>

      <!-- Filter bar -->
      <div class="shrink-0 flex items-stretch gap-2 px-4 py-2 border-b border-terminal-border bg-terminal-surface/40">
        <input
          v-model="search"
          placeholder="search loaded nodes…"
          class="flex-1 bg-terminal-bg border border-terminal-border px-2 py-1 text-[10px] outline-none focus:border-accent"
        />
        <select
          v-model="nodeClassFilter"
          class="bg-terminal-bg border border-terminal-border px-2 py-1 text-[10px] outline-none focus:border-accent"
        >
          <option value="variables">Variables</option>
          <option value="methods">Methods</option>
          <option value="all">All</option>
        </select>
      </div>

      <!-- Body: tree + detail -->
      <div class="flex-1 grid grid-cols-2 overflow-hidden">
        <!-- Tree -->
        <div class="overflow-y-auto border-r border-terminal-border p-2">
          <div v-if="initialising" class="text-[10px] text-terminal-text-dim">Loading…</div>
          <div v-else-if="initError" class="text-[10px] text-red-400">
            {{ initError }}
          </div>
          <div v-else-if="root">
            <OpcuaBrowserTreeNode
              :node="root"
              :selected-ids="selectedIds"
              :existing-set="existingSet"
              :focused="focused"
              :search="search"
              :is-selectable="isSelectable"
              :is-visible="isVisible"
              :matches-search="matchesSearch"
              :depth="0"
              @toggle="toggleNode"
              @focus="focusNode"
              @select="toggleSelection"
            />
          </div>
        </div>

        <!-- Detail panel -->
        <div class="overflow-y-auto p-3 text-[10px]">
          <template v-if="focused">
            <div class="mb-2 text-accent text-[11px] font-bold uppercase tracking-wider">
              {{ focused.child.displayName || focused.child.browseName }}
            </div>
            <p
              v-if="focused.child.synthetic"
              class="mb-2 text-[10px] text-terminal-text-dim leading-relaxed"
            >
              Struct field. Selecting it adds the parent variable to the list —
              OPC UA delivers the whole structure value, you address the field
              client-side via <code class="font-mono">msg.payload.value.{{ focused.child.browseName }}</code>.
            </p>
            <dl class="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1">
              <dt class="text-terminal-text-dim">NodeID</dt>
              <dd class="font-mono break-all">{{ focused.child.nodeId }}</dd>
              <dt class="text-terminal-text-dim">BrowseName</dt>
              <dd>{{ focused.child.browseName }}</dd>
              <dt class="text-terminal-text-dim">NodeClass</dt>
              <dd>{{ focused.child.nodeClass }}</dd>
              <template v-if="focused.child.dataType">
                <dt class="text-terminal-text-dim">DataType</dt>
                <dd>
                  {{ focused.child.dataType.name || focused.child.dataType.nodeId }}
                  <span v-if="focused.child.dataType.isStructure" class="text-[#22d3ee]">· struct</span>
                </dd>
              </template>
              <template v-if="focused.child.accessLevel">
                <dt class="text-terminal-text-dim">Access</dt>
                <dd>{{ focused.child.accessLevel }}</dd>
              </template>
              <template v-if="focused.child.description">
                <dt class="text-terminal-text-dim">Description</dt>
                <dd>{{ focused.child.description }}</dd>
              </template>
            </dl>

            <!-- Read now: only on real (non-synthetic) variables. -->
            <div
              v-if="focused.child.nodeClass === 'Variable' && !focused.child.synthetic"
              class="mt-3 flex flex-col gap-1.5"
            >
              <button
                class="self-start px-2 py-0.5 text-[10px] uppercase tracking-wider
                       border border-terminal-border text-terminal-text-dim
                       hover:text-accent hover:border-accent transition-colors disabled:opacity-50"
                :disabled="reading"
                @click="readNow"
              >
                {{ reading ? 'Reading…' : 'Read now' }}
              </button>
              <p v-if="readError" class="text-[10px] text-red-400">{{ readError }}</p>
              <div v-if="readResult" class="flex flex-col gap-1 mt-1">
                <p class="text-[9px] uppercase tracking-wider text-terminal-text-dim">
                  Status: {{ readResult.statusCode }}<span
                    v-if="readResult.dataType"
                  > · {{ readResult.dataType }}</span><span
                    v-if="readResult.sourceTimestamp"
                  > · {{ readResult.sourceTimestamp }}</span>
                </p>
                <pre
                  class="text-[10px] font-mono bg-terminal-bg border border-terminal-border
                         p-2 max-h-48 overflow-auto whitespace-pre-wrap"
                >{{ formatReadValue(readResult.value) }}</pre>
              </div>
            </div>
          </template>
          <p v-else class="text-terminal-text-dim">Select a node to inspect.</p>
        </div>
      </div>

      <!-- Footer -->
      <div class="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-terminal-border bg-terminal-surface">
        <span class="text-[10px] text-terminal-text-dim">
          {{ selectedCount }} selected
        </span>
        <div class="flex-1" />
        <button
          class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
                 bg-accent/10 text-accent border border-accent/30
                 hover:bg-accent/20 disabled:opacity-50 disabled:cursor-not-allowed"
          :disabled="selectedCount === 0"
          @click="commit"
        >
          Add {{ selectedCount }} to list
        </button>
        <button
          class="px-3 py-1 text-[10px] uppercase tracking-wider
                 text-terminal-text-dim border border-terminal-border
                 hover:text-terminal-text"
          @click="close"
        >Cancel</button>
      </div>
    </div>
  </div>
</template>

