// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package opcua

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the OPC UA node group with the central registry hub.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "opcua",
		Description: "OPC UA read / write / subscribe nodes (binary protocol, IEC 62541)",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "opcua-read", Factory: NewOpcuaReadNode, Info: OpcuaReadTypeInfo()},
			{Type: "opcua-write", Factory: NewOpcuaWriteNode, Info: OpcuaWriteTypeInfo()},
			{Type: "opcua-subscribe", Factory: NewOpcuaSubscribeNode, Info: OpcuaSubscribeTypeInfo()},
		},
		ConfigNodes: []nodes.ConfigNodeRegistration{
			{Type: "opcua-server", Factory: NewOpcuaServer, Info: OpcuaServerConfigTypeInfo()},
		},
	})
}
