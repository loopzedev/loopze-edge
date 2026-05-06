// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"
)

// ErrUnknownHandle is returned by ResponseRegistry.Complete when no
// slot is registered for the given handle ID. The slot may never have
// existed, may have been timed out by the sweeper, or may have been
// completed by an earlier caller that won the race.
var ErrUnknownHandle = errors.New("flow: unknown response handle")

// ErrAlreadyCompleted is returned by ResponseRegistry.Complete when the
// slot existed but a previous caller (typically the sweeper firing the
// timeout fallback, or another http-response branch on a fan-out) had
// already written the response.
var ErrAlreadyCompleted = errors.New("flow: response already sent")

// ResponseWriteFunc is the body of a one-shot HTTP response. The
// http-response node (and the registry's own timeout fallback) build
// such a closure and hand it to ResponseSlot.Complete so the at-most-
// once write is guarded by sync.Once.
type ResponseWriteFunc func(w http.ResponseWriter, r *http.Request)

// ResponseSlot owns the live http.ResponseWriter / *http.Request pair
// for a request that an http-in node has handed off to the flow. It is
// resolved by handle ID through the ResponseRegistry. Only one Complete
// call ever writes to the underlying writer; subsequent calls are
// no-ops and report `false`.
type ResponseSlot struct {
	w        http.ResponseWriter
	r        *http.Request
	done     chan struct{}
	once     sync.Once
	deadline time.Time // zero means no timeout

	completed bool         // guarded by mu
	mu        sync.Mutex   // guards `completed`

	nodeID string // source http-in node ID — for diagnostics
	flowID string // source flow ID
}

// NodeID returns the http-in node ID that registered this slot.
func (s *ResponseSlot) NodeID() string { return s.nodeID }

// FlowID returns the flow ID the source http-in node belongs to.
func (s *ResponseSlot) FlowID() string { return s.flowID }

// Request exposes the underlying *http.Request so callers can read
// headers, query params, etc. The body has typically already been
// consumed by the http-in node; do not assume it is replayable.
func (s *ResponseSlot) Request() *http.Request { return s.r }

// Done returns a channel that closes when the slot has been completed
// (whether by an http-response, the timeout sweeper, or DrainAll).
func (s *ResponseSlot) Done() <-chan struct{} { return s.done }

// IsCompleted reports whether the slot has already been written to.
// Safe to call from any goroutine.
func (s *ResponseSlot) IsCompleted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

// Complete writes the response under a sync.Once and signals done.
// Returns true if THIS call performed the write, false if another
// caller had already completed the slot.
func (s *ResponseSlot) Complete(write ResponseWriteFunc) bool {
	fired := false
	s.once.Do(func() {
		fired = true
		if write != nil {
			write(s.w, s.r)
		}
		s.mu.Lock()
		s.completed = true
		s.mu.Unlock()
		close(s.done)
	})
	return fired
}

// ResponseRegistry tracks live request handles across the entire engine
// so that a message — including one routed via Link nodes across flows
// — can carry an opaque response handle and have a paired http-response
// node resolve and answer the request.
//
// Lifecycle:
//   - http-in handler calls Register with the live (w, r) pair, gets a
//     handle and a done channel, attaches the handle to msg.res, fires
//     the message into the flow, and blocks on <-done.
//   - http-response calls Complete(handleID, fn). The fn writes status
//     and body via the slot's writer. close(done) unblocks the http-in
//     handler.
//   - If no Complete call arrives before the configured deadline, the
//     sweeper goroutine fires the configured TimeoutFallback (default
//     504) and unblocks the handler.
//   - On engine Stop, DrainAll closes every open slot with the
//     configured ShutdownFallback (default 503).
type ResponseRegistry struct {
	mu    sync.Mutex
	slots map[string]*ResponseSlot

	// TimeoutFallback is the response written by the sweeper when a
	// slot's deadline elapses without a Complete call. Defaults to
	// defaultTimeoutFallback (504 + JSON error body).
	TimeoutFallback ResponseWriteFunc

	// ShutdownFallback is the response written to every still-live slot
	// when DrainAll is called (engine Stop / re-deploy boundary).
	// Defaults to defaultShutdownFallback (503 + JSON error body).
	ShutdownFallback ResponseWriteFunc

	// SweepInterval controls how often the timeout sweeper runs.
	// Defaults to 1s when zero.
	SweepInterval time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	now    func() time.Time // injectable for tests
}

// NewResponseRegistry returns a registry with default timeout / shutdown
// responses. Run(ctx) starts the sweeper goroutine; without it, slots
// never time out (only DrainAll cleans up).
func NewResponseRegistry() *ResponseRegistry {
	return &ResponseRegistry{
		slots:            make(map[string]*ResponseSlot),
		TimeoutFallback:  defaultTimeoutFallback,
		ShutdownFallback: defaultShutdownFallback,
		SweepInterval:    time.Second,
		now:              time.Now,
	}
}

