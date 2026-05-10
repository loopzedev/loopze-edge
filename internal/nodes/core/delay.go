// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Per-message override field names. They are stripped from the message before
// further processing so they do not leak into downstream nodes.
const (
	delayFieldDelay = "delay"
	delayFieldReset = "reset"
	delayFieldFlush = "flush"
)

const (
	delayModeFixed  = "delay"
	delayModeRate   = "rate"
	delayModeRandom = "random"

	rateBehaviourQueue = "queue"
	rateBehaviourDrop  = "drop"

	defaultMaxQueueLength = 1000
)

// DelayNode buffers, delays or rate-limits incoming messages.
//
// Three modes:
//   - "delay":  every message is held for a fixed duration before being forwarded
//   - "rate":   at most rate / rateInterval messages pass through; behaviour
//               decides what happens to overflow (queue or drop)
//   - "random": every message is held for a random duration in [min, max]
//
// Per-message overrides on the incoming msg:
//   - msg.delay  (number, ms): override the delay for this message
//   - msg.reset  (truthy):     discard all currently pending messages
//   - msg.flush  (truthy):     send all currently pending messages immediately
//
// On Stop() all pending messages are discarded.
type DelayNode struct {
	config flow.NodeConfig
	nodes.BaseNode
	mode           string
	timeout        time.Duration
	randomMin      time.Duration
	randomMax      time.Duration
	rateInterval   time.Duration
	behaviour      string
	maxQueueLength int

	// queue is the rate-limit buffer (only used in mode "rate").
	queue chan *flow.Message

	// flushSignal broadcasts a flush event to all pending delay/random goroutines.
	// Replaced (under mu) on each flush so a fresh channel is available for the
	// next batch of pending messages.
	//
	// resetSignal works the same way for the reset event; receivers discard their
	// payload instead of sending it.
	//
	// Both signals are broadcast via close() — every blocked goroutine wakes up
	// at once. We then swap in a fresh channel so the next wave of pending
	// messages is unaffected by the prior broadcast.
	mu          sync.Mutex
	flushSignal chan struct{}
	resetSignal chan struct{}

	// lastSent tracks the timestamp of the most recently forwarded message in
	// rate/drop mode. Used to gate new sends without any buffering. Guarded by mu.
	lastSent time.Time

	// pending is the count of in-flight messages (timers running or queue items)
	// for status reporting.
	pending atomic.Int64

	done chan struct{}
	wg   sync.WaitGroup
}

// NewDelayNode is the NodeFactory for the delay node type.
func NewDelayNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &DelayNode{
		config:      config,
		done:        make(chan struct{}),
		flushSignal: make(chan struct{}),
		resetSignal: make(chan struct{}),
	}, nil
}

// Init parses and validates configuration from Properties.
func (n *DelayNode) Init() error {
	p := n.config.Properties

	n.mode = nodes.StringVal(p, "mode", delayModeFixed)

	timeout, _ := p["timeout"].(float64)
	n.timeout = parseDelayUnits(timeout, nodes.StringVal(p, "timeoutUnits", "milliseconds"))

	first, _ := p["randomFirst"].(float64)
	last, _ := p["randomLast"].(float64)
	randomUnits := nodes.StringVal(p, "randomUnits", "milliseconds")
	n.randomMin = parseDelayUnits(first, randomUnits)
	n.randomMax = parseDelayUnits(last, randomUnits)
	if n.randomMax < n.randomMin {
		n.randomMin, n.randomMax = n.randomMax, n.randomMin
	}

	rate, _ := p["rate"].(float64)
	rateUnits := nodes.StringVal(p, "rateUnits", "second")
	if rate > 0 {
		unit := parseDelayUnits(1, rateUnits)
		if unit > 0 {
			n.rateInterval = time.Duration(float64(unit) / rate)
		}
	}

	n.behaviour = nodes.StringVal(p, "behaviour", rateBehaviourQueue)
	if n.behaviour != rateBehaviourDrop {
		n.behaviour = rateBehaviourQueue
	}

	n.maxQueueLength = defaultMaxQueueLength
	if v, ok := p["maxQueueLength"].(float64); ok && v > 0 {
		n.maxQueueLength = int(v)
	}

	// Only the queue behaviour buffers messages. The drop behaviour is
	// stateless (single timestamp gate), so no queue and no worker are needed.
	if n.mode == delayModeRate && n.behaviour == rateBehaviourQueue {
		n.queue = make(chan *flow.Message, n.maxQueueLength)
	}

	return nil
}

