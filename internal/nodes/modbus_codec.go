// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ByteOrder controls the byte ordering within a single 16-bit register.
// Modbus wire format always transmits each register big-endian; "littleEndian"
// here means the device stores its values byte-swapped within each register.
type ByteOrder string

const (
	ByteOrderBig    ByteOrder = "bigEndian"
	ByteOrderLittle ByteOrder = "littleEndian"
)

// WordOrder controls the order of registers when a value spans multiple registers.
// "bigEndian" (ABCD) puts the most significant register first; "littleEndian" (CDAB)
// puts the least significant first. Both combinations are common in the field —
// no standard exists, the device's datasheet decides.
type WordOrder string

const (
	WordOrderBig    WordOrder = "bigEndian"
	WordOrderLittle WordOrder = "littleEndian"
)

// parseByteOrder normalises a string from config or msg into a ByteOrder.
// Empty or unknown values default to big-endian.
func parseByteOrder(s string) ByteOrder {
	if s == string(ByteOrderLittle) {
		return ByteOrderLittle
	}
	return ByteOrderBig
}

// parseWordOrder normalises a string from config or msg into a WordOrder.
// Empty or unknown values default to big-endian.
func parseWordOrder(s string) WordOrder {
	if s == string(WordOrderLittle) {
		return WordOrderLittle
	}
	return WordOrderBig
}

// RegistersForType returns the number of 16-bit registers a single value of the
// given data type occupies. For "raw" and "string" the caller-provided quantity
// is returned unchanged.
func RegistersForType(dataType string, quantity int) int {
	switch dataType {
	case "bool", "int16", "uint16":
		return 1
	case "int32", "uint32", "float32":
		return 2
	case "int64", "uint64", "float64":
		return 4
	case "string", "raw":
		if quantity < 1 {
			return 1
		}
		return quantity
	default:
		if quantity < 1 {
			return 1
		}
		return quantity
	}
}

// RegistersToBytes packs a register slice as Modbus-wire bytes — each register
// big-endian, two bytes per register.
func RegistersToBytes(regs []uint16) []byte {
	out := make([]byte, 2*len(regs))
	for i, r := range regs {
		binary.BigEndian.PutUint16(out[2*i:], r)
	}
	return out
}

