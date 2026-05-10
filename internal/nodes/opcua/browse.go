// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package opcua

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// OpcuaBrowseChild is the wire form of one browse-result child as the editor
// expects it. Encoded in `json` tags so api/opcua_handlers can return slices
// of these directly.
type OpcuaBrowseChild struct {
	NodeID      string             `json:"nodeId"`
	BrowseName  string             `json:"browseName"`
	DisplayName string             `json:"displayName"`
	NodeClass   string             `json:"nodeClass"`
	HasChildren bool               `json:"hasChildren"`
	DataType    *OpcuaDataTypeInfo `json:"dataType,omitempty"`
	ValueRank   int32              `json:"valueRank,omitempty"`
	AccessLevel string             `json:"accessLevel,omitempty"`
	Description string             `json:"description,omitempty"`
	// Synthetic flags a tree node that doesn't exist in the server address
	// space — typically a struct field surfaced from a DataTypeDefinition.
	// The frontend renders synthetic children non-selectable because they
	// can't be subscribed to directly.
	Synthetic bool `json:"synthetic,omitempty"`
}

// OpcuaDataTypeInfo accompanies Variable nodes so the editor can pre-fill the
// DataType field on a Write or recognise structures.
type OpcuaDataTypeInfo struct {
	NodeID      string `json:"nodeId"`
	Name        string `json:"name,omitempty"`
	IsStructure bool   `json:"isStructure"`
}

// OpcuaBrowseResult is the response shape for one browse call: the parent's
// metadata (so the UI can show the breadcrumb) plus its children. The
// continuation point lets paginated browses pick up where they left off.
type OpcuaBrowseResult struct {
	Parent            *OpcuaBrowseChild  `json:"parent,omitempty"`
	Children          []OpcuaBrowseChild `json:"children"`
	ContinuationPoint string             `json:"continuationPoint,omitempty"`
}

// nodeClassName turns the bitmask enum into a stable string the frontend can
// switch on — empty for unspecified nodes.
func nodeClassName(c ua.NodeClass) string {
	switch c {
	case ua.NodeClassObject:
		return "Object"
	case ua.NodeClassVariable:
		return "Variable"
	case ua.NodeClassMethod:
		return "Method"
	case ua.NodeClassObjectType:
		return "ObjectType"
	case ua.NodeClassVariableType:
		return "VariableType"
	case ua.NodeClassReferenceType:
		return "ReferenceType"
	case ua.NodeClassDataType:
		return "DataType"
	case ua.NodeClassView:
		return "View"
	}
	return ""
}

// accessLevelName renders the AccessLevel byte the wire returns into human
// terms. Read/Write/Hist are the bits flow authors actually care about.
func accessLevelName(a byte) string {
	parts := []string{}
	if a&0x01 != 0 {
		parts = append(parts, "Read")
	}
	if a&0x02 != 0 {
		parts = append(parts, "Write")
	}
	if a&0x04 != 0 {
		parts = append(parts, "HistoryRead")
	}
	if a&0x08 != 0 {
		parts = append(parts, "HistoryWrite")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, "|")
}

