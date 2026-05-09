// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"math"
	"strings"
	"testing"
)

func TestDecodeS7Scalar_RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		typ    string
		signed bool
		val    any
	}{
		{"byte unsigned mid", "byte", false, uint8(0x42)},
		{"byte unsigned max", "byte", false, uint8(0xFF)},
		{"byte unsigned zero", "byte", false, uint8(0x00)},
		{"byte signed positive", "byte", true, int8(100)},
		{"byte signed negative", "byte", true, int8(-50)},
		{"byte signed min", "byte", true, int8(-128)},
		{"byte signed max", "byte", true, int8(127)},
		{"char same as byte", "char", false, uint8(0x41)}, // 'A'

		{"word unsigned mid", "word", false, uint16(0x1234)},
		{"word unsigned max", "word", false, uint16(0xFFFF)},
		{"word signed pos", "word", true, int16(1234)},
		{"word signed neg", "word", true, int16(-1)}, // 0xFFFF as signed
		{"int positive", "int", false, int16(12345)},
		{"int negative", "int", false, int16(-12345)},
		{"int zero", "int", false, int16(0)},

		{"dword unsigned", "dword", false, uint32(0xDEADBEEF)},
		{"dword signed", "dword", true, int32(-1)},
		{"dint positive", "dint", false, int32(123456789)},
		{"dint negative", "dint", false, int32(-123456789)},

		{"real zero", "real", false, float32(0)},
		{"real one", "real", false, float32(1)},
		{"real negative", "real", false, float32(-3.14)},
		{"real small", "real", false, float32(1e-30)},
		{"real large", "real", false, float32(1e30)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := EncodeS7Scalar(c.typ, c.signed, c.val)
			if err != nil {
				t.Fatalf("EncodeS7Scalar(%s, %v, %v) error: %v", c.typ, c.signed, c.val, err)
			}
			got, err := DecodeS7Scalar(c.typ, c.signed, b)
			if err != nil {
				t.Fatalf("DecodeS7Scalar(%s, %v, %v) error: %v", c.typ, c.signed, b, err)
			}
			if got != c.val {
				t.Errorf("round-trip mismatch: got %v (%T), want %v (%T)", got, got, c.val, c.val)
			}
		})
	}
}

func TestDecodeS7Scalar_ExplicitBytePatterns(t *testing.T) {
	// Verify byte-perfect wire layout against known patterns. Catches accidental
	// endianness or signedness regressions.
	cases := []struct {
		name   string
		typ    string
		signed bool
		bytes  []byte
		want   any
	}{
		// REAL (IEEE 754 single, big-endian)
		{"real 1.0", "real", false, []byte{0x3F, 0x80, 0x00, 0x00}, float32(1.0)},
		{"real -1.0", "real", false, []byte{0xBF, 0x80, 0x00, 0x00}, float32(-1.0)},
		{"real 21.5", "real", false, []byte{0x41, 0xAC, 0x00, 0x00}, float32(21.5)},
		{"real 3.14159", "real", false, []byte{0x40, 0x49, 0x0F, 0xD0}, float32(3.14159)},

		// INT/DINT in two's complement big-endian
		{"int -1", "int", false, []byte{0xFF, 0xFF}, int16(-1)},
		{"int 256", "int", false, []byte{0x01, 0x00}, int16(256)},
		{"dint -1", "dint", false, []byte{0xFF, 0xFF, 0xFF, 0xFF}, int32(-1)},
		{"dint 1000000", "dint", false, []byte{0x00, 0x0F, 0x42, 0x40}, int32(1000000)},

		// WORD/DWORD as unsigned vs. signed
		{"word unsigned 0xFFFF", "word", false, []byte{0xFF, 0xFF}, uint16(0xFFFF)},
		{"word signed 0xFFFF", "word", true, []byte{0xFF, 0xFF}, int16(-1)},
		{"dword unsigned 0xFFFFFFFF", "dword", false, []byte{0xFF, 0xFF, 0xFF, 0xFF}, uint32(0xFFFFFFFF)},
		{"dword signed 0xFFFFFFFF", "dword", true, []byte{0xFF, 0xFF, 0xFF, 0xFF}, int32(-1)},

		// BYTE
		{"byte unsigned 0xAB", "byte", false, []byte{0xAB}, uint8(0xAB)},
		{"byte signed 0xFF", "byte", true, []byte{0xFF}, int8(-1)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DecodeS7Scalar(c.typ, c.signed, c.bytes)
			if err != nil {
				t.Fatalf("DecodeS7Scalar error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, c.want, c.want)
			}
		})
	}
}

