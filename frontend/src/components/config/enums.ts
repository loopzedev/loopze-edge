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

// ── mqtt-out publish target ──────────────────────────────────────────

export const MQTT_OUT_TARGETS: OptionEntry<string>[] = [
  { value: 'topic',         label: 'Topic' },
  { value: 'responseTopic', label: 'Response to responseTopic' },
]

// ── mqtt-request timeout mode ────────────────────────────────────────

export const MQTT_REQUEST_TIMEOUT_MODES: OptionEntry<string>[] = [
  { value: 'error',       label: 'Error (catchable)' },
  { value: 'passthrough', label: 'Passthrough (msg.timedOut=true)' },
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

// ── SIEMENS S7 ─────────────────────────────────────────────────────────────

// Connection-type presets the user picks in the S7 PLC config. The backend
// derives rack / slot / TSAP from this enum (s7ConnectionDefaults in
// internal/nodes/s7_plc.go).
export const S7_CONNECTION_TYPES: OptionEntry<string>[] = [
  { value: 's7-1200-1500', label: 'S7-1200 / S7-1500 (rack 0, slot 1)' },
  { value: 's7-300-400',   label: 'S7-300 / S7-400 (rack 0, slot 2)' },
  { value: 'logo',         label: 'LOGO! / S7-200 Smart' },
  { value: 'custom',       label: 'Custom (manual rack / slot)' },
]

// S7 data types the codec understands. `bool` requires a bit-addressable form
// (M0.0, DB.DBX, …); `string` requires the DB.STRING<byte>.<maxLen> form.
// See internal/nodes/s7_codec.go for the wire mapping.
export const S7_DATA_TYPES: OptionEntry<string>[] = [
  { value: 'bool',    label: 'BOOL (1 bit)' },
  { value: 'byte',    label: 'BYTE (1 byte)' },
  { value: 'sint',    label: 'SINT (1 byte, signed)' },
  { value: 'usint',   label: 'USINT (1 byte, unsigned)' },
  { value: 'char',    label: 'CHAR (1 byte ASCII)' },
  { value: 'word',    label: 'WORD (2 bytes, unsigned)' },
  { value: 'int',     label: 'INT (2 bytes, signed)' },
  { value: 'uint',    label: 'UINT (2 bytes, unsigned)' },
  { value: 'wchar',   label: 'WCHAR (2 bytes, UCS-2 char)' },
  { value: 'date',    label: 'DATE (2 bytes, days since 1990-01-01)' },
  { value: 'dword',   label: 'DWORD (4 bytes, unsigned)' },
  { value: 'dint',    label: 'DINT (4 bytes, signed)' },
  { value: 'udint',   label: 'UDINT (4 bytes, unsigned)' },
  { value: 'real',    label: 'REAL (4 bytes, IEEE 754)' },
  { value: 'time',    label: 'TIME (4 bytes, signed ms duration)' },
  { value: 'tod',     label: 'TOD (4 bytes, ms since midnight)' },
  { value: 'lreal',   label: 'LREAL (8 bytes, IEEE 754 double)' },
  { value: 'lint',    label: 'LINT (8 bytes, signed)' },
  { value: 'ulint',   label: 'ULINT (8 bytes, unsigned)' },
  { value: 'lword',   label: 'LWORD (8 bytes, bitfield)' },
  { value: 'ltime',   label: 'LTIME (8 bytes, signed ns duration)' },
  { value: 'ltod',    label: 'LTOD (8 bytes, ns since midnight)' },
  { value: 'ldt',     label: 'LDT (8 bytes, ns since 1970-01-01 UTC)' },
  { value: 'dt',      label: 'DT (8 bytes, BCD date+time)' },
  { value: 'dtl',     label: 'DTL (12 bytes, structured date+time)' },
  { value: 'string',  label: 'STRING (n+2 bytes, ASCII)' },
  { value: 'wstring', label: 'WSTRING (4+2n bytes, UCS-2)' },
  { value: 'counter', label: 'COUNTER (BCD)' },
  { value: 'timer',   label: 'TIMER (S5Time)' },
]

// Areas valid in `s7-read`/`s7-write` block mode. The PE area is read-only
// (writes are rejected at the manager level).
export const S7_BLOCK_AREAS: OptionEntry<string>[] = [
  { value: 'DB', label: 'DB (Data Block)' },
  { value: 'M',  label: 'M (Merker / Flags)' },
  { value: 'I',  label: 'I (Inputs / PE)' },
  { value: 'Q',  label: 'Q (Outputs / PA)' },
]

// Same as S7_BLOCK_AREAS but without 'I' — used by the s7-write block-mode
// area dropdown.
export const S7_WRITE_BLOCK_AREAS: OptionEntry<string>[] = [
  { value: 'DB', label: 'DB (Data Block)' },
  { value: 'M',  label: 'M (Merker / Flags)' },
  { value: 'Q',  label: 'Q (Outputs / PA)' },
]

// Output-shape options for s7-read in variables mode.
export const S7_OUTPUT_SHAPES: OptionEntry<string>[] = [
  { value: 'single', label: 'Single (1 variable only)' },
  { value: 'array',  label: 'Array (one msg with all values)' },
  { value: 'object', label: 'Object (name → value map, default >1 var)' },
]

// Value-source options per variable in s7-write. `static` bakes the value
// into the config; `msg` pulls it from `msg.<valuePath>` at write time.
export const S7_VALUE_SOURCES: OptionEntry<string>[] = [
  { value: 'msg',    label: 'From message (msg.<path>)' },
  { value: 'static', label: 'Static (baked-in value)' },
]

// S7 address validator — see internal/nodes/s7_address.go for the canonical
// regex set. Kept in sync via comment cross-reference: any addition here must
// land in the Go parser too (and vice versa). The matcher is best-effort
// validation for user feedback; the Go side does authoritative parsing.
// Type-set constants — mirror s7_address.go (s7ByteTypes, s7WordTypes,
// s7DWordTypes, s7LongTypes). Any addition here must land in the Go parser
// too (and vice versa).
const S7_BYTE_TYPES  = ['byte', 'char', 'sint', 'usint']
const S7_WORD_TYPES  = ['word', 'int', 'uint', 'wchar', 'date']
const S7_DWORD_TYPES = ['dword', 'dint', 'udint', 'real', 'time', 'tod']
const S7_LONG_TYPES  = ['lreal', 'lint', 'ulint', 'lword', 'ltime', 'ltod', 'ldt', 'dt']

export const S7_ADDRESS_PATTERNS: { re: RegExp; types: string[]; hint: string }[] = [
  { re: /^DB\d+\.DBX\d+\.[0-7]$/i,         types: ['bool'],                          hint: 'DB bit (DBX)' },
  { re: /^DB\d+\.DBB\d+$/i,                types: S7_BYTE_TYPES,                     hint: 'DB byte (DBB)' },
  { re: /^DB\d+\.DBW\d+$/i,                types: S7_WORD_TYPES,                     hint: 'DB word (DBW)' },
  { re: /^DB\d+\.DBD\d+$/i,                types: S7_DWORD_TYPES,                    hint: 'DB dword (DBD)' },
  { re: /^DB\d+\.DBL\d+$/i,                types: S7_LONG_TYPES,                     hint: 'DB long (DBL, 8 bytes)' },
  { re: /^DB\d+\.DTL\d+$/i,                types: ['dtl'],                           hint: 'DB DTL (12 bytes structured date+time)' },
  { re: /^DB\d+\.STRING\d+\.\d+$/i,        types: ['string'],                        hint: 'DB string' },
  { re: /^DB\d+\.WSTRING\d+\.\d+$/i,       types: ['wstring'],                       hint: 'DB wide string' },
  { re: /^M\d+\.[0-7]$/i,                  types: ['bool'],                          hint: 'Merker bit' },
  { re: /^MB\d+$/i,                        types: S7_BYTE_TYPES,                     hint: 'Merker byte' },
  { re: /^MW\d+$/i,                        types: S7_WORD_TYPES,                     hint: 'Merker word' },
  { re: /^MD\d+$/i,                        types: S7_DWORD_TYPES,                    hint: 'Merker dword' },
  { re: /^I\d+\.[0-7]$/i,                  types: ['bool'],                          hint: 'Input bit' },
  { re: /^IB\d+$/i,                        types: S7_BYTE_TYPES,                     hint: 'Input byte' },
  { re: /^IW\d+$/i,                        types: S7_WORD_TYPES,                     hint: 'Input word' },
  { re: /^ID\d+$/i,                        types: S7_DWORD_TYPES,                    hint: 'Input dword' },
  { re: /^Q\d+\.[0-7]$/i,                  types: ['bool'],                          hint: 'Output bit' },
  { re: /^QB\d+$/i,                        types: S7_BYTE_TYPES,                     hint: 'Output byte' },
  { re: /^QW\d+$/i,                        types: S7_WORD_TYPES,                     hint: 'Output word' },
  { re: /^QD\d+$/i,                        types: S7_DWORD_TYPES,                    hint: 'Output dword' },
  { re: /^C\d+$/i,                         types: ['int', 'counter'],                hint: 'Counter' },
  { re: /^T\d+$/i,                         types: ['int', 'timer'],                  hint: 'Timer' },
]

// Symbolic-DB pattern (DB1.MotorSpeed) — for the OPC UA hint on the address
// input. Mirrors reDBSym in internal/nodes/s7_address.go.
export const S7_SYMBOLIC_PATTERN = /^DB\d+\.[A-Za-z_][A-Za-z0-9_]{3,}$/

/**
 * Validate an S7 address against the supported forms. Returns:
 *   - { valid: true, hint }  — recognised, dataType is compatible
 *   - { valid: false, error } — malformed or dataType mismatch (with a hint
 *     pointing to OPC UA when the address looks symbolic)
 */
export function validateS7Address(addr: string, dataType: string): { valid: boolean; error?: string; hint?: string } {
  const trimmed = (addr ?? '').trim()
  if (!trimmed) return { valid: false, error: 'address required' }
  for (const { re, types, hint } of S7_ADDRESS_PATTERNS) {
    if (re.test(trimmed)) {
      if (dataType && !types.includes(dataType.toLowerCase())) {
        return { valid: false, error: `${hint} requires dataType ${types.join(' / ')}` }
      }
      return { valid: true, hint }
    }
  }
  if (S7_SYMBOLIC_PATTERN.test(trimmed)) {
    return { valid: false, error: 'symbolic address — un-tick "Optimized block access" in TIA Portal, or use OPC UA' }
  }
  return { valid: false, error: 'unknown S7 form (DB.<DBX|DBB|DBW|DBD|DBL|DTL|STRING|WSTRING>, M/MB/MW/MD, I/IB/IW/ID, Q/QB/QW/QD, C, T)' }
}

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
