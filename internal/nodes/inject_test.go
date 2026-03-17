// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"sync"
	"testing"
	"time"

	"github.com/niceclouds/flint/internal/flow"
)

// collector is a test helper that captures messages sent via SendFunc.
type collector struct {
	mu   sync.Mutex
	msgs []*flow.Message
}

func (c *collector) send(_ int, msg *flow.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, msg)
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

func (c *collector) last() *flow.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.msgs) == 0 {
		return nil
	}
	return c.msgs[len(c.msgs)-1]
}

func TestInjectOnce(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-once",
		Type: "inject",
		Properties: map[string]any{
			"once":    true,
			"payload": "hello",
			"topic":   "test/topic",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	if c.count() != 1 {
		t.Fatalf("expected 1 message, got %d", c.count())
	}

	msg := c.last()
	if msg.Payload() != "hello" {
		t.Errorf("expected payload 'hello', got %v", msg.Payload())
	}
	if msg.Topic() != "test/topic" {
		t.Errorf("expected topic 'test/topic', got %q", msg.Topic())
	}
	if msg.ID() == "" {
		t.Error("expected message to have an ID")
	}
}

func TestInjectInterval(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-interval",
		Type: "inject",
		Properties: map[string]any{
			"interval": float64(50), // 50ms
			"payload":  42.0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for at least 3 ticks.
	time.Sleep(180 * time.Millisecond)
	node.Stop()

	count := c.count()
	if count < 3 {
		t.Fatalf("expected at least 3 messages, got %d", count)
	}

	msg := c.last()
	if msg.Payload() != 42.0 {
		t.Errorf("expected payload 42, got %v", msg.Payload())
	}
}

func TestInjectOnceAndInterval(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-both",
		Type: "inject",
		Properties: map[string]any{
			"once":     true,
			"interval": float64(50),
			"payload":  "combo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}

	// 1 immediate + at least 2 from ticker.
	time.Sleep(120 * time.Millisecond)
	node.Stop()

	count := c.count()
	if count < 3 {
		t.Fatalf("expected at least 3 messages (1 once + 2 interval), got %d", count)
	}
}

func TestInjectDefaultPayloadIsTimestamp(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-default",
		Type: "inject",
		Properties: map[string]any{
			"once": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	msg := c.last()
	ts, ok := msg.Payload().(string)
	if !ok {
		t.Fatalf("expected string payload (timestamp), got %T", msg.Payload())
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("expected RFC3339Nano timestamp, got %q: %v", ts, err)
	}
}

func TestInjectManualTrigger(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-manual",
		Type: "inject",
		Properties: map[string]any{
			"payload": "manual",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// No messages yet (neither once nor interval).
	if c.count() != 0 {
		t.Fatalf("expected 0 messages before manual trigger, got %d", c.count())
	}

	// Manual trigger via HandleMessage.
	node.HandleMessage(nil)

	if c.count() != 1 {
		t.Fatalf("expected 1 message after manual trigger, got %d", c.count())
	}
	if c.last().Payload() != "manual" {
		t.Errorf("expected payload 'manual', got %v", c.last().Payload())
	}
}

func TestInjectStopIsClean(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-stop",
		Type: "inject",
		Properties: map[string]any{
			"interval": float64(10), // fast ticker
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	node.Stop()

	countAfterStop := c.count()
	time.Sleep(50 * time.Millisecond)

	// No new messages after stop.
	if c.count() != countAfterStop {
		t.Errorf("messages sent after Stop(): before=%d after=%d", countAfterStop, c.count())
	}
}

func TestInjectMessageIDsAreUnique(t *testing.T) {
	c := &collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:   "test-unique-ids",
		Type: "inject",
		Properties: map[string]any{
			"interval": float64(10),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.send)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(60 * time.Millisecond)
	node.Stop()

	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[string]bool)
	for _, msg := range c.msgs {
		if seen[msg.ID()] {
			t.Errorf("duplicate message ID: %s", msg.ID())
		}
		seen[msg.ID()] = true
	}
}
