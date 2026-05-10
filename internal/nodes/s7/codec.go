// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/robinson/gos7"
)

// s7DateEpoch is 1990-01-01 UTC — the reference point for S7's DATE type
// (uint16 days offset). All DATE conversions go through this constant.
var s7DateEpoch = time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)

// s7Helper is a stateless gos7 helper. Used for the formats whose bit-tricks
// are non-obvious (REAL via Float32frombits, S7 STRING with the [maxLen][actLen]
// header, BCD counters, S5 timers). For straight scalars we use stdlib
// encoding/binary directly so the codec stays independent of any future gos7
// API churn.
var s7Helper = &gos7.Helper{}

// DecodeS7Scalar decodes wire bytes into a Go value for a non-bit S7 scalar
// type. Bit (`bool`), `string`, and `raw` have their own dedicated functions.
//
// `signed` only applies to `byte`/`char`/`word`/`dword` (it picks signed vs.
// unsigned interpretation). Types with inherent signedness (`int`, `dint`,
// `real`, `counter`, `timer`) ignore the flag.
//
// On a too-short buffer the function returns an error rather than panicking,
// because the buffer comes from a multi-read response where a single per-item
// failure must not crash the whole transaction.
func DecodeS7Scalar(typ string, signed bool, b []byte) (any, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	need := S7TypeByteSize(typ, 0)
	if need == 0 {
		return nil, fmt.Errorf("unsupported S7 scalar type %q", typ)
	}
	if len(b) < need {
		return nil, fmt.Errorf("decode %s: need %d bytes, got %d", typ, need, len(b))
	}
	switch typ {
	case "byte", "char":
		if signed {
			return int8(b[0]), nil
		}
		return uint8(b[0]), nil
	case "sint":
		// SINT is always signed; the explicit type spares users from passing
		// `signed: true` on `byte`. Wire layout identical to a signed BYTE.
		return int8(b[0]), nil
	case "usint":
		// USINT is always unsigned (alias of BYTE without the signed flag).
		return uint8(b[0]), nil
	case "word":
		v := binary.BigEndian.Uint16(b)
		if signed {
			return int16(v), nil
		}
		return v, nil
	case "int":
		return int16(binary.BigEndian.Uint16(b)), nil
	case "uint":
		return binary.BigEndian.Uint16(b), nil
	case "wchar":
		// 16-bit Unicode code point — emit as a 1-char string so users can
		// concatenate with WSTRING fields without case-splitting.
		return string(rune(binary.BigEndian.Uint16(b))), nil
	case "dword":
		v := binary.BigEndian.Uint32(b)
		if signed {
			return int32(v), nil
		}
		return v, nil
	case "dint":
		return int32(binary.BigEndian.Uint32(b)), nil
	case "udint":
		return binary.BigEndian.Uint32(b), nil
	case "real":
		return s7Helper.GetRealAt(b, 0), nil
	case "time":
		// Signed int32 milliseconds. Emitted as plain int — duration math is
		// easier for downstream consumers than parsing an RFC3339 duration.
		return int(int32(binary.BigEndian.Uint32(b))), nil
	case "tod":
		// uint32 milliseconds since midnight (TIME_OF_DAY). Emit as int — same
		// reasoning as TIME.
		return int(binary.BigEndian.Uint32(b)), nil
	case "date":
		// uint16 days since 1990-01-01. Emit as ISO 8601 date string for
		// human-readable debug output. Round-trips through EncodeS7Scalar.
		days := binary.BigEndian.Uint16(b)
		return s7DateEpoch.AddDate(0, 0, int(days)).Format("2006-01-02"), nil
	case "lreal":
		// 64-bit IEEE 754 double-precision, big-endian (S7-1500).
		return s7Helper.GetLRealAt(b, 0), nil
	case "lint":
		// 64-bit signed integer, big-endian, two's complement.
		return int64(binary.BigEndian.Uint64(b)), nil
	case "ulint", "lword":
		// LWORD = 64-bit bitfield (TIA "unsigned long word") with the same
		// wire layout as ULINT — kept as separate label so the parser layout
		// can convey intent ("this is a flag word" vs. "this is a counter").
		return binary.BigEndian.Uint64(b), nil
	case "ltime":
		// Signed int64 nanoseconds.
		return int64(binary.BigEndian.Uint64(b)), nil
	case "ltod":
		// uint64 nanoseconds since midnight.
		return binary.BigEndian.Uint64(b), nil
	case "ldt":
		// Signed int64 nanoseconds since 1970-01-01 UTC. Emit as RFC3339Nano
		// so downstream parsers and humans both get a usable representation.
		ns := int64(binary.BigEndian.Uint64(b))
		return time.Unix(0, ns).UTC().Format(time.RFC3339Nano), nil
	case "dt":
		// 8-byte BCD encoding (year offset, month, day, hour, min, sec, ms[2],
		// ms[1]+weekday). No timezone — interpreted as wall-clock.
		return decodeS7DT(b)
	case "dtl":
		// 12-byte structured DateTime (year u16, month, day, weekday, hour,
		// min, sec, ns u32). No timezone — wall-clock.
		return decodeS7DTL(b)
	case "counter":
		// S7 counter is a 2-byte BCD value with an unusual nibble layout:
		// low byte holds the hundreds digit, high byte holds tens+units. The
		// helper handles the asymmetry — we just hand it the uint16.
		v := binary.BigEndian.Uint16(b)
		return s7Helper.GetCounter(v), nil
	case "timer":
		// S5Time decodes to a time.Duration; we surface it as integer
		// milliseconds because downstream JSON consumers prefer numbers over
		// stringified durations.
		return int(s7Helper.GetS5TimeAt(b, 0).Milliseconds()), nil
	}
	return nil, fmt.Errorf("unsupported S7 scalar type %q", typ)
}

