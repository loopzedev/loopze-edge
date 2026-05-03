// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gopcua/opcua/ua"
)

// nodeIDPattern requires the canonical "[ns=N;]<i|s|g|b>=…" form. The gopcua
// parser is otherwise quite forgiving and would happily turn any free-form
// string into a namespace-0 string identifier, hiding typos that the user
// almost certainly meant to be flagged.
var nodeIDPattern = regexp.MustCompile(`^(?:ns=\d+;)?[isgb]=.+$`)

// ParseOpcuaNodeID parses an OPC UA NodeID string in the standard form
// "ns=<index>;<i|s|g|b>=<identifier>". A bare "i=<n>" is treated as namespace 0.
func ParseOpcuaNodeID(s string) (*ua.NodeID, error) {
	if s == "" {
		return nil, fmt.Errorf("empty NodeID")
	}
	if !nodeIDPattern.MatchString(s) {
		return nil, fmt.Errorf("invalid NodeID syntax %q (expected [ns=N;]<i|s|g|b>=…)", s)
	}
	return ua.ParseNodeID(s)
}

// FormatOpcuaNodeID renders a NodeID back into its canonical "ns=N;X=…" string
// form. Used so we always emit a stable, round-trippable representation in
// outgoing flow messages regardless of how the server delivered the NodeID
// internally.
func FormatOpcuaNodeID(id *ua.NodeID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// opcuaScalarTypes maps LOOPZE's stable string names to gopcua TypeIDs for
// scalar Value-encoding. ExtensionObject is a sentinel that requires extra
// schema lookup and is handled separately by the type resolver.
var opcuaScalarTypes = map[string]ua.TypeID{
	"Boolean":         ua.TypeIDBoolean,
	"SByte":           ua.TypeIDSByte,
	"Byte":            ua.TypeIDByte,
	"Int16":           ua.TypeIDInt16,
	"UInt16":          ua.TypeIDUint16,
	"Int32":           ua.TypeIDInt32,
	"UInt32":          ua.TypeIDUint32,
	"Int64":           ua.TypeIDInt64,
	"UInt64":          ua.TypeIDUint64,
	"Float":           ua.TypeIDFloat,
	"Double":          ua.TypeIDDouble,
	"String":          ua.TypeIDString,
	"DateTime":        ua.TypeIDDateTime,
	"GUID":            ua.TypeIDGUID,
	"ByteString":      ua.TypeIDByteString,
	"NodeID":          ua.TypeIDNodeID,
	"ExtensionObject": ua.TypeIDExtensionObject,
	"Variant":         ua.TypeIDVariant,
}

// opcuaTypeNames is the inverse of opcuaScalarTypes for stable name output.
var opcuaTypeNames = func() map[ua.TypeID]string {
	m := make(map[ua.TypeID]string, len(opcuaScalarTypes))
	for name, id := range opcuaScalarTypes {
		m[id] = name
	}
	return m
}()

// OpcuaTypeIDFromName resolves a LOOPZE-stable type name to a gopcua TypeID.
// Returns false for unknown names; callers fall back to Variant.
func OpcuaTypeIDFromName(name string) (ua.TypeID, bool) {
	t, ok := opcuaScalarTypes[name]
	return t, ok
}

// OpcuaTypeName returns a stable LOOPZE string for a TypeID, or "Unknown" if
// the TypeID is not in the well-known set. Used in flow message metadata.
func OpcuaTypeName(t ua.TypeID) string {
	if name, ok := opcuaTypeNames[t]; ok {
		return name
	}
	return "Unknown"
}

// opcuaAttributeIDs lists the attributes LOOPZE exposes by name in flow messages
// and config. Other attributes can still be requested by passing the numeric
// AttributeID, but the named ones cover the operational subset.
var opcuaAttributeIDs = map[string]ua.AttributeID{
	"NodeID":              ua.AttributeIDNodeID,
	"NodeClass":           ua.AttributeIDNodeClass,
	"BrowseName":          ua.AttributeIDBrowseName,
	"DisplayName":         ua.AttributeIDDisplayName,
	"Description":         ua.AttributeIDDescription,
	"Value":               ua.AttributeIDValue,
	"DataType":            ua.AttributeIDDataType,
	"ValueRank":           ua.AttributeIDValueRank,
	"ArrayDimensions":     ua.AttributeIDArrayDimensions,
	"AccessLevel":         ua.AttributeIDAccessLevel,
	"UserAccessLevel":     ua.AttributeIDUserAccessLevel,
	"Historizing":         ua.AttributeIDHistorizing,
	"DataTypeDefinition":  ua.AttributeIDDataTypeDefinition,
	"AccessLevelEx":       ua.AttributeIDAccessLevelEx,
}

// OpcuaAttributeIDFromName returns the AttributeID for a stable name, or
// AttributeIDValue as the operational default if the name is empty/unknown.
// The bool indicates whether the lookup hit a known name.
func OpcuaAttributeIDFromName(name string) (ua.AttributeID, bool) {
	if name == "" {
		return ua.AttributeIDValue, false
	}
	id, ok := opcuaAttributeIDs[name]
	if !ok {
		return ua.AttributeIDValue, false
	}
	return id, true
}

// OpcuaStatusCodeName returns the symbolic status name (e.g. "Good",
// "BadNodeIDUnknown") for a StatusCode. The gopcua names carry a "Status"
// prefix which we strip so callers get the spec-aligned identifier. Unknown
// codes are formatted as "Bad_0x<hex>" so callers always get a non-empty,
// comparable string.
func OpcuaStatusCodeName(code ua.StatusCode) string {
	if desc, ok := ua.StatusCodes[code]; ok {
		return strings.TrimPrefix(desc.Name, "Status")
	}
	if code == 0 {
		return "Good"
	}
	return fmt.Sprintf("Bad_0x%X", uint32(code))
}

// OpcuaStatusCodeIsGood reports whether a StatusCode is in the Good severity
// band. Severity is encoded in the top two bits per OPC UA Part 4.
func OpcuaStatusCodeIsGood(code ua.StatusCode) bool {
	return uint32(code)&0xC0000000 == 0
}
