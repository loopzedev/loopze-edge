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

// dataTypeNodeIDToTypeID maps the canonical OPC UA built-in DataType NodeIDs
// (namespace 0) to the Variant TypeID used when wire-encoding values. The set
// covers everything the Write node currently supports; complex/abstract types
// (Structure=22) resolve to ExtensionObject and are handled by Phase 5's
// dedicated schema layer.
//
// Source: OPC UA Part 6 §5.1.2 (Built-in Types).
var dataTypeNodeIDToTypeID = map[uint32]ua.TypeID{
	1:  ua.TypeIDBoolean,
	2:  ua.TypeIDSByte,
	3:  ua.TypeIDByte,
	4:  ua.TypeIDInt16,
	5:  ua.TypeIDUint16,
	6:  ua.TypeIDInt32,
	7:  ua.TypeIDUint32,
	8:  ua.TypeIDInt64,
	9:  ua.TypeIDUint64,
	10: ua.TypeIDFloat,
	11: ua.TypeIDDouble,
	12: ua.TypeIDString,
	13: ua.TypeIDDateTime,
	14: ua.TypeIDGUID,
	15: ua.TypeIDByteString,
	16: ua.TypeIDXMLElement,
	17: ua.TypeIDNodeID,
	18: ua.TypeIDExpandedNodeID,
	19: ua.TypeIDStatusCode,
	20: ua.TypeIDQualifiedName,
	21: ua.TypeIDLocalizedText,
	22: ua.TypeIDExtensionObject, // Structure
	24: ua.TypeIDVariant,         // BaseDataType
}

// typeCache is the per-OpcuaServer cache of NodeID → TypeID lookups. Fully
// invalidated on reconnect so the next Write picks up any server-side schema
// changes that happened while we were disconnected.
type typeCache struct {
	mu      sync.RWMutex
	entries map[string]ua.TypeID
}

func newTypeCache() *typeCache {
	return &typeCache{entries: make(map[string]ua.TypeID)}
}

func (c *typeCache) get(key string) (ua.TypeID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.entries[key]
	return t, ok
}

func (c *typeCache) set(key string, t ua.TypeID) {
	c.mu.Lock()
	c.entries[key] = t
	c.mu.Unlock()
}

func (c *typeCache) reset() {
	c.mu.Lock()
	c.entries = make(map[string]ua.TypeID)
	c.mu.Unlock()
}

// LookupDataType returns the wire-level TypeID for the value attribute of the
// given NodeID, consulting the cache first and falling back to a Read of the
// DataType attribute on miss. Never blocks the caller longer than the
// configured request timeout.
func (s *OpcuaServer) LookupDataType(ctx context.Context, nodeID *ua.NodeID) (ua.TypeID, error) {
	if nodeID == nil {
		return 0, fmt.Errorf("nil nodeID")
	}
	key := nodeID.String()

	s.mu.RLock()
	cache := s.typeCache
	client := s.client
	s.mu.RUnlock()

	if cache != nil {
		if t, ok := cache.get(key); ok {
			return t, nil
		}
	}
	if client == nil {
		return 0, fmt.Errorf("no session")
	}

	resp, err := client.Read(ctx, &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: nodeID, AttributeID: ua.AttributeIDDataType},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("read DataType attr: %w", err)
	}
	if len(resp.Results) == 0 || resp.Results[0].Status != ua.StatusOK {
		status := ua.StatusOK
		if len(resp.Results) > 0 {
			status = resp.Results[0].Status
		}
		return 0, fmt.Errorf("read DataType attr: %s", OpcuaStatusCodeName(status))
	}
	dv := resp.Results[0]
	if dv.Value == nil {
		return 0, fmt.Errorf("DataType attribute returned no value")
	}
	dtNodeID, ok := dv.Value.Value().(*ua.NodeID)
	if !ok || dtNodeID == nil {
		return 0, fmt.Errorf("unexpected DataType attribute kind: %T", dv.Value.Value())
	}
	// Built-in DataTypes live in namespace 0 with numeric IDs we can map directly.
	// Custom DataTypes (server-defined structures) will be ExtensionObject — the
	// Phase-5 type resolver picks those up by NodeID.
	if dtNodeID.Namespace() == 0 {
		if t, ok := dataTypeNodeIDToTypeID[dtNodeID.IntID()]; ok {
			if cache != nil {
				cache.set(key, t)
			}
			return t, nil
		}
	}
	// Fallback: treat unknown DataTypes as structures (ExtensionObject).
	if cache != nil {
		cache.set(key, ua.TypeIDExtensionObject)
	}
	return ua.TypeIDExtensionObject, nil
}

// ResetTypeCache drops every cached DataType. Called from watchState whenever
// the connection drops so the next Write/Subscribe round-trip re-discovers
// the schema rather than acting on stale info.
func (s *OpcuaServer) ResetTypeCache() {
	s.mu.RLock()
	c := s.typeCache
	s.mu.RUnlock()
	if c != nil {
		c.reset()
	}
}