// Start launches the rate worker goroutine when running in rate mode.
// Modes "delay" and "random" spawn per-message goroutines on demand.
func (n *DelayNode) Start() error {
	if n.Send == nil {
		return fmt.Errorf("delay node %s: send function not set", n.config.ID)
	}

	// Only the queue behaviour needs a background dispatcher.
	if n.mode == delayModeRate && n.behaviour == rateBehaviourQueue && n.rateInterval > 0 {
		n.wg.Add(1)
		go n.rateWorker()
	}

	slog.Info("delay node started",
		"node_id", n.config.ID,
		"mode", n.mode,
		"timeout", n.timeout,
		"rate_interval", n.rateInterval,
	)
	return nil
}

// HandleMessage routes the incoming message based on the configured mode.
//
// Three per-message overrides are honoured before the mode-specific logic
// runs. If any of them is set, the message itself is consumed (not forwarded):
//   - msg.reset  → drop everything currently pending
//   - msg.flush  → release everything currently pending immediately
//   - msg.delay  → schedule this message after the given number of milliseconds
//                  (overrides the configured timeout / random range; the field
//                  is stripped before the message is forwarded)
func (n *DelayNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	if v, ok := msg.Get(delayFieldReset).(bool); ok && v {
		n.resetPending()
		return nil, nil
	}
	if v, ok := msg.Get(delayFieldFlush).(bool); ok && v {
		n.flushPending()
		return nil, nil
	}
	if d, ok := readNumber(msg.Get(delayFieldDelay)); ok {
		msg.Delete(delayFieldDelay)
		n.scheduleAfter(time.Duration(d)*time.Millisecond, msg)
		return nil, nil
	}

	switch n.mode {
	case delayModeFixed:
		n.scheduleAfter(n.timeout, msg)
	case delayModeRandom:
		n.scheduleAfter(n.pickRandomDelay(), msg)
	case delayModeRate:
		n.handleRate(msg)
	default:
		// Unknown mode: pass through unchanged so the node never silently drops.
		n.Send(0, msg)
	}
	return nil, nil
}

// resetPending discards every currently pending message. For delay/random it
// closes the shared resetSignal channel — every blocked scheduler goroutine
// wakes up and exits without sending. For rate mode it drains the queue.
// A fresh resetSignal is installed for the next wave of messages.
func (n *DelayNode) resetPending() {
	n.mu.Lock()
	old := n.resetSignal
	n.resetSignal = make(chan struct{})
	n.mu.Unlock()
	close(old)

	if n.queue != nil {
		for {
			select {
			case <-n.queue:
				n.pending.Add(-1)
			default:
				n.updateStatus()
				return
			}
		}
	}
}

// flushPending releases every currently pending message immediately. For
// delay/random the broadcast wakes per-message goroutines, which then send.
// For rate mode the queue is drained synchronously here, in FIFO order.
func (n *DelayNode) flushPending() {
	n.mu.Lock()
	old := n.flushSignal
	n.flushSignal = make(chan struct{})
	n.mu.Unlock()
	close(old)

	if n.queue != nil {
		for {
			select {
			case msg := <-n.queue:
				n.Send(0, msg)
				n.pending.Add(-1)
			default:
				n.updateStatus()
				return
			}
		}
	}
}

// readNumber extracts a numeric value from an interface, accepting the common
// JSON number type (float64) plus a few integer types for resilience against
// callers that bypass JSON decoding.
func readNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// Stop signals all pending goroutines to terminate and waits for them to exit.
// Pending messages are discarded — see issue NODE_DELAY.md for rationale.
func (n *DelayNode) Stop() error {
	close(n.done)
	n.wg.Wait()
	slog.Info("delay node stopped", "node_id", n.config.ID)
	return nil
}

// scheduleAfter spawns a goroutine that holds msg for d and then sends it.
// Zero or negative durations short-circuit to an immediate send so callers do
// not have to special-case them.
func (n *DelayNode) scheduleAfter(d time.Duration, msg *flow.Message) {
	if d <= 0 {
		n.Send(0, msg)
		return
	}

	n.mu.Lock()
	flushCh := n.flushSignal
	resetCh := n.resetSignal
	n.mu.Unlock()

	n.pending.Add(1)
	n.updateStatus()

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		defer func() {
			n.pending.Add(-1)
			n.updateStatus()
		}()

		timer := time.NewTimer(d)
		defer timer.Stop()

		select {
		case <-timer.C:
			n.Send(0, msg)
		case <-flushCh:
			n.Send(0, msg)
		case <-resetCh:
			return // reset broadcast — discard
		case <-n.done:
			return // node is stopping — discard
		}
	}()
}

