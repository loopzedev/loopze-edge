// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import "github.com/loopzedev/loopze-edge/internal/flow"

// BaseNode provides the engine-injected callbacks (Send / Status / Debug) and
// their setters. Concrete flow nodes embed this struct to avoid copying the
// same setter boilerplate in every implementation.
//
// Usage:
//
//	type MyNode struct {
//	    BaseNode
//	    // your config / state fields here
//	}
//
// The embedded methods (SetSend, SetStatus, SetDebug) automatically satisfy
// the corresponding parts of the flow.NodeInstance interface, so concrete
// nodes don't need to declare them.
type BaseNode struct {
	Send   flow.SendFunc
	Status flow.StatusFunc
	Debug  flow.DebugFunc
}

// SetSend is called by the engine before Start() to inject the send callback.
func (b *BaseNode) SetSend(fn flow.SendFunc) { b.Send = fn }

// SetStatus is called by the engine before Start() to inject the status callback.
func (b *BaseNode) SetStatus(fn flow.StatusFunc) { b.Status = fn }

// SetDebug is called by the engine before Start() to inject the debug callback.
func (b *BaseNode) SetDebug(fn flow.DebugFunc) { b.Debug = fn }
