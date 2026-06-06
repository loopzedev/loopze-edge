<script setup lang="ts">
import { computed } from 'vue'
import type { LayoutWidget, Sample } from '../types'

const props = defineProps<{
  widget: LayoutWidget
  value?: unknown
  samples?: Sample[]
}>()

const cfg = computed(() => (props.widget.config ?? {}) as Record<string, unknown>)

// ─── Label / Sublabel ─────────────────────────────────────────────────────────
const label    = computed(() => stringCfg('label', ''))
const sublabel = computed(() => stringCfg('sublabel', ''))
const labelSep = computed(() => stringCfg('labelSeparator', '·'))
const hasLabel = computed(() => label.value !== '' || sublabel.value !== '')

// ─── Value extraction ─────────────────────────────────────────────────────────
// Hub payload is {value, delta?}. Older messages or out-of-band sources may
// deliver a bare number — handle both shapes defensively.
const numericValue = computed<number | null>(() => {
  const v = props.value
  if (typeof v === 'number') return v
  if (v && typeof v === 'object' && 'value' in v) {
    const inner = (v as { value?: unknown }).value
    if (typeof inner === 'number') return inner
  }
  return null
})

const deltaValue = computed<number | null>(() => {
  const v = props.value
  if (v && typeof v === 'object' && 'delta' in v) {
    const d = (v as { delta?: unknown }).delta
    if (typeof d === 'number') return d
  }
  return null
})

// ─── Formatting ───────────────────────────────────────────────────────────────
const decimals     = computed(() => Math.max(0, numCfg('decimals', 0)))
const thousandsSep = computed(() => stringCfg('thousandsSeparator', 'space'))
const decimalSep   = computed(() => stringCfg('decimalSeparator', 'dot'))
const prefix       = computed(() => stringCfg('prefix', ''))
const suffix       = computed(() => stringCfg('suffix', ''))
const valueColor   = computed(() => stringCfg('valueColor', ''))

const SEP_MAP: Record<string, string> = { space: ' ', comma: ',', dot: '.', none: '' }

function formatNumber(value: number, dec: number, tSep: string, dSep: string): string {
  const fixed = value.toFixed(dec)
  const [intPart, decPart] = fixed.split('.')
  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, SEP_MAP[tSep] ?? ' ')
  return decPart ? grouped + (SEP_MAP[dSep] ?? '.') + decPart : grouped
}

const hasValue = computed(() => numericValue.value !== null)

const formattedValue = computed(() => {
  if (numericValue.value === null) return 'no data'
  return prefix.value + formatNumber(numericValue.value, decimals.value, thousandsSep.value, decimalSep.value) + suffix.value
})

// ─── Delta ────────────────────────────────────────────────────────────────────
const showDelta       = computed(() => boolCfg('showDelta', true))
const deltaFormat     = computed(() => stringCfg('deltaFormat', 'percent'))
const deltaDecimals   = computed(() => Math.max(0, numCfg('deltaDecimals', 1)))
const deltaContext    = computed(() => stringCfg('deltaContext', ''))
const deltaDirection  = computed(() => stringCfg('deltaDirection', 'up-is-good'))

const renderDelta = computed(() => showDelta.value && deltaValue.value !== null)

const STATUS_RUNNING = '#3fb950'
const STATUS_FAULT   = '#f85149'

const deltaColor = computed(() => {
  const d = deltaValue.value
  if (d === null || d === 0 || deltaDirection.value === 'neutral') return 'var(--muted)'
  const isPositive = d > 0
  const isGood = deltaDirection.value === 'up-is-good' ? isPositive : !isPositive
  return isGood ? STATUS_RUNNING : STATUS_FAULT
})

const formattedDelta = computed(() => {
  const d = deltaValue.value
  if (d === null) return ''
  const sign = d > 0 ? '+' : '' // negative already carries the minus
  const num  = d.toFixed(deltaDecimals.value)
  const suf  = deltaFormat.value === 'absolute' ? '' : '%'
  return `${sign}${num}${suf}`
})

// ─── Sparkline ────────────────────────────────────────────────────────────────
const showSparkline  = computed(() => boolCfg('showSparkline', true))
const sparklineFill  = computed(() => boolCfg('sparklineFill', true))
const sparklineColor = computed(() => stringCfg('sparklineColor', ''))

// Effective sparkline color: explicit config → delta color → accent.
const effectiveSparklineColor = computed(() => {
  if (sparklineColor.value) return sparklineColor.value
  // Match delta tint when present and non-neutral. Renders the same
  // "rising and bad → red across all elements" effect described in L-8.
  if (deltaValue.value !== null && deltaValue.value !== 0 &&
      deltaDirection.value !== 'neutral') {
    return deltaColor.value
  }
  return 'var(--accent, #58a6ff)'
})

const renderSparkline = computed(() =>
  showSparkline.value && Array.isArray(props.samples) && props.samples.length >= 2,
)

