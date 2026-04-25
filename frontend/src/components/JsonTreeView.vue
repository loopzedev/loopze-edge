<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDebugStore } from '@/stores/debugStore'

interface Props {
  data: unknown
  path?: string
  rootKey?: string
  depth?: number
  defaultExpandDepth?: number
  nodeId?: string
}

const props = withDefaults(defineProps<Props>(), {
  path: '',
  rootKey: '',
  depth: 0,
  defaultExpandDepth: 1,
  nodeId: '',
})

defineOptions({ name: 'JsonTreeView' })

const debugStore = useDebugStore()

const STRING_TRUNCATE = 200

type ValueKind = 'string' | 'number' | 'boolean' | 'null' | 'undefined' | 'object' | 'array'

function kindOf(v: unknown): ValueKind {
  if (v === null) return 'null'
  if (v === undefined) return 'undefined'
  if (Array.isArray(v)) return 'array'
  const t = typeof v
  if (t === 'string' || t === 'number' || t === 'boolean' || t === 'object') return t
  return 'string'
}

const kind = computed<ValueKind>(() => kindOf(props.data))
const isContainer = computed(() => kind.value === 'object' || kind.value === 'array')

const expanded = ref(props.depth < props.defaultExpandDepth)
const stringExpanded = ref(false)
const copiedPath = ref(false)
const copiedValue = ref(false)

const pinnedPaths = computed(() =>
  props.nodeId ? debugStore.pinnedPathsForNode(props.nodeId) : null,
)

const isPinned = computed(
  () => !!pinnedPaths.value && pinnedPaths.value.has(props.path),
)

const isOnPinnedPath = computed(() => {
  const pins = pinnedPaths.value
  if (!pins || pins.size === 0) return false
  if (props.path === '') return true
  for (const p of pins) {
    if (p === props.path) return true
    if (p.startsWith(props.path + '.')) return true
    if (p.startsWith(props.path + '[')) return true
  }
  return false
})

const effectiveExpanded = computed(() => isOnPinnedPath.value || expanded.value)

const childEntries = computed<Array<[string | number, unknown]>>(() => {
  if (kind.value === 'array') {
    return (props.data as unknown[]).map((v, i) => [i, v] as [number, unknown])
  }
  if (kind.value === 'object') {
    return Object.entries(props.data as Record<string, unknown>)
  }
  return []
})

const containerSummary = computed(() => {
  if (kind.value === 'array') {
    const len = (props.data as unknown[]).length
    return `[ … ] ${len} item${len === 1 ? '' : 's'}`
  }
  if (kind.value === 'object') {
    const len = Object.keys(props.data as object).length
    return `{ … } ${len} key${len === 1 ? '' : 's'}`
  }
  return ''
})

const childCount = computed(() => childEntries.value.length)

const truncatedString = computed(() => {
  if (kind.value !== 'string') return ''
  const s = props.data as string
  if (s.length <= STRING_TRUNCATE || stringExpanded.value) return s
  return s.slice(0, STRING_TRUNCATE) + '…'
})

const stringNeedsTruncate = computed(
  () => kind.value === 'string' && (props.data as string).length > STRING_TRUNCATE,
)

function buildChildPath(key: string | number): string {
  if (typeof key === 'number') {
    return `${props.path}[${key}]`
  }
  if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(key)) {
    return props.path === '' ? key : `${props.path}.${key}`
  }
  const escaped = key.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
  return `${props.path}["${escaped}"]`
}

function valueAsString(v: unknown): string {
  if (typeof v === 'string') return v
  if (v === null) return 'null'
  if (v === undefined) return 'undefined'
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

async function copy(text: string, target: 'path' | 'value') {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    return
  }
  if (target === 'path') {
    copiedPath.value = true
    setTimeout(() => (copiedPath.value = false), 1000)
  } else {
    copiedValue.value = true
    setTimeout(() => (copiedValue.value = false), 1000)
  }
}

