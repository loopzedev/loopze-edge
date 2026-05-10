// S7-specific option lists. Keep in sync with internal/nodes/s7/*.go
// (codec.go for the type table, address.go for the address regex set).

import type { OptionEntry } from '@/components/config/enums'

// Connection-type presets the user picks in the S7 PLC config. The backend
// derives rack / slot / TSAP from this enum (s7ConnectionDefaults in
// internal/nodes/s7/plc.go).
export const S7_CONNECTION_TYPES: OptionEntry<string>[] = [
  { value: 's7-1200-1500', label: 'S7-1200 / S7-1500 (rack 0, slot 1)' },
  { value: 's7-300-400',   label: 'S7-300 / S7-400 (rack 0, slot 2)' },
  { value: 'logo',         label: 'LOGO! / S7-200 Smart' },
  { value: 'custom',       label: 'Custom (manual rack / slot)' },
]

// S7 data types the codec understands. `bool` requires a bit-addressable form
// (M0.0, DB.DBX, …); `string` requires the DB.STRING<byte>.<maxLen> form.
// See internal/nodes/s7/codec.go for the wire mapping.
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

// S7 address validator — see internal/nodes/s7/address.go for the canonical
// regex set. Type-set constants mirror the Go parser; any addition there
// must land here too (and vice versa).
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
// input. Mirrors reDBSym in internal/nodes/s7/address.go.
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