// OpcuaBrowse walks one level of children below the given NodeID. It uses an
// already-running session (preferred) or, if none is supplied, opens a
// short-lived one from a transient ConfigNode.
//
// nodeIDStr accepts an empty string as shorthand for the Objects folder
// (i=85) — the typical browse root for industrial address spaces.
//
// server (optional) is consulted for two enrichments that aren't in the wire
// protocol: struct-field expansion of ExtensionObject variables, and
// $field-suffix synthetic NodeIDs for those fields.
func OpcuaBrowse(ctx context.Context, client *opcua.Client, server *OpcuaServer, nodeIDStr string) (*OpcuaBrowseResult, error) {
	if client == nil {
		return nil, fmt.Errorf("no session")
	}
	if nodeIDStr == "" {
		nodeIDStr = "i=85"
	}
	// Synthetic struct-field NodeIDs use a "$field=Name" suffix that is not
	// valid OPC UA — short-circuit those into a schema-only response so the
	// frontend can render nested struct fields recursively without involving
	// the server.
	if syntheticPath, ok := splitSyntheticNodeID(nodeIDStr); ok {
		if server == nil {
			return nil, fmt.Errorf("synthetic struct fields require a server context")
		}
		return buildSyntheticChildren(ctx, server, nodeIDStr, syntheticPath)
	}
	parent, err := ParseOpcuaNodeID(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid nodeId: %w", err)
	}

	// If the requested NodeID is a Variable with a structured DataType, the
	// server-side browse will typically be empty (struct fields aren't
	// reified as Address-Space children). Surface the schema fields instead
	// so the user can drill into them.
	if server != nil && isStructVariable(ctx, client, parent) {
		path := syntheticPath{base: nodeIDStr}
		if syn, err := buildSyntheticChildren(ctx, server, nodeIDStr, path); err == nil {
			return syn, nil
		}
		// fall through to regular browse on schema-resolution failure
	}

	// Browse the children: forward, all node classes, hierarchical references
	// only (skip e.g. Aggregates that pull in cross-cutting data).
	resp, err := client.Browse(ctx, &ua.BrowseRequest{
		RequestedMaxReferencesPerNode: 1000,
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          parent,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 33), // HierarchicalReferences
				IncludeSubtypes: true,
				NodeClassMask:   0, // any
				ResultMask:      0x3F, // all bits
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("browse: empty response")
	}
	result := resp.Results[0]
	if !OpcuaStatusCodeIsGood(result.StatusCode) {
		return nil, fmt.Errorf("browse status: %s", OpcuaStatusCodeName(result.StatusCode))
	}

	out := &OpcuaBrowseResult{
		Parent: &OpcuaBrowseChild{
			NodeID: nodeIDStr,
		},
		Children: make([]OpcuaBrowseChild, 0, len(result.References)),
	}
	if len(result.ContinuationPoint) > 0 {
		out.ContinuationPoint = fmt.Sprintf("%x", result.ContinuationPoint)
	}

	// Read the parent's metadata in parallel — purely for the breadcrumb,
	// errors here are non-fatal.
	if pmeta, err := readBrowseMetadata(ctx, client, parent); err == nil && pmeta != nil {
		pmeta.NodeID = nodeIDStr
		out.Parent = pmeta
	}

	for _, ref := range result.References {
		if ref == nil || ref.NodeID == nil || ref.NodeID.NodeID == nil {
			continue
		}
		child := OpcuaBrowseChild{
			NodeID:      FormatOpcuaNodeID(ref.NodeID.NodeID),
			BrowseName:  qualifiedNameString(ref.BrowseName),
			DisplayName: localizedTextString(ref.DisplayName),
			NodeClass:   nodeClassName(ref.NodeClass),
			HasChildren: ref.NodeClass == ua.NodeClassObject || ref.NodeClass == ua.NodeClassView,
		}
		// For Variables also pull DataType / ValueRank / AccessLevel so the
		// frontend can pre-fill Write configs without a second round-trip.
		// Browse-only metadata for non-Variables stays light.
		if ref.NodeClass == ua.NodeClassVariable {
			if attrs, err := readVariableMetadata(ctx, client, ref.NodeID.NodeID); err == nil && attrs != nil {
				child.DataType = attrs.DataType
				child.ValueRank = attrs.ValueRank
				child.AccessLevel = attrs.AccessLevel
				child.Description = attrs.Description
				// Struct-typed variables get a synthetic "expandable" hint so
				// the editor can drill into the schema-defined fields even
				// though the server doesn't expose them as real children.
				if attrs.DataType != nil && attrs.DataType.IsStructure {
					child.HasChildren = true
				}
			}
		}
		out.Children = append(out.Children, child)
	}
	return out, nil
}

// splitSyntheticNodeID detects our "$field=…" suffix and returns the base
// NodeID plus the field-path tokens. Real OPC UA NodeIDs never contain
// "$field=" so the check is collision-free.
func splitSyntheticNodeID(s string) (path syntheticPath, ok bool) {
	idx := strings.Index(s, "$field=")
	if idx == -1 {
		return syntheticPath{}, false
	}
	path.base = s[:idx]
	rest := s[idx+len("$field="):]
	path.fields = strings.Split(rest, ".")
	return path, true
}

