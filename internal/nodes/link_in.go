// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"log/slog"

	"github.com/niceclouds/flint/internal/flow"
)

// LinkInNode is a source node that receives messages from link-out or link-call
// nodes via the LinkSendFunc mechanism and forwards them to its output port.
// It has 0 canvas inputs and 1 canvas output.
type LinkInNode struct {
	config   flow.NodeConfig
	send     flow.SendFunc
	status   flow.StatusFunc
	debug    flow.DebugFunc
	linkSend flow.LinkSendFunc

	links []string // IDs of link-out nodes (for bidirectional display in UI)
}

// NewLinkInNode is the NodeFactory for the link-in node type.
func NewLinkInNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &LinkInNode{config: config}, nil
}

// Init parses the node configuration.
func (n *LinkInNode) Init() error {
	if raw, ok := n.config.Properties["links"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					n.links = append(n.links, s)
				}
			}
		}
	}
	return nil
}

func (n *LinkInNode) SetSend(fn flow.SendFunc)       { n.send = fn }
func (n *LinkInNode) SetStatus(fn flow.StatusFunc)    { n.status = fn }
func (n *LinkInNode) SetDebug(fn flow.DebugFunc)      { n.debug = fn }
func (n *LinkInNode) SetLinkSend(fn flow.LinkSendFunc) { n.linkSend = fn }

// Start is a no-op for link-in (it only reacts to incoming messages).
func (n *LinkInNode) Start() error {
	slog.Info("link-in node started", "node_id", n.config.ID)
	return nil
}

// HandleMessage forwards the received message to output port 0.
// The engine's normal wire routing takes over from here.
func (n *LinkInNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	return [][]*flow.Message{{msg}}, nil
}

// Stop is a no-op for link-in.
func (n *LinkInNode) Stop() error {
	slog.Info("link-in node stopped", "node_id", n.config.ID)
	return nil
}

// LinkInTypeInfo returns the NodeTypeInfo for registering the link-in node.
func LinkInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "link-in",
		Category:    "common",
		Label:       "Link Input",
		Description: "Receives messages from Link Output nodes across flows",
		Icon:        "mdi-link",
		Defaults: map[string]any{
			"links": []any{},
		},
		Inputs:  0,
		Outputs: 1,
	}
}