// BytesToRegisters parses Modbus-wire bytes (big-endian per register) into a
// register slice. An odd-length input is padded with a trailing zero byte.
func BytesToRegisters(b []byte) []uint16 {
	if len(b)%2 != 0 {
		b = append(b, 0)
	}
	regs := make([]uint16, len(b)/2)
	for i := range regs {
		regs[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return regs
}

// reorderForDecode applies word order, then byte order, to convert raw registers
// into a flat big-endian byte sequence ready for binary.BigEndian.* readers.
//
// Combinations for a 4-byte value 0x12345678 (A=0x12, B=0x34, C=0x56, D=0x78):
//
//	BigByte / BigWord  (ABCD) → regs [0x1234, 0x5678] → bytes 12 34 56 78
//	BigByte / LittleWord (CDAB) → regs [0x5678, 0x1234] → bytes 12 34 56 78 after reorder
//	LittleByte / BigWord (BADC) → regs [0x3412, 0x7856] → bytes 12 34 56 78 after reorder
//	LittleByte / LittleWord (DCBA) → regs [0x7856, 0x3412] → bytes 12 34 56 78 after reorder
func reorderForDecode(regs []uint16, bo ByteOrder, wo WordOrder) []byte {
	ordered := make([]uint16, len(regs))
	if wo == WordOrderLittle {
		for i, r := range regs {
			ordered[len(regs)-1-i] = r
		}
	} else {
		copy(ordered, regs)
	}
	out := make([]byte, len(ordered)*2)
	for i, r := range ordered {
		if bo == ByteOrderLittle {
			out[2*i] = byte(r & 0xff)
			out[2*i+1] = byte(r >> 8)
		} else {
			out[2*i] = byte(r >> 8)
			out[2*i+1] = byte(r & 0xff)
		}
	}
	return out
}

// reorderForEncode is the inverse of reorderForDecode: takes flat big-endian
// bytes and produces registers in the device's expected byte/word order.
func reorderForEncode(b []byte, bo ByteOrder, wo WordOrder) []uint16 {
	if len(b)%2 != 0 {
		b = append(b, 0)
	}
	regs := make([]uint16, len(b)/2)
	for i := range regs {
		var hi, lo byte
		if bo == ByteOrderLittle {
			hi = b[2*i+1]
			lo = b[2*i]
		} else {
			hi = b[2*i]
			lo = b[2*i+1]
		}
		regs[i] = uint16(hi)<<8 | uint16(lo)
	}
	if wo == WordOrderLittle {
		for i, j := 0, len(regs)-1; i < j; i, j = i+1, j-1 {
			regs[i], regs[j] = regs[j], regs[i]
		}
	}
	return regs
}

// DecodeRegisters decodes raw 16-bit registers into a typed Go value as
// requested by dataType. Multi-register types apply the supplied byte and word
// order. The "raw" type returns the registers as a JSON-friendly []int.
func DecodeRegisters(dataType string, bo ByteOrder, wo WordOrder, regs []uint16) (any, error) {
	switch dataType {
	case "raw", "":
		out := make([]int, len(regs))
		for i, r := range regs {
			out[i] = int(r)
		}
		return out, nil

	case "int16":
		if len(regs) < 1 {
			return nil, fmt.Errorf("int16 requires 1 register, got %d", len(regs))
		}
		b := reorderForDecode(regs[:1], bo, WordOrderBig)
		return int(int16(binary.BigEndian.Uint16(b))), nil

	case "uint16":
		if len(regs) < 1 {
			return nil, fmt.Errorf("uint16 requires 1 register, got %d", len(regs))
		}
		b := reorderForDecode(regs[:1], bo, WordOrderBig)
		return int(binary.BigEndian.Uint16(b)), nil

	case "int32":
		if len(regs) < 2 {
			return nil, fmt.Errorf("int32 requires 2 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:2], bo, wo)
		return int(int32(binary.BigEndian.Uint32(b))), nil

	case "uint32":
		if len(regs) < 2 {
			return nil, fmt.Errorf("uint32 requires 2 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:2], bo, wo)
		return int64(binary.BigEndian.Uint32(b)), nil

	case "float32":
		if len(regs) < 2 {
			return nil, fmt.Errorf("float32 requires 2 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:2], bo, wo)
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil

	case "int64":
		if len(regs) < 4 {
			return nil, fmt.Errorf("int64 requires 4 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:4], bo, wo)
		return int64(binary.BigEndian.Uint64(b)), nil

	case "uint64":
		if len(regs) < 4 {
			return nil, fmt.Errorf("uint64 requires 4 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:4], bo, wo)
		return binary.BigEndian.Uint64(b), nil

	case "float64":
		if len(regs) < 4 {
			return nil, fmt.Errorf("float64 requires 4 registers, got %d", len(regs))
		}
		b := reorderForDecode(regs[:4], bo, wo)
		return math.Float64frombits(binary.BigEndian.Uint64(b)), nil

	case "string":
		if len(regs) == 0 {
			return "", nil
		}
		b := reorderForDecode(regs, bo, wo)
		// Trim trailing NULs (typical Modbus C-string convention).
		end := len(b)
		for end > 0 && b[end-1] == 0 {
			end--
		}
		return string(b[:end]), nil

	default:
		return nil, fmt.Errorf("unknown dataType %q", dataType)
	}
}

// EncodeRegisters converts a typed value into 16-bit registers ready to send
// on the wire. The returned slice has exactly RegistersForType(dataType, …) entries.
func EncodeRegisters(dataType string, bo ByteOrder, wo WordOrder, value any) ([]uint16, error) {
	switch dataType {
	case "raw", "":
		regs, err := toUint16Slice(value)
		if err != nil {
			return nil, fmt.Errorf("raw: %w", err)
		}
		return regs, nil

	case "int16":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("int16: %w", err)
		}
		if n < math.MinInt16 || n > math.MaxInt16 {
			return nil, fmt.Errorf("int16: value %d out of range", n)
		}
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(int16(n)))
		return reorderForEncode(b[:], bo, WordOrderBig), nil

	case "uint16":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("uint16: %w", err)
		}
		if n < 0 || n > math.MaxUint16 {
			return nil, fmt.Errorf("uint16: value %d out of range", n)
		}
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(n))
		return reorderForEncode(b[:], bo, WordOrderBig), nil

	case "int32":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("int32: %w", err)
		}
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("int32: value %d out of range", n)
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(int32(n)))
		return reorderForEncode(b[:], bo, wo), nil

	case "uint32":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("uint32: %w", err)
		}
		if n < 0 || n > math.MaxUint32 {
			return nil, fmt.Errorf("uint32: value %d out of range", n)
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(n))
		return reorderForEncode(b[:], bo, wo), nil

	case "float32":
		f, err := toFloat64(value)
		if err != nil {
			return nil, fmt.Errorf("float32: %w", err)
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], math.Float32bits(float32(f)))
		return reorderForEncode(b[:], bo, wo), nil

	case "int64":
		n, err := toInt64(value)
		if err != nil {
			return nil, fmt.Errorf("int64: %w", err)
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		return reorderForEncode(b[:], bo, wo), nil

	case "uint64":
		u, err := toUint64(value)
		if err != nil {
			return nil, fmt.Errorf("uint64: %w", err)
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], u)
		return reorderForEncode(b[:], bo, wo), nil

	case "float64":
		f, err := toFloat64(value)
		if err != nil {
			return nil, fmt.Errorf("float64: %w", err)
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], math.Float64bits(f))
		return reorderForEncode(b[:], bo, wo), nil

	case "string":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("string: expected string value, got %T", value)
		}
		b := []byte(s)
		// Pad with trailing NULs to an even byte count so reorderForEncode
		// produces whole registers without a leftover odd byte.
		if len(b)%2 != 0 {
			b = append(b, 0)
		}
		return reorderForEncode(b, bo, wo), nil

	default:
		return nil, fmt.Errorf("unknown dataType %q", dataType)
	}
}

// DecodeCoils unpacks Modbus-packed coil bytes (LSB-first within each byte)
// into either a single bool (when dataType=="bool" and quantity==1) or a
// []bool of the requested length.
func DecodeCoils(packed []byte, quantity int, dataType string) (any, error) {
	if quantity < 0 {
		return nil, fmt.Errorf("coils: negative quantity %d", quantity)
	}
	bools := unpackCoilBits(packed, quantity)
	if dataType == "bool" {
		if quantity != 1 {
			return nil, fmt.Errorf("bool dataType requires quantity 1, got %d", quantity)
		}
		return bools[0], nil
	}
	return bools, nil
}

// EncodeCoils packs a value (bool, []bool, or []any of bools) into Modbus
// coil-wire bytes (LSB-first within each byte). Returns the packed bytes and
// the coil count for the wire-level write call.
func EncodeCoils(value any) ([]byte, int, error) {
	bools, err := toBoolSlice(value)
	if err != nil {
		return nil, 0, err
	}
	if len(bools) == 0 {
		return nil, 0, fmt.Errorf("coils: empty value")
	}
	return packCoilBits(bools), len(bools), nil
}

// unpackCoilBits unpacks Modbus-packed bytes into a []bool of length=quantity.
// Modbus packs coils LSB-first within each byte (coil 0 is bit 0 of byte 0).
func unpackCoilBits(packed []byte, quantity int) []bool {
	out := make([]bool, quantity)
	for i := 0; i < quantity; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if byteIdx >= len(packed) {
			break
		}
		out[i] = (packed[byteIdx]>>bitIdx)&1 == 1
	}
	return out
}

// packCoilBits packs a []bool into Modbus-wire bytes, LSB-first within each byte.
func packCoilBits(bools []bool) []byte {
	out := make([]byte, (len(bools)+7)/8)
	for i, b := range bools {
		if b {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	return out
}

// ApplyScale returns scale*value + offset. The result is always float64 when
// scaling is active — no attempt to preserve the input integer type, since
// any non-trivial scale loses precision for typical industrial uses (0.1 °C,
// 0.01 bar, …) and surfacing the lossy truncation downstream is worse than
// shipping a float64 the user expected anyway. For scale=1, offset=0 the
// value passes through untouched.
//
// Accepts the full set of Go numeric types the codecs (modbus, s7) emit:
// int / int8 / int16 / int32 / int64, uint / uint8 / uint16 / uint32 / uint64,
// float32 / float64. Anything else passes through unchanged.
func ApplyScale(value any, scale, offset float64) any {
	if scale == 1 && offset == 0 {
		return value
	}
	switch v := value.(type) {
	case int:
		return float64(v)*scale + offset
	case int8:
		return float64(v)*scale + offset
	case int16:
		return float64(v)*scale + offset
	case int32:
		return float64(v)*scale + offset
	case int64:
		return float64(v)*scale + offset
	case uint:
		return float64(v)*scale + offset
	case uint8:
		return float64(v)*scale + offset
	case uint16:
		return float64(v)*scale + offset
	case uint32:
		return float64(v)*scale + offset
	case uint64:
		return float64(v)*scale + offset
	case float32:
		return float64(v)*scale + offset
	case float64:
		return v*scale + offset
	default:
		return value
	}
}

// UnapplyScale is the inverse of ApplyScale: (raw - offset) / scale. Used in
// the write path to reverse the user's scaling before encoding to registers.
func UnapplyScale(value any, scale, offset float64) (float64, error) {
	if scale == 0 {
		return 0, fmt.Errorf("scale must not be zero")
	}
	f, err := toFloat64(value)
	if err != nil {
		return 0, err
	}
	return (f - offset) / scale, nil
}

// toInt64 converts the typical wire-format numeric values (float64 from JSON,
// int from Go callers, etc.) into a single int64. Strings are not accepted —
// the codec only parses numbers, callers must convert text upstream.
func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case uint:
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		if x > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 %d exceeds int64 max", x)
		}
		return int64(x), nil
	case float64:
		return int64(x), nil
	case float32:
		return int64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// toUint64 converts numeric values into uint64. Negative values are rejected.
func toUint64(v any) (uint64, error) {
	switch x := v.(type) {
	case uint64:
		return x, nil
	case float64:
		if x < 0 {
			return 0, fmt.Errorf("negative value %g cannot be uint64", x)
		}
		return uint64(x), nil
	default:
		n, err := toInt64(v)
		if err != nil {
			return 0, err
		}
		if n < 0 {
			return 0, fmt.Errorf("negative value %d cannot be uint64", n)
		}
		return uint64(n), nil
	}
}

// toFloat64 converts numeric values into float64.
func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	default:
		n, err := toInt64(v)
		if err != nil {
			return 0, err
		}
		return float64(n), nil
	}
}

