// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package opcua

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua/ua"
)

// CoerceOpcuaValue converts a JSON-decoded Go value (string, float64, bool,
// []any, …) into the concrete Go type expected by ua.NewVariant for the given
// target TypeID. Returns an error with a clear message on type mismatch or
// out-of-range — the Write node turns those into BadTypeMismatch result
// entries with the field path attached.
//
// ExtensionObject coercion is intentionally deferred to Phase 5; a placeholder
// branch returns a stable error so callers know to plug in the schema-aware
// resolver later.
func CoerceOpcuaValue(value any, target ua.TypeID) (any, error) {
	switch target {
	case ua.TypeIDBoolean:
		return coerceBool(value)
	case ua.TypeIDSByte:
		v, err := coerceInt(value, math.MinInt8, math.MaxInt8)
		return int8(v), err
	case ua.TypeIDByte:
		v, err := coerceUint(value, 0, math.MaxUint8)
		return uint8(v), err
	case ua.TypeIDInt16:
		v, err := coerceInt(value, math.MinInt16, math.MaxInt16)
		return int16(v), err
	case ua.TypeIDUint16:
		v, err := coerceUint(value, 0, math.MaxUint16)
		return uint16(v), err
	case ua.TypeIDInt32:
		v, err := coerceInt(value, math.MinInt32, math.MaxInt32)
		return int32(v), err
	case ua.TypeIDUint32:
		v, err := coerceUint(value, 0, math.MaxUint32)
		return uint32(v), err
	case ua.TypeIDInt64:
		v, err := coerceInt(value, math.MinInt64, math.MaxInt64)
		return v, err
	case ua.TypeIDUint64:
		v, err := coerceUint(value, 0, math.MaxUint64)
		return v, err
	case ua.TypeIDFloat:
		v, err := coerceFloat(value)
		if err != nil {
			return float32(0), err
		}
		return float32(v), nil
	case ua.TypeIDDouble:
		return coerceFloat(value)
	case ua.TypeIDString:
		return coerceString(value)
	case ua.TypeIDDateTime:
		return coerceDateTime(value)
	case ua.TypeIDByteString:
		return coerceByteString(value)
	case ua.TypeIDNodeID:
		return coerceNodeID(value)
	case ua.TypeIDExtensionObject:
		// ExtensionObject coercion is delegated: the Write node already has
		// access to the type resolver, so it builds the ExtensionObject
		// itself and bypasses CoerceOpcuaValue. Reaching this branch means
		// the caller did not plumb the resolver — surface that clearly.
		return nil, fmt.Errorf("ExtensionObject coercion requires server context (use OpcuaServer.EncodeStructValue)")
	case ua.TypeIDVariant:
		// Pass-through: ua.NewVariant infers the wire type from the Go value.
		return value, nil
	}
	return nil, fmt.Errorf("unsupported target TypeID %d", target)
}

func coerceBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(x) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
		return false, fmt.Errorf("string %q is not a boolean literal", x)
	case float64:
		return x != 0, nil
	case int:
		return x != 0, nil
	}
	return false, fmt.Errorf("cannot coerce %T to Boolean", v)
}

func coerceInt(v any, min, max int64) (int64, error) {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int32:
		n = int64(x)
	case int64:
		n = x
	case float64:
		if math.Trunc(x) != x {
			return 0, fmt.Errorf("non-integer value %v cannot be coerced to integer", x)
		}
		n = int64(x)
	case bool:
		if x {
			n = 1
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as integer: %w", x, err)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("cannot coerce %T to integer", v)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", n, min, max)
	}
	return n, nil
}

func coerceUint(v any, min, max uint64) (uint64, error) {
	var n uint64
	switch x := v.(type) {
	case int:
		if x < 0 {
			return 0, fmt.Errorf("negative value %d not allowed for unsigned target", x)
		}
		n = uint64(x)
	case int64:
		if x < 0 {
			return 0, fmt.Errorf("negative value %d not allowed for unsigned target", x)
		}
		n = uint64(x)
	case uint64:
		n = x
	case float64:
		if x < 0 || math.Trunc(x) != x {
			return 0, fmt.Errorf("value %v not a non-negative integer", x)
		}
		n = uint64(x)
	case bool:
		if x {
			n = 1
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as unsigned integer: %w", x, err)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("cannot coerce %T to unsigned integer", v)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", n, min, max)
	}
	return n, nil
}

func coerceFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as float: %w", x, err)
		}
		return f, nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("cannot coerce %T to float", v)
}

func coerceString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case int, int32, int64, uint, uint32, uint64:
		return fmt.Sprintf("%d", x), nil
	}
	return "", fmt.Errorf("cannot coerce %T to String", v)
}

func coerceDateTime(v any) (time.Time, error) {
	switch x := v.(type) {
	case time.Time:
		return x, nil
	case string:
		t, err := time.Parse(time.RFC3339Nano, x)
		if err != nil {
			return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339 timestamp: %w", x, err)
		}
		return t, nil
	case float64:
		// Treat plain numbers as Unix milliseconds — easy to feed from inject
		// nodes that emit timestamps as numeric.
		return time.UnixMilli(int64(x)).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("cannot coerce %T to DateTime", v)
}

// coerceByteString accepts:
//   - []byte → direct
//   - string with hex/base64 prefix → decoded bytes
//   - []any of small numbers → byte slice (mirror of mqtt-out's buffer form,
//     so a value round-tripped through Read of a ByteString writes back cleanly)
func coerceByteString(v any) ([]byte, error) {
	switch x := v.(type) {
	case []byte:
		return x, nil
	case string:
		if rest, ok := strings.CutPrefix(x, "0x"); ok {
			b, err := hex.DecodeString(rest)
			if err != nil {
				return nil, fmt.Errorf("invalid hex bytes: %w", err)
			}
			return b, nil
		}
		if b, err := base64.StdEncoding.DecodeString(x); err == nil {
			return b, nil
		}
		return []byte(x), nil
	case []int:
		out := make([]byte, len(x))
		for i, n := range x {
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("byte at index %d out of range: %d", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	case []any:
		out := make([]byte, len(x))
		for i, e := range x {
			n, ok := e.(float64)
			if !ok {
				return nil, fmt.Errorf("byte at index %d is not a number: %T", i, e)
			}
			if n < 0 || n > 255 || math.Trunc(n) != n {
				return nil, fmt.Errorf("byte at index %d out of range: %v", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	}
	return nil, fmt.Errorf("cannot coerce %T to ByteString", v)
}

func coerceNodeID(v any) (*ua.NodeID, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("cannot coerce %T to NodeID (expected string)", v)
	}
	return ParseOpcuaNodeID(s)
}
