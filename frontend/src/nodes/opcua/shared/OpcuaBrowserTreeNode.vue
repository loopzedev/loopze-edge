<script lang="ts">
import type { OpcuaBrowseChild as Child } from '@/composables/useApi'

export interface InternalTreeNode {
  child: Child
  children?: InternalTreeNode[]
  loading: boolean
  error?: string
  expanded: boolean
}
</script>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    node: InternalTreeNode
    selectedIds: Set<string>
    existingSet: Set<string>
    focused: InternalTreeNode | null
    search: string
    isSelectable: (c: Child) => boolean
    isVisible: (c: Child) => boolean
    matchesSearch: (c: Child) => boolean
    depth?: number
  }>(),
  { depth: 0 },
)

const emit = defineEmits<{
  toggle: [node: InternalTreeNode]
  focus: [node: InternalTreeNode]
  select: [node: InternalTreeNode]
}>()

const c = computed(() => props.node.child)

const visible = computed(
  () =>
    props.isVisible(c.value) &&
    (props.matchesSearch(c.value) || hasMatchingDescendant(props.node, props.matchesSearch)),
)
const isFocused = computed(() => props.focused === props.node)
const isExisting = computed(() => props.existingSet.has(c.value.nodeId))
const isSelected = computed(() => props.selectedIds.has(c.value.nodeId))
const selectable = computed(() => props.isSelectable(c.value))
const indentPx = computed(() => props.depth * 12 + 4)

function onCaretClick(e: Event) {
  e.stopPropagation()
  if (c.value.hasChildren) emit('toggle', props.node)
}

function onCheckboxClick(e: Event) {
  e.stopPropagation()
  emit('select', props.node)
}

function hasMatchingDescendant(node: InternalTreeNode, matches: (c: Child) => boolean): boolean {
  if (!node.children) return false
  for (const child of node.children) {
    if (matches(child.child)) return true
    if (hasMatchingDescendant(child, matches)) return true
  }
  return false
}

const kindClass = computed(() => {
  switch (c.value.nodeClass) {
    case 'Variable':
      return 'text-[#22d3ee]'
    case 'Method':
      return 'text-[#c084fc]'
    case 'Object':
    case 'View':
      return 'text-terminal-text'
    default:
      return 'text-terminal-text-dim'
  }
})

const kindIcon = computed(() => {
  const x = c.value
  switch (x.nodeClass) {
    case 'Object':
      return '▢'
    case 'View':
      return '⊞'
    case 'Method':
      return 'ƒ'
    case 'ObjectType':
    case 'VariableType':
    case 'ReferenceType':
      return '⊜'
    case 'DataType':
      return '⊕'
    case 'Variable':
      if (x.dataType?.isStructure) return '◆'
      if (x.valueRank && x.valueRank > 0) return '▤'
      return '◇'
  }
  return '·'
})

const kindLabel = computed(() => {
  const x = c.value
  switch (x.nodeClass) {
    case 'Object':
      return 'Object / Folder'
    case 'View':
      return 'View'
    case 'Method':
      return 'Method'
    case 'ObjectType':
      return 'Object Type'
    case 'VariableType':
      return 'Variable Type'
    case 'ReferenceType':
      return 'Reference Type'
    case 'DataType':
      return 'Data Type'
    case 'Variable':
      if (x.dataType?.isStructure) return 'Variable (structure)'
      if (x.valueRank && x.valueRank > 0) return 'Variable (array)'
      return 'Variable (scalar)'
  }
  return x.nodeClass
})

const dataTypeSuffix = computed(() => {
  const x = c.value
  if (x.nodeClass !== 'Variable' || !x.dataType) return ''
  const arr = x.valueRank && x.valueRank > 0 ? '[]' : ''
  return `· ${x.dataType.name || 'Unknown'}${arr}`
})
</script>

<template>
  <div v-if="visible">
    <div
      class="flex items-center gap-1 px-1 py-0.5 cursor-pointer"
      :class="[
        isFocused ? 'bg-accent/10' : 'hover:bg-terminal-surface/60',
        isExisting ? 'opacity-50' : '',
      ]"
      :style="{ paddingLeft: `${indentPx}px` }"
      @click="emit('focus', node)"
    >
      <!-- Expand caret -->
      <span
        class="w-3 inline-block text-terminal-text-dim text-center"
        @click="onCaretClick"
      >{{ c.hasChildren ? (node.expanded ? '▾' : '▸') : ' ' }}</span>

      <!-- Checkbox or filler -->
      <input
        v-if="selectable"
        type="checkbox"
        class="mr-1"
        :checked="isSelected || isExisting"
        :disabled="isExisting"
        @click="onCheckboxClick"
      />
      <span v-else class="inline-block w-3 mr-1" />

      <!-- Kind glyph: tells Object/Variable/Struct/Array/Method apart -->
      <span
        class="inline-block w-4 mr-1 text-xs leading-none text-center"
        :class="kindClass"
        :title="kindLabel"
      >{{ kindIcon }}</span>

      <!-- Display name -->
      <span
        class="text-[10px] truncate"
        :class="kindClass"
        :title="c.nodeId"
      >{{ c.displayName || c.browseName || c.nodeId }}</span>

      <!-- DataType hint suffix for variables -->
      <span
        v-if="dataTypeSuffix"
        class="ml-1 text-[9px] text-terminal-text-dim shrink-0"
      >{{ dataTypeSuffix }}</span>
    </div>

    <!-- Children -->
    <template v-if="node.expanded && node.children">
      <OpcuaBrowserTreeNode
        v-for="(child, idx) in node.children"
        :key="idx"
        :node="child"
        :selected-ids="selectedIds"
        :existing-set="existingSet"
        :focused="focused"
        :search="search"
        :is-selectable="isSelectable"
        :is-visible="isVisible"
        :matches-search="matchesSearch"
        :depth="depth + 1"
        @toggle="(n) => emit('toggle', n)"
        @focus="(n) => emit('focus', n)"
        @select="(n) => emit('select', n)"
      />
    </template>

    <!-- Loading / error indicators -->
    <div v-if="node.loading" class="pl-6 text-[10px] text-terminal-text-dim">loading…</div>
    <div v-if="node.error" class="pl-6 text-[10px] text-red-400">{{ node.error }}</div>
  </div>
</template>
