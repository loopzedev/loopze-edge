// Shared option lists used across multiple node config panels.
// Adding/changing values here propagates everywhere — keep in sync with the
// backend node definitions (e.g. internal/nodes/*.go).

export interface OptionEntry<T = string | number> {
  value: T
  label: string
}

// ── Value types (msg field source) ───────────────────────────────────

export const VALUE_TYPES: OptionEntry[] = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
  { value: 'str', label: 'string' },
  { value: 'num', label: 'number' },
  { value: 'bool', label: 'boolean' },
  { value: 'json', label: 'JSON' },
  { value: 'date', label: 'timestamp' },
  { value: 'env', label: 'env' },
]

export type ValueTypeFamily = 'context' | 'literal' | 'dynamic'

export const VALUE_TYPE_FAMILY: Record<string, ValueTypeFamily> = {
  msg: 'context',
  flow: 'context',
  global: 'context',
  env: 'context',
  str: 'literal',
  num: 'literal',
  bool: 'literal',
  json: 'literal',
  date: 'dynamic',
}

export const TIMESTAMP_FORMATS: OptionEntry[] = [
  { value: 'epoch', label: 'milliseconds since epoch' },
  { value: 'rfc3339', label: 'YYYY-MM-DDTHH:mm:ss.sssZ' },
]

// ── Storage scope (for flow./global. context references) ─────────────

export const STORAGE_TYPES: OptionEntry[] = [
  { value: 'memory', label: 'memory' },
  { value: 'persistent', label: 'persist' },
]

export function isContextScope(vt: string): boolean {
  return vt === 'flow' || vt === 'global'
}

// ── Interval presets (Inject) ────────────────────────────────────────

export const INTERVAL_PRESETS: OptionEntry<number>[] = [
  { value: 0, label: 'None' },
  { value: 100, label: '100ms' },
  { value: 500, label: '500ms' },
  { value: 1000, label: '1s' },
  { value: 5000, label: '5s' },
  { value: 10000, label: '10s' },
  { value: 30000, label: '30s' },
  { value: 60000, label: '1min' },
]

// ── MQTT QoS levels ──────────────────────────────────────────────────

export const QOS_LEVELS: OptionEntry<number>[] = [
  { value: 0, label: '0 — at most once' },
  { value: 1, label: '1 — at least once' },
  { value: 2, label: '2 — exactly once' },
]

// ── Placeholder per value type ───────────────────────────────────────

export function placeholderFor(vt: string): string {
  switch (vt) {
    case 'json': return '{...}'
    case 'bool': return 'true / false'
    case 'env': return 'ENV_VAR_NAME'
    case 'msg': return 'property path'
    case 'flow':
    case 'global': return 'key'
    default: return 'value'
  }
}