// Register installs a slot keyed by a fresh opaque handle ID. timeout
// of zero means no deadline (the slot lives until Complete or DrainAll).
// The returned channel closes when the slot is completed.
func (rg *ResponseRegistry) Register(w http.ResponseWriter, r *http.Request, timeout time.Duration, nodeID, flowID string) (*ResponseHandle, <-chan struct{}) {
	id := newHandleID()
	slot := &ResponseSlot{
		w:      w,
		r:      r,
		done:   make(chan struct{}),
		nodeID: nodeID,
		flowID: flowID,
	}
	if timeout > 0 {
		slot.deadline = rg.now().Add(timeout)
	}

	rg.mu.Lock()
	rg.slots[id] = slot
	rg.mu.Unlock()

	return &ResponseHandle{id: id}, slot.done
}

// Resolve returns the slot stored under the given handle ID, if any.
// A missing entry can mean either "never registered" or "already
// completed and cleaned up" — callers should treat both the same way.
func (rg *ResponseRegistry) Resolve(id string) (*ResponseSlot, bool) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	slot, ok := rg.slots[id]
	return slot, ok
}

// Complete looks up the slot and invokes its Complete with the given
// writer. Returns:
//   - (true, nil)        if this call performed the write
//   - (false, ErrUnknownHandle)   if no slot is registered for the ID
//   - (false, ErrAlreadyCompleted) if the slot existed but was already
//     completed by an earlier caller (or the sweeper)
//
// Slots are deleted from the map after a successful completion.
func (rg *ResponseRegistry) Complete(id string, write ResponseWriteFunc) (bool, error) {
	rg.mu.Lock()
	slot, ok := rg.slots[id]
	rg.mu.Unlock()

	if !ok {
		return false, ErrUnknownHandle
	}
	if !slot.Complete(write) {
		return false, ErrAlreadyCompleted
	}
	rg.mu.Lock()
	delete(rg.slots, id)
	rg.mu.Unlock()
	return true, nil
}

// DrainAll completes every still-live slot with ShutdownFallback. Used
// on engine Stop to cancel all in-flight HTTP handlers cleanly.
func (rg *ResponseRegistry) DrainAll() {
	write := rg.ShutdownFallback
	if write == nil {
		write = defaultShutdownFallback
	}

	rg.mu.Lock()
	victims := make([]*ResponseSlot, 0, len(rg.slots))
	for _, s := range rg.slots {
		victims = append(victims, s)
	}
	rg.slots = make(map[string]*ResponseSlot)
	rg.mu.Unlock()

	for _, s := range victims {
		s.Complete(write)
	}
}

// Run starts the sweeper goroutine; call Stop() to halt it. Safe to
// call multiple times — only the first invocation starts a goroutine.
func (rg *ResponseRegistry) Run() {
	if rg.stopCh != nil {
		return
	}
	rg.stopCh = make(chan struct{})
	rg.wg.Add(1)
	go func() {
		defer rg.wg.Done()
		interval := rg.SweepInterval
		if interval <= 0 {
			interval = time.Second
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-rg.stopCh:
				return
			case <-t.C:
				rg.sweepExpired()
			}
		}
	}()
}

// Stop signals the sweeper goroutine to exit and waits for it.
func (rg *ResponseRegistry) Stop() {
	if rg.stopCh == nil {
		return
	}
	select {
	case <-rg.stopCh:
		// already closed
	default:
		close(rg.stopCh)
	}
	rg.wg.Wait()
	rg.stopCh = nil
}

// sweepExpired completes every slot whose deadline has elapsed.
func (rg *ResponseRegistry) sweepExpired() {
	now := rg.now()
	write := rg.TimeoutFallback
	if write == nil {
		write = defaultTimeoutFallback
	}

	rg.mu.Lock()
	expired := make([]struct {
		id   string
		slot *ResponseSlot
	}, 0)
	for id, s := range rg.slots {
		if s.deadline.IsZero() {
			continue
		}
		if !now.After(s.deadline) {
			continue
		}
		expired = append(expired, struct {
			id   string
			slot *ResponseSlot
		}{id, s})
	}
	for _, e := range expired {
		delete(rg.slots, e.id)
	}
	rg.mu.Unlock()

	for _, e := range expired {
		e.slot.Complete(write)
	}
}

// Len returns the current number of live slots — useful for tests and
// metrics.
func (rg *ResponseRegistry) Len() int {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	return len(rg.slots)
}

// SetNowForTest overrides the registry's clock source. Test-only escape
// hatch so the sweeper can be exercised without real-time waits.
func (rg *ResponseRegistry) SetNowForTest(fn func() time.Time) {
	rg.now = fn
}

// newHandleID returns 16 random hex characters. Collision probability
// is negligible for the per-engine slot map (a handle's lifetime is at
// most a few seconds).
func newHandleID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// defaultTimeoutFallback writes a 504 with a JSON error body. Replaced
// by callers that want a different error contract.
func defaultTimeoutFallback(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGatewayTimeout)
	_, _ = w.Write([]byte(`{"error":"flow did not respond"}`))
}

// defaultShutdownFallback writes a 503 with a JSON error body when the
// engine is stopping or redeploying.
func defaultShutdownFallback(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":"service stopping"}`))
}
