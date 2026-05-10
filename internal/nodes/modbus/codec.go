// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package modbus

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
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
		regs, err := nodes.ToUint16Slice(value)
		if err != nil {
			return nil, fmt.Errorf("raw: %w", err)
		}
		return regs, nil

	case "int16":
		n, err := nodes.ToInt64(value)
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
		n, err := nodes.ToInt64(value)
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
		n, err := nodes.ToInt64(value)
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
		n, err := nodes.ToInt64(value)
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
		f, err := nodes.ToFloat64(value)
		if err != nil {
			return nil, fmt.Errorf("float32: %w", err)
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], math.Float32bits(float32(f)))
		return reorderForEncode(b[:], bo, wo), nil

	case "int64":
		n, err := nodes.ToInt64(value)
		if err != nil {
			return nil, fmt.Errorf("int64: %w", err)
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		return reorderForEncode(b[:], bo, wo), nil

	case "uint64":
		u, err := nodes.ToUint64(value)
		if err != nil {
			return nil, fmt.Errorf("uint64: %w", err)
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], u)
		return reorderForEncode(b[:], bo, wo), nil

	case "float64":
		f, err := nodes.ToFloat64(value)
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
	bools, err := nodes.ToBoolSlice(value)
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

