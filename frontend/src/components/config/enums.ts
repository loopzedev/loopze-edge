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

// ── MQTT QoS levels ──────────────────────────────────────────────────

export const QOS_LEVELS: OptionEntry<number>[] = [
  { value: 0, label: '0 — at most once' },
  { value: 1, label: '1 — at least once' },
  { value: 2, label: '2 — exactly once' },
]

// ── MQTT v5 Retain Handling (subscribe option) ───────────────────────

export const RETAIN_HANDLING_OPTIONS: OptionEntry<number>[] = [
  { value: 0, label: '0 — send retained at every subscribe' },
  { value: 1, label: '1 — send retained only on new subscription' },
  { value: 2, label: '2 — never send retained' },
]

// ── MQTT v5 Payload Format Indicator (publish property) ──────────────

export const PAYLOAD_FORMAT_OPTIONS: OptionEntry<number>[] = [
  { value: 0, label: '0 — bytes / unspecified' },
  { value: 1, label: '1 — UTF-8 text' },
]

// ── mqtt-in output payload format ────────────────────────────────────

export const MQTT_IN_OUTPUT_FORMATS: OptionEntry<string>[] = [
  { value: 'string', label: 'String' },
  { value: 'json',   label: 'JSON (parsed)' },
  { value: 'buffer', label: 'Buffer (raw bytes)' },
]

// ── Modbus function codes ────────────────────────────────────────────

export const MODBUS_READ_FCS: OptionEntry<number>[] = [
  { value: 1, label: 'FC1 — Read Coils' },
  { value: 2, label: 'FC2 — Read Discrete Inputs' },
  { value: 3, label: 'FC3 — Read Holding Registers' },
  { value: 4, label: 'FC4 — Read Input Registers' },
]

export const MODBUS_WRITE_FCS: OptionEntry<number>[] = [
  { value: 5, label: 'FC5 — Write Single Coil' },
  { value: 6, label: 'FC6 — Write Single Register' },
  { value: 15, label: 'FC15 — Write Multiple Coils' },
  { value: 16, label: 'FC16 — Write Multiple Registers' },
]

// FC values that operate on coils (bool) rather than 16-bit registers.
export const MODBUS_COIL_FCS = new Set([1, 2, 5, 15])

export const MODBUS_DATA_TYPES: OptionEntry<string>[] = [
  { value: 'raw',     label: 'Raw (registers / coils)' },
  { value: 'bool',    label: 'Bool (coil only)' },
  { value: 'int16',   label: 'Int16 (1 reg)' },
  { value: 'uint16',  label: 'UInt16 (1 reg)' },
  { value: 'int32',   label: 'Int32 (2 regs)' },
  { value: 'uint32',  label: 'UInt32 (2 regs)' },
  { value: 'float32', label: 'Float32 (2 regs)' },
  { value: 'int64',   label: 'Int64 (4 regs)' },
  { value: 'uint64',  label: 'UInt64 (4 regs)' },
  { value: 'float64', label: 'Float64 (4 regs)' },
  { value: 'string',  label: 'String (n regs)' },
]

export const MODBUS_BYTE_ORDERS: OptionEntry<string>[] = [
  { value: 'bigEndian',    label: 'Big Endian (default)' },
  { value: 'littleEndian', label: 'Little Endian' },
]

export const MODBUS_WORD_ORDERS: OptionEntry<string>[] = [
  { value: 'bigEndian',    label: 'Big Endian (ABCD, default)' },
  { value: 'littleEndian', label: 'Little Endian (CDAB)' },
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
