<script setup lang="ts">
import { computed } from 'vue'
import type { LogEntry, LogLevel } from '@/types/events'

const props = defineProps<{ entry: LogEntry }>()

const levelClass = computed<string>(() => {
  switch (props.entry.level) {
    case 'DEBUG': return 'text-zinc-500'
    case 'INFO':  return 'text-blue-400'
    case 'WARN':  return 'text-yellow-400'
    case 'ERROR': return 'text-red-400'
    default:      return 'text-terminal-text'
  }
})

const formattedTime = computed<string>(() => {
  const d = new Date(props.entry.time)
  if (isNaN(d.getTime())) return props.entry.time
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${h}:${m}:${s}.${ms}`
})

const paddedLevel = computed<string>(() => {
  const lvl: LogLevel = props.entry.level
  return lvl.padEnd(5, ' ')
})

const renderedAttrs = computed<string>(() => {
  if (!props.entry.attrs) return ''
  const parts: string[] = []
  for (const [k, v] of Object.entries(props.entry.attrs)) {
    parts.push(`${k}=${formatAttrValue(v)}`)
  }
  return parts.join(' ')
})

function formatAttrValue(v: unknown): string {
  if (v === null || v === undefined) return String(v)
  let s: string
  if (typeof v === 'string') s = v
  else if (typeof v === 'number' || typeof v === 'boolean') s = String(v)
  else {
    try { s = JSON.stringify(v) } catch { s = String(v) }
  }
  return /\s/.test(s) ? `"${s.replace(/"/g, '\\"')}"` : s
}
</script>

<template>
  <div
    class="whitespace-pre font-mono text-[11px] leading-[1.45] px-3 py-px hover:bg-terminal-surface-alt/40"
  >
    <span class="text-terminal-text-dim tabular-nums">{{ formattedTime }}</span>
    <span class="text-terminal-text-dim"> [</span>
    <span :class="levelClass">{{ paddedLevel }}</span>
    <span class="text-terminal-text-dim">] </span>
    <span class="text-terminal-text">{{ entry.message }}</span>
    <span v-if="renderedAttrs" class="text-terminal-text-dim"> {{ renderedAttrs }}</span>
  </div>
</template>
