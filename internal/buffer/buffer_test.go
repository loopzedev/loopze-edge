// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package buffer

import (
	"math"
	"testing"
)

// ── Constructors ────────────────────────────────────────────────────────────

func TestAlloc(t *testing.T) {
	buf := Alloc(8)
	if buf.Length() != 8 {
		t.Fatalf("expected length 8, got %d", buf.Length())
	}
	for i, b := range buf.Bytes() {
		if b != 0 {
			t.Fatalf("expected zero at index %d, got %d", i, b)
		}
	}
}

func TestFrom(t *testing.T) {
	src := []byte{1, 2, 3}
	buf := From(src)
	// Mutate source — buffer should be independent.
	src[0] = 99
	if buf.Bytes()[0] != 1 {
		t.Fatal("From should copy data, not reference it")
	}
}

func TestFromString(t *testing.T) {
	buf := FromString("Hello")
	if buf.ToString() != "Hello" {
		t.Fatalf("expected 'Hello', got %q", buf.ToString())
	}
}

func TestFromHex(t *testing.T) {
	buf, err := FromHex("48656c6c6f")
	if err != nil {
		t.Fatal(err)
	}
	if buf.ToString() != "Hello" {
		t.Fatalf("expected 'Hello', got %q", buf.ToString())
	}

	_, err = FromHex("zzzz")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestFromBase64(t *testing.T) {
	buf, err := FromBase64("SGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if buf.ToString() != "Hello" {
		t.Fatalf("expected 'Hello', got %q", buf.ToString())
	}

	_, err = FromBase64("!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestConcat(t *testing.T) {
	a := FromString("Hel")
	b := FromString("lo")
	c := Concat(a, b)
	if c.ToString() != "Hello" {
		t.Fatalf("expected 'Hello', got %q", c.ToString())
	}
	if c.Length() != 5 {
		t.Fatalf("expected length 5, got %d", c.Length())
	}
}

// ── Read Integer ────────────────────────────────────────────────────────────

func TestReadUInt8(t *testing.T) {
	buf := From([]byte{0x00, 0x7F, 0xFF})
	v, _ := buf.ReadUInt8(0)
	if v != 0 {
		t.Fatalf("expected 0, got %d", v)
	}
	v, _ = buf.ReadUInt8(2)
	if v != 255 {
		t.Fatalf("expected 255, got %d", v)
	}
}

func TestReadInt8(t *testing.T) {
	buf := From([]byte{0x00, 0x7F, 0x80, 0xFF})
	v, _ := buf.ReadInt8(2)
	if v != -128 {
		t.Fatalf("expected -128, got %d", v)
	}
	v, _ = buf.ReadInt8(3)
	if v != -1 {
		t.Fatalf("expected -1, got %d", v)
	}
}

func TestReadUInt16(t *testing.T) {
	buf := From([]byte{0x01, 0x02})
	be, _ := buf.ReadUInt16BE(0)
	le, _ := buf.ReadUInt16LE(0)
	if be != 0x0102 {
		t.Fatalf("BE: expected 0x0102, got 0x%04X", be)
	}
	if le != 0x0201 {
		t.Fatalf("LE: expected 0x0201, got 0x%04X", le)
	}
}

func TestReadInt16(t *testing.T) {
	buf := From([]byte{0xFF, 0xFE})
	v, _ := buf.ReadInt16BE(0)
	if v != -2 {
		t.Fatalf("expected -2, got %d", v)
	}
}

func TestReadUInt32(t *testing.T) {
	buf := From([]byte{0x00, 0x01, 0x00, 0x00})
	v, _ := buf.ReadUInt32BE(0)
	if v != 65536 {
		t.Fatalf("expected 65536, got %d", v)
	}
}

func TestReadInt32(t *testing.T) {
	buf := From([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	v, _ := buf.ReadInt32BE(0)
	if v != -1 {
		t.Fatalf("expected -1, got %d", v)
	}
}

func TestReadBigUInt64(t *testing.T) {
	buf := From([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00})
	v, _ := buf.ReadBigUInt64BE(0)
	if v != 256 {
		t.Fatalf("expected 256, got %d", v)
	}
}

func TestReadBigInt64(t *testing.T) {
	buf := From([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	v, _ := buf.ReadBigInt64BE(0)
	if v != -1 {
		t.Fatalf("expected -1, got %d", v)
	}
}

// ── Read Variable Length ────────────────────────────────────────────────────

func TestReadUIntBE(t *testing.T) {
	buf := From([]byte{0x01, 0x02, 0x03})

	v, _ := buf.ReadUIntBE(0, 1)
	if v != 1 {
		t.Fatalf("1-byte: expected 1, got %d", v)
	}

	v, _ = buf.ReadUIntBE(0, 2)
	if v != 0x0102 {
		t.Fatalf("2-byte: expected 0x0102, got 0x%X", v)
	}

	v, _ = buf.ReadUIntBE(0, 3)
	if v != 0x010203 {
		t.Fatalf("3-byte: expected 0x010203, got 0x%X", v)
	}
}

func TestReadIntBE_SignExtend(t *testing.T) {
	buf := From([]byte{0xFF, 0xFE})
	v, _ := buf.ReadIntBE(0, 2)
	if v != -2 {
		t.Fatalf("expected -2, got %d", v)
	}
}

func TestReadUIntLE(t *testing.T) {
	buf := From([]byte{0x03, 0x02, 0x01})
	v, _ := buf.ReadUIntLE(0, 3)
	if v != 0x010203 {
		t.Fatalf("expected 0x010203, got 0x%X", v)
	}
}

// ── Read Float ──────────────────────────────────────────────────────────────

func TestReadFloatBE_Roundtrip(t *testing.T) {
	buf := Alloc(4)
	expected := float32(3.14)
	_ = buf.WriteFloatBE(expected, 0)
	got, _ := buf.ReadFloatBE(0)
	if got != expected {
		t.Fatalf("expected %f, got %f", expected, got)
	}
}

func TestReadDoubleBE_Roundtrip(t *testing.T) {
	buf := Alloc(8)
	expected := 3.141592653589793
	_ = buf.WriteDoubleBE(expected, 0)
	got, _ := buf.ReadDoubleBE(0)
	if got != expected {
		t.Fatalf("expected %f, got %f", expected, got)
	}
}

func TestReadFloatLE_Roundtrip(t *testing.T) {
	buf := Alloc(4)
	expected := float32(-42.5)
	_ = buf.WriteFloatLE(expected, 0)
	got, _ := buf.ReadFloatLE(0)
	if got != expected {
		t.Fatalf("expected %f, got %f", expected, got)
	}
}

func TestReadDoubleLE_Roundtrip(t *testing.T) {
	buf := Alloc(8)
	expected := math.Pi
	_ = buf.WriteDoubleLE(expected, 0)
	got, _ := buf.ReadDoubleLE(0)
	if got != expected {
		t.Fatalf("expected %f, got %f", expected, got)
	}
}

// ── Write + Read Roundtrip ──────────────────────────────────────────────────

func TestWriteReadRoundtrip_AllTypes(t *testing.T) {
	buf := Alloc(32)

	_ = buf.WriteUInt8(200, 0)
	_ = buf.WriteInt8(-100, 1)
	_ = buf.WriteUInt16BE(1234, 2)
	_ = buf.WriteInt16LE(-5678, 4)
	_ = buf.WriteUInt32BE(123456789, 6)
	_ = buf.WriteInt32LE(-987654321, 10)
	_ = buf.WriteFloatBE(2.718, 14)
	_ = buf.WriteDoubleBE(1.41421356, 18)

	if v, _ := buf.ReadUInt8(0); v != 200 {
		t.Fatalf("UInt8: expected 200, got %d", v)
	}
	if v, _ := buf.ReadInt8(1); v != -100 {
		t.Fatalf("Int8: expected -100, got %d", v)
	}
	if v, _ := buf.ReadUInt16BE(2); v != 1234 {
		t.Fatalf("UInt16BE: expected 1234, got %d", v)
	}
	if v, _ := buf.ReadInt16LE(4); v != -5678 {
		t.Fatalf("Int16LE: expected -5678, got %d", v)
	}
	if v, _ := buf.ReadUInt32BE(6); v != 123456789 {
		t.Fatalf("UInt32BE: expected 123456789, got %d", v)
	}
	if v, _ := buf.ReadInt32LE(10); v != -987654321 {
		t.Fatalf("Int32LE: expected -987654321, got %d", v)
	}
	if v, _ := buf.ReadFloatBE(14); v != float32(2.718) {
		t.Fatalf("FloatBE: expected 2.718, got %f", v)
	}
	if v, _ := buf.ReadDoubleBE(18); v != 1.41421356 {
		t.Fatalf("DoubleBE: expected 1.41421356, got %f", v)
	}
}

func TestWriteReadRoundtrip_BigInt64(t *testing.T) {
	buf := Alloc(16)
	_ = buf.WriteBigUInt64BE(0xDEADBEEFCAFEBABE, 0)
	_ = buf.WriteBigInt64LE(-42, 8)

	v1, _ := buf.ReadBigUInt64BE(0)
	if v1 != 0xDEADBEEFCAFEBABE {
		t.Fatalf("expected 0xDEADBEEFCAFEBABE, got 0x%X", v1)
	}
	v2, _ := buf.ReadBigInt64LE(8)
	if v2 != -42 {
		t.Fatalf("expected -42, got %d", v2)
	}
}

func TestWriteReadRoundtrip_VariableLength(t *testing.T) {
	buf := Alloc(6)
	_ = buf.WriteUIntBE(0xABCDEF, 0, 3)
	v, _ := buf.ReadUIntBE(0, 3)
	if v != 0xABCDEF {
		t.Fatalf("expected 0xABCDEF, got 0x%X", v)
	}

	_ = buf.WriteUIntLE(0x123456, 3, 3)
	v, _ = buf.ReadUIntLE(3, 3)
	if v != 0x123456 {
		t.Fatalf("expected 0x123456, got 0x%X", v)
	}
}

// ── Swap ────────────────────────────────────────────────────────────────────

func TestSwap16(t *testing.T) {
	buf := From([]byte{0x01, 0x02, 0x03, 0x04})
	_ = buf.Swap16()
	expected := []byte{0x02, 0x01, 0x04, 0x03}
	for i, v := range buf.Bytes() {
		if v != expected[i] {
			t.Fatalf("index %d: expected 0x%02X, got 0x%02X", i, expected[i], v)
		}
	}
}

func TestSwap32(t *testing.T) {
	buf := From([]byte{0x01, 0x02, 0x03, 0x04})
	_ = buf.Swap32()
	expected := []byte{0x04, 0x03, 0x02, 0x01}
	for i, v := range buf.Bytes() {
		if v != expected[i] {
			t.Fatalf("index %d: expected 0x%02X, got 0x%02X", i, expected[i], v)
		}
	}
}

func TestSwap64(t *testing.T) {
	buf := From([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	_ = buf.Swap64()
	expected := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	for i, v := range buf.Bytes() {
		if v != expected[i] {
			t.Fatalf("index %d: expected 0x%02X, got 0x%02X", i, expected[i], v)
		}
	}
}

func TestSwap_InvalidLength(t *testing.T) {
	if err := From([]byte{1, 2, 3}).Swap16(); err == nil {
		t.Fatal("swap16 should fail on odd length")
	}
	if err := From([]byte{1, 2, 3}).Swap32(); err == nil {
		t.Fatal("swap32 should fail on non-multiple-of-4 length")
	}
	if err := From([]byte{1, 2, 3, 4}).Swap64(); err == nil {
		t.Fatal("swap64 should fail on non-multiple-of-8 length")
	}
}

// ── Bounds Checking ─────────────────────────────────────────────────────────

func TestBoundsCheck(t *testing.T) {
	buf := Alloc(2)

	if _, err := buf.ReadUInt32BE(0); err == nil {
		t.Fatal("readUInt32BE on 2-byte buffer should fail")
	}
	if _, err := buf.ReadUInt16BE(1); err == nil {
		t.Fatal("readUInt16BE at offset 1 on 2-byte buffer should fail")
	}
	if _, err := buf.ReadUInt8(-1); err == nil {
		t.Fatal("negative offset should fail")
	}
	if err := buf.WriteUInt32BE(0, 0); err == nil {
		t.Fatal("writeUInt32BE on 2-byte buffer should fail")
	}
}

func TestBoundsCheck_VariableLength(t *testing.T) {
	buf := Alloc(2)
	if _, err := buf.ReadUIntBE(0, 0); err == nil {
		t.Fatal("byteLength 0 should fail")
	}
	if _, err := buf.ReadUIntBE(0, 7); err == nil {
		t.Fatal("byteLength 7 should fail")
	}
	if _, err := buf.ReadUIntBE(0, 3); err == nil {
		t.Fatal("reading 3 bytes from 2-byte buffer should fail")
	}
}

// ── Conversion ──────────────────────────────────────────────────────────────

func TestToHex(t *testing.T) {
	buf := FromString("Hello")
	if buf.ToHex() != "48656c6c6f" {
		t.Fatalf("expected '48656c6c6f', got %q", buf.ToHex())
	}
}

func TestToBase64(t *testing.T) {
	buf := FromString("Hello")
	if buf.ToBase64() != "SGVsbG8=" {
		t.Fatalf("expected 'SGVsbG8=', got %q", buf.ToBase64())
	}
}

func TestToJSON(t *testing.T) {
	buf := From([]byte{72, 101, 108})
	j := buf.ToJSON()
	if len(j) != 3 || j[0] != 72 || j[1] != 101 || j[2] != 108 {
		t.Fatalf("unexpected JSON: %v", j)
	}
}

func TestSlice(t *testing.T) {
	buf := FromString("Hello")
	s := buf.Slice(1, 4)
	if s.ToString() != "ell" {
		t.Fatalf("expected 'ell', got %q", s.ToString())
	}
	if s.Length() != 3 {
		t.Fatalf("expected length 3, got %d", s.Length())
	}
	// Slice should be a copy.
	s.Bytes()[0] = 'X'
	if buf.Bytes()[1] == 'X' {
		t.Fatal("slice should be independent of source")
	}
}

func TestSlice_Clamping(t *testing.T) {
	buf := FromString("Hi")
	s := buf.Slice(-5, 100)
	if s.ToString() != "Hi" {
		t.Fatalf("expected 'Hi', got %q", s.ToString())
	}
	s2 := buf.Slice(3, 1)
	if s2.Length() != 0 {
		t.Fatalf("start > end should return empty buffer, got length %d", s2.Length())
	}
}

func TestCopy(t *testing.T) {
	src := FromString("Hello")
	dst := Alloc(5)
	n := src.Copy(dst, 0, 0, 5)
	if n != 5 {
		t.Fatalf("expected 5 bytes copied, got %d", n)
	}
	if dst.ToString() != "Hello" {
		t.Fatalf("expected 'Hello', got %q", dst.ToString())
	}
}

func TestCopy_Partial(t *testing.T) {
	src := FromString("Hello")
	dst := Alloc(3)
	n := src.Copy(dst, 0, 1, 4)
	if n != 3 {
		t.Fatalf("expected 3 bytes copied, got %d", n)
	}
	if dst.ToString() != "ell" {
		t.Fatalf("expected 'ell', got %q", dst.ToString())
	}
}