// EncodeS7Scalar is the inverse of DecodeS7Scalar. Out-of-range numeric
// inputs are rejected with a clear "expected X, got Y" error so the calling
// node can surface BadOutOfRange to the upstream catch path.
//
// Counter and Timer encoding are intentionally NOT supported — writing to
// counters/timers from a client is rare in industrial practice (CPU-internal
// state) and the wire encoding is asymmetric / fragile; we keep the door open
// for a v1.x addition once a customer needs it.
func EncodeS7Scalar(typ string, signed bool, v any) ([]byte, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch typ {
	case "byte", "char":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", typ, err)
		}
		if signed {
			if n < math.MinInt8 || n > math.MaxInt8 {
				return nil, fmt.Errorf("encode %s (signed): value %d out of range [%d, %d]", typ, n, math.MinInt8, math.MaxInt8)
			}
		} else {
			if n < 0 || n > math.MaxUint8 {
				return nil, fmt.Errorf("encode %s: value %d out of range [0, %d]", typ, n, math.MaxUint8)
			}
		}
		return []byte{byte(n)}, nil

	case "sint":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode sint: %w", err)
		}
		if n < math.MinInt8 || n > math.MaxInt8 {
			return nil, fmt.Errorf("encode sint: value %d out of range [%d, %d]", n, math.MinInt8, math.MaxInt8)
		}
		return []byte{byte(int8(n))}, nil

	case "usint":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode usint: %w", err)
		}
		if n < 0 || n > math.MaxUint8 {
			return nil, fmt.Errorf("encode usint: value %d out of range [0, %d]", n, math.MaxUint8)
		}
		return []byte{byte(n)}, nil

	case "word":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode word: %w", err)
		}
		if signed {
			if n < math.MinInt16 || n > math.MaxInt16 {
				return nil, fmt.Errorf("encode word (signed): value %d out of range [%d, %d]", n, math.MinInt16, math.MaxInt16)
			}
		} else {
			if n < 0 || n > math.MaxUint16 {
				return nil, fmt.Errorf("encode word: value %d out of range [0, %d]", n, math.MaxUint16)
			}
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(n))
		return out, nil

	case "int":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode int: %w", err)
		}
		if n < math.MinInt16 || n > math.MaxInt16 {
			return nil, fmt.Errorf("encode int: value %d out of range [%d, %d]", n, math.MinInt16, math.MaxInt16)
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(int16(n)))
		return out, nil

	case "uint":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode uint: %w", err)
		}
		if n < 0 || n > math.MaxUint16 {
			return nil, fmt.Errorf("encode uint: value %d out of range [0, %d]", n, math.MaxUint16)
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(n))
		return out, nil

	case "wchar":
		// Take the first rune of the input string and write it as uint16 BE.
		// Callers passing more than one char get a hard error — encoding more
		// would silently lose data, which is worse than failing loudly.
		s, err := toStringValue(v)
		if err != nil {
			return nil, fmt.Errorf("encode wchar: %w", err)
		}
		runes := []rune(s)
		if len(runes) == 0 {
			return nil, fmt.Errorf("encode wchar: empty string")
		}
		if len(runes) > 1 {
			return nil, fmt.Errorf("encode wchar: input %q has %d chars; WCHAR holds exactly 1 (use WSTRING for multi-char strings)", s, len(runes))
		}
		if runes[0] > 0xFFFF {
			return nil, fmt.Errorf("encode wchar: code point U+%04X exceeds BMP — WCHAR is UCS-2 only", runes[0])
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(runes[0]))
		return out, nil

	case "dword":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode dword: %w", err)
		}
		if signed {
			if n < math.MinInt32 || n > math.MaxInt32 {
				return nil, fmt.Errorf("encode dword (signed): value %d out of range [%d, %d]", n, math.MinInt32, math.MaxInt32)
			}
		} else {
			if n < 0 || n > math.MaxUint32 {
				return nil, fmt.Errorf("encode dword: value %d out of range [0, %d]", n, uint32(math.MaxUint32))
			}
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(n))
		return out, nil

	case "dint":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode dint: %w", err)
		}
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("encode dint: value %d out of range [%d, %d]", n, math.MinInt32, math.MaxInt32)
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(int32(n)))
		return out, nil

	case "udint":
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode udint: %w", err)
		}
		if n < 0 || n > math.MaxUint32 {
			return nil, fmt.Errorf("encode udint: value %d out of range [0, %d]", n, uint32(math.MaxUint32))
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(n))
		return out, nil

	case "time":
		// Signed int32 milliseconds. JSON arrives as float64 → toInt64 caps
		// precision at 2^53 ms (~9 quintillion ms), well above the int32
		// range, so range-check after conversion.
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode time: %w", err)
		}
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("encode time: value %d ms out of int32 range (TIME is signed 32-bit)", n)
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(int32(n)))
		return out, nil

	case "tod":
		// uint32 ms since midnight (TIME_OF_DAY). 24h in ms = 86_400_000;
		// values beyond that are nonsensical but the wire allows them, so
		// only flag truly out-of-uint32 cases.
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode tod: %w", err)
		}
		if n < 0 || n > math.MaxUint32 {
			return nil, fmt.Errorf("encode tod: value %d ms out of uint32 range", n)
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(n))
		return out, nil

	case "date":
		// Accept ISO date string ("2026-05-10") OR raw days as integer.
		// Compute days since 1990-01-01. uint16 caps at 2168-12-31.
		days, err := s7DateInputDays(v)
		if err != nil {
			return nil, fmt.Errorf("encode date: %w", err)
		}
		if days < 0 || days > math.MaxUint16 {
			return nil, fmt.Errorf("encode date: %d days outside [0, 65535] (= 1990-01-01..2168-12-31)", days)
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(days))
		return out, nil

	case "real":
		f, err := nodes.ToFloat64(v)
		if err != nil {
			return nil, fmt.Errorf("encode real: %w", err)
		}
		// Range check — IEEE 754 single-precision saturates to ±Inf above
		// ~3.4e38, which the user never wants silently.
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("encode real: value %v is not finite", f)
		}
		if math.Abs(f) > math.MaxFloat32 {
			return nil, fmt.Errorf("encode real: value %v out of float32 range", f)
		}
		out := make([]byte, 4)
		s7Helper.SetRealAt(out, 0, float32(f))
		return out, nil

	case "lreal":
		f, err := nodes.ToFloat64(v)
		if err != nil {
			return nil, fmt.Errorf("encode lreal: %w", err)
		}
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("encode lreal: value %v is not finite", f)
		}
		out := make([]byte, 8)
		s7Helper.SetLRealAt(out, 0, f)
		return out, nil

	case "lint":
		// LINT is signed int64 — toInt64 already returns int64, full range fits.
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode lint: %w", err)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(n))
		return out, nil

	case "ulint":
		// ULINT is unsigned int64. Direct uint64 callers get the full range;
		// JSON-decoded values arrive as float64 and are limited to 2^53 by
		// IEEE-754 precision (which is 9 quadrillion — adequate for industry).
		if u, ok := v.(uint64); ok {
			out := make([]byte, 8)
			binary.BigEndian.PutUint64(out, u)
			return out, nil
		}
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode ulint: %w", err)
		}
		if n < 0 {
			return nil, fmt.Errorf("encode ulint: value %d is negative (ULINT is unsigned)", n)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(n))
		return out, nil

	case "lword":
		// Wire-equivalent to ULINT. Same rules: prefer uint64 when the caller
		// hands a raw value, otherwise route through int64 with a sign check.
		if u, ok := v.(uint64); ok {
			out := make([]byte, 8)
			binary.BigEndian.PutUint64(out, u)
			return out, nil
		}
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode lword: %w", err)
		}
		if n < 0 {
			return nil, fmt.Errorf("encode lword: value %d is negative (LWORD is unsigned)", n)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(n))
		return out, nil

	case "ltime":
		// Signed int64 nanoseconds. JSON arrives as float64 → toInt64 caps at
		// 2^53 ns (~104 days) which is plenty for industrial duration math; if
		// callers need the full LTIME range they pass an int64 directly.
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode ltime: %w", err)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(n))
		return out, nil

	case "ltod":
		// uint64 nanoseconds since midnight. Negative values are invalid.
		if u, ok := v.(uint64); ok {
			out := make([]byte, 8)
			binary.BigEndian.PutUint64(out, u)
			return out, nil
		}
		n, err := nodes.ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode ltod: %w", err)
		}
		if n < 0 {
			return nil, fmt.Errorf("encode ltod: value %d ns is negative (LTOD is unsigned ns since midnight)", n)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(n))
		return out, nil

	case "ldt":
		// RFC3339 string in, int64 ns since 1970-01-01 UTC out. We accept the
		// full RFC3339Nano subset that Go's time.Parse handles.
		t, err := s7ParseRFC3339(v)
		if err != nil {
			return nil, fmt.Errorf("encode ldt: %w", err)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint64(out, uint64(t.UnixNano()))
		return out, nil

	case "dt":
		// 8-byte BCD wire layout. Accept RFC3339 string for consistency with
		// LDT; the wall-clock/UTC distinction is documented.
		t, err := s7ParseRFC3339(v)
		if err != nil {
			return nil, fmt.Errorf("encode dt: %w", err)
		}
		return encodeS7DT(t), nil

	case "dtl":
		// 12-byte structured layout. Same input contract as DT/LDT.
		t, err := s7ParseRFC3339(v)
		if err != nil {
			return nil, fmt.Errorf("encode dtl: %w", err)
		}
		return encodeS7DTL(t), nil

	case "counter", "timer":
		return nil, fmt.Errorf("encode %s: writing to counters/timers is not supported in v1 — use a Function node if your CPU model accepts client writes", typ)
	}
	return nil, fmt.Errorf("unsupported S7 scalar type %q", typ)
}