func TestEncodeS7Scalar_OutOfRange(t *testing.T) {
	cases := []struct {
		name   string
		typ    string
		signed bool
		val    any
		hint   string // substring expected in error
	}{
		{"byte unsigned negative", "byte", false, -1, "out of range"},
		{"byte unsigned overflow", "byte", false, 256, "out of range"},
		{"byte signed underflow", "byte", true, -129, "out of range"},
		{"byte signed overflow", "byte", true, 128, "out of range"},
		{"word unsigned negative", "word", false, -1, "out of range"},
		{"word unsigned overflow", "word", false, 65536, "out of range"},
		{"int overflow", "int", false, 32768, "out of range"},
		{"int underflow", "int", false, -32769, "out of range"},
		{"dint overflow", "dint", false, int64(1) << 31, "out of range"},
		{"real NaN", "real", false, math.NaN(), "not finite"},
		{"real +Inf", "real", false, math.Inf(1), "not finite"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeS7Scalar(c.typ, c.signed, c.val)
			if err == nil {
				t.Fatalf("EncodeS7Scalar(%s, %v, %v) succeeded; expected error containing %q", c.typ, c.signed, c.val, c.hint)
			}
			if !strings.Contains(err.Error(), c.hint) {
				t.Errorf("error %q does not contain %q", err.Error(), c.hint)
			}
		})
	}
}

func TestDecodeS7Scalar_BufferTooShort(t *testing.T) {
	cases := []struct {
		typ string
		buf []byte
	}{
		{"byte", []byte{}},
		{"word", []byte{0x00}},
		{"int", []byte{0x00}},
		{"dword", []byte{0x00, 0x00, 0x00}},
		{"dint", []byte{0x00, 0x00, 0x00}},
		{"real", []byte{0x00, 0x00, 0x00}},
		{"counter", []byte{0x00}},
		{"timer", []byte{0x00}},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			_, err := DecodeS7Scalar(c.typ, false, c.buf)
			if err == nil {
				t.Errorf("DecodeS7Scalar(%s, %v) on too-short buffer should error", c.typ, c.buf)
			}
		})
	}
}

func TestDecodeS7Bit(t *testing.T) {
	for bit := 0; bit < 8; bit++ {
		// Byte with only this bit set
		b := byte(1) << uint(bit)
		got, err := DecodeS7Bit(b, bit)
		if err != nil {
			t.Fatalf("bit %d: error: %v", bit, err)
		}
		if !got {
			t.Errorf("bit %d set in 0x%02x but DecodeS7Bit returned false", bit, b)
		}
		// All other bits return false for this byte
		for other := 0; other < 8; other++ {
			if other == bit {
				continue
			}
			got, err := DecodeS7Bit(b, other)
			if err != nil {
				t.Fatalf("bit %d (other): error: %v", other, err)
			}
			if got {
				t.Errorf("bit %d should be clear in 0x%02x but DecodeS7Bit returned true", other, b)
			}
		}
	}
}

func TestDecodeS7Bit_OutOfRange(t *testing.T) {
	if _, err := DecodeS7Bit(0xFF, -1); err == nil {
		t.Error("DecodeS7Bit(_, -1) should error")
	}
	if _, err := DecodeS7Bit(0xFF, 8); err == nil {
		t.Error("DecodeS7Bit(_, 8) should error")
	}
}

func TestEncodeS7Bit(t *testing.T) {
	// Setting bit 3 in 0x00 → 0x08
	got, err := EncodeS7Bit(0x00, 3, true)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != 0x08 {
		t.Errorf("set bit 3 in 0x00: got 0x%02x, want 0x08", got)
	}
	// Clearing bit 3 in 0xFF → 0xF7 (preserves other bits)
	got, err = EncodeS7Bit(0xFF, 3, false)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != 0xF7 {
		t.Errorf("clear bit 3 in 0xFF: got 0x%02x, want 0xF7", got)
	}
	// Setting bit 0 in 0x80 → 0x81 (preserves other bits)
	got, err = EncodeS7Bit(0x80, 0, true)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != 0x81 {
		t.Errorf("set bit 0 in 0x80: got 0x%02x, want 0x81", got)
	}
}

