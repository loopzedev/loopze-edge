// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

// ErrUnknownSession is returned by SessionRegistry.Resolve / Write when
// no slot is registered for the given handle ID. The slot may never
// have existed, may have been closed by the owning tcp-in reader
// goroutine on EOF, or may have been torn down by CloseByOwner /
// CloseAll on engine stop.
var ErrUnknownSession = errors.New("flow: unknown tcp session")

// ErrSessionClosed is returned by SessionRegistry.Resolve when the slot
// existed but its underlying connection has already been closed.
// Callers should treat this the same as ErrUnknownSession (a catchable
// error on tcp-out reply); the distinction exists for diagnostics.
var ErrSessionClosed = errors.New("flow: tcp session closed")

// SessionSlot owns a live net.Conn that a tcp-in node has accepted (or
// dialled, in client mode) and handed off to the flow via msg.session.
// It is resolved by handle ID through the SessionRegistry. Mu serialises
// concurrent writes from multiple tcp-out reply branches that may have
// fanned out from the same upstream message; readers (tcp-in's per-conn
// goroutine) own the read direction and do not contend on Mu.
type SessionSlot struct {
	// Conn is the underlying net.Conn. Writes must hold Mu.
	Conn net.Conn

	// Mu serialises concurrent writers.
	Mu sync.Mutex

	// Closed reports whether the connection has already been closed.
	// Set by SessionRegistry.Close before the actual net.Conn.Close
	// call so callers can short-circuit a write that is racing with
	// teardown.
	Closed atomic.Bool

	owner string        // tcp-in node id that registered this slot
	id    string        // registry key (also embedded in *SessionHandle)
	done  chan struct{} // closed when the slot has been removed from the registry
}

// Owner returns the tcp-in node ID that produced this session.
func (s *SessionSlot) Owner() string { return s.owner }

// ID returns the opaque handle ID for this slot.
func (s *SessionSlot) ID() string { return s.id }

// Done returns a channel that closes when the slot has been removed
// from the registry (via Close, CloseByOwner, or CloseAll). Useful for
// tcp-in's per-conn reader goroutine to wait on teardown without
// holding the registry lock.
func (s *SessionSlot) Done() <-chan struct{} { return s.done }

// SessionRegistry tracks live TCP sessions across the entire engine so
// that a flow message — including one routed via Link nodes across
// flows — can carry an opaque session handle and have a paired tcp-out
// reply (or server-broadcast) node resolve and write to the matching
// connection.
//
// Lifecycle:
//   - tcp-in (server mode) accepts a conn and calls Register(conn,
//     ownerID) → receives a *SessionHandle and a done channel; attaches
//     the handle to msg.session, fires per-frame messages into the
//     flow, then on EOF / read error calls Close(id).
//   - tcp-out (reply mode) calls Resolve(handleID); on success takes
//     slot.Mu, writes the payload, releases.
//   - tcp-out (server-broadcast mode) calls SessionsByOwner(targetID)
//     and writes to every returned slot in turn.
//   - On tcp-in.Stop() the node calls CloseByOwner(itself) so every
//     accepted conn is torn down.
//   - On engine.Stop() the engine calls CloseAll() so every still-live
//     conn (across all owners) is torn down.
//
// All public methods are safe for concurrent use.
type SessionRegistry struct {
	mu      sync.RWMutex
	slots   map[string]*SessionSlot          // id → slot
	byOwner map[string]map[string]*SessionSlot // ownerID → id → slot
}

// NewSessionRegistry returns an empty registry ready for use.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		slots:   make(map[string]*SessionSlot),
		byOwner: make(map[string]map[string]*SessionSlot),
	}
}

// Register installs a new slot keyed by a fresh opaque handle ID and
// returns the public handle plus a done channel that closes when the
// slot is removed from the registry. ownerID identifies the tcp-in
// node that produced this session — used by SessionsByOwner for
// server-broadcast and by CloseByOwner on tcp-in.Stop.
func (rg *SessionRegistry) Register(conn net.Conn, ownerID string) (*SessionHandle, *SessionSlot) {
	id := newHandleID()
	slot := &SessionSlot{
		Conn:  conn,
		owner: ownerID,
		id:    id,
		done:  make(chan struct{}),
	}

	rg.mu.Lock()
	rg.slots[id] = slot
	owners := rg.byOwner[ownerID]
	if owners == nil {
		owners = make(map[string]*SessionSlot)
		rg.byOwner[ownerID] = owners
	}
	owners[id] = slot
	rg.mu.Unlock()

	return &SessionHandle{id: id}, slot
}

