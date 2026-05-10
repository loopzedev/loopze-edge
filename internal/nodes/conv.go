// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"math"
)

// Numeric and boolean conversion helpers shared across protocol packages
// (modbus, s7, …). These accept the typical wire formats: native Go ints,
// JSON-derived float64, and a few string forms for booleans. Strings are
// not accepted by the integer/float helpers — text values must be parsed
// upstream before reaching the codec.

// ToInt64 converts numeric and boolean values into int64.
func ToInt64(v any) (int64, error) {
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

// ToUint64 converts numeric values into uint64. Negative values are rejected.
func ToUint64(v any) (uint64, error) {
	switch x := v.(type) {
	case uint64:
		return x, nil
	case float64:
		if x < 0 {
			return 0, fmt.Errorf("negative value %g cannot be uint64", x)
		}
		return uint64(x), nil
	default:
		n, err := ToInt64(v)
		if err != nil {
			return 0, err
		}
		if n < 0 {
			return 0, fmt.Errorf("negative value %d cannot be uint64", n)
		}
		return uint64(n), nil
	}
}

// ToFloat64 converts numeric values into float64.
func ToFloat64(v any) (float64, error) {
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
		n, err := ToInt64(v)
		if err != nil {
			return 0, err
		}
		return float64(n), nil
	}
}

// ToBool converts a single value into bool. Numeric values follow the standard
// non-zero rule. Strings accept "true"/"false"/"1"/"0" (case-insensitive
// for true/false).
func ToBool(v any) (bool, error) {
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

// ToUint16Slice accepts the typical wire formats for register arrays:
// []uint16, []int, []any of numbers, or a single number (treated as one register).
func ToUint16Slice(v any) ([]uint16, error) {
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
			n, err := ToInt64(e)
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
		n, err := ToInt64(v)
		if err != nil {
			return nil, fmt.Errorf("expected register array or number, got %T", v)
		}
		if n < 0 || n > 0xffff {
			return nil, fmt.Errorf("value %d out of uint16 range", n)
		}
		return []uint16{uint16(n)}, nil
	}
}

// ToBoolSlice accepts a single bool, []bool, []any of bools, or a single
// truthy/falsy number/string. Numeric arrays are interpreted bit-by-bit
// (any non-zero element is true).
func ToBoolSlice(v any) ([]bool, error) {
	switch x := v.(type) {
	case bool:
		return []bool{x}, nil
	case []bool:
		return x, nil
	case []any:
		out := make([]bool, len(x))
		for i, e := range x {
			b, err := ToBool(e)
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
		b, err := ToBool(v)
		if err != nil {
			return nil, err
		}
		return []bool{b}, nil
	}
}
