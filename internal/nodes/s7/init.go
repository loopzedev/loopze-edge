// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the SIEMENS S7 node group with the central registry hub.
// The group is enabled by default; users may opt out via runtime config.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "s7",
		Description: "SIEMENS S7 read / write / parser nodes (RFC1006 / ISO-on-TCP)",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "s7-read", Factory: NewS7ReadNode, Info: S7ReadTypeInfo()},
			{Type: "s7-write", Factory: NewS7WriteNode, Info: S7WriteTypeInfo()},
			{Type: "s7-parser", Factory: NewS7ParserNode, Info: S7ParserTypeInfo()},
		},
		ConfigNodes: []nodes.ConfigNodeRegistration{
			{Type: "s7-plc", Factory: NewS7PLC, Info: S7PLCConfigTypeInfo()},
		},
	})
}
