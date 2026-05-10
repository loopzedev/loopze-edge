// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package modbus

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"math"
	"reflect"
	"testing"
)

// 0x12345678 split into bytes: A=0x12, B=0x34, C=0x56, D=0x78.
// The four byte/word-order combinations produce these wire registers:
//
//	BigByte / BigWord    (ABCD) → [0x1234, 0x5678]
//	BigByte / LittleWord (CDAB) → [0x5678, 0x1234]
//	LittleByte / BigWord (BADC) → [0x3412, 0x7856]
//	LittleByte / LittleWord (DCBA) → [0x7856, 0x3412]
func TestReorderForDecode4Combos(t *testing.T) {
	tests := []struct {
		name string
		bo   ByteOrder
		wo   WordOrder
		regs []uint16
		want []byte
	}{
		{"ABCD", ByteOrderBig, WordOrderBig, []uint16{0x1234, 0x5678}, []byte{0x12, 0x34, 0x56, 0x78}},
		{"CDAB", ByteOrderBig, WordOrderLittle, []uint16{0x5678, 0x1234}, []byte{0x12, 0x34, 0x56, 0x78}},
		{"BADC", ByteOrderLittle, WordOrderBig, []uint16{0x3412, 0x7856}, []byte{0x12, 0x34, 0x56, 0x78}},
		{"DCBA", ByteOrderLittle, WordOrderLittle, []uint16{0x7856, 0x3412}, []byte{0x12, 0x34, 0x56, 0x78}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderForDecode(tt.regs, tt.bo, tt.wo)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %x, want %x", got, tt.want)
			}
		})
	}
}