func TestEncodeS7Bit_OutOfRange(t *testing.T) {
	if _, err := EncodeS7Bit(0x00, -1, true); err == nil {
		t.Error("EncodeS7Bit(_, -1, _) should error")
	}
	if _, err := EncodeS7Bit(0x00, 8, true); err == nil {
		t.Error("EncodeS7Bit(_, 8, _) should error")
	}
}

func TestS7String_RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		maxLen int
		input  string
		want   string // what decode should yield (== input unless truncated)
	}{
		{"normal", 20, "LOOPZE-S7-DEMO", "LOOPZE-S7-DEMO"},
		{"empty", 10, "", ""},
		{"exactly maxLen", 5, "HELLO", "HELLO"},
		{"truncated", 5, "TOO LONG", "TOO L"},
		{"single char", 1, "X", "X"},
		{"max maxLen", 254, strings.Repeat("a", 100), strings.Repeat("a", 100)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := EncodeS7String(c.maxLen, c.input)
			if err != nil {
				t.Fatalf("EncodeS7String error: %v", err)
			}
			if len(b) != c.maxLen+2 {
				t.Errorf("encoded length %d, want %d", len(b), c.maxLen+2)
			}
			if int(b[0]) != c.maxLen {
				t.Errorf("header maxLen byte = %d, want %d", b[0], c.maxLen)
			}
			got, err := DecodeS7String(b)
			if err != nil {
				t.Fatalf("DecodeS7String error: %v", err)
			}
			if got != c.want {
				t.Errorf("round-trip got %q, want %q", got, c.want)
			}
		})
	}
}

func TestEncodeS7String_InvalidMaxLen(t *testing.T) {
	for _, maxLen := range []int{0, -1, 255, 1000} {
		_, err := EncodeS7String(maxLen, "x")
		if err == nil {
			t.Errorf("EncodeS7String(maxLen=%d, _) should error", maxLen)
		}
	}
}

func TestDecodeS7String_ExplicitWire(t *testing.T) {
	// Verify the actual S7 STRING wire layout: [maxLen][actLen][chars × maxLen]
	// "LOOPZE-S7-DEMO" with maxLen=20 ⇒ header bytes 20, 14, then 14 ASCII chars,
	// then 6 zero-padded slots.
	wire := []byte{
		20, 14, // header
		'L', 'O', 'O', 'P', 'Z', 'E', '-', 'S', '7', '-', 'D', 'E', 'M', 'O',
		0, 0, 0, 0, 0, 0,
	}
	got, err := DecodeS7String(wire)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "LOOPZE-S7-DEMO" {
		t.Errorf("got %q, want %q", got, "LOOPZE-S7-DEMO")
	}
}

func TestDecodeS7String_CorruptActLenClamps(t *testing.T) {
	// Some misbehaving devices report actLen > maxLen. We clamp to maxLen
	// silently (the upstream node logs a warning).
	wire := []byte{5, 10, 'A', 'B', 'C', 'D', 'E'} // actLen 10 > maxLen 5
	got, err := DecodeS7String(wire)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "ABCDE" {
		t.Errorf("got %q, want %q (clamped to maxLen 5)", got, "ABCDE")
	}
}

func TestDecodeS7Scalar_Counter(t *testing.T) {
	// S7 counter wire format is 2 bytes BCD with an asymmetric layout:
	// low byte holds hundreds, high byte holds tens+units. For value 123:
	//   hundreds=1 → encodeBcd(1)=0x01 → goes into byte 1 (low byte of uint16)
	//   tens+units=23 → encodeBcd(23)=0x23 → goes into byte 0 (high byte)
	// → wire bytes [0x23, 0x01] → uint16 BE = 0x2301 → decode = 123
	got, err := DecodeS7Scalar("counter", false, []byte{0x23, 0x01})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != 123 {
		t.Errorf("counter decode: got %v, want 123", got)
	}
}

