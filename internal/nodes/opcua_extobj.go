// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua/ua"
)

// opcuaRawExtObj is the marker Go type we register with gopcua's extension
// object registry for every server-defined structure type the type resolver
// discovers. gopcua's stock decoder discards the body of any extension object
// whose TypeID isn't pre-registered; by inserting this trivial type we hold
// onto the raw bytes so the schema-driven decoder downstream can still read
// them.
//
// Implements ua.BinaryEncoder/BinaryDecoder — callers see a *opcuaRawExtObj
// inside ExtensionObject.Value and can fish the Bytes back out.
type opcuaRawExtObj struct {
	Bytes []byte
}

// Decode is called by gopcua's buffer with the trimmed body bytes of one
// extension object instance. We just snapshot a copy.
func (o *opcuaRawExtObj) Decode(b []byte) (int, error) {
	o.Bytes = append([]byte(nil), b...)
	return len(b), nil
}

// Encode is required by the BinaryEncoder interface but never called on our
// side: when we write structures we build a fresh *ua.ExtensionObject with a
// concrete byte slice; the marker type is decode-only.
func (o *opcuaRawExtObj) Encode() ([]byte, error) {
	return o.Bytes, nil
}

// extObjRegistry tracks which encoding NodeIDs we've already registered with
// gopcua so we don't re-register and don't race. gopcua's RegisterExtensionObject
// itself panics on duplicate-with-different-type, but is tolerant of
// duplicate-with-same-type — the local cache just keeps the lookup cheap.
var (
	extObjRegistryMu sync.Mutex
	extObjRegistered = make(map[string]struct{})
)

// registerRawExtObjType plumbs an encoding NodeID through to gopcua so the
// next ReadResponse carrying that TypeID lands in our marker type rather than
// being silently dropped. Idempotent and safe under concurrent calls.
func registerRawExtObjType(encodingID *ua.NodeID) {
	if encodingID == nil {
		return
	}
	key := encodingID.String()
	extObjRegistryMu.Lock()
	defer extObjRegistryMu.Unlock()
	if _, ok := extObjRegistered[key]; ok {
		return
	}
	// Spread instances so each registration carries its own *opcuaRawExtObj
	// even though the underlying type is shared. gopcua's TypeRegistry is
	// fine with the same Go type behind multiple NodeIDs.
	defer func() { _ = recover() }() // be defensive — Register panics on type-mismatch
	ua.RegisterExtensionObject(encodingID, &opcuaRawExtObj{})
	extObjRegistered[key] = struct{}{}
}

// StructDef is LOOPZE's in-memory rendering of a server-side
// ua.StructureDefinition. It carries everything the binary codec needs to
// decode / encode a value without further server round-trips.
type StructDef struct {
	NodeID            *ua.NodeID // the DataType NodeID (NOT the encoding ID)
	Name              string     // BrowseName of the DataType (best-effort)
	EncodingID        *ua.NodeID // DefaultBinary encoding NodeID — used when wiring up writes
	StructureType     ua.StructureType
	Fields            []StructField
	OptionalFieldCount int
}

// StructField mirrors ua.StructureField but with the resolved-by-resolver
// pointers needed at codec time.
type StructField struct {
	Name        string
	DataType    *ua.NodeID    // primitive or nested struct DataType
	IsArray     bool
	IsOptional  bool
	BuiltinType ua.TypeID     // resolved from DataType when it's a built-in
	NestedDef   *StructDef    // populated when the field is itself a structure
	EnumDef     *EnumDef      // populated when the field is an enumeration
}

// EnumDef captures both directions of an enum mapping so we can render
// notifications with the symbolic name *and* serialise writes that supply
// either form.
type EnumDef struct {
	NodeID    *ua.NodeID
	Name      string
	ByValue   map[int64]string
	ByName    map[string]int64
}

// DecodeStructBinary turns a binary ExtensionObject body into a JSON-shaped
// map[string]any guided by the resolver's schema. Walks fields in spec order
// because OPC UA structures are positional, not name-keyed.
func DecodeStructBinary(body []byte, def *StructDef) (map[string]any, error) {
	if def == nil {
		return nil, fmt.Errorf("nil StructDef")
	}
	buf := ua.NewBuffer(body)
	return decodeStructIntoBuffer(buf, def)
}

