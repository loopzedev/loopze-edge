// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package flow

// LinkSendFunc is a callback that link nodes use to send messages directly
// to another node by its ID — independent of wire-based routing.
// This enables cross-flow messaging between link-in, link-out, and link-call nodes.
type LinkSendFunc func(targetNodeID string, msg *Message)

// LinkProvider is an optional interface that link nodes implement to receive
// a callback for cross-flow messaging. The engine checks each NodeInstance
// after wiring:
//
//	if lp, ok := instance.(LinkProvider); ok {
//	    lp.SetLinkSend(linkSendFunc)
//	}
type LinkProvider interface {
	SetLinkSend(fn LinkSendFunc)
}
