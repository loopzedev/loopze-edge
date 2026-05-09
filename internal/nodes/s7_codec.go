// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/robinson/gos7"
)

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
	case "word":
		v := binary.BigEndian.Uint16(b)
		if signed {
			return int16(v), nil
		}
		return v, nil
	case "int":
		return int16(binary.BigEndian.Uint16(b)), nil
	case "dword":
		v := binary.BigEndian.Uint32(b)
		if signed {
			return int32(v), nil
		}
		return v, nil
	case "dint":
		return int32(binary.BigEndian.Uint32(b)), nil
	case "real":
		return s7Helper.GetRealAt(b, 0), nil
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
		n, err := toInt64(v)
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

	case "word":
		n, err := toInt64(v)
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
		n, err := toInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode int: %w", err)
		}
		if n < math.MinInt16 || n > math.MaxInt16 {
			return nil, fmt.Errorf("encode int: value %d out of range [%d, %d]", n, math.MinInt16, math.MaxInt16)
		}
		out := make([]byte, 2)
		binary.BigEndian.PutUint16(out, uint16(int16(n)))
		return out, nil

	case "dword":
		n, err := toInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode dword: %w", err)
		}
		if signed {
			if n < math.MinInt32 || n > math.MaxInt32 {
				return nil, fmt.Errorf("encode dword (signed): value %d out of range [%d, %d]", n, math.MinInt32, math.MaxInt32)
			}
		} else {
			if n < 0 || n > math.MaxUint32 {
				return nil, fmt.Errorf("encode dword: value %d out of range [0, %d]", n, math.MaxUint32)
			}
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(n))
		return out, nil

	case "dint":
		n, err := toInt64(v)
		if err != nil {
			return nil, fmt.Errorf("encode dint: %w", err)
		}
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("encode dint: value %d out of range [%d, %d]", n, math.MinInt32, math.MaxInt32)
		}
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(int32(n)))
		return out, nil

	case "real":
		f, err := toFloat64(v)
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

	case "counter", "timer":
		return nil, fmt.Errorf("encode %s: writing to counters/timers is not supported in v1 — use a Function node if your CPU model accepts client writes", typ)
	}
	return nil, fmt.Errorf("unsupported S7 scalar type %q", typ)
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

// S7TypeWordLen returns the wire-level WordLen constant for a given dataType.
// `bool` maps to Bit, `string`/`raw` map to Byte (the read happens byte-wise,
// type-specific decode comes later). Unknown types fall back to Byte so the
// caller never gets a zero (which would mean "no read").
func S7TypeWordLen(typ string) int {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "bool":
		return S7WLBit
	case "byte", "char", "string", "raw":
		return S7WLByte
	case "word", "int":
		return S7WLWord
	case "dword", "dint", "real":
		return S7WLDWord
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
	case "bool", "byte":
		return 1
	case "char":
		if length > 1 {
			return length
		}
		return 1
	case "word", "int", "counter", "timer":
		return 2
	case "dword", "dint", "real":
		return 4
	case "string":
		if length < 1 {
			return 0
		}
		return length + 2
	case "raw":
		return length
	}
	return 0
}
