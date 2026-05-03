// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
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

func startInject(t *testing.T, properties map[string]any) (*collector, flow.NodeInstance) {
	t.Helper()
	c := &collector{}
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
	node.SetSend(c.send)
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
	c, node := startInject(t, map[string]any{
		"interval": float64(50),
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "num", "v": "42"},
		),
	})

	time.Sleep(180 * time.Millisecond)
	node.Stop()

	count := c.count()
	if count < 3 {
		t.Fatalf("expected at least 3 messages, got %d", count)
	}
	if c.last().Payload() != float64(42) {
		t.Errorf("expected payload 42, got %v", c.last().Payload())
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

	if c.count() < 3 {
		t.Fatalf("expected at least 3 messages (1 once + 2 interval), got %d", c.count())
	}
}

func TestInjectDefaultPayloadIsTimestamp(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		// no props → default: payload=timestamp(rfc3339)
	})
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

func TestInjectPayloadDateEpoch(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"once": true,
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "date", "v": "epoch"},
		),
	})
	defer node.Stop()

	epoch, ok := c.last().Payload().(float64)
	if !ok {
		t.Fatalf("expected float64 payload (epoch ms), got %T", c.last().Payload())
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

	m, ok := c.last().Payload().(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", c.last().Payload())
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

	if c.last().Payload() != true {
		t.Errorf("expected payload true, got %v", c.last().Payload())
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

	if c.last().Payload() != "env-value" {
		t.Errorf("expected payload 'env-value', got %v", c.last().Payload())
	}
}

func TestInjectManualTrigger(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"props": makeProps(
			map[string]any{"p": "payload", "vt": "str", "v": "manual"},
		),
	})
	defer node.Stop()

	if c.count() != 0 {
		t.Fatalf("expected 0 messages before manual trigger, got %d", c.count())
	}

	node.HandleMessage(nil)

	if c.count() != 1 {
		t.Fatalf("expected 1 message after manual trigger, got %d", c.count())
	}
	if c.last().Payload() != "manual" {
		t.Errorf("expected payload 'manual', got %v", c.last().Payload())
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

	msg := c.last()
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
	msg := c.last()
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

	countAfterStop := c.count()
	time.Sleep(50 * time.Millisecond)

	if c.count() != countAfterStop {
		t.Errorf("messages sent after Stop(): before=%d after=%d", countAfterStop, c.count())
	}
}

func TestInjectMessageIDsAreUnique(t *testing.T) {
	c, node := startInject(t, map[string]any{
		"interval": float64(10),
	})

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