// s7DateInputDays accepts either an ISO-8601 date string ("2026-05-10") or an
// integer number of days since 1990-01-01, returning the day offset. Used by
// the DATE encoder so Function nodes can supply whichever form is natural.
func s7DateInputDays(v any) (int, error) {
	if s, ok := v.(string); ok {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
		if err != nil {
			return 0, fmt.Errorf("date string %q: %w (expected YYYY-MM-DD)", s, err)
		}
		return int(t.Sub(s7DateEpoch).Hours() / 24), nil
	}
	n, err := nodes.ToInt64(v)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// s7ParseRFC3339 accepts an RFC3339 / RFC3339Nano string OR a time.Time and
// returns the time in UTC. Used by LDT/DT/DTL encoders so they share one input
// contract.
func s7ParseRFC3339(v any) (time.Time, error) {
	if t, ok := v.(time.Time); ok {
		return t.UTC(), nil
	}
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("expected RFC3339 string, got %T", v)
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse %q: %w (expected RFC3339, e.g. 2026-05-10T12:34:56Z)", s, err)
	}
	return t.UTC(), nil
}

// decodeS7DT decodes the 8-byte BCD DateTime format. Layout:
//
//	[year][month][day][hour][min][sec][ms_high+ms_mid][ms_low+weekday]
//
// Year is BCD with the convention <90 means 20xx, ≥90 means 19xx (Siemens
// "century rollover at 1990"). The milliseconds field spans 12 bits across
// the last two bytes; the low nibble of the last byte holds the weekday
// (1=Sunday) which we discard since RFC3339 already encodes it implicitly.
func decodeS7DT(b []byte) (string, error) {
	bcd := func(byt byte) int { return int(byt>>4)*10 + int(byt&0x0F) }
	yy := bcd(b[0])
	year := 2000 + yy
	if yy >= 90 {
		year = 1900 + yy
	}
	month := bcd(b[1])
	day := bcd(b[2])
	hour := bcd(b[3])
	min := bcd(b[4])
	sec := bcd(b[5])
	// ms = first byte (BCD hundreds+tens) * 10 + (high nibble of byte 7)
	msHigh := bcd(b[6])
	msLow := int(b[7] >> 4)
	ms := msHigh*10 + msLow
	t := time.Date(year, time.Month(month), day, hour, min, sec, ms*int(time.Millisecond), time.UTC)
	return t.Format(time.RFC3339Nano), nil
}

