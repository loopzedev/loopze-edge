// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"encoding/base64"
	"time"

	"github.com/gopcua/opcua/ua"
)

// OpcuaValueToJSON converts a *ua.Variant value into a JSON-friendly Go value.
//
// For ExtensionObjects the conversion is best-effort: with a server attached
// and a resolved schema we render full structures; without (or for unresolved
// types) we fall back to a {typeId, raw} marker so callers at least see what
// went past.
func OpcuaValueToJSON(v *ua.Variant, server *OpcuaServer) any {
	if v == nil {
		return nil
	}
	return convertOpcuaValue(v.Value(), server)
}

func convertOpcuaValue(v any, server *OpcuaServer) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, string,
		int8, int16, int32, int64,
		uint8, uint16, uint32, uint64,
		float32, float64:
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case []byte:
		// Bytes that arrive on the JSON layer turn into a numeric array so the
		// debug viewer renders them readably and round-trips don't get
		// silently base64-encoded by encoding/json.
		out := make([]int, len(x))
		for i, b := range x {
			out[i] = int(b)
		}
		return out
	case *ua.NodeID:
		return FormatOpcuaNodeID(x)
	case *ua.ExpandedNodeID:
		if x == nil || x.NodeID == nil {
			return nil
		}
		return FormatOpcuaNodeID(x.NodeID)
	case *ua.LocalizedText:
		if x == nil {
			return ""
		}
		return x.Text
	case *ua.QualifiedName:
		if x == nil {
			return ""
		}
		return x.Name
	case *ua.ExtensionObject:
		return convertExtensionObject(x, server)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convertOpcuaValue(e, server)
		}
		return out
	}

	// Fallback for slices of concrete types (e.g. []float64, []*ua.NodeID) —
	// reflect would be heavy, so we cover the common cases explicitly.
	switch s := v.(type) {
	case []bool:
		return toAnySliceTyped(s, func(b bool) any { return b })
	case []int32:
		return toAnySliceTyped(s, func(i int32) any { return i })
	case []uint32:
		return toAnySliceTyped(s, func(i uint32) any { return i })
	case []int64:
		return toAnySliceTyped(s, func(i int64) any { return i })
	case []uint64:
		return toAnySliceTyped(s, func(i uint64) any { return i })
	case []float32:
		return toAnySliceTyped(s, func(f float32) any { return f })
	case []float64:
		return toAnySliceTyped(s, func(f float64) any { return f })
	case []string:
		return toAnySliceTyped(s, func(s string) any { return s })
	case []time.Time:
		return toAnySliceTyped(s, func(t time.Time) any { return t.UTC().Format(time.RFC3339Nano) })
	case []*ua.NodeID:
		return toAnySliceTyped(s, func(n *ua.NodeID) any { return FormatOpcuaNodeID(n) })
	case []*ua.ExtensionObject:
		return toAnySliceTyped(s, func(e *ua.ExtensionObject) any { return convertExtensionObject(e, server) })
	}

	return v
}

// convertExtensionObject is the schema-aware path that turns a server-side
// structure back into a JSON-friendly map. The successful-decode shape is
// flat: struct fields sit directly on the returned map with no wrappers
// or metadata fields — consumers address `msg.payload.value.<FieldName>`
// straight up.
//
// On decode failure or missing schema we fall back to a placeholder map
// with underscore-prefixed diagnostic fields (`_typeId`, `_decodeError`,
// `_raw`) so the user at least sees why no real fields are present.
func convertExtensionObject(x *ua.ExtensionObject, server *OpcuaServer) any {
	if x == nil {
		return nil
	}
	var encodingID *ua.NodeID
	if x.TypeID != nil && x.TypeID.NodeID != nil {
		encodingID = x.TypeID.NodeID
	}

	rawBytes, hasRaw := extObjBytes(x)
	if hasRaw && server != nil {
		def := server.LookupStructByEncodingID(encodingID)
		if def != nil {
			decoded, err := DecodeStructBinary(rawBytes, def)
			if err == nil {
				return decoded
			}
			out := errorMarker(encodingID)
			out["_decodeError"] = err.Error()
			if hasRaw {
				out["_raw"] = base64.StdEncoding.EncodeToString(rawBytes)
			}
			return out
		}
	}

	// No schema available — emit a placeholder that at least carries the
	// wire-level type and raw bytes so the user can debug and the value is
	// still routable.
	out := errorMarker(encodingID)
	if hasRaw {
		out["_raw"] = base64.StdEncoding.EncodeToString(rawBytes)
	} else if x.Value != nil {
		// Built-in ExtensionObjects (e.g. ServerStatusDataType) come back
		// with a concrete struct populated by gopcua. Relay it through the
		// generic converter.
		converted := convertOpcuaValue(x.Value, server)
		if m, ok := converted.(map[string]any); ok {
			for k, v := range m {
				out[k] = v
			}
		} else {
			out["_value"] = converted
		}
	}
	return out
}

func errorMarker(encodingID *ua.NodeID) map[string]any {
	out := map[string]any{}
	if encodingID != nil {
		out["_typeId"] = FormatOpcuaNodeID(encodingID)
	}
	return out
}

// extObjBytes pulls the raw body bytes out of the wrapper our marker type
// installed on Decode. Returns (nil, false) for ExtensionObjects whose Value
// is anything else (e.g. a built-in concrete struct).
func extObjBytes(x *ua.ExtensionObject) ([]byte, bool) {
	if x == nil {
		return nil, false
	}
	if raw, ok := x.Value.(*opcuaRawExtObj); ok && raw != nil {
		return raw.Bytes, true
	}
	if raw, ok := x.Value.([]byte); ok {
		return raw, true
	}
	return nil, false
}

func toAnySliceTyped[T any](src []T, conv func(T) any) []any {
	out := make([]any, len(src))
	for i, v := range src {
		out[i] = conv(v)
	}
	return out
}
