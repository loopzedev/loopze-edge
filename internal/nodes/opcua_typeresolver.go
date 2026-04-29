// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"context"
	"fmt"
	"sync"

	"github.com/gopcua/opcua/ua"
)

// structResolver loads server-side StructureDefinitions on demand, caches
// them, and registers the encoding NodeID with gopcua so subsequent reads
// retain the body bytes. One instance lives on each OpcuaServer.
type structResolver struct {
	server *OpcuaServer

	mu      sync.RWMutex
	structs map[string]*StructDef // key: DataType NodeID string
	enums   map[string]*EnumDef   // key: DataType NodeID string

	// encodingIndex lets a Read or Subscribe callback look up the schema by
	// the *encoding* NodeID it sees on the wire — the user-facing field
	// resolver works in the DataType NodeID space, but the wire works in
	// the encoding NodeID space, so we maintain both lookup directions.
	encodingIndex map[string]*StructDef
}

func newStructResolver(server *OpcuaServer) *structResolver {
	return &structResolver{
		server:        server,
		structs:       make(map[string]*StructDef),
		enums:         make(map[string]*EnumDef),
		encodingIndex: make(map[string]*StructDef),
	}
}

func (r *structResolver) reset() {
	r.mu.Lock()
	r.structs = make(map[string]*StructDef)
	r.enums = make(map[string]*EnumDef)
	r.encodingIndex = make(map[string]*StructDef)
	r.mu.Unlock()
}

// resolveStruct loads and caches a struct definition. visited prevents
// infinite recursion when struct A references struct A (rare but legal).
func (r *structResolver) resolveStruct(ctx context.Context, dataType *ua.NodeID, visited map[string]bool) (*StructDef, error) {
	if dataType == nil {
		return nil, fmt.Errorf("nil DataType")
	}
	key := dataType.String()

	r.mu.RLock()
	if def, ok := r.structs[key]; ok {
		r.mu.RUnlock()
		return def, nil
	}
	r.mu.RUnlock()

	if visited[key] {
		return nil, fmt.Errorf("recursive struct reference: %s", key)
	}
	visited[key] = true

	client := r.server.Client()
	if client == nil {
		return nil, fmt.Errorf("no session")
	}

	resp, err := client.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: dataType, AttributeID: ua.AttributeIDDataTypeDefinition},
			{NodeID: dataType, AttributeID: ua.AttributeIDBrowseName},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("read DataTypeDefinition: %w", err)
	}
	if len(resp.Results) < 2 {
		return nil, fmt.Errorf("unexpected DataTypeDefinition read response")
	}
	defResult := resp.Results[0]
	if !OpcuaStatusCodeIsGood(defResult.Status) {
		return nil, fmt.Errorf("DataTypeDefinition status: %s", OpcuaStatusCodeName(defResult.Status))
	}
	if defResult.Value == nil || defResult.Value.Value() == nil {
		return nil, fmt.Errorf("DataTypeDefinition empty for %s", key)
	}

	// Server-defined StructureDefinitions arrive as ExtensionObjects whose
	// gopcua-known concrete type is *ua.StructureDefinition.
	extObj, ok := defResult.Value.Value().(*ua.ExtensionObject)
	if !ok || extObj == nil {
		return nil, fmt.Errorf("unexpected DataTypeDefinition value type %T", defResult.Value.Value())
	}
	switch v := extObj.Value.(type) {
	case *ua.StructureDefinition:
		name := ""
		if qn, ok := resp.Results[1].Value.Value().(*ua.QualifiedName); ok && qn != nil {
			name = qn.Name
		}
		def, err := r.buildStructDef(ctx, dataType, v, name, visited)
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		r.structs[key] = def
		if def.EncodingID != nil {
			r.encodingIndex[def.EncodingID.String()] = def
		}
		r.mu.Unlock()

		// Plumb the encoding ID through to gopcua so future reads
		// preserve the body bytes for our schema-driven decoder.
		if def.EncodingID != nil {
			registerRawExtObjType(def.EncodingID)
		}
		return def, nil
	case *ua.EnumDefinition:
		// We hit an enum where a struct was expected. Record it in the enum
		// cache but signal "not a struct" so the caller can decide.
		_, _ = r.cacheEnum(dataType, v, key)
		return nil, fmt.Errorf("DataType %s is an enum, not a struct", key)
	default:
		return nil, fmt.Errorf("unsupported DataTypeDefinition kind %T", v)
	}
}

