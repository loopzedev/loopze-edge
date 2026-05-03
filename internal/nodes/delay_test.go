// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// startDelay wires up a DelayNode with a collector and runs through the lifecycle.
func startDelay(t *testing.T, properties map[string]any) (*collector, flow.NodeInstance) {
	t.Helper()
	c := &collector{}
	node, err := NewDelayNode(flow.NodeConfig{
		ID:         t.Name(),
		Type:       "delay",
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

// makeMsg builds a fresh message with the given payload (and optional extra fields).
func makeMsg(payload any, extra ...map[string]any) *flow.Message {
	m := flow.NewMessage()
	m.SetPayload(payload)
	for _, kv := range extra {
		for k, v := range kv {
			m.Set(k, v)
		}
	}
	return m
}

// --- Phase 2: mode "delay" -------------------------------------------------

func TestDelayMode_FixedTimeout(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(80),
		"timeoutUnits": "milliseconds",
	})
	defer node.Stop()

	start := time.Now()
	node.HandleMessage(makeMsg("hi"))

	// Immediately after the call the message must not yet have been forwarded.
	if c.count() != 0 {
		t.Fatalf("expected 0 messages right after handle, got %d", c.count())
	}

	// Within a generous window the timer should have fired.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	if c.count() != 1 {
		t.Fatalf("expected 1 message after timeout, got %d", c.count())
	}
	elapsed := time.Since(start)
	if elapsed < 70*time.Millisecond {
		t.Errorf("message released too early: %v < 70ms", elapsed)
	}
	if c.last().Payload() != "hi" {
		t.Errorf("payload mismatch: got %v", c.last().Payload())
	}
}

func TestDelayMode_PreservesOrder(t *testing.T) {
	// All messages have the same fixed delay → FIFO must be preserved.
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(60),
		"timeoutUnits": "milliseconds",
	})
	defer node.Stop()

	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
		time.Sleep(5 * time.Millisecond)
	}

	deadline := time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() < 5 {
		time.Sleep(5 * time.Millisecond)
	}

	if c.count() != 5 {
		t.Fatalf("expected 5 messages, got %d", c.count())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, m := range c.msgs {
		if m.Payload() != float64(i) {
			t.Errorf("message %d: want payload %d, got %v", i, i, m.Payload())
		}
	}
}

func TestDelayMode_ZeroTimeoutPassesThrough(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode":    "delay",
		"timeout": float64(0),
	})
	defer node.Stop()

	node.HandleMessage(makeMsg("x"))

	if c.count() != 1 {
		t.Fatalf("expected immediate pass-through, got %d", c.count())
	}
}

func TestUnitsConversion(t *testing.T) {
	cases := []struct {
		unit string
		want time.Duration
	}{
		{"milliseconds", 2 * time.Millisecond},
		{"millisecond", 2 * time.Millisecond},
		{"ms", 2 * time.Millisecond},
		{"seconds", 2 * time.Second},
		{"second", 2 * time.Second},
		{"s", 2 * time.Second},
		{"minutes", 2 * time.Minute},
		{"hours", 2 * time.Hour},
		{"day", 48 * time.Hour},
	}
	for _, tc := range cases {
		got := parseDelayUnits(2, tc.unit)
		if got != tc.want {
			t.Errorf("unit=%q: want %v, got %v", tc.unit, tc.want, got)
		}
	}

	// Unknown unit falls back to milliseconds.
	if got := parseDelayUnits(2, "fortnights"); got != 2*time.Millisecond {
		t.Errorf("unknown unit: want 2ms fallback, got %v", got)
	}

	// Zero / negative values produce zero duration.
	if got := parseDelayUnits(0, "seconds"); got != 0 {
		t.Errorf("zero value: want 0, got %v", got)
	}
}

func TestStopDiscardsPending(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(500),
		"timeoutUnits": "milliseconds",
	})

	for i := 0; i < 10; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}

	// Stop before any timer fires.
	time.Sleep(20 * time.Millisecond)
	if c.count() != 0 {
		t.Fatalf("expected 0 messages before stop, got %d", c.count())
	}

	node.Stop()

	// Wait past the original timeout and confirm nothing slipped through.
	time.Sleep(600 * time.Millisecond)
	if c.count() != 0 {
		t.Errorf("expected pending messages to be discarded on stop, got %d", c.count())
	}
}