// Resolve returns the slot for the given handle ID. The second return
// value is one of:
//   - nil if the slot is live (caller should proceed to take slot.Mu)
//   - ErrUnknownSession if no slot is registered (never existed, or
//     already closed and removed)
//   - ErrSessionClosed if the slot is registered but its Closed flag
//     is set (mid-teardown — treat the same as unknown)
func (rg *SessionRegistry) Resolve(id string) (*SessionSlot, error) {
	rg.mu.RLock()
	slot, ok := rg.slots[id]
	rg.mu.RUnlock()
	if !ok {
		return nil, ErrUnknownSession
	}
	if slot.Closed.Load() {
		return nil, ErrSessionClosed
	}
	return slot, nil
}

// SessionsByOwner returns a snapshot of every slot registered under
// ownerID. Used by tcp-out (server-broadcast mode) to fan out a
// payload to every connection a given tcp-in has accepted. The
// returned slice is a fresh allocation; callers may iterate without
// holding the registry lock, but each per-slot write must still take
// slot.Mu and check slot.Closed.
func (rg *SessionRegistry) SessionsByOwner(ownerID string) []*SessionSlot {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	owners := rg.byOwner[ownerID]
	if len(owners) == 0 {
		return nil
	}
	out := make([]*SessionSlot, 0, len(owners))
	for _, s := range owners {
		out = append(out, s)
	}
	return out
}

// Close removes the slot from the registry, marks it closed, closes
// its done channel, and closes the underlying net.Conn. Safe to call
// more than once — subsequent calls are no-ops.
func (rg *SessionRegistry) Close(id string) {
	rg.mu.Lock()
	slot, ok := rg.slots[id]
	if !ok {
		rg.mu.Unlock()
		return
	}
	delete(rg.slots, id)
	if owners := rg.byOwner[slot.owner]; owners != nil {
		delete(owners, id)
		if len(owners) == 0 {
			delete(rg.byOwner, slot.owner)
		}
	}
	rg.mu.Unlock()

	rg.teardown(slot)
}

// CloseByOwner tears down every slot registered under ownerID. Called
// by tcp-in.Stop() so every accepted conn is closed when the node
// goes away (deploy diff, engine stop).
func (rg *SessionRegistry) CloseByOwner(ownerID string) {
	rg.mu.Lock()
	owners := rg.byOwner[ownerID]
	if len(owners) == 0 {
		rg.mu.Unlock()
		return
	}
	victims := make([]*SessionSlot, 0, len(owners))
	for id, slot := range owners {
		victims = append(victims, slot)
		delete(rg.slots, id)
	}
	delete(rg.byOwner, ownerID)
	rg.mu.Unlock()

	for _, s := range victims {
		rg.teardown(s)
	}
}

// CloseAll tears down every live slot. Called by engine.Stop() to
// ensure no goroutine is left blocked on a dangling Read when the
// engine shuts down.
func (rg *SessionRegistry) CloseAll() {
	rg.mu.Lock()
	victims := make([]*SessionSlot, 0, len(rg.slots))
	for _, s := range rg.slots {
		victims = append(victims, s)
	}
	rg.slots = make(map[string]*SessionSlot)
	rg.byOwner = make(map[string]map[string]*SessionSlot)
	rg.mu.Unlock()

	for _, s := range victims {
		rg.teardown(s)
	}
}

// teardown is the per-slot half of Close / CloseByOwner / CloseAll:
// flag the slot closed, signal Done, and close the net.Conn. Idempotent
// via the atomic.CompareAndSwap on Closed.
func (rg *SessionRegistry) teardown(s *SessionSlot) {
	if !s.Closed.CompareAndSwap(false, true) {
		return
	}
	close(s.done)
	if s.Conn != nil {
		_ = s.Conn.Close()
	}
}

// Len returns the total number of live sessions across all owners.
// Useful for tests and metrics.
func (rg *SessionRegistry) Len() int {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return len(rg.slots)
}

// LenByOwner returns the number of live sessions belonging to a given
// owner. Used by tcp-in's status text ("listening · :7000 · 3 conn").
func (rg *SessionRegistry) LenByOwner(ownerID string) int {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return len(rg.byOwner[ownerID])
}
