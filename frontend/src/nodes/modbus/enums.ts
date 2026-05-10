// Modbus-specific option lists. Keep in sync with internal/nodes/modbus/*.go.

import type { OptionEntry } from '@/components/config/enums'

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
