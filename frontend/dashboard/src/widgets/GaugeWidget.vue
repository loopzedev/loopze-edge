<script setup lang="ts">
import { computed } from 'vue'
import type { LayoutWidget } from '../types'

const props = defineProps<{
  widget: LayoutWidget
  value?: unknown
}>()

interface Threshold {
  value: number
  color: string
}

const cfg = computed(() => (props.widget.config ?? {}) as Record<string, unknown>)

const min = computed(() => numFromCfg('min', 0))
const max = computed(() => numFromCfg('max', 100))
const decimals = computed(() => Math.max(0, numFromCfg('decimals', 1)))
const unit = computed(() => {
  const v = cfg.value.unit
  return typeof v === 'string' ? v : ''
})
const showValue = computed(() => {
  const v = cfg.value.showValue
  return typeof v === 'boolean' ? v : true
})
const label = computed(() => {
  if (typeof cfg.value.label === 'string' && cfg.value.label) return cfg.value.label as string
  return props.widget.label || props.widget.name || ''
})

const thresholds = computed<Threshold[]>(() => {
  const raw = cfg.value.thresholds
  if (!Array.isArray(raw)) return []
  return raw
    .map((t) => t as Record<string, unknown>)
    .filter((t) => typeof t?.value === 'number' && typeof t?.color === 'string')
    .map((t) => ({ value: t.value as number, color: t.color as string }))
    .sort((a, b) => a.value - b.value)
})

function numFromCfg(key: string, fallback: number): number {
  const v = cfg.value[key]
  return typeof v === 'number' ? v : fallback
}

// Read raw value (number, {value:N}, or null).
const numericValue = computed<number | null>(() => {
  const v = props.value
  if (typeof v === 'number') return v
  if (v && typeof v === 'object' && 'value' in v) {
    const inner = (v as { value?: unknown }).value
    if (typeof inner === 'number') return inner
  }
  return null
})

const clamped = computed(() => {
  const v = numericValue.value
  if (v === null) return null
  if (v < min.value) return min.value
  if (v > max.value) return max.value
  return v
})

const progress = computed(() => {
  const v = clamped.value
  if (v === null) return 0
  const span = max.value - min.value
  if (span <= 0) return 0
  return (v - min.value) / span
})

// Pick the threshold band whose value the current reading has reached
// (highest band ≤ value). Falls back to the accent color.
const arcColor = computed(() => {
  const v = numericValue.value
  if (v === null) return 'var(--accent, #58a6ff)'
  let chosen: string | null = null
  for (const t of thresholds.value) {
    if (v >= t.value) chosen = t.color
  }
  return chosen ?? 'var(--accent, #58a6ff)'
})

// Geometry: 270° gauge from -135° to +135°.
const SIZE = 160
const STROKE = 14
const CX = SIZE / 2
const CY = SIZE / 2
const R = (SIZE - STROKE) / 2
const START_ANGLE = -135
const SWEEP = 270

function polar(angleDeg: number): { x: number; y: number } {
  const rad = (angleDeg * Math.PI) / 180
  return { x: CX + R * Math.sin(rad), y: CY - R * Math.cos(rad) }
}

const bgPath = computed(() => arcPath(START_ANGLE, SWEEP))
const fgPath = computed(() => arcPath(START_ANGLE, SWEEP * progress.value))

function arcPath(startDeg: number, sweepDeg: number): string {
  if (sweepDeg <= 0) return ''
  const start = polar(startDeg)
  const end = polar(startDeg + sweepDeg)
  const largeArc = sweepDeg > 180 ? 1 : 0
  return `M ${start.x.toFixed(2)} ${start.y.toFixed(2)} A ${R} ${R} 0 ${largeArc} 1 ${end.x.toFixed(2)} ${end.y.toFixed(2)}`
}

const valueText = computed(() => {
  const v = numericValue.value
  if (v === null) return '—'
  return v.toFixed(decimals.value) + (unit.value ? ' ' + unit.value : '')
})
</script>

<template>
  <div class="gauge-widget" :title="widget.tooltip || ''">
    <svg :width="SIZE" :height="SIZE" :viewBox="`0 0 ${SIZE} ${SIZE}`" class="gauge-svg">
      <path :d="bgPath" stroke="var(--border)" :stroke-width="STROKE" fill="none" stroke-linecap="round" />
      <path
        v-if="numericValue !== null"
        :d="fgPath"
        :stroke="arcColor"
        :stroke-width="STROKE"
        fill="none"
        stroke-linecap="round"
      />
    </svg>
    <div class="gauge-readout">
      <div v-if="label" class="gauge-label">{{ label }}</div>
      <div v-if="showValue" class="gauge-value">{{ valueText }}</div>
    </div>
  </div>
</template>

<style scoped>
.gauge-widget {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}
.gauge-svg {
  display: block;
}
.gauge-readout {
  margin-top: -3rem;
  text-align: center;
  pointer-events: none;
}
.gauge-label {
  font-size: 0.75rem;
  color: var(--muted);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.gauge-value {
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--fg);
  font-variant-numeric: tabular-nums;
  margin-top: 0.15rem;
}
</style>
