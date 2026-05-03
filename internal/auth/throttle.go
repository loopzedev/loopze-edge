// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"sync"
	"time"
)

// Throttle defaults follow the issue spec: five failed attempts within
// fifteen minutes lock the username for the same window.
const (
	throttleMaxFailures = 5
	throttleWindow      = 15 * time.Minute
)

// LoginThrottle tracks failed login attempts per username to prevent
// password brute-forcing. State is in-memory only; a server restart
// clears all counters. That is acceptable: persistent brute-force across
// restarts is not a realistic threat against a single-instance edge
// deployment, and avoiding a backend simplifies reasoning about the
// throttle.
type LoginThrottle struct {
	now func() time.Time

	mu   sync.Mutex
	fail map[string]throttleEntry
}

type throttleEntry struct {
	// firstFailureAt is the timestamp of the first failure in the
	// current window. The window resets when this is older than
	// throttleWindow.
	firstFailureAt time.Time
	// failures counts consecutive failures within the active window.
	failures int
	// lockedUntil, if non-zero, is the time at which the lock expires.
	lockedUntil time.Time
}

// NewLoginThrottle returns a ready-to-use throttle. The optional `now`
// function is for tests; callers normally pass nil.
func NewLoginThrottle(now func() time.Time) *LoginThrottle {
	if now == nil {
		now = time.Now
	}
	return &LoginThrottle{
		now:  now,
		fail: make(map[string]throttleEntry),
	}
}

// Allow reports whether a login attempt for the given username should be
// processed right now. If false, the username is currently locked out.
func (t *LoginThrottle) Allow(username string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	e, ok := t.fail[username]
	if !ok {
		return true
	}
	now := t.now()
	if !e.lockedUntil.IsZero() && now.Before(e.lockedUntil) {
		return false
	}
	if !e.lockedUntil.IsZero() && !now.Before(e.lockedUntil) {
		// Lock expired: clear the entry so the next failure starts a
		// fresh window.
		delete(t.fail, username)
	}
	return true
}

// RecordFailure increments the per-username failure counter and locks
// the account for throttleWindow if the threshold is reached.
func (t *LoginThrottle) RecordFailure(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	e, ok := t.fail[username]
	if !ok || now.Sub(e.firstFailureAt) > throttleWindow {
		// Start a fresh window.
		e = throttleEntry{firstFailureAt: now}
	}
	e.failures++
	if e.failures >= throttleMaxFailures {
		e.lockedUntil = now.Add(throttleWindow)
	}
	t.fail[username] = e
}

// RecordSuccess clears the failure counter for the given username.
func (t *LoginThrottle) RecordSuccess(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fail, username)
}
