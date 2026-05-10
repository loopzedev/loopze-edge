// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the network node group (TCP / UDP / HTTP) with the central
// registry hub. Framing helpers are exported from this package and reused
// by all stream-based clients.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "network",
		Description: "TCP / UDP / HTTP nodes (raw transport + request-response)",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "http-in", Factory: NewHTTPInNode, Info: HTTPInTypeInfo()},
			{Type: "http-response", Factory: NewHTTPResponseNode, Info: HTTPResponseTypeInfo()},
			{Type: "http-request", Factory: NewHTTPRequestNode, Info: HTTPRequestTypeInfo()},
			{Type: "tcp-in", Factory: NewTCPInNode, Info: TCPInTypeInfo()},
			{Type: "tcp-out", Factory: NewTCPOutNode, Info: TCPOutTypeInfo()},
			{Type: "tcp-request", Factory: NewTCPRequestNode, Info: TCPRequestTypeInfo()},
			{Type: "udp-in", Factory: NewUDPInNode, Info: UDPInTypeInfo()},
			{Type: "udp-out", Factory: NewUDPOutNode, Info: UDPOutTypeInfo()},
		},
	})
}