func TestReorderForEncode4Combos(t *testing.T) {
	source := []byte{0x12, 0x34, 0x56, 0x78}
	tests := []struct {
		name string
		bo   ByteOrder
		wo   WordOrder
		want []uint16
	}{
		{"ABCD", ByteOrderBig, WordOrderBig, []uint16{0x1234, 0x5678}},
		{"CDAB", ByteOrderBig, WordOrderLittle, []uint16{0x5678, 0x1234}},
		{"BADC", ByteOrderLittle, WordOrderBig, []uint16{0x3412, 0x7856}},
		{"DCBA", ByteOrderLittle, WordOrderLittle, []uint16{0x7856, 0x3412}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderForEncode(source, tt.bo, tt.wo)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReorderRoundtrip(t *testing.T) {
	source := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
	for _, bo := range []ByteOrder{ByteOrderBig, ByteOrderLittle} {
		for _, wo := range []WordOrder{WordOrderBig, WordOrderLittle} {
			t.Run(string(bo)+"/"+string(wo), func(t *testing.T) {
				regs := reorderForEncode(source, bo, wo)
				back := reorderForDecode(regs, bo, wo)
				if !reflect.DeepEqual(back, source) {
					t.Errorf("roundtrip mismatch: got %x, want %x", back, source)
				}
			})
		}
	}
}

func TestRegistersToBytesRoundtrip(t *testing.T) {
	regs := []uint16{0x0102, 0x0304, 0x0506, 0xFFFE}
	got := BytesToRegisters(RegistersToBytes(regs))
	if !reflect.DeepEqual(got, regs) {
		t.Errorf("roundtrip mismatch: got %v, want %v", got, regs)
	}
}

func TestBytesToRegistersOddPad(t *testing.T) {
	got := BytesToRegisters([]byte{0xAB})
	want := []uint16{0xAB00}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRegistersForType(t *testing.T) {
	tests := []struct {
		dataType string
		quantity int
		want     int
	}{
		{"bool", 1, 1},
		{"int16", 1, 1},
		{"uint16", 1, 1},
		{"int32", 1, 2},
		{"uint32", 1, 2},
		{"float32", 1, 2},
		{"int64", 1, 4},
		{"uint64", 1, 4},
		{"float64", 1, 4},
		{"string", 5, 5},
		{"raw", 10, 10},
		{"raw", 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			if got := RegistersForType(tt.dataType, tt.quantity); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDecodeRegistersScalars(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		bo       ByteOrder
		wo       WordOrder
		regs     []uint16
		want     any
	}{
		{"int16 positive", "int16", ByteOrderBig, WordOrderBig, []uint16{0x0042}, 66},
		{"int16 negative", "int16", ByteOrderBig, WordOrderBig, []uint16{0xFFFE}, -2},
		{"uint16", "uint16", ByteOrderBig, WordOrderBig, []uint16{0x8000}, 32768},

		{"int32 positive ABCD", "int32", ByteOrderBig, WordOrderBig, []uint16{0x0001, 0x0000}, 65536},
		{"int32 negative ABCD", "int32", ByteOrderBig, WordOrderBig, []uint16{0xFFFF, 0xFFFE}, -2},
		{"int32 positive CDAB", "int32", ByteOrderBig, WordOrderLittle, []uint16{0x0000, 0x0001}, 65536},

		{"uint32 ABCD", "uint32", ByteOrderBig, WordOrderBig, []uint16{0xFFFF, 0xFFFF}, int64(0xFFFFFFFF)},

		{"int64 ABCD", "int64", ByteOrderBig, WordOrderBig, []uint16{0x0000, 0x0000, 0x0000, 0x0001}, int64(1)},
		{"int64 negative ABCD", "int64", ByteOrderBig, WordOrderBig, []uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}, int64(-1)},

		{"uint64 ABCD", "uint64", ByteOrderBig, WordOrderBig, []uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF}, uint64(math.MaxUint64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeRegisters(tt.dataType, tt.bo, tt.wo, tt.regs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestDecodeRegistersFloat32(t *testing.T) {
	// 1.0 in IEEE 754: 0x3F800000 → bytes 3F 80 00 00 → ABCD regs [0x3F80, 0x0000]
	tests := []struct {
		name string
		bo   ByteOrder
		wo   WordOrder
		regs []uint16
	}{
		{"ABCD", ByteOrderBig, WordOrderBig, []uint16{0x3F80, 0x0000}},
		{"CDAB", ByteOrderBig, WordOrderLittle, []uint16{0x0000, 0x3F80}},
		{"BADC", ByteOrderLittle, WordOrderBig, []uint16{0x803F, 0x0000}},
		{"DCBA", ByteOrderLittle, WordOrderLittle, []uint16{0x0000, 0x803F}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeRegisters("float32", tt.bo, tt.wo, tt.regs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			f, ok := got.(float64)
			if !ok || f != 1.0 {
				t.Errorf("got %v (%T), want 1.0", got, got)
			}
		})
	}
}

func TestDecodeRegistersFloat64(t *testing.T) {
	// 1.0 in IEEE 754 double: 0x3FF0000000000000 → ABCD regs [0x3FF0, 0x0000, 0x0000, 0x0000]
	got, err := DecodeRegisters("float64", ByteOrderBig, WordOrderBig,
		[]uint16{0x3FF0, 0x0000, 0x0000, 0x0000})
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 1.0 {
		t.Errorf("got %v, want 1.0", got)
	}
}

func TestDecodeRegistersString(t *testing.T) {
	// "AB" → bytes 0x41 0x42, big-endian regs [0x4142]
	got, err := DecodeRegisters("string", ByteOrderBig, WordOrderBig, []uint16{0x4142})
	if err != nil {
		t.Fatal(err)
	}
	if got != "AB" {
		t.Errorf("got %q, want %q", got, "AB")
	}

	// "Hello" padded to 6 bytes / 3 registers, ABCD layout.
	got, err = DecodeRegisters("string", ByteOrderBig, WordOrderBig, []uint16{0x4865, 0x6C6C, 0x6F00})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello" {
		t.Errorf("got %q, want %q (NUL stripping)", got, "Hello")
	}
}

func TestDecodeRegistersRaw(t *testing.T) {
	got, err := DecodeRegisters("raw", ByteOrderBig, WordOrderBig, []uint16{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDecodeRegistersInsufficient(t *testing.T) {
	cases := []struct {
		dataType string
		regs     []uint16
	}{
		{"int16", []uint16{}},
		{"int32", []uint16{1}},
		{"float32", []uint16{1}},
		{"int64", []uint16{1, 2, 3}},
		{"float64", []uint16{1, 2, 3}},
	}
	for _, c := range cases {
		t.Run(c.dataType, func(t *testing.T) {
			if _, err := DecodeRegisters(c.dataType, ByteOrderBig, WordOrderBig, c.regs); err == nil {
				t.Errorf("expected error for %s with %d regs", c.dataType, len(c.regs))
			}
		})
	}
}

func TestEncodeDecodeRoundtripScalars(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		value    any
	}{
		{"int16", "int16", 42},
		{"int16 negative", "int16", -1},
		{"uint16", "uint16", 65535},
		{"int32 max", "int32", int(math.MaxInt32)},
		{"int32 min", "int32", int(math.MinInt32)},
		{"uint32 max", "uint32", int64(math.MaxUint32)},
		{"int64", "int64", int64(-9876543210)},
		{"uint64", "uint64", uint64(18446744073709551610)},
		{"float32", "float32", 3.14},
		{"float64", "float64", 1234.5678},
	}
	for _, tt := range tests {
		for _, bo := range []ByteOrder{ByteOrderBig, ByteOrderLittle} {
			for _, wo := range []WordOrder{WordOrderBig, WordOrderLittle} {
				name := tt.name + "/" + string(bo) + "/" + string(wo)
				t.Run(name, func(t *testing.T) {
					regs, err := EncodeRegisters(tt.dataType, bo, wo, tt.value)
					if err != nil {
						t.Fatalf("encode failed: %v", err)
					}
					got, err := DecodeRegisters(tt.dataType, bo, wo, regs)
					if err != nil {
						t.Fatalf("decode failed: %v", err)
					}

					// Compare with type tolerance: encoded ints might decode into
					// different int width, floats compared with epsilon.
					if !numericEqual(got, tt.value) {
						t.Errorf("roundtrip mismatch: got %v (%T), want %v (%T)", got, got, tt.value, tt.value)
					}
				})
			}
		}
	}
}

func TestEncodeDecodeStringRoundtrip(t *testing.T) {
	for _, bo := range []ByteOrder{ByteOrderBig, ByteOrderLittle} {
		for _, wo := range []WordOrder{WordOrderBig, WordOrderLittle} {
			t.Run(string(bo)+"/"+string(wo), func(t *testing.T) {
				input := "Hello"
				regs, err := EncodeRegisters("string", bo, wo, input)
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
				got, err := DecodeRegisters("string", bo, wo, regs)
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got != input {
					t.Errorf("got %q, want %q", got, input)
				}
			})
		}
	}
}

func TestEncodeRangeChecks(t *testing.T) {
	cases := []struct {
		name     string
		dataType string
		value    any
	}{
		{"int16 overflow", "int16", 40000},
		{"int16 underflow", "int16", -40000},
		{"uint16 negative", "uint16", -1},
		{"uint16 overflow", "uint16", 70000},
		{"int32 overflow", "int32", int64(math.MaxInt32) + 1},
		{"uint32 negative", "uint32", -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := EncodeRegisters(c.dataType, ByteOrderBig, WordOrderBig, c.value); err == nil {
				t.Errorf("expected range error")
			}
		})
	}
}

func TestEncodeRaw(t *testing.T) {
	// Wire format: []any from JSON-decoded msg.payload.
	got, err := EncodeRegisters("raw", ByteOrderBig, WordOrderBig, []any{float64(1), float64(2), float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDecodeCoils(t *testing.T) {
	// 5 coils packed LSB-first: bits 0,1,1,0,1 → byte = 0b00010110 = 0x16
	got, err := DecodeCoils([]byte{0x16}, 5, "raw")
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{false, true, true, false, true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Single coil with dataType=bool
	gotBool, err := DecodeCoils([]byte{0x01}, 1, "bool")
	if err != nil {
		t.Fatal(err)
	}
	if gotBool != true {
		t.Errorf("got %v, want true", gotBool)
	}

	// 10 coils across 2 bytes
	got, err = DecodeCoils([]byte{0xAA, 0x02}, 10, "raw")
	if err != nil {
		t.Fatal(err)
	}
	want = []bool{false, true, false, true, false, true, false, true, false, true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEncodeCoils(t *testing.T) {
	// Single bool true → byte 0x01
	data, qty, err := EncodeCoils(true)
	if err != nil {
		t.Fatal(err)
	}
	if qty != 1 || !reflect.DeepEqual(data, []byte{0x01}) {
		t.Errorf("got %x qty=%d, want [01] qty=1", data, qty)
	}

	// []bool {false, true, true, false, true} → byte 0x16, 5 coils
	data, qty, err = EncodeCoils([]bool{false, true, true, false, true})
	if err != nil {
		t.Fatal(err)
	}
	if qty != 5 || !reflect.DeepEqual(data, []byte{0x16}) {
		t.Errorf("got %x qty=%d, want [16] qty=5", data, qty)
	}

	// []any with bools (JSON wire format)
	data, qty, err = EncodeCoils([]any{true, false, true})
	if err != nil {
		t.Fatal(err)
	}
	if qty != 3 || !reflect.DeepEqual(data, []byte{0x05}) {
		t.Errorf("got %x qty=%d, want [05] qty=3", data, qty)
	}
}

func TestCoilsRoundtrip(t *testing.T) {
	original := []bool{true, false, true, true, false, false, true, true, false, true, false}
	packed := packCoilBits(original)
	got := unpackCoilBits(packed, len(original))
	if !reflect.DeepEqual(got, original) {
		t.Errorf("roundtrip mismatch: got %v, want %v", got, original)
	}
}

func TestApplyScale(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		scale  float64
		offset float64
		want   any
	}{
		{"identity int", 100, 1, 0, 100},
		{"identity float", 1.5, 1, 0, 1.5},
		{"div 100", 1234, 0.01, 0, 12.34},
		{"offset only", 50, 1, -10, 40.0},
		{"scale and offset", 100, 0.1, 5, 15.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodes.ApplyScale(tt.value, tt.scale, tt.offset)
			if !numericEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnapplyScale(t *testing.T) {
	got, err := nodes.UnapplyScale(12.34, 0.01, 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-1234) > 1e-9 {
		t.Errorf("got %v, want 1234", got)
	}

	if _, err := nodes.UnapplyScale(1.0, 0, 0); err == nil {
		t.Error("expected error for zero scale")
	}
}

// numericEqual compares two numeric values with relative tolerance for floats
// and exact equality after type-normalisation for integers.
func numericEqual(got, want any) bool {
	gf, gok := asFloat(got)
	wf, wok := asFloat(want)
	if !gok || !wok {
		return reflect.DeepEqual(got, want)
	}
	if math.IsNaN(gf) && math.IsNaN(wf) {
		return true
	}
	if wf == 0 {
		return math.Abs(gf) < 1e-6
	}
	return math.Abs(gf-wf)/math.Abs(wf) < 1e-6
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}