function toggle() {
  if (!isContainer.value) return
  // Pin dominates manual toggle: while a pin keeps us expanded, ignore clicks.
  if (isOnPinnedPath.value) return
  expanded.value = !expanded.value
}

function togglePin() {
  if (!props.nodeId || !props.path) return
  debugStore.togglePinnedPath(props.nodeId, props.path)
}
</script>

<template>
  <div class="font-mono text-[11px] leading-relaxed">
    <div
      class="group flex items-start gap-1 rounded px-1 -mx-1"
      :class="
        isPinned
          ? 'bg-accent/10 border-l-2 border-accent pl-0.5'
          : 'hover:bg-terminal-surface/40'
      "
    >
      <button
        v-if="isContainer"
        class="shrink-0 w-3 text-terminal-text-dim hover:text-accent transition-colors text-[9px] leading-relaxed text-left"
        :title="effectiveExpanded ? 'Collapse' : 'Expand'"
        @click="toggle"
      >{{ effectiveExpanded ? '▼' : '▶' }}</button>
      <span v-else class="shrink-0 w-3"></span>

      <div class="flex-1 min-w-0 break-all">
        <span
          v-if="rootKey"
          class="text-terminal-text-dim"
          :class="{ 'cursor-pointer': isContainer }"
          @click="toggle"
        >{{ rootKey }}<span class="text-terminal-text-dim/60">: </span></span>

        <template v-if="isContainer">
          <span
            v-if="!effectiveExpanded"
            class="text-terminal-text-dim cursor-pointer"
            @click="toggle"
          >{{ containerSummary }}</span>
          <span
            v-else
            class="text-terminal-text-dim/60"
          >{{ childCount === 0 ? (kind === 'array' ? '[ ]' : '{ }') : '' }}</span>
        </template>

        <template v-else>
          <span v-if="kind === 'string'" class="text-emerald-400">"{{ truncatedString }}"<button
            v-if="stringNeedsTruncate"
            class="ml-1 text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent"
            @click.stop="stringExpanded = !stringExpanded"
          >{{ stringExpanded ? 'less' : 'more' }}</button></span>
          <span v-else-if="kind === 'number'" class="text-amber-400">{{ data }}</span>
          <span v-else-if="kind === 'boolean'" class="text-purple-400">{{ data }}</span>
          <span v-else-if="kind === 'null'" class="text-terminal-text-dim italic">null</span>
          <span v-else-if="kind === 'undefined'" class="text-terminal-text-dim italic">undefined</span>
        </template>
      </div>

      <div
        class="shrink-0 flex items-center gap-0.5 transition-opacity"
        :class="isPinned ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
      >
        <button
          v-if="path"
          class="px-1 text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent rounded"
          title="Copy path"
          @click.stop="copy(path, 'path')"
        >{{ copiedPath ? '✓' : 'path' }}</button>
        <button
          class="px-1 text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent rounded"
          title="Copy value"
          @click.stop="copy(valueAsString(data), 'value')"
        >{{ copiedValue ? '✓' : 'val' }}</button>
        <button
          v-if="nodeId && path"
          class="px-1 text-[9px] uppercase tracking-wider rounded"
          :class="isPinned ? 'text-accent' : 'text-terminal-text-dim hover:text-accent'"
          :title="isPinned ? 'Unpin path (auto-expand off)' : 'Pin path (auto-expand in all messages of this node)'"
          @click.stop="togglePin"
        >{{ isPinned ? '★' : 'pin' }}</button>
      </div>
    </div>

    <div
      v-if="isContainer && effectiveExpanded && childCount > 0"
      class="ml-3 border-l border-terminal-border/40 pl-2"
    >
      <JsonTreeView
        v-for="[key, value] in childEntries"
        :key="String(key)"
        :data="value"
        :path="buildChildPath(key)"
        :root-key="String(key)"
        :depth="depth + 1"
        :default-expand-depth="defaultExpandDepth"
        :node-id="nodeId"
      />
    </div>
  </div>
</template>
