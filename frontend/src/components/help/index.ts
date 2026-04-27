import type { Component } from 'vue'
import InjectHelp from './InjectHelp.vue'

export { getNodeHelpDoc } from './docs'
export type { NodeHelpDoc } from './types'

export const nodeHelpComponents: Record<string, Component> = {
  inject: InjectHelp,
}

export function getNodeHelp(nodeType: string | undefined | null): Component | null {
  if (!nodeType) return null
  return nodeHelpComponents[nodeType] ?? null
}

// ── Live summaries ───────────────────────────────────────────────────
// Pure functions that translate a node's current config into a one-liner
// shown in the property panel header. Each summary should stay short and
// describe the *behaviour* (not just echo settings).

type ConfigRecord = Record<string, unknown>

export type NodeSummaryFn = (config: ConfigRecord) => string

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms % 1000 === 0 ? 0 : 1)}s`
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(ms % 60_000 === 0 ? 0 : 1)}min`
  return `${(ms / 3_600_000).toFixed(1)}h`
}

function pluralize(n: number, singular: string, plural?: string): string {
  return `${n} ${n === 1 ? singular : plural ?? singular + 's'}`
}

const summaries: Record<string, NodeSummaryFn> = {
  inject(cfg) {
    const parts: string[] = []
    const once = !!cfg.once
    const interval = Number(cfg.interval ?? 0)
    if (once && interval > 0) parts.push(`Fires once + every ${formatDuration(interval)}`)
    else if (once) parts.push('Fires once at startup')
    else if (interval > 0) parts.push(`Fires every ${formatDuration(interval)}`)
    else parts.push('Manual trigger only')

    const fieldCount = Array.isArray(cfg.props) ? cfg.props.length : 0
    if (fieldCount > 0) parts.push(pluralize(fieldCount, 'field'))
    return parts.join(' · ')
  },

  change(cfg) {
    const rules = Array.isArray(cfg.rules) ? cfg.rules : []
    if (rules.length === 0) return 'No rules configured'
    const ops = rules.reduce<Record<string, number>>((acc, r: any) => {
      const t = (r?.t as string) ?? 'set'
      acc[t] = (acc[t] ?? 0) + 1
      return acc
    }, {})
    const opSummary = Object.entries(ops)
      .map(([op, n]) => `${n}× ${op}`)
      .join(', ')
    return `${pluralize(rules.length, 'rule')} · ${opSummary}`
  },

  debug(cfg) {
    const out = (cfg.output as string) ?? 'property'
    const map: Record<string, string> = {
      property: `msg.${(cfg.property as string) || 'payload'}`,
      message: 'complete message',
      gjson: `gjson: ${(cfg.property as string) || '—'}`,
    }
    const main = map[out] ?? out
    return cfg.statusEnabled ? `${main} · status enabled` : main
  },

  function(cfg) {
    const outputs = Number(cfg.outputs ?? 1)
    return pluralize(outputs, 'output port')
  },

  'function-expr'(cfg) {
    const expr = String(cfg.expression ?? '').trim()
    const out = String(cfg.outputProperty ?? 'payload')
    const through = cfg.passThrough ? ' · pass-through' : ''
    if (!expr) return `→ msg.${out}${through}`
    const preview = expr.length > 32 ? expr.slice(0, 32) + '…' : expr
    return `${preview} → msg.${out}${through}`
  },

  'function-go'(cfg) {
    const outputs = Number(cfg.outputs ?? 1)
    const code = String(cfg.code ?? '')
    const sig = code.split('\n').map(l => l.trim()).find(l => l.startsWith('func handle'))
    const ports = pluralize(outputs, 'output port')
    if (!sig) return ports
    const trimmed = sig.length > 40 ? sig.slice(0, 40) + '…' : sig
    return `${trimmed} · ${ports}`
  },

  'link-in'(cfg) {
    const links = Array.isArray(cfg.links) ? cfg.links.length : 0
    return links === 0 ? 'No targets linked' : `${pluralize(links, 'target')} linked`
  },

  'link-out'(cfg) {
    const links = Array.isArray(cfg.links) ? cfg.links.length : 0
    return links === 0 ? 'No targets linked' : `${pluralize(links, 'target')} linked`
  },

  'link-call'(cfg) {
    return cfg.linkTarget ? '1 target selected' : 'No target selected'
  },

  'mqtt-in'(cfg) {
    const qos = Number(cfg.qos ?? 0)
    if (cfg.mode === 'dynamic') return `Dynamic · QoS ${qos}`
    const topic = (cfg.topic as string) || '—'
    return `Subscribed to ${topic} · QoS ${qos}`
  },

  'mqtt-out'(cfg) {
    const topic = (cfg.topic as string) || 'msg.topic'
    const qos = Number(cfg.qos ?? 0)
    const retain = !!cfg.retain
    return `Publish to ${topic} · QoS ${qos}${retain ? ' · retain' : ''}`
  },

  statemachine(cfg) {
    let states = 0
    try {
      const machine = JSON.parse((cfg.machine as string) ?? '{}')
      states = machine?.states ? Object.keys(machine.states).length : 0
    } catch { /* ignore */ }
    const persist = cfg.persist ? ' · persistent' : ''
    return states > 0 ? `${pluralize(states, 'state')}${persist}` : 'No machine defined'
  },

  'context-watch'(cfg) {
    const scope = (cfg.scope as string) ?? 'global'
    const storage = (cfg.storage as string) ?? 'memory'
    const pattern = (cfg.keyPattern as string) || '>'
    return `${scope}.${pattern} · ${storage}`
  },

  json(cfg) {
    const property = (cfg.property as string) || 'payload'
    const action = (cfg.action as string) || 'auto'
    const indent = Number(cfg.indent ?? 0)
    if (action === 'parse') return `parse msg.${property}`
    if (action === 'stringify') {
      return `stringify msg.${property}${indent > 0 ? ` (indent ${indent})` : ''}`
    }
    return `auto-detect on msg.${property}`
  },

  delay(cfg) {
    const mode = (cfg.mode as string) ?? 'delay'
    const fmt = (n: number, unit: string) => `${n}${unit === 'milliseconds' ? 'ms' : unit === 'day' ? 'd' : unit[0]}`

    if (mode === 'rate') {
      const rate = Number(cfg.rate ?? 1)
      const unit = (cfg.rateUnits as string) ?? 'second'
      const behaviour = (cfg.behaviour as string) ?? 'queue'
      const max = Number(cfg.maxQueueLength ?? 1000)
      const tail = behaviour === 'drop' ? '· drop' : `· queue (max ${max})`
      return `${rate} msg/${unit} ${tail}`
    }
    if (mode === 'random') {
      const a = Number(cfg.randomFirst ?? 0)
      const b = Number(cfg.randomLast ?? 0)
      const unit = (cfg.randomUnits as string) ?? 'milliseconds'
      return `Random ${fmt(a, unit)}–${fmt(b, unit)}`
    }
    // mode === 'delay'
    const t = Number(cfg.timeout ?? 0)
    const unit = (cfg.timeoutUnits as string) ?? 'milliseconds'
    return `Delay ${fmt(t, unit)}`
  },
}

export function getNodeSummary(
  nodeType: string | undefined | null,
  config: unknown,
): string | null {
  if (!nodeType) return null
  const fn = summaries[nodeType]
  if (!fn) return null
  return fn((config ?? {}) as ConfigRecord)
}