// updateStatus reflects the current pending count to the editor UI.
//
// The read of pending and the status callback must happen under a lock so
// concurrent goroutines (e.g. two timers firing near-simultaneously) cannot
// interleave such that an earlier read writes its status after a later one,
// leaving stale "pending: N" text after the queue has drained.
func (n *DelayNode) updateStatus() {
	if n.Status == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	p := n.pending.Load()
	if p == 0 {
		n.Status("", "")
		return
	}
	n.Status("blue", fmt.Sprintf("pending: %d", p))
}

// handleRate routes a rate-mode message through either the timestamp gate
// (drop) or the FIFO buffer (queue).
func (n *DelayNode) handleRate(msg *flow.Message) {
	if n.behaviour == rateBehaviourDrop {
		n.handleRateDrop(msg)
		return
	}
	n.handleRateQueue(msg)
}

// handleRateDrop forwards msg only if rateInterval has elapsed since the last
// forwarded message. There is no buffering — anything arriving inside the
// cooldown window is silently discarded.
func (n *DelayNode) handleRateDrop(msg *flow.Message) {
	if n.rateInterval <= 0 {
		// No effective rate limit configured — pass through.
		n.Send(0, msg)
		return
	}

	now := time.Now()
	n.mu.Lock()
	if !n.lastSent.IsZero() && now.Sub(n.lastSent) < n.rateInterval {
		n.mu.Unlock()
		return // silently drop — high drop rates would flood the log
	}
	n.lastSent = now
	n.mu.Unlock()
	n.Send(0, msg)
}

// handleRateQueue places msg into the rate-mode buffer. When the queue is
// full the oldest entry is evicted to make room.
func (n *DelayNode) handleRateQueue(msg *flow.Message) {
	if n.queue == nil {
		return
	}

	select {
	case n.queue <- msg:
		n.pending.Add(1)
		n.updateStatus()
		return
	default:
	}

	// Queue full: silently discard the oldest entry, then push the new one.
	// Drops are intentionally not logged — high overflow rates would flood
	// the log; the dropped-count is observable via the pending counter and
	// the difference between input and output messages.
	select {
	case <-n.queue:
		n.pending.Add(-1)
	default:
	}
	select {
	case n.queue <- msg:
		n.pending.Add(1)
	default:
		// Should not happen — we just made room.
	}
	n.updateStatus()
}

// rateWorker dispatches at most one queued message per rateInterval tick.
// Empty ticks are skipped silently so the goroutine produces zero load when
// idle.
func (n *DelayNode) rateWorker() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.rateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.done:
			return
		case <-ticker.C:
			select {
			case msg := <-n.queue:
				n.Send(0, msg)
				n.pending.Add(-1)
				n.updateStatus()
			default:
				// queue empty — skip this tick
			}
		}
	}
}

// parseDelayUnits converts a numeric value plus a unit name to a time.Duration.
// Accepts both plural and singular forms ("seconds"/"second") for robustness.
// Unknown units fall back to milliseconds.
func parseDelayUnits(value float64, unit string) time.Duration {
	if value <= 0 {
		return 0
	}
	var unitDur time.Duration
	switch unit {
	case "milliseconds", "millisecond", "msecs", "msec", "ms":
		unitDur = time.Millisecond
	case "seconds", "second", "secs", "sec", "s":
		unitDur = time.Second
	case "minutes", "minute", "mins", "min":
		unitDur = time.Minute
	case "hours", "hour", "hrs", "hr", "h":
		unitDur = time.Hour
	case "day", "days", "d":
		unitDur = 24 * time.Hour
	default:
		unitDur = time.Millisecond
	}
	return time.Duration(value * float64(unitDur))
}

// pickRandomDelay picks a duration uniformly at random from [randomMin, randomMax].
// Returns randomMin when min equals max.
func (n *DelayNode) pickRandomDelay() time.Duration {
	if n.randomMax <= n.randomMin {
		return n.randomMin
	}
	span := n.randomMax - n.randomMin
	return n.randomMin + time.Duration(rand.Int64N(int64(span)+1))
}

// DelayTypeInfo returns the NodeTypeInfo for registering the delay node.
func DelayTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "delay",
		Category:    "function",
		Label:       "Delay",
		Description: "Delays messages, rate-limits a stream, or applies a random delay",
		Icon:        "mdi-timer",
		Defaults: map[string]any{
			"mode":           "delay",
			"timeout":        500.0,
			"timeoutUnits":   "milliseconds",
			"rate":           1.0,
			"rateUnits":      "second",
			"behaviour":      "queue",
			"maxQueueLength": 1000.0,
			"randomFirst":    0.0,
			"randomLast":     1000.0,
			"randomUnits":    "milliseconds",
		},
		Inputs:  1,
		Outputs: 1,
	}
}