// decodeStructIntoBuffer is the recursive variant: nested structs in
// OPC UA's binary encoding are inlined (no length prefix), so they share a
// single buffer with the parent.
func decodeStructIntoBuffer(buf *ua.Buffer, def *StructDef) (map[string]any, error) {
	// StructureWithOptionalFields prefixes the body with a 32-bit "encoding
	// mask" telling us which optional fields are present. Any mandatory
	// field always sits in its declared position; optional ones consume a
	// bit each in the mask.
	var optionalMask uint32
	if def.StructureType == ua.StructureTypeStructureWithOptionalFields {
		optionalMask = buf.ReadUint32()
		if err := buf.Error(); err != nil {
			return nil, err
		}
	}
	// StructureTypeUnion stores a 32-bit "switch field" in front: 1-based
	// index of the active member (0 = none). Only that one field is encoded.
	var unionSwitch uint32
	if def.StructureType == ua.StructureTypeUnion {
		unionSwitch = buf.ReadUint32()
		if err := buf.Error(); err != nil {
			return nil, err
		}
	}

	out := make(map[string]any, len(def.Fields))
	optionalSeen := 0
	for i, f := range def.Fields {
		// Union: skip every field except the active one.
		if def.StructureType == ua.StructureTypeUnion {
			if uint32(i+1) != unionSwitch {
				continue
			}
		}
		// OptionalFields: respect the mask.
		if f.IsOptional {
			present := optionalMask&(1<<uint32(optionalSeen)) != 0
			optionalSeen++
			if !present {
				continue
			}
		}
		v, err := decodeField(buf, f)
		if err != nil {
			return out, fmt.Errorf("field %q: %w", f.Name, err)
		}
		out[f.Name] = v
	}
	return out, buf.Error()
}

// decodeField reads one field according to its resolved type, recursing for
// nested structures and walking arrays element-by-element.
func decodeField(buf *ua.Buffer, f StructField) (any, error) {
	if f.IsArray {
		length := buf.ReadInt32()
		if length == -1 {
			return nil, buf.Error()
		}
		out := make([]any, 0, length)
		for i := int32(0); i < length; i++ {
			v, err := decodeScalarField(buf, f)
			if err != nil {
				return out, err
			}
			out = append(out, v)
		}
		return out, buf.Error()
	}
	return decodeScalarField(buf, f)
}

func decodeScalarField(buf *ua.Buffer, f StructField) (any, error) {
	if f.NestedDef != nil {
		// Nested structures are inlined — no length prefix — so we recurse
		// straight into the parent buffer.
		return decodeStructIntoBuffer(buf, f.NestedDef)
	}
	if f.EnumDef != nil {
		// Enums are wire-encoded as Int32. We surface the symbol name to the
		// user; "<name>__raw" gives programmatic consumers the original code.
		raw := buf.ReadInt32()
		if err := buf.Error(); err != nil {
			return nil, err
		}
		if name, ok := f.EnumDef.ByValue[int64(raw)]; ok {
			return map[string]any{"name": name, "value": int64(raw)}, nil
		}
		return map[string]any{"value": int64(raw)}, nil
	}
	switch f.BuiltinType {
	case ua.TypeIDBoolean:
		return buf.ReadBool(), buf.Error()
	case ua.TypeIDSByte:
		return buf.ReadInt8(), buf.Error()
	case ua.TypeIDByte:
		return buf.ReadByte(), buf.Error()
	case ua.TypeIDInt16:
		return buf.ReadInt16(), buf.Error()
	case ua.TypeIDUint16:
		return buf.ReadUint16(), buf.Error()
	case ua.TypeIDInt32:
		return buf.ReadInt32(), buf.Error()
	case ua.TypeIDUint32:
		return buf.ReadUint32(), buf.Error()
	case ua.TypeIDInt64:
		return buf.ReadInt64(), buf.Error()
	case ua.TypeIDUint64:
		return buf.ReadUint64(), buf.Error()
	case ua.TypeIDFloat:
		return buf.ReadFloat32(), buf.Error()
	case ua.TypeIDDouble:
		return buf.ReadFloat64(), buf.Error()
	case ua.TypeIDString:
		return buf.ReadString(), buf.Error()
	case ua.TypeIDDateTime:
		t := buf.ReadTime()
		if t.IsZero() {
			return nil, buf.Error()
		}
		return t.UTC().Format(time.RFC3339Nano), buf.Error()
	case ua.TypeIDByteString:
		return buf.ReadBytes(), buf.Error()
	}
	return nil, fmt.Errorf("unsupported builtin type %d for field %q", f.BuiltinType, f.Name)
}

// EncodeStructBinary inverts DecodeStructBinary. Missing fields fall back to
// type zero values; extra fields are ignored.
func EncodeStructBinary(value map[string]any, def *StructDef) ([]byte, error) {
	if def == nil {
		return nil, fmt.Errorf("nil StructDef")
	}
	buf := ua.NewBuffer(nil)
	if err := encodeStructIntoBuffer(buf, value, def); err != nil {
		return nil, err
	}
	return buf.Bytes(), buf.Error()
}

func encodeStructIntoBuffer(buf *ua.Buffer, value map[string]any, def *StructDef) error {
	if def.StructureType == ua.StructureTypeStructureWithOptionalFields {
		var mask uint32
		bit := 0
		for _, f := range def.Fields {
			if !f.IsOptional {
				continue
			}
			if _, present := value[f.Name]; present {
				mask |= 1 << uint32(bit)
			}
			bit++
		}
		buf.WriteUint32(mask)
	}
	if def.StructureType == ua.StructureTypeUnion {
		var sw uint32
		for i, f := range def.Fields {
			if _, ok := value[f.Name]; ok {
				sw = uint32(i + 1)
				break
			}
		}
		buf.WriteUint32(sw)
		if sw == 0 {
			return buf.Error()
		}
		f := def.Fields[sw-1]
		if err := encodeField(buf, value[f.Name], f); err != nil {
			return fmt.Errorf("union field %q: %w", f.Name, err)
		}
		return buf.Error()
	}

	for _, f := range def.Fields {
		v, present := value[f.Name]
		if f.IsOptional && !present {
			continue
		}
		if err := encodeField(buf, v, f); err != nil {
			return fmt.Errorf("field %q: %w", f.Name, err)
		}
	}
	return buf.Error()
}