// encodeS7DT writes the 8-byte BCD DateTime format. Weekday nibble follows
// Siemens convention: 1=Sunday … 7=Saturday.
func encodeS7DT(t time.Time) []byte {
	bcd := func(n int) byte { return byte((n/10)<<4 | (n % 10)) }
	t = t.UTC()
	year := t.Year()
	yy := year % 100
	ms := t.Nanosecond() / int(time.Millisecond)
	msHigh := ms / 10
	msLow := ms % 10
	weekday := int(t.Weekday()) + 1 // Go: Sunday=0; Siemens: Sunday=1
	out := make([]byte, 8)
	out[0] = bcd(yy)
	out[1] = bcd(int(t.Month()))
	out[2] = bcd(t.Day())
	out[3] = bcd(t.Hour())
	out[4] = bcd(t.Minute())
	out[5] = bcd(t.Second())
	out[6] = bcd(msHigh)
	out[7] = byte((msLow << 4) | (weekday & 0x0F))
	return out
}

// decodeS7DTL decodes the 12-byte structured DateTime. Layout:
//
//	[year u16 BE][month u8][day u8][weekday u8][hour u8][min u8][sec u8][ns u32 BE]
//
// Weekday: 1=Sunday … 7=Saturday (informational; we recompute from the date
// and ignore whatever the PLC sent).
func decodeS7DTL(b []byte) (string, error) {
	year := int(binary.BigEndian.Uint16(b[0:2]))
	month := int(b[2])
	day := int(b[3])
	// b[4] = weekday (ignored)
	hour := int(b[5])
	min := int(b[6])
	sec := int(b[7])
	ns := int(binary.BigEndian.Uint32(b[8:12]))
	t := time.Date(year, time.Month(month), day, hour, min, sec, ns, time.UTC)
	return t.Format(time.RFC3339Nano), nil
}

