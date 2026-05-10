// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodestest

import (
	"sync"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Collector captures messages sent via flow.SendFunc. Tests wire its Send
// method onto a node's BaseNode.Send field and then assert on the captured
// stream via Count, Last, or Snapshot.
type Collector struct {
	mu   sync.Mutex
	msgs []*flow.Message
}

// Send is the SendFunc-shaped capture method.
func (c *Collector) Send(_ int, msg *flow.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
}

// Count returns the number of messages captured so far.
func (c *Collector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

// At returns the message at index i, or nil if out of range.
func (c *Collector) At(i int) *flow.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < 0 || i >= len(c.msgs) {
		return nil
	}
	return c.msgs[i]
}

// Last returns the most recent captured message, or nil if none yet.
func (c *Collector) Last() *flow.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.msgs) == 0 {
		return nil
	}
	return c.msgs[len(c.msgs)-1]
}

// Snapshot returns a copy of the captured stream. Use this instead of reading
// the underlying slice directly when a producer may still be writing — direct
// reads race with Send.
func (c *Collector) Snapshot() []*flow.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*flow.Message, len(c.msgs))
	copy(out, c.msgs)
	return out
}