type syntheticPath struct {
	base   string   // the original Variable NodeID (or DataType NodeID)
	fields []string // field path inside the struct, deepest last
}

// makeSyntheticNodeID produces the NodeID a synthetic field child is
// addressable under. Reverse of splitSyntheticNodeID.
func makeSyntheticNodeID(base string, fields []string) string {
	return base + "$field=" + strings.Join(fields, ".")
}

// buildSyntheticChildren resolves the struct schema for a Variable (or a
// nested field) and returns its fields as virtual tree children. The
// tree-level NodeIDs use the "$field=" suffix so subsequent expansions of
// nested struct fields keep working through the same Browse endpoint.
func buildSyntheticChildren(ctx context.Context, server *OpcuaServer, requestedNodeID string, path syntheticPath) (*OpcuaBrowseResult, error) {
	// First: resolve the base variable's DataType.
	baseParsed, err := ParseOpcuaNodeID(path.base)
	if err != nil {
		return nil, fmt.Errorf("invalid base nodeId: %w", err)
	}
	def, err := resolveStructForVariable(ctx, server, baseParsed)
	if err != nil {
		return nil, err
	}

	// Walk the field path to land on the right StructDef.
	for _, fieldName := range path.fields {
		next, found := stepIntoField(def, fieldName)
		if next == nil {
			return nil, fmt.Errorf("field %q not found or not a struct", fieldName)
		}
		def = next
		_ = found
	}

	out := &OpcuaBrowseResult{
		Parent: &OpcuaBrowseChild{
			NodeID:      requestedNodeID,
			BrowseName:  def.Name,
			DisplayName: def.Name,
			NodeClass:   "Variable",
		},
		Children: make([]OpcuaBrowseChild, 0, len(def.Fields)),
	}
	for _, f := range def.Fields {
		child := OpcuaBrowseChild{
			NodeID:      makeSyntheticNodeID(path.base, append(append([]string{}, path.fields...), f.Name)),
			BrowseName:  f.Name,
			DisplayName: f.Name,
			NodeClass:   "Variable",
			Synthetic:   true,
		}
		// Datatype hint: nested struct → IsStructure=true so the row sprouts
		// an expand caret and gets the struct icon.
		if f.NestedDef != nil {
			child.DataType = &OpcuaDataTypeInfo{
				NodeID:      FormatOpcuaNodeID(f.NestedDef.NodeID),
				Name:        f.NestedDef.Name,
				IsStructure: true,
			}
			child.HasChildren = true
		} else if f.EnumDef != nil {
			child.DataType = &OpcuaDataTypeInfo{
				NodeID: FormatOpcuaNodeID(f.EnumDef.NodeID),
				Name:   "Enum:" + f.EnumDef.Name,
			}
		} else {
			child.DataType = &OpcuaDataTypeInfo{
				NodeID: FormatOpcuaNodeID(f.DataType),
				Name:   OpcuaTypeName(f.BuiltinType),
			}
		}
		if f.IsArray {
			child.ValueRank = 1
		}
		out.Children = append(out.Children, child)
	}
	return out, nil
}

// isStructVariable returns true when the given NodeID points at a Variable
// whose DataType is server-defined (i.e. a structure). One read combines
// NodeClass + DataType so the cost is bounded — and the result is only
// consulted to decide whether to take the synthetic-children path.
func isStructVariable(ctx context.Context, client *opcua.Client, id *ua.NodeID) bool {
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id, AttributeID: ua.AttributeIDNodeClass, DataEncoding: &ua.QualifiedName{}},
			{NodeID: id, AttributeID: ua.AttributeIDDataType, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil || len(resp.Results) < 2 {
		return false
	}
	nc, ok := opcuaScalar(resp.Results[0]).(int32)
	if !ok || ua.NodeClass(nc) != ua.NodeClassVariable {
		return false
	}
	dt, ok := opcuaScalar(resp.Results[1]).(*ua.NodeID)
	if !ok || dt == nil {
		return false
	}
	if dt.Namespace() == 0 {
		t, hit := dataTypeNodeIDToTypeID[dt.IntID()]
		// Built-in types are not structs — except the explicit ExtObj/Struct
		// data type which would require schema resolution anyway.
		return hit && t == ua.TypeIDExtensionObject
	}
	// Server-defined data types are structs in practice.
	return true
}