// --- Phase 3: mode "random" + per-message overrides -----------------------

func TestRandomMode_WithinBounds(t *testing.T) {
	const minMs = 30
	const maxMs = 90
	c, node := startDelay(t, map[string]any{
		"mode":         "random",
		"randomFirst":  float64(minMs),
		"randomLast":   float64(maxMs),
		"randomUnits":  "milliseconds",
	})
	defer node.Stop()

	const samples = 30
	starts := make([]time.Time, samples)
	for i := 0; i < samples; i++ {
		starts[i] = time.Now()
		node.HandleMessage(makeMsg(float64(i)))
	}

	// Wait for everything to drain (max delay + buffer).
	deadline := time.Now().Add(time.Duration(maxMs+200) * time.Millisecond)
	for time.Now().Before(deadline) && c.count() < samples {
		time.Sleep(5 * time.Millisecond)
	}
	if c.count() != samples {
		t.Fatalf("expected %d messages, got %d", samples, c.count())
	}

	// We can't tie outputs back to inputs by index (random reorders),
	// but we can verify total elapsed never exceeds (max + grace) for any send.
	// Instead, check the time between first input and last output is <= maxMs+grace.
	elapsed := time.Since(starts[0])
	if elapsed < (minMs-5)*time.Millisecond {
		t.Errorf("drained too quickly: %v < min %dms", elapsed, minMs)
	}
	if elapsed > (maxMs+200)*time.Millisecond {
		t.Errorf("drained too slowly: %v > max+grace", elapsed)
	}
}

func TestRandomMode_EqualBounds(t *testing.T) {
	// When min == max, the node behaves exactly like fixed delay.
	c, node := startDelay(t, map[string]any{
		"mode":        "random",
		"randomFirst": float64(50),
		"randomLast":  float64(50),
		"randomUnits": "milliseconds",
	})
	defer node.Stop()

	start := time.Now()
	node.HandleMessage(makeMsg("x"))

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if c.count() != 1 {
		t.Fatalf("expected 1 message, got %d", c.count())
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("released too early: %v", elapsed)
	}
}

func TestDelayMode_MsgDelayOverride(t *testing.T) {
	// Configured timeout is huge — msg.delay should override it.
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(10000),
		"timeoutUnits": "milliseconds",
	})
	defer node.Stop()

	start := time.Now()
	node.HandleMessage(makeMsg("override-me", map[string]any{
		"delay": float64(50),
	}))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	if c.count() != 1 {
		t.Fatalf("expected 1 message via override, got %d", c.count())
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("override ignored — took %v", elapsed)
	}
	if c.last().Payload() != "override-me" {
		t.Errorf("payload mismatch: %v", c.last().Payload())
	}
}

func TestDelayMode_RemovesOverrideField(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode": "delay",
	})
	defer node.Stop()

	node.HandleMessage(makeMsg("p", map[string]any{
		"delay": float64(20),
	}))

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if c.count() != 1 {
		t.Fatalf("expected 1 message, got %d", c.count())
	}
	if got := c.last().Get("delay"); got != nil {
		t.Errorf("expected msg.delay to be stripped, got %v", got)
	}
}

func TestMsgFlush(t *testing.T) {
	// Configure a large delay; flush should release immediately.
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(10000),
		"timeoutUnits": "milliseconds",
	})
	defer node.Stop()

	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	// Give goroutines a moment to register.
	time.Sleep(20 * time.Millisecond)
	if c.count() != 0 {
		t.Fatalf("expected 0 messages before flush, got %d", c.count())
	}

	// Flush via control message.
	node.HandleMessage(makeMsg(nil, map[string]any{"flush": true}))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() < 5 {
		time.Sleep(5 * time.Millisecond)
	}
	if c.count() != 5 {
		t.Errorf("expected 5 messages after flush, got %d", c.count())
	}

	// The flush control message itself must NOT be forwarded.
	for _, m := range c.msgs {
		if got := m.Get("flush"); got != nil {
			t.Errorf("flush control message leaked downstream: %v", got)
		}
	}
}

