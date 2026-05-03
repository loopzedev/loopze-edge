// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T, ttl time.Duration) *SessionManager {
	t.Helper()
	key := make([]byte, sessionKeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	m, err := NewSessionManager(NewMemorySessionStore(), key, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSessionManagerCreateAndGet(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := context.Background()

	s, err := m.Create(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if s.UserID != "user-1" || s.ID == "" || s.ExpiresAt.Before(time.Now()) {
		t.Errorf("unexpected session: %+v", s)
	}

	got, err := m.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != s.ID {
		t.Error("Get returned wrong session")
	}
}

func TestSessionManagerExpiry(t *testing.T) {
	m := newTestManager(t, 10*time.Millisecond)
	ctx := context.Background()

	s, err := m.Create(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := m.Get(ctx, s.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expired session: got %v, want ErrSessionNotFound", err)
	}
}

func TestSessionManagerRefresh(t *testing.T) {
	m := newTestManager(t, 50*time.Millisecond)
	ctx := context.Background()

	s, err := m.Create(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	originalExpiry := s.ExpiresAt

	time.Sleep(20 * time.Millisecond)

	refreshed, err := m.Refresh(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.ExpiresAt.After(originalExpiry) {
		t.Errorf("Refresh did not extend expiry: %v <= %v", refreshed.ExpiresAt, originalExpiry)
	}
}

func TestSessionManagerRefreshExpired(t *testing.T) {
	m := newTestManager(t, 10*time.Millisecond)
	ctx := context.Background()

	s, err := m.Create(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := m.Refresh(ctx, s.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Refresh on expired: got %v, want ErrSessionNotFound", err)
	}
}

func TestSessionManagerDelete(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := context.Background()

	s, err := m.Create(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, s.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("after Delete: got %v, want ErrSessionNotFound", err)
	}

	// Deleting a non-existent session is a no-op.
	if err := m.Delete(ctx, "does-not-exist"); err != nil {
		t.Errorf("Delete missing: %v", err)
	}
}

func TestSessionManagerDeleteAllForUser(t *testing.T) {
	m := newTestManager(t, time.Minute)
	ctx := context.Background()

	a1, _ := m.Create(ctx, "user-A")
	a2, _ := m.Create(ctx, "user-A")
	b1, _ := m.Create(ctx, "user-B")

	if err := m.DeleteAllForUser(ctx, "user-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, a1.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Error("a1 should be gone")
	}
	if _, err := m.Get(ctx, a2.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Error("a2 should be gone")
	}
	if _, err := m.Get(ctx, b1.ID); err != nil {
		t.Errorf("b1 should remain: %v", err)
	}
}

func TestSessionCookieSignAndVerify(t *testing.T) {
	m := newTestManager(t, time.Minute)

	cookie := m.SignCookieValue("session-abc")
	id, err := m.VerifyCookieValue(cookie)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id != "session-abc" {
		t.Errorf("got %q, want %q", id, "session-abc")
	}
}

func TestSessionCookieRejectsTampering(t *testing.T) {
	m := newTestManager(t, time.Minute)

	cookie := m.SignCookieValue("session-abc")
	tampered := []string{
		"",
		"session-abc",                     // no signature
		"session-abc.",                    // empty signature
		".sig",                            // empty id
		"session-XYZ" + cookie[len("session-abc"):], // wrong id, original sig
		cookie + "garbage",                // appended bytes
		"session-abc.@@@",                 // non-base64
	}
	for _, c := range tampered {
		if _, err := m.VerifyCookieValue(c); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("Verify(%q): got %v, want ErrSessionInvalid", c, err)
		}
	}
}

func TestSessionCookieDifferentKeyRejects(t *testing.T) {
	m1 := newTestManager(t, time.Minute)

	// Different key = different signature.
	otherKey := make([]byte, sessionKeySize)
	for i := range otherKey {
		otherKey[i] = byte(i + 99)
	}
	m2, err := NewSessionManager(NewMemorySessionStore(), otherKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	cookie := m1.SignCookieValue("session-abc")
	if _, err := m2.VerifyCookieValue(cookie); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("foreign key: got %v, want ErrSessionInvalid", err)
	}
}

func TestNewSessionManagerValidation(t *testing.T) {
	if _, err := NewSessionManager(nil, make([]byte, sessionKeySize), time.Minute); err == nil {
		t.Error("nil store should fail")
	}
	if _, err := NewSessionManager(NewMemorySessionStore(), []byte("short"), time.Minute); err == nil {
		t.Error("short key should fail")
	}
	if _, err := NewSessionManager(NewMemorySessionStore(), make([]byte, sessionKeySize), 0); err == nil {
		t.Error("zero ttl should fail")
	}
}

func TestEnsureSessionKeyCreatesAndReuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "loopze.session.key")

	first, err := EnsureSessionKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != sessionKeySize {
		t.Fatalf("key length = %d, want %d", len(first), sessionKeySize)
	}

	second, err := EnsureSessionKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("EnsureSessionKey did not return the same bytes on reload")
	}
}

func TestEnsureSessionKeyRejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "loopze.session.key")

	if err := os.WriteFile(path, []byte("too short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureSessionKey(path); err == nil {
		t.Error("short existing key should fail")
	}
}