// resolveStructForVariable looks up a Variable's DataType, then its struct
// definition. Returns an error if the DataType isn't a structure (struct-
// only browsing makes no sense for plain scalars).
func resolveStructForVariable(ctx context.Context, server *OpcuaServer, variableNodeID *ua.NodeID) (*StructDef, error) {
	client := server.Client()
	if client == nil {
		return nil, fmt.Errorf("no session")
	}
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: variableNodeID, AttributeID: ua.AttributeIDDataType, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil || len(resp.Results) == 0 {
		return nil, fmt.Errorf("read DataType: %w", err)
	}
	dv := resp.Results[0]
	if !OpcuaStatusCodeIsGood(dv.Status) || dv.Value == nil {
		return nil, fmt.Errorf("DataType read status: %s", OpcuaStatusCodeName(dv.Status))
	}
	dataType, ok := dv.Value.Value().(*ua.NodeID)
	if !ok || dataType == nil {
		return nil, fmt.Errorf("unexpected DataType value: %T", dv.Value.Value())
	}
	def, err := server.ResolveStructForDataType(ctx, dataType)
	if err != nil {
		return nil, fmt.Errorf("resolve struct: %w", err)
	}
	return def, nil
}

// stepIntoField walks one level deeper into a struct field. Returns nil if
// the field doesn't exist or isn't itself a struct.
func stepIntoField(def *StructDef, fieldName string) (*StructDef, bool) {
	if def == nil {
		return nil, false
	}
	for _, f := range def.Fields {
		if f.Name != fieldName {
			continue
		}
		if f.NestedDef != nil {
			return f.NestedDef, true
		}
		return nil, true // found but not a struct
	}
	return nil, false
}

// readBrowseMetadata fetches BrowseName/DisplayName/NodeClass/Description for
// a given node — used for the breadcrumb-style parent rendering.
func readBrowseMetadata(ctx context.Context, client *opcua.Client, id *ua.NodeID) (*OpcuaBrowseChild, error) {
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id, AttributeID: ua.AttributeIDBrowseName},
			{NodeID: id, AttributeID: ua.AttributeIDDisplayName},
			{NodeID: id, AttributeID: ua.AttributeIDNodeClass},
			{NodeID: id, AttributeID: ua.AttributeIDDescription},
		},
	})
	if err != nil || len(resp.Results) < 4 {
		return nil, err
	}
	out := &OpcuaBrowseChild{}
	if qn, ok := opcuaScalar(resp.Results[0]).(*ua.QualifiedName); ok {
		out.BrowseName = qualifiedNameString(qn)
	}
	if lt, ok := opcuaScalar(resp.Results[1]).(*ua.LocalizedText); ok {
		out.DisplayName = localizedTextString(lt)
	}
	if nc, ok := opcuaScalar(resp.Results[2]).(int32); ok {
		out.NodeClass = nodeClassName(ua.NodeClass(nc))
	}
	if lt, ok := opcuaScalar(resp.Results[3]).(*ua.LocalizedText); ok {
		out.Description = localizedTextString(lt)
	}
	return out, nil
}

