// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package modbus

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the Modbus node group with the central registry hub.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "modbus",
		Description: "Modbus TCP / RTU read / write / parser nodes",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "modbus-read", Factory: NewModbusReadNode, Info: ModbusReadTypeInfo()},
			{Type: "modbus-write", Factory: NewModbusWriteNode, Info: ModbusWriteTypeInfo()},
			{Type: "modbus-parser", Factory: NewModbusParserNode, Info: ModbusParserTypeInfo()},
		},
		ConfigNodes: []nodes.ConfigNodeRegistration{
			{Type: "modbus-server", Factory: NewModbusServer, Info: ModbusServerConfigTypeInfo()},
		},
	})
}
