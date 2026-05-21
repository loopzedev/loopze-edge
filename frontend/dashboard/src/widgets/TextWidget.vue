<script setup lang="ts">
import { computed } from 'vue'
import type { LayoutWidget } from '../types'

const props = defineProps<{
  widget: LayoutWidget
  value?: unknown
}>()

const cfg = computed(() => (props.widget.config ?? {}) as Record<string, unknown>)
const label = computed(() => {
  const c = cfg.value
  if (typeof c.label === 'string' && c.label) return c.label
  if (props.widget.label) return props.widget.label
  return props.widget.name || ''
})

const layout = computed(() => {
  const v = cfg.value.layout
  return typeof v === 'string' ? v : 'row-spread'
})
const format = computed(() => {
  const v = cfg.value.format
  return typeof v === 'string' ? v : 'text'
})
const decimals = computed(() => {
  const v = cfg.value.decimals
  return typeof v === 'number' ? v : 2
})
const unit = computed(() => {
  const v = cfg.value.unit
  return typeof v === 'string' ? v : ''
})
const color = computed(() => {
  const v = cfg.value.color
  return typeof v === 'string' && v ? v : ''
})

const formattedValue = computed(() => {
  const v = props.value
  if (v === undefined || v === null) return '—'
  switch (format.value) {
    case 'number':
      if (typeof v === 'number') return v.toFixed(decimals.value) + unit.value
      return String(v) + unit.value
    case 'json':
      try {
        return JSON.stringify(v, null, 2)
      } catch {
        return String(v)
      }
    default:
      return String(v) + (unit.value ? ' ' + unit.value : '')
  }
})

const containerClass = computed(() => `text-widget layout-${layout.value}`)
</script>

<template>
  <div :class="containerClass" :title="widget.tooltip || ''">
    <span v-if="label" class="text-label">{{ label }}</span>
    <span class="text-value" :style="color ? { color } : undefined">
      <template v-if="format === 'json'">
        <pre>{{ formattedValue }}</pre>
      </template>
      <template v-else>{{ formattedValue }}</template>
    </span>
  </div>
</template>

<style scoped>
.text-widget {
  display: flex;
  width: 100%;
  align-items: baseline;
  gap: 0.75rem;
  padding: 0.25rem 0;
}
.layout-row-spread { justify-content: space-between; }
.layout-row-left { justify-content: flex-start; }
.layout-row-right { justify-content: flex-end; flex-direction: row-reverse; }
.layout-row-center { justify-content: center; }
.layout-col-center {
  flex-direction: column;
  align-items: center;
  text-align: center;
}
.text-label {
  font-size: 0.8rem;
  color: var(--muted);
}
.text-value {
  font-size: 1rem;
  font-weight: 600;
  color: var(--fg);
  font-variant-numeric: tabular-nums;
}
.text-value pre {
  margin: 0;
  font-size: 0.75rem;
  background: rgba(0, 0, 0, 0.3);
  padding: 0.5rem;
  border-radius: 3px;
  max-height: 12rem;
  overflow: auto;
}
</style>
