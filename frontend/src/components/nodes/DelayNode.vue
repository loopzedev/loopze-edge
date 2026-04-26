<script setup lang="ts">
import { computed } from 'vue'
import BaseNode from './BaseNode.vue'

interface Props {
  id: string
  data: {
    label?: string
    nodeType?: string
    config?: Record<string, any>
    status?: any
    inputs?: number
    outputs?: number
    disabled?: boolean
  }
  selected?: boolean
}

const props = withDefaults(defineProps<Props>(), { selected: false })

defineOptions({ inheritAttrs: false })

// Compact body text — mirrors getNodeSummary but is intentionally shorter so
// it fits the node body. Format: "delay 500ms" / "10/sec drop" / "rand 0–2s".
const bodyText = computed(() => {
  const cfg = props.data.config ?? {}
  const mode = (cfg.mode as string) ?? 'delay'

  if (mode === 'rate') {
    const rate = Number(cfg.rate ?? 1)
    const unit = (cfg.rateUnits as string) ?? 'second'
    const tail = cfg.behaviour === 'drop' ? ' drop' : ''
    return `${rate}/${shortUnit(unit)}${tail}`
  }
  if (mode === 'random') {
    const a = Number(cfg.randomFirst ?? 0)
    const b = Number(cfg.randomLast ?? 0)
    const unit = (cfg.randomUnits as string) ?? 'milliseconds'
    return `rand ${a}–${b}${shortUnit(unit)}`
  }
  const t = Number(cfg.timeout ?? 0)
  const unit = (cfg.timeoutUnits as string) ?? 'milliseconds'
  return `delay ${t}${shortUnit(unit)}`
})

function shortUnit(u: string): string {
  switch (u) {
    case 'milliseconds': return 'ms'
    case 'seconds': case 'second': return 's'
    case 'minutes': case 'minute': return 'min'
    case 'hours': case 'hour': return 'h'
    case 'day': return 'd'
    default: return u
  }
}
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="delay"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? 1"
    :selected="props.selected"
    :disabled="props.data.disabled"
    :status="props.data.status"
  >
    <template #body>
      <span class="truncate">{{ bodyText }}</span>
    </template>
  </BaseNode>
</template>