// toUint16Slice accepts the wire formats commonly seen for register arrays:
// []uint16, []int, []any of numbers, or a single number (treated as one register).
func toUint16Slice(v any) ([]uint16, error) {
	switch x := v.(type) {
	case []uint16:
		return x, nil
	case []int:
		out := make([]uint16, len(x))
		for i, n := range x {
			if n < 0 || n > 0xffff {
				return nil, fmt.Errorf("register[%d] %d out of uint16 range", i, n)
			}
			out[i] = uint16(n)
		}
		return out, nil
	case []any:
		out := make([]uint16, len(x))
		for i, e := range x {
			n, err := toInt64(e)
			if err != nil {
				return nil, fmt.Errorf("register[%d]: %w", i, err)
			}
			if n < 0 || n > 0xffff {
				return nil, fmt.Errorf("register[%d] %d out of uint16 range", i, n)
			}
			out[i] = uint16(n)
		}
		return out, nil
	case nil:
		return nil, fmt.Errorf("nil value")
	default:
		// Single number → one-element slice.
		n, err := toInt64(v)
		if err != nil {
			return nil, fmt.Errorf("expected register array or number, got %T", v)
		}
		if n < 0 || n > 0xffff {
			return nil, fmt.Errorf("value %d out of uint16 range", n)
		}
		return []uint16{uint16(n)}, nil
	}
}

