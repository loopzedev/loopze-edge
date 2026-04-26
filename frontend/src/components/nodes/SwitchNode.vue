<script setup lang="ts">
import { computed } from 'vue'
import BaseNode from './BaseNode.vue'

interface Rule {
  t: string
  v?: string
  vt?: string
  v2?: string
  v2t?: string
  case?: boolean
}

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

const rules = computed<Rule[]>(() => {
  const r = props.data.config?.rules
  return Array.isArray(r) ? r : []
})

const ruleCount = computed(() => rules.value.length)

const propertyDisplay = computed(() => {
  const cfg = props.data.config ?? {}
  const scope = cfg.propertyType ?? 'msg'
  const path = cfg.property ?? 'payload'
  return `${scope}.${path}`
})

// Render the right-hand side of a rule for the tooltip. Strings get quoted,
// context references get their scope prefix, env gets a $ marker.
function formatValue(v?: string, vt?: string): string {
  if (v == null || v === '') return ''
  switch (vt) {
    case 'str':              return `"${v}"`
    case 'msg':              return `msg.${v}`
    case 'flow':             return `flow.${v}`
    case 'global':           return `global.${v}`
    case 'env':              return `$${v}`
    default:                 return v
  }
}

const operatorLabels: Record<string, string> = {
  eq: '==',  neq: '!=',
  lt: '<',   lte: '<=', gt: '>', gte: '>=',
  cont: 'contains', regex: 'matches',
  true: 'is true', false: 'is false',
  null: 'is null', nnull: 'is not null',
  empty: 'is empty', nempty: 'is not empty',
}

function ruleLabel(rule: Rule): string {
  const propLabel = propertyDisplay.value
  switch (rule.t) {
    case 'else':
      return 'otherwise'
    case 'btwn': {
      const a = formatValue(rule.v, rule.vt)
      const b = formatValue(rule.v2, rule.v2t)
      return `${propLabel} between ${a} and ${b}`
    }
    case 'istype':
      return `${propLabel} is of type ${rule.v ?? '?'}`
    case 'regex': {
      const flag = rule.case ? '' : 'i'
      return `${propLabel} matches /${rule.v ?? ''}/${flag}`
    }
    case 'cont': {
      const sensitive = rule.case ? '' : ' (case-insensitive)'
      return `${propLabel} contains ${formatValue(rule.v, rule.vt)}${sensitive}`
    }
    case 'true': case 'false': case 'null': case 'nnull':
    case 'empty': case 'nempty':
      return `${propLabel} ${operatorLabels[rule.t]}`
    default: {
      const op = operatorLabels[rule.t] ?? rule.t
      return `${propLabel} ${op} ${formatValue(rule.v, rule.vt)}`
    }
  }
}

const outputLabels = computed(() => rules.value.map(ruleLabel))
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="props.data.label"
    node-type="switch"
    :inputs="props.data.inputs ?? 1"
    :outputs="props.data.outputs ?? ruleCount"
    :output-labels="outputLabels"
    :selected="props.selected"
    :disabled="props.data.disabled"
    :status="props.data.status"
  >
    <template #body>
      <span class="font-mono">{{ propertyDisplay }}</span>
      <span class="text-terminal-text-dim ml-1">· {{ ruleCount }} rule{{ ruleCount !== 1 ? 's' : '' }}</span>
    </template>
  </BaseNode>
</template>