// resolveEnum looks up (and caches) an enum definition.
func (r *structResolver) resolveEnum(ctx context.Context, dataType *ua.NodeID) (*EnumDef, error) {
	if dataType == nil {
		return nil, fmt.Errorf("nil enum DataType")
	}
	key := dataType.String()
	r.mu.RLock()
	if def, ok := r.enums[key]; ok {
		r.mu.RUnlock()
		return def, nil
	}
	r.mu.RUnlock()

	client := r.server.Client()
	if client == nil {
		return nil, fmt.Errorf("no session")
	}
	resp, err := client.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: dataType, AttributeID: ua.AttributeIDDataTypeDefinition},
		},
	})
	if err != nil || len(resp.Results) == 0 {
		return nil, fmt.Errorf("read enum definition: %w", err)
	}
	dv := resp.Results[0]
	if dv.Value == nil || dv.Value.Value() == nil {
		return nil, fmt.Errorf("enum definition empty for %s", key)
	}
	extObj, ok := dv.Value.Value().(*ua.ExtensionObject)
	if !ok || extObj == nil {
		return nil, fmt.Errorf("unexpected enum definition value %T", dv.Value.Value())
	}
	enumDef, ok := extObj.Value.(*ua.EnumDefinition)
	if !ok {
		return nil, fmt.Errorf("not an EnumDefinition: %T", extObj.Value)
	}
	return r.cacheEnum(dataType, enumDef, key)
}

func (r *structResolver) cacheEnum(dataType *ua.NodeID, enumDef *ua.EnumDefinition, key string) (*EnumDef, error) {
	def := &EnumDef{
		NodeID:  dataType,
		ByValue: make(map[int64]string, len(enumDef.Fields)),
		ByName:  make(map[string]int64, len(enumDef.Fields)),
	}
	for _, f := range enumDef.Fields {
		def.ByValue[f.Value] = f.Name
		def.ByName[f.Name] = f.Value
	}
	r.mu.Lock()
	r.enums[key] = def
	r.mu.Unlock()
	return def, nil
}

// buildStructDef walks every field, resolves nested struct/enum references,
// and produces a self-contained StructDef the codec can use without any
// further server round-trips.
func (r *structResolver) buildStructDef(ctx context.Context, dataType *ua.NodeID, sd *ua.StructureDefinition, name string, visited map[string]bool) (*StructDef, error) {
	encodingID := sd.DefaultEncodingID
	if isEmptyNodeID(encodingID) {
		// Some servers (incl. our Deno test server) ship StructureDefinitions
		// without DefaultEncodingID. Fall back to a HasEncoding browse — the
		// "Default Binary" child of the DataType node carries the wire-level
		// Encoding NodeID we actually need to register.
		if found := r.findBinaryEncodingID(ctx, dataType); found != nil {
			encodingID = found
		}
	}
	out := &StructDef{
		NodeID:        dataType,
		Name:          name,
		StructureType: sd.StructureType,
		EncodingID:    encodingID,
	}
	for _, sf := range sd.Fields {
		field := StructField{
			Name:       sf.Name,
			DataType:   sf.DataType,
			IsArray:    sf.ValueRank > 0,
			IsOptional: sf.IsOptional,
		}
		if sf.IsOptional {
			out.OptionalFieldCount++
		}
		// Built-in types live in namespace 0 and map via dataTypeNodeIDToTypeID.
		if sf.DataType != nil && sf.DataType.Namespace() == 0 {
			if t, ok := dataTypeNodeIDToTypeID[sf.DataType.IntID()]; ok && t != ua.TypeIDExtensionObject {
				field.BuiltinType = t
				out.Fields = append(out.Fields, field)
				continue
			}
		}
		// Otherwise it's either a nested struct or an enum — try struct first,
		// fall back to enum on that specific failure.
		nested, err := r.resolveStruct(ctx, sf.DataType, visited)
		if err == nil {
			field.NestedDef = nested
			out.Fields = append(out.Fields, field)
			continue
		}
		// resolveStruct returned "is an enum, not a struct" → load it.
		enum, errEnum := r.resolveEnum(ctx, sf.DataType)
		if errEnum == nil {
			field.EnumDef = enum
			field.BuiltinType = ua.TypeIDInt32 // enums are wire-encoded as Int32
			out.Fields = append(out.Fields, field)
			continue
		}
		return nil, fmt.Errorf("field %q (DataType %s): %v / %v", sf.Name, sf.DataType, err, errEnum)
	}
	return out, nil
}

// ResolveStructForDataType is the public entry point for callers that have a
// DataType NodeID (typically obtained via DataType-attribute Read on a
// regular Variable node).
func (s *OpcuaServer) ResolveStructForDataType(ctx context.Context, dataType *ua.NodeID) (*StructDef, error) {
	s.mu.RLock()
	resolver := s.structResolver
	s.mu.RUnlock()
	if resolver == nil {
		return nil, fmt.Errorf("no resolver")
	}
	return resolver.resolveStruct(ctx, dataType, map[string]bool{})
}

