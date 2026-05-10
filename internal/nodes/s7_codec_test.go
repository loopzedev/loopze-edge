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

		// 64-bit S7-1500 types.
		{"lreal zero", "lreal", false, float64(0)},
		{"lreal one", "lreal", false, float64(1)},
		{"lreal pi", "lreal", false, float64(3.141592653589793)},
		{"lreal large", "lreal", false, float64(1e300)},
		{"lreal small", "lreal", false, float64(1e-300)},
		{"lreal negative", "lreal", false, float64(-2.718281828459045)},

		{"lint zero", "lint", false, int64(0)},
		{"lint positive", "lint", false, int64(9_000_000_000_000)},
		{"lint negative", "lint", false, int64(-9_000_000_000_000)},
		{"lint min", "lint", false, int64(-1) << 62}, // -4.6e18, well inside int64
		{"lint max", "lint", false, (int64(1) << 62) - 1},

		{"ulint zero", "ulint", false, uint64(0)},
		{"ulint mid", "ulint", false, uint64(0xCAFEBABEDEADBEEF)},
		{"ulint max", "ulint", false, ^uint64(0)},
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

func TestS7WString_RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		maxLen int
		input  string
		want   string
	}{
		{"ascii", 20, "LOOPZE-S7", "LOOPZE-S7"},
		{"empty", 10, "", ""},
		{"exact maxLen", 5, "HELLO", "HELLO"},
		{"truncated", 5, "TOO LONG", "TOO L"},
		{"single", 1, "X", "X"},
		// BMP characters: umlauts and a Greek letter.
		{"non-ascii BMP", 10, "Größe-π", "Größe-π"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := EncodeS7WString(c.maxLen, c.input)
			if err != nil {
				t.Fatalf("EncodeS7WString error: %v", err)
			}
			if len(b) != 4+c.maxLen*2 {
				t.Errorf("encoded length %d, want %d", len(b), 4+c.maxLen*2)
			}
			got, err := DecodeS7WString(b)
			if err != nil {
				t.Fatalf("DecodeS7WString error: %v", err)
			}
			if got != c.want {
				t.Errorf("round-trip got %q, want %q", got, c.want)
			}
		})
	}
}

func TestEncodeS7WString_InvalidMaxLen(t *testing.T) {
	for _, maxLen := range []int{0, -1, 16383, 100000} {
		_, err := EncodeS7WString(maxLen, "x")
		if err == nil {
			t.Errorf("EncodeS7WString(maxLen=%d, _) should error", maxLen)
		}
	}
}