// encodeS7DTL writes the 12-byte structured DateTime.
func encodeS7DTL(t time.Time) []byte {
	t = t.UTC()
	out := make([]byte, 12)
	binary.BigEndian.PutUint16(out[0:2], uint16(t.Year()))
	out[2] = byte(t.Month())
	out[3] = byte(t.Day())
	out[4] = byte(int(t.Weekday()) + 1) // Go: Sunday=0; Siemens: Sunday=1
	out[5] = byte(t.Hour())
	out[6] = byte(t.Minute())
	out[7] = byte(t.Second())
	binary.BigEndian.PutUint32(out[8:12], uint32(t.Nanosecond()))
	return out
}

// DecodeS7Bit extracts a single bit from a byte. The address parser fixes the
// wire access at byte boundaries (1 byte ships from the PLC even for a bit
// read), so the read node calls this with the byte it just received and the
// declared bit position.
func DecodeS7Bit(b byte, bitPos int) (bool, error) {
	if bitPos < 0 || bitPos > 7 {
		return false, fmt.Errorf("decode bool: bit %d outside [0, 7]", bitPos)
	}
	return (b>>uint(bitPos))&1 == 1, nil
}

// EncodeS7Bit returns the input byte with the named bit set to `value`. The
// other bits are left untouched — important for read-modify-write patterns
// where the caller fetched the byte first to preserve neighbouring bits.
func EncodeS7Bit(b byte, bitPos int, value bool) (byte, error) {
	if bitPos < 0 || bitPos > 7 {
		return 0, fmt.Errorf("encode bool: bit %d outside [0, 7]", bitPos)
	}
	mask := byte(1) << uint(bitPos)
	if value {
		return b | mask, nil
	}
	return b &^ mask, nil
}