// LookupStructByEncodingID retrieves a previously-resolved struct definition
// by its encoding NodeID — used by the read/subscribe path when the wire
// hands us only the encoding ID.
func (s *OpcuaServer) LookupStructByEncodingID(encodingID *ua.NodeID) *StructDef {
	if encodingID == nil {
		return nil
	}
	s.mu.RLock()
	resolver := s.structResolver
	s.mu.RUnlock()
	if resolver == nil {
		return nil
	}
	resolver.mu.RLock()
	defer resolver.mu.RUnlock()
	return resolver.encodingIndex[encodingID.String()]
}

// isEmptyNodeID treats nil and the zero-NodeID (ns=0, IntID 0) as "no
// encoding ID supplied" — both forms appear in the wild on servers that
// publish StructureDefinitions without filling DefaultEncodingID.
func isEmptyNodeID(n *ua.NodeID) bool {
	if n == nil {
		return true
	}
	if n.Namespace() == 0 && n.IntID() == 0 && n.StringID() == "" {
		return true
	}
	return false
}

// findBinaryEncodingID browses the HasEncoding references forward from the
// given DataType node and returns the NodeID of its "Default Binary" child.
// Returns nil if the server doesn't expose the relationship — the schema is
// then unusable for ExtensionObject decoding.
func (r *structResolver) findBinaryEncodingID(ctx context.Context, dataType *ua.NodeID) *ua.NodeID {
	client := r.server.Client()
	if client == nil || dataType == nil {
		return nil
	}
	resp, err := client.Browse(ctx, &ua.BrowseRequest{
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          dataType,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 38), // HasEncoding
				IncludeSubtypes: false,
				NodeClassMask:   0,
				ResultMask:      0x3F,
			},
		},
	})
	if err != nil || len(resp.Results) == 0 {
		return nil
	}
	for _, ref := range resp.Results[0].References {
		if ref == nil || ref.BrowseName == nil || ref.NodeID == nil || ref.NodeID.NodeID == nil {
			continue
		}
		if ref.BrowseName.Name == "Default Binary" {
			return ref.NodeID.NodeID
		}
	}
	return nil
}

// PrewarmStructForVariable looks up the DataType of the given Variable node,
// and — if it's a structure — drives the resolver to load the schema and
// register its encoding NodeID with gopcua so the next Read/Subscribe of
// that variable arrives with the body bytes intact rather than being
// silently dropped by the library's "unknown extobj type" path.
//
// Best-effort: errors are logged via the resolver but do not surface to
// callers, because a missing schema only degrades to the {typeId} fallback
// at decode time.
func (s *OpcuaServer) PrewarmStructForVariable(ctx context.Context, variableNodeID *ua.NodeID) {
	if variableNodeID == nil {
		return
	}
	client := s.Client()
	if client == nil {
		return
	}
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: variableNodeID, AttributeID: ua.AttributeIDDataType, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil || len(resp.Results) == 0 {
		return
	}
	dv := resp.Results[0]
	if !OpcuaStatusCodeIsGood(dv.Status) || dv.Value == nil {
		return
	}
	dataType, ok := dv.Value.Value().(*ua.NodeID)
	if !ok || dataType == nil {
		return
	}
	// Built-in primitive types live in namespace 0 with a known IntID and
	// don't need schema resolution.
	if dataType.Namespace() == 0 {
		if t, hit := dataTypeNodeIDToTypeID[dataType.IntID()]; hit && t != ua.TypeIDExtensionObject {
			return
		}
	}
	// Best-effort: any error here just leaves the resolver cache empty,
	// which means later notifications get the {typeId} fallback shape.
	_, _ = s.ResolveStructForDataType(ctx, dataType)
}

// EncodeStructValue is the Write-side entry point: turn a JSON-shaped
// map[string]any into a *ua.ExtensionObject the wire codec can ship. The
// optional structureType arg lets the caller pick the schema explicitly
// (typical when writing); when empty we fall back to looking the schema up
// by the target NodeID's DataType attribute.
func (s *OpcuaServer) EncodeStructValue(ctx context.Context, structureType *ua.NodeID, value map[string]any) (*ua.ExtensionObject, error) {
	if structureType == nil {
		return nil, fmt.Errorf("structureType is required for ExtensionObject writes")
	}
	def, err := s.ResolveStructForDataType(ctx, structureType)
	if err != nil {
		return nil, err
	}
	body, err := EncodeStructBinary(value, def)
	if err != nil {
		return nil, fmt.Errorf("encode body: %w", err)
	}
	if def.EncodingID == nil {
		return nil, fmt.Errorf("schema for %s has no DefaultEncodingID", structureType)
	}
	return &ua.ExtensionObject{
		EncodingMask: ua.ExtensionObjectBinary,
		TypeID:       &ua.ExpandedNodeID{NodeID: def.EncodingID},
		Value:        &opcuaRawExtObj{Bytes: body},
	}, nil
}