func TestDecodeS7WString_ExplicitWire(t *testing.T) {
	// Build the wire bytes for a 5-char WSTRING containing "Hi !".
	// Layout: maxLen(u16) | actLen(u16) | char × 5 × u16
	// For maxLen=5, actLen=4, chars [H, i, ' ', !]:
	//   00 05 | 00 04 | 00 48 | 00 69 | 00 20 | 00 21 | 00 00 (zero-padded last)
	wire := []byte{
		0x00, 0x05, // maxLen = 5
		0x00, 0x04, // actLen = 4
		0x00, 'H',
		0x00, 'i',
		0x00, ' ',
		0x00, '!',
		0x00, 0x00, // unused slot zero-padded
	}
	got, err := DecodeS7WString(wire)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "Hi !" {
		t.Errorf("got %q, want %q", got, "Hi !")
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
		{"wstring", S7WLByte},
		{"raw", S7WLByte},
		{"word", S7WLWord},
		{"int", S7WLWord},
		{"dword", S7WLDWord},
		{"dint", S7WLDWord},
		{"real", S7WLDWord},
		{"lreal", S7WLByte}, // 8-byte types ride on Byte+Amount=8
		{"lint", S7WLByte},
		{"ulint", S7WLByte},
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
		{"lreal", 0, 8},
		{"lint", 0, 8},
		{"ulint", 0, 8},
		{"counter", 0, 2},
		{"timer", 0, 2},
		{"string", 20, 22},   // [maxLen][actLen][20 chars]
		{"string", 254, 256},
		{"string", 0, 0},     // string with no maxLen returns 0 — caller responsibility
		{"wstring", 20, 44},  // 4-byte header + 20*2 chars
		{"wstring", 100, 204},
		{"wstring", 0, 0},
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

// TestDecodeS7Scalar_NewTypes covers the TIA datatypes added beyond the
// original LREAL/LINT/ULINT set: integer aliases, durations, and date/time.
// Round-trips that survive Encode → Decode use the inputs the user actually
// supplies (strings for date/dt/ldt/dtl/wchar, ints for time/tod/etc.).
func TestDecodeS7Scalar_NewTypes_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		val  any
		want any // optional override when round-trip type differs from input
	}{
		{"sint positive", "sint", int8(100), nil},
		{"sint negative", "sint", int8(-50), nil},
		{"sint min", "sint", int8(-128), nil},
		{"usint zero", "usint", uint8(0), nil},
		{"usint max", "usint", uint8(255), nil},
		{"uint zero", "uint", uint16(0), nil},
		{"uint mid", "uint", uint16(50_000), nil},
		{"uint max", "uint", uint16(65_535), nil},
		{"udint zero", "udint", uint32(0), nil},
		{"udint mid", "udint", uint32(4_000_000_000), nil},
		{"udint max", "udint", uint32(0xFFFFFFFF), nil},
		{"lword zero", "lword", uint64(0), nil},
		{"lword mid", "lword", uint64(0xFEEDFACECAFEBEEF), nil},
		{"lword max", "lword", ^uint64(0), nil},

		// Time scalars surface as int / int64.
		{"time positive", "time", 93_784_567, nil},
		{"time negative", "time", -1_000_000, nil},
		{"tod midday", "tod", 45_296_789, nil},
		{"tod zero", "tod", 0, nil},
		{"ltime ns", "ltime", int64(123_456_789_012_345), nil},
		{"ltime negative", "ltime", int64(-1_000_000_000), nil},
		{"ltod ns", "ltod", uint64(45_296_123_456_789), nil},

		// Char-based types decode back as 1-character strings.
		{"wchar ascii", "wchar", "A", nil},
		{"wchar greek", "wchar", "Ω", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := EncodeS7Scalar(c.typ, false, c.val)
			if err != nil {
				t.Fatalf("EncodeS7Scalar(%s, %v) error: %v", c.typ, c.val, err)
			}
			got, err := DecodeS7Scalar(c.typ, false, b)
			if err != nil {
				t.Fatalf("DecodeS7Scalar(%s, %v) error: %v", c.typ, b, err)
			}
			want := c.want
			if want == nil {
				want = c.val
			}
			if got != want {
				t.Errorf("round-trip %s: got %v (%T), want %v (%T)", c.typ, got, got, want, want)
			}
		})
	}
}

// TestDecodeS7Scalar_DateTimeRoundTrip checks the string-shaped time types.
// Encode accepts an ISO date / RFC3339 timestamp; decode returns the
// canonical representation that may differ in formatting (e.g. always UTC).
func TestDecodeS7Scalar_DateTimeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		in   string
		want string
	}{
		{"date", "date", "2026-05-10", "2026-05-10"},
		{"date epoch", "date", "1990-01-01", "1990-01-01"},
		{"dt midday", "dt", "2026-05-10T12:34:56.789Z", "2026-05-10T12:34:56.789Z"},
		{"ldt nanos", "ldt", "2026-05-10T12:34:56.123456789Z", "2026-05-10T12:34:56.123456789Z"},
		{"dtl nanos", "dtl", "2026-05-10T12:34:56.123456789Z", "2026-05-10T12:34:56.123456789Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := EncodeS7Scalar(c.typ, false, c.in)
			if err != nil {
				t.Fatalf("EncodeS7Scalar(%s, %q) error: %v", c.typ, c.in, err)
			}
			got, err := DecodeS7Scalar(c.typ, false, b)
			if err != nil {
				t.Fatalf("DecodeS7Scalar(%s) error: %v", c.typ, err)
			}
			if got != c.want {
				t.Errorf("round-trip %s: got %v, want %v", c.typ, got, c.want)
			}
		})
	}
}

// TestEncodeS7Scalar_NewTypes_OutOfRange exercises the per-type range checks
// the encoders apply before writing to the wire.
func TestEncodeS7Scalar_NewTypes_OutOfRange(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		val  any
		hint string
	}{
		{"sint underflow", "sint", -129, "out of range"},
		{"sint overflow", "sint", 128, "out of range"},
		{"usint negative", "usint", -1, "out of range"},
		{"usint overflow", "usint", 256, "out of range"},
		{"uint negative", "uint", -1, "out of range"},
		{"uint overflow", "uint", 65536, "out of range"},
		{"udint negative", "udint", -1, "out of range"},
		{"udint overflow", "udint", int64(1) << 32, "out of range"},
		{"time overflow", "time", int64(1) << 31, "out of int32"},
		{"tod negative", "tod", -1, "out of uint32"},
		{"lword negative", "lword", -1, "is negative"},
		{"ltod negative", "ltod", -1, "is negative"},
		{"wchar empty", "wchar", "", "empty"},
		{"wchar multi", "wchar", "AB", "exactly 1"},
		{"date bad string", "date", "not-a-date", "expected YYYY-MM-DD"},
		{"ldt bad string", "ldt", "not-a-timestamp", "expected RFC3339"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := EncodeS7Scalar(c.typ, false, c.val)
			if err == nil {
				t.Fatalf("EncodeS7Scalar(%s, %v) succeeded; expected error containing %q", c.typ, c.val, c.hint)
			}
			if !strings.Contains(err.Error(), c.hint) {
				t.Errorf("error %q does not contain %q", err.Error(), c.hint)
			}
		})
	}
}