func TestDecodeS7Scalar_Timer(t *testing.T) {
	// S5Time encoding (gos7 reference): byte 0 = (timeBase << 4) | hundreds-BCD,
	// byte 1 = tens-BCD-units-BCD. timeBase 0=10ms, 1=100ms, 2=1s, 3=10s.
	// For 5000 ms (5 seconds) using the 100ms time base: ms/100 = 50.
	//   byte 0 = (1<<4) | encodeBcd(0) = 0x10
	//   byte 1 = encodeBcd(50)         = 0x50
	// → buffer [0x10, 0x50] should decode to time.Duration(5000ms)
	got, err := DecodeS7Scalar("timer", false, []byte{0x10, 0x50})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != 5000 {
		t.Errorf("timer decode: got %v ms, want 5000", got)
	}
}

func TestEncodeS7Scalar_CounterAndTimerRejected(t *testing.T) {
	for _, typ := range []string{"counter", "timer"} {
		_, err := EncodeS7Scalar(typ, false, 100)
		if err == nil {
			t.Errorf("EncodeS7Scalar(%s, _, _) should reject (write not supported in v1)", typ)
		}
	}
}

func TestDecodeS7Scalar_UnknownType(t *testing.T) {
	_, err := DecodeS7Scalar("nibble", false, []byte{0x00})
	if err == nil {
		t.Error("DecodeS7Scalar(unknown type) should error")
	}
}

func TestEncodeS7Scalar_UnknownType(t *testing.T) {
	_, err := EncodeS7Scalar("nibble", false, 0)
	if err == nil {
		t.Error("EncodeS7Scalar(unknown type) should error")
	}
}

func TestEncodeS7Scalar_NormalisesType(t *testing.T) {
	// Type names are case- and whitespace-insensitive.
	b, err := EncodeS7Scalar("  REAL  ", false, float32(1.0))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(b) != 4 {
		t.Errorf("encoded length %d, want 4", len(b))
	}
}

func TestS7TypeWordLen(t *testing.T) {
	cases := []struct {
		typ  string
		want int
	}{
		{"bool", S7WLBit},
		{"byte", S7WLByte},
		{"char", S7WLByte},
		{"string", S7WLByte},
		{"raw", S7WLByte},
		{"word", S7WLWord},
		{"int", S7WLWord},
		{"dword", S7WLDWord},
		{"dint", S7WLDWord},
		{"real", S7WLDWord},
		{"counter", S7WLCounter},
		{"timer", S7WLTimer},
		{"unknown", S7WLByte}, // fallback so callers always get a non-zero
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			if got := S7TypeWordLen(c.typ); got != c.want {
				t.Errorf("S7TypeWordLen(%q) = 0x%02x, want 0x%02x", c.typ, got, c.want)
			}
		})
	}
}

func TestS7TypeByteSize(t *testing.T) {
	cases := []struct {
		typ    string
		length int
		want   int
	}{
		{"bool", 0, 1},
		{"byte", 0, 1},
		{"char", 0, 1},
		{"char", 10, 10},
		{"word", 0, 2},
		{"int", 0, 2},
		{"dword", 0, 4},
		{"dint", 0, 4},
		{"real", 0, 4},
		{"counter", 0, 2},
		{"timer", 0, 2},
		{"string", 20, 22}, // [maxLen][actLen][20 chars]
		{"string", 254, 256},
		{"string", 0, 0}, // string with no maxLen returns 0 — caller responsibility
		{"raw", 100, 100},
		{"unknown", 5, 0},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			if got := S7TypeByteSize(c.typ, c.length); got != c.want {
				t.Errorf("S7TypeByteSize(%q, %d) = %d, want %d", c.typ, c.length, got, c.want)
			}
		})
	}
}

func TestS7Codec_ReusesApplyScale(t *testing.T) {
	// Smoke test: ApplyScale lives in modbus_codec.go (package nodes) and is
	// reused by the S7 read pipeline. This test just confirms the function
	// is callable from this file (compile-time guarantee) and behaves as
	// documented.
	got := ApplyScale(int(100), 0.1, 0)
	if f, ok := got.(float64); !ok || math.Abs(f-10.0) > 1e-9 {
		t.Errorf("ApplyScale(100, 0.1, 0) = %v, want 10.0", got)
	}
	un, err := UnapplyScale(10.0, 0.1, 0)
	if err != nil {
		t.Fatalf("UnapplyScale error: %v", err)
	}
	if math.Abs(un-100.0) > 1e-9 {
		t.Errorf("UnapplyScale(10.0, 0.1, 0) = %v, want 100.0", un)
	}
}
