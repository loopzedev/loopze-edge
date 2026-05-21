<script setup lang="ts">
import { computed } from 'vue'
import type { LayoutWidget } from '../types'

const props = defineProps<{
  widget: LayoutWidget
  value?: unknown
}>()

interface StateRule {
  when: unknown
  color?: string
  label?: string
}

const cfg = computed(() => (props.widget.config ?? {}) as Record<string, unknown>)

const states = computed<StateRule[]>(() => {
  const raw = cfg.value.states
  if (!Array.isArray(raw)) return []
  return raw.filter((r) => r && typeof r === 'object') as StateRule[]
})

const offColor = computed(() => {
  const c = cfg.value.offColor
  return typeof c === 'string' && c ? c : '#444'
})
const glow = computed(() => Boolean(cfg.value.glow))

// Pick the first matching rule. The match operator is intentionally
// strict equality on primitives — the spec mentions richer matchers
// ("> 30") as a future feature.
const active = computed<StateRule | null>(() => {
  const v = props.value
  for (const rule of states.value) {
    if (matches(rule.when, v)) return rule
  }
  return null
})

function matches(when: unknown, value: unknown): boolean {
  return when === value
}

const dotColor = computed(() => active.value?.color ?? offColor.value)
const dotLabel = computed(() => {
  if (active.value?.label) return active.value.label
  if (props.widget.label) return props.widget.label
  return props.widget.name || ''
})
</script>

<template>
  <div class="led-widget" :title="widget.tooltip || ''">
    <span
      class="led-dot"
      :class="{ glow }"
      :style="{ background: dotColor, '--dot-color': dotColor }"
    />
    <span v-if="dotLabel" class="led-label">{{ dotLabel }}</span>
  </div>
</template>

<style scoped>
.led-widget {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0;
}
.led-dot {
  width: 0.85rem;
  height: 0.85rem;
  border-radius: 50%;
  flex-shrink: 0;
  background: #444;
}
.led-dot.glow {
  box-shadow: 0 0 6px 1px var(--dot-color, #444);
}
.led-label {
  font-size: 0.9rem;
  color: var(--fg);
}
</style>