func TestMsgReset(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode":         "delay",
		"timeout":      float64(150),
		"timeoutUnits": "milliseconds",
	})
	defer node.Stop()

	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	time.Sleep(20 * time.Millisecond)

	node.HandleMessage(makeMsg(nil, map[string]any{"reset": true}))

	// Wait past the original timeout.
	time.Sleep(250 * time.Millisecond)

	if c.count() != 0 {
		t.Errorf("expected pending to be discarded by reset, got %d sent", c.count())
	}

	// New messages after reset should still flow normally.
	node.HandleMessage(makeMsg("after-reset"))
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if c.count() != 1 {
		t.Fatalf("expected 1 message after reset, got %d", c.count())
	}
	if c.last().Payload() != "after-reset" {
		t.Errorf("payload mismatch: %v", c.last().Payload())
	}
}

// --- Phase 4: mode "rate" --------------------------------------------------

func TestRateMode_QueueRespectsRate(t *testing.T) {
	// 10 msg / sec → tick every 100ms. 5 messages should take roughly
	// 400-500ms to drain (first goes out at t≈100, last at t≈500).
	c, node := startDelay(t, map[string]any{
		"mode":           "rate",
		"rate":           float64(10),
		"rateUnits":      "second",
		"behaviour":      "queue",
		"maxQueueLength": float64(100),
	})
	defer node.Stop()

	const n = 5
	burstStart := time.Now()
	for i := 0; i < n; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}

	// Wait up to 1.5x expected time for full drain.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() < n {
		time.Sleep(5 * time.Millisecond)
	}

	if c.count() != n {
		t.Fatalf("expected %d messages, got %d", n, c.count())
	}
	elapsed := time.Since(burstStart)
	// 5 messages at 10/s → first ~100ms after burst, last ~500ms.
	if elapsed < 350*time.Millisecond {
		t.Errorf("drained too fast: %v (rate not respected)", elapsed)
	}
	if elapsed > 900*time.Millisecond {
		t.Errorf("drained too slow: %v", elapsed)
	}
	// FIFO check.
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, m := range c.msgs {
		if m.Payload() != float64(i) {
			t.Errorf("message %d: want payload %d, got %v", i, i, m.Payload())
		}
	}
}

func TestRateMode_DropBehaviour(t *testing.T) {
	// behaviour=drop must not buffer at all: a burst of N messages forwards
	// the first one immediately and silently discards the rest. After the
	// cooldown (rateInterval) elapses, the next arrival is forwarded again.
	c, node := startDelay(t, map[string]any{
		"mode":      "rate",
		"rate":      float64(5), // 5/s → 200ms cooldown
		"rateUnits": "second",
		"behaviour": "drop",
	})
	defer node.Stop()

	// Burst of 10 messages well inside the cooldown window.
	for i := 0; i < 10; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	// Tiny grace period for the send goroutine.
	time.Sleep(20 * time.Millisecond)

	if got := c.count(); got != 1 {
		t.Fatalf("drop behaviour, burst: expected exactly 1 forwarded, got %d", got)
	}
	if c.last().Payload() != float64(0) {
		t.Errorf("drop behaviour: expected first burst message (payload 0) to pass, got %v",
			c.last().Payload())
	}

	// Wait past the cooldown and send one more — it should pass.
	time.Sleep(250 * time.Millisecond)
	node.HandleMessage(makeMsg(float64(99)))
	time.Sleep(20 * time.Millisecond)

	if got := c.count(); got != 2 {
		t.Errorf("drop behaviour, post-cooldown: expected 2 total, got %d", got)
	}
	if c.last().Payload() != float64(99) {
		t.Errorf("drop behaviour: expected second send (payload 99), got %v",
			c.last().Payload())
	}
}

