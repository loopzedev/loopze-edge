// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"log/slog"

	"github.com/niceclouds/flint/internal/flow"
)

// LinkCallNode implements a request/response pattern across flows.
// It sends a message to a configured link-in node and waits for the response
// to arrive back via a link-out node. It has 1 canvas input and 1 canvas output.
//
// The mechanism works by setting "_linkSource" to the node's own ID on outgoing
// messages. When a link-out node encounters this field, it routes the message
// back to this node instead of its normal targets. The returning message is
// detected by matching _linkSource == own ID, cleaned up, and emitted at port 0.
type LinkCallNode struct {
	config   flow.NodeConfig
	send     flow.SendFunc
	status   flow.StatusFunc
	debug    flow.DebugFunc
	linkSend flow.LinkSendFunc

	linkTarget string // Single link-in node ID to send requests to
}

// NewLinkCallNode is the NodeFactory for the link-call node type.
func NewLinkCallNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &LinkCallNode{config: config}, nil
}

// Init parses the node configuration.
func (n *LinkCallNode) Init() error {
	if v, ok := n.config.Properties["linkTarget"].(string); ok {
		n.linkTarget = v
	}
	return nil
}

func (n *LinkCallNode) SetSend(fn flow.SendFunc)       { n.send = fn }
func (n *LinkCallNode) SetStatus(fn flow.StatusFunc)    { n.status = fn }
func (n *LinkCallNode) SetDebug(fn flow.DebugFunc)      { n.debug = fn }
func (n *LinkCallNode) SetLinkSend(fn flow.LinkSendFunc) { n.linkSend = fn }

// Start is a no-op for link-call.
func (n *LinkCallNode) Start() error {
	slog.Info("link-call node started", "node_id", n.config.ID, "target", n.linkTarget)
	return nil
}

// HandleMessage processes incoming messages. Two cases:
//  1. Response: The message has _linkSource == own ID → clean up and emit at port 0.
//  2. New request: Set _linkSource to own ID and send to the configured link-in target.
func (n *LinkCallNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil || n.linkSend == nil {
		return nil, nil
	}

	// Case 1: This is a response coming back from the target flow.
	if linkSource, ok := msg.Get("_linkSource").(string); ok && linkSource == n.config.ID {
		msg.Delete("_linkSource")
		return [][]*flow.Message{{msg}}, nil
	}

	// Case 2: New request — stamp with own ID and send to target.
	if n.linkTarget == "" {
		return nil, nil
	}
	msg.Set("_linkSource", n.config.ID)
	n.linkSend(n.linkTarget, msg)

	return nil, nil
}

// Stop is a no-op for link-call.
func (n *LinkCallNode) Stop() error {
	slog.Info("link-call node stopped", "node_id", n.config.ID)
	return nil
}

// LinkCallTypeInfo returns the NodeTypeInfo for registering the link-call node.
func LinkCallTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "link-call",
		Category:    "common",
		Label:       "Link Request",
		Description: "Sends a request to a Link Input node and receives the response",
		Icon:        "mdi-link",
		Defaults: map[string]any{
			"linkTarget": "",
		},
		Inputs:  1,
		Outputs: 1,
	}
}