// DecodeS7String decodes a buffer in the S7 STRING wire format
//
//	[maxLen][actLen][char × maxLen]
//
// into a Go string of length actLen. The maxLen byte is read for sanity (a
// corrupt value where actLen > maxLen is clamped to maxLen and reported back
// via a logged warning at a higher layer — this function silently clamps and
// returns the trimmed string so the read pipeline doesn't fail on a single
// mis-encoded item).
//
// The input buffer must be at least `maxLen + 2` bytes — typically the read
// node passes exactly that based on the address (`DB1.STRING50.20` → 22 B).
func DecodeS7String(b []byte) (string, error) {
	if len(b) < 2 {
		return "", fmt.Errorf("decode string: need ≥2 header bytes, got %d", len(b))
	}
	maxLen := int(b[0])
	actLen := int(b[1])
	if actLen > maxLen {
		actLen = maxLen
	}
	if 2+actLen > len(b) {
		return "", fmt.Errorf("decode string: actLen %d exceeds buffer size %d", actLen, len(b))
	}
	return string(b[2 : 2+actLen]), nil
}

// EncodeS7String produces a buffer in the S7 STRING wire format. The output
// is exactly `maxLen + 2` bytes — header + maxLen-sized character slot, with
// the unused tail zero-padded.
//
// Strings longer than maxLen are truncated; the truncation is silent here
// because the codec has no logger — the caller (read/write/parser node) sets
// a once-per-lifetime warning when it observes the truncation.
func EncodeS7String(maxLen int, s string) ([]byte, error) {
	if maxLen < 1 || maxLen > 254 {
		return nil, fmt.Errorf("encode string: maxLen %d outside [1, 254]", maxLen)
	}
	out := make([]byte, maxLen+2)
	s7Helper.SetStringAt(out, 0, maxLen, s)
	return out, nil
}

// DecodeS7WString decodes a buffer in the S7 WSTRING wire format
//
//	[maxLen u16][actLen u16][char × maxLen × u16]
//
// into a Go string. WSTRING characters are UCS-2 (16-bit big-endian per char),
// so the codec handles the BMP (Basic Multilingual Plane) — code points above
// U+FFFF would be lost on encode, but those are exceedingly rare in industrial
// text data (machine names, recipe IDs, error messages).
//
// The input buffer must be at least `4 + 2*maxLen` bytes — typically the read
// node passes exactly that based on the address (`DB1.WSTRING50.20` → 44 B).
func DecodeS7WString(b []byte) (string, error) {
	if len(b) < 4 {
		return "", fmt.Errorf("decode wstring: need ≥4 header bytes, got %d", len(b))
	}
	maxLen := int(binary.BigEndian.Uint16(b[0:2]))
	actLen := int(binary.BigEndian.Uint16(b[2:4]))
	if actLen > maxLen {
		actLen = maxLen
	}
	if 4+actLen*2 > len(b) {
		return "", fmt.Errorf("decode wstring: actLen %d exceeds buffer size %d", actLen, len(b))
	}
	runes := make([]rune, actLen)
	for i := 0; i < actLen; i++ {
		runes[i] = rune(binary.BigEndian.Uint16(b[4+i*2:]))
	}
	return string(runes), nil
}

