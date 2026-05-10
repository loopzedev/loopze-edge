// Cross-group option lists used by shared config helpers (MsgFieldEditor,
// ValueTypeInput, InjectConfig, ChangeConfig, SwitchConfig, TemplateConfig).
//
// Group-specific enums live with their owning group:
//   - frontend/src/nodes/s7/enums.ts     — S7 data types, address patterns
//   - frontend/src/nodes/modbus/enums.ts — Modbus function codes, byte/word order
//   - frontend/src/nodes/mqtt/enums.ts   — MQTT QoS, MQTT v5 properties
// New per-group enums must NOT land here.

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
  { value: 'expr', label: 'expr' },
]

export type ValueTypeFamily = 'context' | 'literal' | 'dynamic' | 'expression'

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
  expr: 'expression',
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

// ── Placeholder per value type ───────────────────────────────────────

export function placeholderFor(vt: string): string {
  switch (vt) {
    case 'json': return '{...}'
    case 'bool': return 'true / false'
    case 'env': return 'ENV_VAR_NAME'
    case 'msg': return 'property path'
    case 'flow':
    case 'global': return 'key'
    case 'expr': return 'payload * 2'
    default: return 'value'
  }
}