func TestRateMode_DropPendingStaysZero(t *testing.T) {
	// Regression: in drop mode the pending counter must never grow, even
	// under sustained over-rate input. (Previously the queue filled up to
	// maxQueueLength before drop kicked in.)
	var lastFill, lastText string
	statusFn := func(fill, text string) {
		lastFill = fill
		lastText = text
	}

	node, err := NewDelayNode(flow.NodeConfig{
		ID:   t.Name(),
		Type: "delay",
		Properties: map[string]any{
			"mode":      "rate",
			"rate":      float64(10),
			"rateUnits": "second",
			"behaviour": "drop",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	node.SetSend(c.send)
	node.SetStatus(statusFn)
	node.SetDebug(noopDebug)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	for i := 0; i < 100; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	time.Sleep(20 * time.Millisecond)

	if lastFill != "" || lastText != "" {
		t.Errorf("drop mode should never report pending, got fill=%q text=%q",
			lastFill, lastText)
	}
}

func TestRateMode_QueueOverflow(t *testing.T) {
	// queue length 3, behaviour=queue → drop oldest. Burst 5 messages before
	// the worker has had a chance to drain, then verify the *first* messages
	// were dropped, not the last ones.
	c, node := startDelay(t, map[string]any{
		"mode":           "rate",
		"rate":           float64(2), // 2/s → 500ms tick (slow enough to fill)
		"rateUnits":      "second",
		"behaviour":      "queue",
		"maxQueueLength": float64(3),
	})
	defer node.Stop()

	// Burst quickly so all 5 land before the first tick fires.
	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}

	// Wait long enough for the worker to drain whatever's queued.
	// 3 items at 500ms each → ~1.5s. Add buffer.
	time.Sleep(2500 * time.Millisecond)

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.msgs) != 3 {
		t.Fatalf("expected 3 messages drained from queue (size=3), got %d", len(c.msgs))
	}
	// The oldest two (0 and 1) were dropped. We expect 2, 3, 4 in order.
	wants := []float64{2, 3, 4}
	for i, m := range c.msgs {
		if m.Payload() != wants[i] {
			t.Errorf("position %d: want payload %v, got %v", i, wants[i], m.Payload())
		}
	}
}

func TestRateMode_FlushDrainsQueue(t *testing.T) {
	// Slow rate (1 / 10s) so the worker does not drain on its own,
	// then flush should release everything immediately in FIFO order.
	c, node := startDelay(t, map[string]any{
		"mode":           "rate",
		"rate":           float64(1),
		"rateUnits":      "minutes", // 1/min ≈ 60s tick
		"behaviour":      "queue",
		"maxQueueLength": float64(10),
	})
	defer node.Stop()

	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	time.Sleep(20 * time.Millisecond)
	if c.count() != 0 {
		t.Fatalf("expected 0 messages before flush, got %d", c.count())
	}

	node.HandleMessage(makeMsg(nil, map[string]any{"flush": true}))

	// Flush is synchronous for rate mode — drain happens in HandleMessage.
	if c.count() != 5 {
		t.Errorf("expected 5 messages after flush, got %d", c.count())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, m := range c.msgs {
		if m.Payload() != float64(i) {
			t.Errorf("position %d: want %d, got %v", i, i, m.Payload())
		}
	}
}

func TestRateMode_ResetClearsQueue(t *testing.T) {
	c, node := startDelay(t, map[string]any{
		"mode":           "rate",
		"rate":           float64(1),
		"rateUnits":      "minutes",
		"behaviour":      "queue",
		"maxQueueLength": float64(10),
	})
	defer node.Stop()

	for i := 0; i < 5; i++ {
		node.HandleMessage(makeMsg(float64(i)))
	}
	time.Sleep(20 * time.Millisecond)

	node.HandleMessage(makeMsg(nil, map[string]any{"reset": true}))

	// Even after waiting, nothing should arrive — queue was cleared.
	time.Sleep(200 * time.Millisecond)
	if c.count() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", c.count())
	}
}

func TestDelayMode_StatusUpdates(t *testing.T) {
	// Capture status calls and verify the pending count is reported.
	var lastFill atomic.Value
	var lastText atomic.Value
	statusFn := func(fill, text string) {
		lastFill.Store(fill)
		lastText.Store(text)
	}

	node, err := NewDelayNode(flow.NodeConfig{
		ID:   t.Name(),
		Type: "delay",
		Properties: map[string]any{
			"mode":         "delay",
			"timeout":      float64(80),
			"timeoutUnits": "milliseconds",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	node.SetSend(c.send)
	node.SetStatus(statusFn)
	node.SetDebug(noopDebug)
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	node.HandleMessage(makeMsg("a"))
	node.HandleMessage(makeMsg("b"))

	// Right after handling, status should report 2 pending.
	if got, _ := lastFill.Load().(string); got != "blue" {
		t.Errorf("expected fill 'blue' while pending, got %q", got)
	}
	if got, _ := lastText.Load().(string); got != "pending: 2" {
		t.Errorf("expected text 'pending: 2', got %q", got)
	}

	// After timer fires, status should clear back to empty.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && c.count() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	// Allow the status callback for the second decrement to complete.
	time.Sleep(20 * time.Millisecond)
	if got, _ := lastText.Load().(string); got != "" {
		t.Errorf("expected empty status after drain, got %q", got)
	}
}