// readVariableMetadata pulls the bits needed to seed a Write configuration:
// DataType (with name + isStructure marker), ValueRank, AccessLevel.
func readVariableMetadata(ctx context.Context, client *opcua.Client, id *ua.NodeID) (*OpcuaBrowseChild, error) {
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: id, AttributeID: ua.AttributeIDDataType, DataEncoding: &ua.QualifiedName{}},
			{NodeID: id, AttributeID: ua.AttributeIDValueRank, DataEncoding: &ua.QualifiedName{}},
			{NodeID: id, AttributeID: ua.AttributeIDAccessLevel, DataEncoding: &ua.QualifiedName{}},
			{NodeID: id, AttributeID: ua.AttributeIDDescription, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil || len(resp.Results) < 4 {
		return nil, err
	}
	out := &OpcuaBrowseChild{}
	if dt, ok := opcuaScalar(resp.Results[0]).(*ua.NodeID); ok && dt != nil {
		info := &OpcuaDataTypeInfo{NodeID: FormatOpcuaNodeID(dt)}
		if dt.Namespace() == 0 {
			if t, hit := dataTypeNodeIDToTypeID[dt.IntID()]; hit {
				info.Name = OpcuaTypeName(t)
				info.IsStructure = t == ua.TypeIDExtensionObject
			}
		} else {
			info.IsStructure = true // server-defined DataType — almost always a struct
			// Best-effort: read the BrowseName of the DataType so the UI shows
			// "SensorReadingType" instead of "Unknown". Errors here are silent
			// because the operation is purely cosmetic.
			if name := readDataTypeBrowseName(ctx, client, dt); name != "" {
				info.Name = name
			}
		}
		out.DataType = info
	}
	if vr, ok := opcuaScalar(resp.Results[1]).(int32); ok {
		out.ValueRank = vr
	}
	if al, ok := opcuaScalar(resp.Results[2]).(byte); ok {
		out.AccessLevel = accessLevelName(al)
	}
	if lt, ok := opcuaScalar(resp.Results[3]).(*ua.LocalizedText); ok {
		out.Description = localizedTextString(lt)
	}
	return out, nil
}

// LookupBrowseName returns the BrowseName.Name of a Variable (or any) NodeID,
// cached per server. First call does a Read of the BrowseName attribute,
// subsequent calls hit the in-memory map. Empty string on any failure so
// the caller can apply its own fallback strategy without surfacing the
// network error.
func (s *OpcuaServer) LookupBrowseName(ctx context.Context, nodeID *ua.NodeID) string {
	if nodeID == nil {
		return ""
	}
	key := nodeID.String()

	s.browseNameMu.RLock()
	if name, ok := s.browseNameCache[key]; ok {
		s.browseNameMu.RUnlock()
		return name
	}
	s.browseNameMu.RUnlock()

	client := s.Client()
	if client == nil {
		return ""
	}
	name := readDataTypeBrowseName(ctx, client, nodeID)

	s.browseNameMu.Lock()
	if s.browseNameCache == nil {
		s.browseNameCache = make(map[string]string)
	}
	s.browseNameCache[key] = name
	s.browseNameMu.Unlock()
	return name
}

// readDataTypeBrowseName fetches just the BrowseName attribute of a DataType
// node. Returns the empty string on any failure — the caller falls back to
// a generic placeholder.
func readDataTypeBrowseName(ctx context.Context, client *opcua.Client, dt *ua.NodeID) string {
	resp, err := client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: dt, AttributeID: ua.AttributeIDBrowseName, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil || len(resp.Results) == 0 {
		return ""
	}
	if qn, ok := opcuaScalar(resp.Results[0]).(*ua.QualifiedName); ok && qn != nil {
		return qn.Name
	}
	return ""
}

func qualifiedNameString(q *ua.QualifiedName) string {
	if q == nil {
		return ""
	}
	return q.Name
}

func localizedTextString(l *ua.LocalizedText) string {
	if l == nil {
		return ""
	}
	return l.Text
}

// OpcuaReadResult is the wire shape of a single one-shot Read for the
// editor's "Read now" button. Mirrors the per-item record the Read node
// emits but stripped of flow-message scaffolding.
type OpcuaReadResult struct {
	NodeID          string `json:"nodeId"`
	StatusCode      string `json:"statusCode"`
	StatusCodeRaw   uint32 `json:"statusCodeRaw"`
	Value           any    `json:"value"`
	DataType        string `json:"dataType,omitempty"`
	SourceTimestamp string `json:"sourceTimestamp,omitempty"`
	ServerTimestamp string `json:"serverTimestamp,omitempty"`
}