// EncodeS7WString produces a buffer in the S7 WSTRING wire format. The output
// is exactly `4 + 2*maxLen` bytes — header (maxLen, actLen) + maxLen-sized
// character slot, with the unused tail zero-padded.
//
// Siemens documents WSTRING max length as 16382 chars (32764 bytes payload +
// 4 bytes header = 32768 = 2^15). Strings longer than maxLen are truncated
// silently; the caller logs a warning if it cares.
//
// `len()` of the input string operates on UTF-8 bytes; we convert to runes
// up-front so the count maps to character positions, not bytes.
func EncodeS7WString(maxLen int, s string) ([]byte, error) {
	if maxLen < 1 || maxLen > 16382 {
		return nil, fmt.Errorf("encode wstring: maxLen %d outside [1, 16382]", maxLen)
	}
	out := make([]byte, 4+maxLen*2)
	binary.BigEndian.PutUint16(out[0:2], uint16(maxLen))

	runes := []rune(s)
	actLen := len(runes)
	if actLen > maxLen {
		actLen = maxLen
	}
	binary.BigEndian.PutUint16(out[2:4], uint16(actLen))
	for i := 0; i < actLen; i++ {
		// UCS-2: take the BMP code point. Surrogate pairs (above U+FFFF) get
		// truncated to their low 16 bits — same as gos7's SetWStringAt.
		binary.BigEndian.PutUint16(out[4+i*2:], uint16(runes[i]))
	}
	return out, nil
}

// S7TypeWordLen returns the wire-level WordLen constant for a given dataType.
// `bool` maps to Bit, `string`/`raw` map to Byte (the read happens byte-wise,
// type-specific decode comes later). Unknown types fall back to Byte so the
// caller never gets a zero (which would mean "no read").
func S7TypeWordLen(typ string) int {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "bool":
		return S7WLBit
	case "byte", "char", "sint", "usint", "string", "raw", "wstring":
		return S7WLByte
	case "word", "int", "uint", "wchar", "date":
		return S7WLWord
	case "dword", "dint", "udint", "real", "time", "tod":
		return S7WLDWord
	case "lreal", "lint", "ulint", "lword", "ltime", "ltod", "ldt", "dt":
		// 8-byte types — S7 protocol has no native 8-byte WordLen, so we read
		// them as 8 bytes (Amount=8, WordLen=Byte). Set by ParseS7Address /
		// the parser node when constructing items.
		return S7WLByte
	case "dtl":
		// 12 bytes — read as a Byte block, decoded by the codec.
		return S7WLByte
	case "counter":
		return S7WLCounter
	case "timer":
		return S7WLTimer
	}
	return S7WLByte
}

// S7TypeByteSize returns the on-wire byte count for one element of the given
// dataType. For variable-length types (`string`, `raw`, `char` arrays) the
// caller passes a non-zero `length`; for scalars the `length` parameter is
// ignored.
//
// `string` returns `length + 2` to account for the [maxLen][actLen] header.
// `raw` and multi-`char` return exactly `length`. `bool` returns 1 because
// even a single bit ships as a full byte from the PLC.
func S7TypeByteSize(typ string, length int) int {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "bool", "byte", "sint", "usint":
		return 1
	case "char":
		if length > 1 {
			return length
		}
		return 1
	case "word", "int", "uint", "wchar", "date", "counter", "timer":
		return 2
	case "dword", "dint", "udint", "real", "time", "tod":
		return 4
	case "lreal", "lint", "ulint", "lword", "ltime", "ltod", "ldt", "dt":
		return 8
	case "dtl":
		return 12
	case "string":
		if length < 1 {
			return 0
		}
		return length + 2
	case "wstring":
		// [maxLen u16][actLen u16][chars × maxLen × u16]
		if length < 1 {
			return 0
		}
		return 4 + length*2
	case "raw":
		return length
	}
	return 0
}