func encodeField(buf *ua.Buffer, v any, f StructField) error {
	if f.IsArray {
		arr, ok := v.([]any)
		if !ok && v != nil {
			return fmt.Errorf("expected array, got %T", v)
		}
		buf.WriteInt32(int32(len(arr)))
		for i, e := range arr {
			if err := encodeScalarField(buf, e, f); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return buf.Error()
	}
	return encodeScalarField(buf, v, f)
}

func encodeScalarField(buf *ua.Buffer, v any, f StructField) error {
	if f.NestedDef != nil {
		m, _ := v.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		// Nested struct is inlined into the parent buffer — no length prefix.
		return encodeStructIntoBuffer(buf, m, f.NestedDef)
	}
	if f.EnumDef != nil {
		val, err := coerceEnumValue(v, f.EnumDef)
		if err != nil {
			return err
		}
		buf.WriteInt32(int32(val))
		return buf.Error()
	}

	// Defaults for missing fields — match the spec's zero value for each type.
	if v == nil {
		switch f.BuiltinType {
		case ua.TypeIDBoolean:
			buf.WriteBool(false)
		case ua.TypeIDSByte:
			buf.WriteInt8(0)
		case ua.TypeIDByte:
			buf.WriteByte(0)
		case ua.TypeIDInt16:
			buf.WriteInt16(0)
		case ua.TypeIDUint16:
			buf.WriteUint16(0)
		case ua.TypeIDInt32:
			buf.WriteInt32(0)
		case ua.TypeIDUint32:
			buf.WriteUint32(0)
		case ua.TypeIDInt64:
			buf.WriteInt64(0)
		case ua.TypeIDUint64:
			buf.WriteUint64(0)
		case ua.TypeIDFloat:
			buf.WriteFloat32(0)
		case ua.TypeIDDouble:
			buf.WriteFloat64(0)
		case ua.TypeIDString:
			buf.WriteString("")
		case ua.TypeIDByteString:
			buf.WriteByteString(nil)
		default:
			return fmt.Errorf("no default for type %d", f.BuiltinType)
		}
		return buf.Error()
	}

	coerced, err := CoerceOpcuaValue(v, f.BuiltinType)
	if err != nil {
		return err
	}
	switch f.BuiltinType {
	case ua.TypeIDBoolean:
		buf.WriteBool(coerced.(bool))
	case ua.TypeIDSByte:
		buf.WriteInt8(coerced.(int8))
	case ua.TypeIDByte:
		buf.WriteByte(coerced.(uint8))
	case ua.TypeIDInt16:
		buf.WriteInt16(coerced.(int16))
	case ua.TypeIDUint16:
		buf.WriteUint16(coerced.(uint16))
	case ua.TypeIDInt32:
		buf.WriteInt32(coerced.(int32))
	case ua.TypeIDUint32:
		buf.WriteUint32(coerced.(uint32))
	case ua.TypeIDInt64:
		buf.WriteInt64(coerced.(int64))
	case ua.TypeIDUint64:
		buf.WriteUint64(coerced.(uint64))
	case ua.TypeIDFloat:
		buf.WriteFloat32(coerced.(float32))
	case ua.TypeIDDouble:
		buf.WriteFloat64(coerced.(float64))
	case ua.TypeIDString:
		buf.WriteString(coerced.(string))
	case ua.TypeIDDateTime:
		buf.WriteTime(coerced.(time.Time))
	case ua.TypeIDByteString:
		buf.WriteByteString(coerced.([]byte))
	default:
		return fmt.Errorf("unsupported builtin type %d for field %q", f.BuiltinType, f.Name)
	}
	return buf.Error()
}

// coerceEnumValue accepts both raw integers and symbolic names, mirroring the
// spec's promise that consumers may write either form.
func coerceEnumValue(v any, def *EnumDef) (int64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case string:
		if val, ok := def.ByName[x]; ok {
			return val, nil
		}
		return 0, fmt.Errorf("unknown enum symbol %q (known: %v)", x, enumNames(def))
	case map[string]any:
		// {"name": "Auto"} or {"value": 3}
		if name, ok := x["name"].(string); ok {
			if val, ok := def.ByName[name]; ok {
				return val, nil
			}
		}
		if num, ok := x["value"].(float64); ok {
			return int64(num), nil
		}
		return 0, fmt.Errorf("enum object missing name/value")
	case float64:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	}
	return 0, fmt.Errorf("cannot coerce %T to enum", v)
}

func enumNames(def *EnumDef) []string {
	out := make([]string, 0, len(def.ByName))
	for k := range def.ByName {
		out = append(out, k)
	}
	return out
}