// OpcuaReadOnce reads the Value attribute of a single NodeID against an
// already-running OpcuaServer session. Returns the decoded value plus
// metadata in JSON-friendly form. Server-defined structures are decoded if
// the schema is known; otherwise the {typeId,…} fallback shape is returned.
func OpcuaReadOnce(ctx context.Context, server *OpcuaServer, nodeIDStr string) (*OpcuaReadResult, error) {
	if server == nil {
		return nil, fmt.Errorf("no server")
	}
	client := server.Client()
	if client == nil {
		return nil, fmt.Errorf("no session")
	}

	// Strip a synthetic $field= suffix if present — single Read can't drill
	// into struct fields, so we read the parent variable and let the caller
	// pluck the field client-side from the structured value.
	if path, ok := splitSyntheticNodeID(nodeIDStr); ok {
		nodeIDStr = path.base
	}

	// Best-effort prewarm so the first Read of a struct already comes back
	// schema-decoded rather than the {typeId} fallback.
	if parsed, err := ParseOpcuaNodeID(nodeIDStr); err == nil {
		server.PrewarmStructForVariable(ctx, parsed)
	}

	parsed, err := ParseOpcuaNodeID(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid nodeId: %w", err)
	}
	resp, err := client.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: parsed, AttributeID: ua.AttributeIDValue, DataEncoding: &ua.QualifiedName{}},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("empty read response")
	}
	dv := resp.Results[0]
	out := &OpcuaReadResult{
		NodeID:        nodeIDStr,
		StatusCode:    OpcuaStatusCodeName(dv.Status),
		StatusCodeRaw: uint32(dv.Status),
	}
	if dv.Value != nil {
		out.Value = OpcuaValueToJSON(dv.Value, server)
		out.DataType = OpcuaTypeName(dv.Value.Type())
		// Mirror the Read/Subscribe-node behaviour: when the schema decoder
		// produced real fields, the value is sound — even if the server
		// flags BadDataTypeIDUnknown.
		if dv.Value.Type() == ua.TypeIDExtensionObject {
			if m, ok := out.Value.(map[string]any); ok {
				if _, hasErr := m["_decodeError"]; !hasErr {
					if _, hasRaw := m["_raw"]; !hasRaw && len(m) > 0 {
						if !OpcuaStatusCodeIsGood(dv.Status) {
							out.StatusCode = "Good"
							out.StatusCodeRaw = 0
						}
					}
				}
			}
		}
	}
	if !dv.SourceTimestamp.IsZero() {
		out.SourceTimestamp = dv.SourceTimestamp.UTC().Format(time.RFC3339Nano)
	}
	if !dv.ServerTimestamp.IsZero() {
		out.ServerTimestamp = dv.ServerTimestamp.UTC().Format(time.RFC3339Nano)
	}
	return out, nil
}

// OpcuaReadOnceAdHoc opens a transient session for one Read, then closes it.
// Mirrors OpcuaBrowseAdHoc for the pre-deploy editing case.
func OpcuaReadOnceAdHoc(ctx context.Context, cfg flow.ConfigNode, nodeID string) (*OpcuaReadResult, error) {
	inst, err := NewOpcuaServer(cfg)
	if err != nil {
		return nil, err
	}
	srv := inst.(*OpcuaServer)
	client, err := opcua.NewClient(srv.endpointURL, srv.clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Close(closeCtx)
		cancel()
	}()
	srv.mu.Lock()
	srv.client = client
	srv.mu.Unlock()
	defer func() {
		srv.mu.Lock()
		srv.client = nil
		srv.mu.Unlock()
	}()
	return OpcuaReadOnce(ctx, srv, nodeID)
}

// OpcuaBrowseAdHoc opens a transient session for one Browse call, then
// closes it. Used by the API endpoint when the editor wants to browse a
// server that is not yet deployed (typical pre-deploy editing).
func OpcuaBrowseAdHoc(ctx context.Context, cfg flow.ConfigNode, nodeID string) (*OpcuaBrowseResult, error) {
	inst, err := NewOpcuaServer(cfg)
	if err != nil {
		return nil, err
	}
	srv := inst.(*OpcuaServer)
	client, err := opcua.NewClient(srv.endpointURL, srv.clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Close(closeCtx)
		cancel()
	}()
	// Wire the transient server through so synthetic struct-field expansions
	// can reach the type resolver. The server's client field hasn't been set
	// (we manage the client locally), so plug it in for the duration of the
	// browse.
	srv.mu.Lock()
	srv.client = client
	srv.mu.Unlock()
	defer func() {
		srv.mu.Lock()
		srv.client = nil
		srv.mu.Unlock()
	}()
	return OpcuaBrowse(ctx, client, srv, nodeID)
}