// SVG geometry — fixed viewBox; preserveAspectRatio="none" stretches it
// to the actual rendered size of the container.
const SVG_W = 200
const SVG_H = 60

interface SparklinePaths {
  line: string
  area: string
}

const sparklinePaths = computed<SparklinePaths>(() => {
  const pts = props.samples ?? []
  if (pts.length < 2) return { line: '', area: '' }

  const values = pts.map((p) => p.v)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1 // avoid zero-division on flat data

  const stepX = SVG_W / (pts.length - 1)
  // Add 2 px padding top/bottom so peaks don't touch the edge.
  const padY = 2
  const usableH = SVG_H - padY * 2

  const coords = pts.map((p, i) => {
    const x = i * stepX
    const y = padY + (1 - (p.v - min) / span) * usableH
    return [x, y] as const
  })

  const line = coords
    .map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`)
    .join(' ')

  // Area: close the path down to the bottom edge of the viewBox.
  const area = `${line} L${SVG_W},${SVG_H} L0,${SVG_H} Z`

  return { line, area }
})

// Unique gradient id per widget instance so multiple stat widgets on
// the same page don't share fills (SVG ids are document-global).
const gradientId = computed(() => `stat-grad-${props.widget.id}`)

// ─── Helpers ──────────────────────────────────────────────────────────────────
function stringCfg(key: string, def: string): string {
  const v = cfg.value[key]
  return typeof v === 'string' ? v : def
}
function numCfg(key: string, def: number): number {
  const v = cfg.value[key]
  return typeof v === 'number' ? v : def
}
function boolCfg(key: string, def: boolean): boolean {
  const v = cfg.value[key]
  return typeof v === 'boolean' ? v : def
}
</script>

<template>
  <div class="stat-widget" :title="widget.tooltip || ''">
    <div v-if="hasLabel" class="stat-label">
      <span v-if="label">{{ label }}</span>
      <span v-if="label && sublabel" class="stat-label-sep">{{ labelSep }}</span>
      <span v-if="sublabel">{{ sublabel }}</span>
    </div>
    <div
      class="stat-value"
      :class="{ 'stat-value-empty': !hasValue }"
      :style="hasValue && valueColor ? { color: valueColor } : undefined"
    >
      {{ formattedValue }}
    </div>
    <div v-if="renderDelta" class="stat-delta">
      <span class="stat-delta-value" :style="{ color: deltaColor }">{{ formattedDelta }}</span>
      <span v-if="deltaContext" class="stat-delta-context">{{ deltaContext }}</span>
    </div>
    <svg
      v-if="renderSparkline"
      class="stat-sparkline"
      :viewBox="`0 0 ${SVG_W} ${SVG_H}`"
      preserveAspectRatio="none"
    >
      <defs>
        <linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%"   :stop-color="effectiveSparklineColor" stop-opacity="0.35" />
          <stop offset="100%" :stop-color="effectiveSparklineColor" stop-opacity="0" />
        </linearGradient>
      </defs>
      <path
        v-if="sparklineFill"
        :d="sparklinePaths.area"
        :fill="`url(#${gradientId})`"
      />
      <path
        :d="sparklinePaths.line"
        fill="none"
        :stroke="effectiveSparklineColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        vector-effect="non-scaling-stroke"
      />
    </svg>
  </div>
</template>

<style scoped>
.stat-widget {
  display: flex;
  flex-direction: column;
  justify-content: center;
  width: 100%;
  height: 100%;
  min-width: 0;
  gap: 0.3rem;
  padding: 0.25rem 0.1rem;
}

.stat-label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--muted);
  font-family: 'IBM Plex Mono', monospace;
}
.stat-label-sep {
  opacity: 0.5;
}

.stat-value {
  font-size: clamp(2rem, 5vw, 3.5rem);
  font-weight: 700;
  letter-spacing: -0.03em;
  color: #ffffff;
  font-variant-numeric: tabular-nums;
  line-height: 1.05;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* Empty-state: subdued, small, italic — so an unconnected widget
   doesn't render as a giant white horizontal bar (the "—" glyph at
   3.5rem looks like a stripe). */
.stat-value-empty {
  font-size: 0.85rem;
  font-weight: 500;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--muted);
  font-style: italic;
  font-family: 'IBM Plex Mono', monospace;
}

.stat-delta {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  font-family: 'IBM Plex Mono', monospace;
  margin-top: -0.1rem;
}
.stat-delta-value {
  font-size: 0.85rem;
  font-weight: 700;
  letter-spacing: 0.04em;
}
.stat-delta-context {
  font-size: 0.65rem;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--muted);
}

.stat-sparkline {
  width: 100%;
  /* Take a fixed share of the widget's content area, like the reference
     design — keeps the big number prominent regardless of cell aspect. */
  height: 40%;
  min-height: 28px;
  margin-top: auto;
  display: block;
}
</style>
