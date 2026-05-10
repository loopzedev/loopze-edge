// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"github.com/loopzedev/loopze-edge/internal/nodes/nodestest"
	"os"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func noopStatus(_ string, _ string) {}
func noopDebug(_ flow.DebugMessage)  {}

// makeProps builds a props config array from InjectProp-like maps.
func makeProps(props ...map[string]any) []any {
	result := make([]any, len(props))
	for i, p := range props {
		result[i] = p
	}
	return result
}

func startInject(t *testing.T, properties map[string]any) (*nodestest.Collector, flow.NodeInstance) {
	t.Helper()
	c := &nodestest.Collector{}
	node, err := NewInjectNode(flow.NodeConfig{
		ID:         t.Name(),
		Type:       "inject",
		Properties: properties,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	node.SetSend(c.Send)
	node.SetStatus(noopStatus)
	node.SetDebug(noopDebug)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	return c, node
}

func TestInjectOnce(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "str", "v": "hello"},
			map[string]any{"p": "topic", "vt": "str", "v": "test/topic"},
		),
	})
	defer node.Stop()

	if c.Count() != 1 {
		t.Fatalf("expected 1 message, got %d", c.Count())
	}

	msg := c.Last()
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
	c, node := startInject(t, map[string]any{
		"interval": float64(50),
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "num", "v": "42"},
		),
	})

	time.Sleep(180 * time.Millisecond)
	node.Stop()

	count := c.Count()
	if count < 3 {
		t.Fatalf("expected at least 3 messages, got %d", count)
	}
	if c.Last().Payload() != float64(42) {
		t.Errorf("expected payload 42, got %v", c.Last().Payload())
	}
}

func TestInjectOnceAndInterval(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once":     true,
		"interval": float64(50),
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "str", "v": "combo"},
		),
	})

	time.Sleep(120 * time.Millisecond)
	node.Stop()

	if c.Count() < 3 {
		t.Fatalf("expected at least 3 messages (1 once + 2 interval), got %d", c.Count())
	}
}

func TestInjectDefaultPayloadIsTimestamp(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		// no props → default: payload=timestamp(rfc3339)
	})
	defer node.Stop()

	msg := c.Last()
	ts, ok := msg.Payload().(string)
	if !ok {
		t.Fatalf("expected string payload (timestamp), got %T", msg.Payload())
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("expected RFC3339Nano timestamp, got %q: %v", ts, err)
	}
}

func TestInjectPayloadDateEpoch(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "date", "v": "epoch"},
		),
	})
	defer node.Stop()

	epoch, ok := c.Last().Payload().(float64)
	if !ok {
		t.Fatalf("expected float64 payload (epoch ms), got %T", c.Last().Payload())
	}
	now := float64(time.Now().UnixMilli())
	if epoch < now-60000 || epoch > now+1000 {
		t.Errorf("epoch %v is not a recent timestamp", epoch)
	}
}

func TestInjectPayloadJSON(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "json", "v": `{"key": "value", "num": 42}`},
		),
	})
	defer node.Stop()

	m, ok := c.Last().Payload().(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", c.Last().Payload())
	}
	if m["key"] != "value" {
		t.Errorf("expected key='value', got %v", m["key"])
	}
}

func TestInjectPayloadBool(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "bool", "v": "true"},
		),
	})
	defer node.Stop()

	if c.Last().Payload() != true {
		t.Errorf("expected payload true, got %v", c.Last().Payload())
	}
}

func TestInjectPayloadEnv(t *testing.T) {
	os.Setenv("LOOPZE_TEST_INJECT", "env-value")
	defer os.Unsetenv("LOOPZE_TEST_INJECT")

	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "env", "v": "LOOPZE_TEST_INJECT"},
		),
	})
	defer node.Stop()

	if c.Last().Payload() != "env-value" {
		t.Errorf("expected payload 'env-value', got %v", c.Last().Payload())
	}
}

func TestInjectManualTrigger(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "str", "v": "manual"},
		),
	})
	defer node.Stop()

	if c.Count() != 0 {
		t.Fatalf("expected 0 messages before manual trigger, got %d", c.Count())
	}

	node.HandleMessage(nil)

	if c.Count() != 1 {
		t.Fatalf("expected 1 message after manual trigger, got %d", c.Count())
	}
	if c.Last().Payload() != "manual" {
		t.Errorf("expected payload 'manual', got %v", c.Last().Payload())
	}
}

func TestInjectMultipleProps(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "str", "v": "hello"},
			map[string]any{"p": "topic", "vt": "str", "v": "test/multi"},
			map[string]any{"p": "qos", "vt": "num", "v": "1"},
			map[string]any{"p": "retain", "vt": "bool", "v": "true"},
		),
	})
	defer node.Stop()

	msg := c.Last()
	if msg.Payload() != "hello" {
		t.Errorf("payload: want 'hello', got %v", msg.Payload())
	}
	if msg.Topic() != "test/multi" {
		t.Errorf("topic: want 'test/multi', got %q", msg.Topic())
	}
	if msg.Get("qos") != float64(1) {
		t.Errorf("qos: want 1, got %v", msg.Get("qos"))
	}
	if msg.Get("retain") != true {
		t.Errorf("retain: want true, got %v", msg.Get("retain"))
	}
}

func TestInjectEmptyProps(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once":  true,
		"props": makeProps(), // empty list
	})
	defer node.Stop()

	// Empty props → default timestamp payload
	msg := c.Last()
	_, ok := msg.Payload().(string)
	if !ok {
		t.Fatalf("expected string payload (default timestamp), got %T", msg.Payload())
	}
}

func TestInjectStopIsClean(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"interval": float64(10),
	})

	time.Sleep(50 * time.Millisecond)
	node.Stop()

	countAfterStop := c.Count()
	time.Sleep(50 * time.Millisecond)

	if c.Count() != countAfterStop {
		t.Errorf("messages sent after Stop(): before=%d after=%d", countAfterStop, c.Count())
	}
}

func TestInjectMessageIDsAreUnique(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"interval": float64(10),
	})

	time.Sleep(60 * time.Millisecond)
	node.Stop()

	seen := make(map[string]bool)
	for _, msg := range c.Snapshot() {
		if seen[msg.ID()] {
			t.Errorf("duplicate message ID: %s", msg.ID())
		}
		seen[msg.ID()] = true
	}
}
