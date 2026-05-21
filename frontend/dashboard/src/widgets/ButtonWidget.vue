<script setup lang="ts">
import { computed } from 'vue'
import type { LayoutWidget } from '../types'

const props = defineProps<{
  widget: LayoutWidget
  /** Push an event frame to the WS hub. */
  emitEvent: (id: string, value: unknown) => void
}>()

const label = computed(() => {
  if (props.widget.label) return props.widget.label
  const fromConfig = props.widget.config?.label
  if (typeof fromConfig === 'string' && fromConfig) return fromConfig
  return props.widget.name || 'Button'
})

const color = computed(() => {
  const c = props.widget.config?.color
  return typeof c === 'string' && c ? c : ''
})

function onClick() {
  // Server fills the payload from the configured payloadType + payload.
  // The wire value here is informational only.
  props.emitEvent(props.widget.id, true)
}
</script>

<template>
  <button
    class="loopze-button-widget"
    :style="color ? { background: color } : undefined"
    :title="widget.tooltip || ''"
    @click="onClick"
  >
    {{ label }}
  </button>
</template>

<style scoped>
.loopze-button-widget {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.6rem 1.2rem;
  font: inherit;
  font-weight: 600;
  color: #0e0e08;
  background: var(--accent, #58a6ff);
  border: none;
  border-radius: 4px;
  cursor: pointer;
  min-width: 6rem;
}
.loopze-button-widget:hover {
  filter: brightness(1.1);
}
.loopze-button-widget:active {
  transform: translateY(1px);
}
</style>
