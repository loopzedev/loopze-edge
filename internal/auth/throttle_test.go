// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import (
	"testing"
	"time"
)

// fakeClock is a minimal mutable clock for throttle tests.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestThrottleAllowsBeforeThreshold(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	for i := 0; i < throttleMaxFailures-1; i++ {
		if !th.Allow("alice") {
			t.Fatalf("Allow returned false at attempt %d", i)
		}
		th.RecordFailure("alice")
	}
	if !th.Allow("alice") {
		t.Error("Allow should still be true at threshold-1 failures")
	}
}

func TestThrottleLocksAtThreshold(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	for i := 0; i < throttleMaxFailures; i++ {
		th.RecordFailure("alice")
	}
	if th.Allow("alice") {
		t.Errorf("Allow should be false after %d failures", throttleMaxFailures)
	}
}

func TestThrottleLockExpires(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	for i := 0; i < throttleMaxFailures; i++ {
		th.RecordFailure("alice")
	}

	// Just before lock expiry: still locked.
	c.now = c.now.Add(throttleWindow - time.Second)
	if th.Allow("alice") {
		t.Error("should still be locked one second before expiry")
	}

	// After expiry: allowed again.
	c.now = c.now.Add(2 * time.Second)
	if !th.Allow("alice") {
		t.Error("should be unlocked after expiry")
	}
}

func TestThrottleWindowReset(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	// Four failures.
	for i := 0; i < throttleMaxFailures-1; i++ {
		th.RecordFailure("alice")
	}

	// Wait past the window without a fifth failure: counter resets.
	c.now = c.now.Add(throttleWindow + time.Second)
	th.RecordFailure("alice") // this is now the start of a fresh window

	// Should still be allowed: only one failure in the new window.
	if !th.Allow("alice") {
		t.Error("Allow should be true after window reset")
	}
}

func TestThrottlePerUsername(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	for i := 0; i < throttleMaxFailures; i++ {
		th.RecordFailure("alice")
	}
	if th.Allow("alice") {
		t.Error("alice should be locked")
	}
	if !th.Allow("bob") {
		t.Error("bob should not be affected by alice's failures")
	}
}

func TestThrottleSuccessResets(t *testing.T) {
	c := &fakeClock{now: time.Unix(0, 0)}
	th := NewLoginThrottle(c.Now)

	for i := 0; i < throttleMaxFailures-1; i++ {
		th.RecordFailure("alice")
	}
	th.RecordSuccess("alice")

	// Counter is cleared; should now allow another full set of failures.
	for i := 0; i < throttleMaxFailures-1; i++ {
		if !th.Allow("alice") {
			t.Fatalf("Allow false after success-reset at attempt %d", i)
		}
		th.RecordFailure("alice")
	}
}
