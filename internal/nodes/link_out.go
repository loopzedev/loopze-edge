// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"log/slog"

	"github.com/niceclouds/loopze/internal/flow"
)

// LinkOutNode is a sink node that forwards messages to configured link-in nodes
// via the LinkSendFunc mechanism. It has 1 canvas input and 0 canvas outputs.
//
// Request/Response: If the incoming message carries a "_linkSource" field
// (set by a link-call node), the message is routed back to the requesting
// node instead of the configured targets.
type LinkOutNode struct {
	config   flow.NodeConfig
	send     flow.SendFunc
	status   flow.StatusFunc
	debug    flow.DebugFunc
	linkSend flow.LinkSendFunc

	links []string // Target link-in node IDs
}

// NewLinkOutNode is the NodeFactory for the link-out node type.
func NewLinkOutNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &LinkOutNode{config: config}, nil
}

// Init parses the node configuration.
func (n *LinkOutNode) Init() error {
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

func (n *LinkOutNode) SetSend(fn flow.SendFunc)       { n.send = fn }
func (n *LinkOutNode) SetStatus(fn flow.StatusFunc)    { n.status = fn }
func (n *LinkOutNode) SetDebug(fn flow.DebugFunc)      { n.debug = fn }
func (n *LinkOutNode) SetLinkSend(fn flow.LinkSendFunc) { n.linkSend = fn }

// Start is a no-op for link-out.
func (n *LinkOutNode) Start() error {
	slog.Info("link-out node started", "node_id", n.config.ID, "targets", len(n.links))
	return nil
}

// HandleMessage routes the message to configured link-in targets, or back to
// the requesting link-call node if _linkSource is present.
func (n *LinkOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.linkSend == nil {
		return nil, nil
	}

	// Request/Response: route back to the calling link-call node.
	if linkSource, ok := msg.Get("_linkSource").(string); ok && linkSource != "" {
		n.linkSend(linkSource, msg)
		return nil, nil
	}

	// Normal mode: forward to all configured link-in targets.
	for _, targetID := range n.links {
		n.linkSend(targetID, msg)
	}

	return nil, nil
}

// Stop is a no-op for link-out.
func (n *LinkOutNode) Stop() error {
	slog.Info("link-out node stopped", "node_id", n.config.ID)
	return nil
}

// LinkOutTypeInfo returns the NodeTypeInfo for registering the link-out node.
func LinkOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "link-out",
		Category:    "common",
		Label:       "Link Output",
		Description: "Sends messages to Link Input nodes across flows",
		Icon:        "mdi-link",
		Defaults: map[string]any{
			"links": []any{},
		},
		Inputs:  1,
		Outputs: 0,
	}
}