// TestDecodeS7Scalar_DTExplicitWire verifies the BCD layout of DT against a
// hand-computed sample. The PLC ships these bytes; we must decode them
// back to the documented timestamp.
func TestDecodeS7Scalar_DTExplicitWire(t *testing.T) {
	// 2026-05-10 12:34:56.789, weekday Sunday (1).
	// BCD: yy=26 → 0x26, month=05 → 0x05, day=10 → 0x10,
	//      hour=12 → 0x12, min=34 → 0x34, sec=56 → 0x56,
	//      ms_high=78 → 0x78, ms_low_nibble=9, weekday=1 → 0x91
	wire := []byte{0x26, 0x05, 0x10, 0x12, 0x34, 0x56, 0x78, 0x91}
	got, err := DecodeS7Scalar("dt", false, wire)
	if err != nil {
		t.Fatalf("DecodeS7Scalar(dt) error: %v", err)
	}
	want := "2026-05-10T12:34:56.789Z"
	if got != want {
		t.Errorf("DT decode: got %v, want %v", got, want)
	}
}

// TestDecodeS7Scalar_DTLExplicitWire mirrors the DT test for the structured
// 12-byte DTL layout.
func TestDecodeS7Scalar_DTLExplicitWire(t *testing.T) {
	// 2026-05-10 12:34:56.123_456_789, year 2026 (BE u16 = 0x07EA),
	// month 5, day 10, weekday 1 (Sunday), hour 12, min 34, sec 56,
	// ns = 123_456_789 → 0x075BCD15
	wire := []byte{
		0x07, 0xEA, // year
		0x05,       // month
		0x0A,       // day
		0x01,       // weekday
		0x0C,       // hour
		0x22,       // min
		0x38,       // sec
		0x07, 0x5B, 0xCD, 0x15, // ns
	}
	got, err := DecodeS7Scalar("dtl", false, wire)
	if err != nil {
		t.Fatalf("DecodeS7Scalar(dtl) error: %v", err)
	}
	want := "2026-05-10T12:34:56.123456789Z"
	if got != want {
		t.Errorf("DTL decode: got %v, want %v", got, want)
	}
}

// TestS7TypeWordLen_NewTypes makes sure the new aliases land in the right
// wire-length bucket. Drift here cascades into AGReadMulti misreads.
func TestS7TypeWordLen_NewTypes(t *testing.T) {
	cases := []struct {
		typ  string
		want int
	}{
		{"sint", S7WLByte}, {"usint", S7WLByte},
		{"uint", S7WLWord}, {"wchar", S7WLWord}, {"date", S7WLWord},
		{"udint", S7WLDWord}, {"time", S7WLDWord}, {"tod", S7WLDWord},
		{"lword", S7WLByte}, {"ltime", S7WLByte}, {"ltod", S7WLByte},
		{"ldt", S7WLByte}, {"dt", S7WLByte}, {"dtl", S7WLByte},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			if got := S7TypeWordLen(c.typ); got != c.want {
				t.Errorf("S7TypeWordLen(%q) = 0x%02x, want 0x%02x", c.typ, got, c.want)
			}
		})
	}
}

func TestS7TypeByteSize_NewTypes(t *testing.T) {
	cases := []struct {
		typ  string
		want int
	}{
		{"sint", 1}, {"usint", 1},
		{"uint", 2}, {"wchar", 2}, {"date", 2},
		{"udint", 4}, {"time", 4}, {"tod", 4},
		{"lword", 8}, {"ltime", 8}, {"ltod", 8}, {"ldt", 8}, {"dt", 8},
		{"dtl", 12},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			if got := S7TypeByteSize(c.typ, 0); got != c.want {
				t.Errorf("S7TypeByteSize(%q, 0) = %d, want %d", c.typ, got, c.want)
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
