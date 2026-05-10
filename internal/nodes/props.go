// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

// Property-map readers used by node Init() implementations to pull strongly
// typed values out of the JSON-decoded property bag. All helpers return the
// supplied fallback when the key is missing, the value is nil, or the value
// has the wrong type — by design Init() should never fail for an unset
// property; defaults take over.

// StringVal returns m[key] if it's a non-empty string, otherwise fallback.
func StringVal(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

// IntVal returns m[key] coerced to int. JSON-decoded numerics arrive as
// float64; native callers may pass int / int64 directly. Non-numeric or
// missing values fall back.
func IntVal(m map[string]any, key string, fallback int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return fallback
}

// FloatVal returns m[key] coerced to float64.
func FloatVal(m map[string]any, key string, fallback float64) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}

// BoolVal returns m[key] coerced to bool. Only an actual bool counts —
// numeric truthiness is not applied here so that misconfigurations don't
// silently turn into a "true" via a stray non-zero default.
func BoolVal(m map[string]any, key string, fallback bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

// IsMapInput reports whether the value looks like a structured object (the
// encode-side input shape for parser nodes).
func IsMapInput(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// NormaliseMapInput converts the wire value into a map[string]any, accepting
// the typical JSON-decoded shape directly.
func NormaliseMapInput(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// ReadPositiveInt parses a positive integer from typical wire formats. Returns
// ok=false for absent, zero, or negative values.
func ReadPositiveInt(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		if x <= 0 {
			return 0, false
		}
		return int(x), true
	case int:
		if x <= 0 {
			return 0, false
		}
		return x, true
	default:
		return 0, false
	}
}