// toBoolSlice accepts a single bool, []bool, []any of bools, or a single
// truthy/falsy number/string. Numeric arrays are interpreted bit-by-bit
// (any non-zero element is true).
func toBoolSlice(v any) ([]bool, error) {
	switch x := v.(type) {
	case bool:
		return []bool{x}, nil
	case []bool:
		return x, nil
	case []any:
		out := make([]bool, len(x))
		for i, e := range x {
			b, err := toBool(e)
			if err != nil {
				return nil, fmt.Errorf("element[%d]: %w", i, err)
			}
			out[i] = b
		}
		return out, nil
	case []int:
		out := make([]bool, len(x))
		for i, n := range x {
			out[i] = n != 0
		}
		return out, nil
	case nil:
		return nil, fmt.Errorf("nil value")
	default:
		b, err := toBool(v)
		if err != nil {
			return nil, err
		}
		return []bool{b}, nil
	}
}

// toBool converts a single value into bool. Numeric values follow the
// standard non-zero rule.
func toBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case int:
		return x != 0, nil
	case int64:
		return x != 0, nil
	case float64:
		return x != 0, nil
	case string:
		switch x {
		case "true", "True", "TRUE", "1":
			return true, nil
		case "false", "False", "FALSE", "0":
			return false, nil
		}
		return false, fmt.Errorf("cannot convert string %q to bool (expected true/false/1/0)", x)
	case nil:
		return false, fmt.Errorf("nil value")
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}
