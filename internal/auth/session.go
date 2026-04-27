// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Session-related sentinel errors returned across all SessionStore
// implementations.
var (
	// ErrSessionNotFound is returned when a session lookup fails (no
	// matching ID, or the session has expired).
	ErrSessionNotFound = errors.New("auth: session not found")

	// ErrSessionInvalid is returned when a cookie value cannot be parsed
	// or its signature does not verify. Callers should treat this as
	// "user is not authenticated", not as a server error.
	ErrSessionInvalid = errors.New("auth: session cookie is invalid")
)

// Session represents an authenticated browser session. It is the minimum
// state we need to authorise subsequent requests after a login.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// IsExpired reports whether the session's ExpiresAt is in the past.
// Comparison uses the wall clock — sessions are not very time-sensitive
// and a small skew is acceptable.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// SessionStore is the persistence interface for Session records. The
// interface is small on purpose so an in-memory implementation can be
// used for tests and a JetStream-KV implementation for production.
type SessionStore interface {
	// Put writes the session with TTL = ExpiresAt - now. Implementations
	// must enforce the TTL: a session past its ExpiresAt must not be
	// returned by Get, even if it physically still lives in the backend.
	Put(ctx context.Context, s Session) error

	// Get returns the session with the given ID or ErrSessionNotFound.
	// Implementations must treat expired sessions as not found and
	// should clean them up on access.
	Get(ctx context.Context, id string) (*Session, error)

	// Delete removes the session with the given ID. It is a no-op if
	// the session does not exist.
	Delete(ctx context.Context, id string) error

	// DeleteAllForUser removes every session whose UserID equals the
	// argument. Used after password reset or account disable to log
	// the user out everywhere.
	DeleteAllForUser(ctx context.Context, userID string) error
}

// SessionManager wires Session lifecycle to a SessionStore and adds
// HMAC-SHA256 cookie signing on top. The manager is the single entry
// point used by the HTTP layer.
type SessionManager struct {
	store   SessionStore
	signKey []byte
	ttl     time.Duration

	// onDeleted is invoked after a session is removed (single Delete or
	// DeleteAllForUser). Used by the server to notify the WebSocket hub
	// to terminate all connections of the affected user. nil = no-op.
	onDeleted func(userID string)
}

// SetOnDeleted installs a callback invoked after every successful
// session removal. The userID passed is that of the user whose session
// (or all of whose sessions) were just deleted. The callback runs
// synchronously on the calling goroutine.
func (m *SessionManager) SetOnDeleted(fn func(userID string)) {
	m.onDeleted = fn
}

// NewSessionManager creates a manager backed by the given store. signKey
// must be at least sessionKeySize bytes (use EnsureSessionKey to obtain
// one). ttl is the lifetime of newly created sessions; Refresh extends
// existing sessions by the same amount (sliding window).
func NewSessionManager(store SessionStore, signKey []byte, ttl time.Duration) (*SessionManager, error) {
	if store == nil {
		return nil, errors.New("auth: nil session store")
	}
	if len(signKey) < sessionKeySize {
		return nil, fmt.Errorf("auth: session sign key too short: %d bytes (need %d)", len(signKey), sessionKeySize)
	}
	if ttl <= 0 {
		return nil, errors.New("auth: session ttl must be positive")
	}
	return &SessionManager{store: store, signKey: signKey, ttl: ttl}, nil
}

// TTL returns the session lifetime configured on this manager.
func (m *SessionManager) TTL() time.Duration { return m.ttl }

// Create generates a new session for the given user, persists it, and
// returns it.
func (m *SessionManager) Create(ctx context.Context, userID string) (*Session, error) {
	id, err := NewID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	s := Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	if err := m.store.Put(ctx, s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get returns the session with the given ID, or ErrSessionNotFound.
func (m *SessionManager) Get(ctx context.Context, id string) (*Session, error) {
	return m.store.Get(ctx, id)
}

// Refresh extends an existing session's expiry by another full TTL
// (sliding window). Returns ErrSessionNotFound if the session does not
// exist or has already expired.
func (m *SessionManager) Refresh(ctx context.Context, id string) (*Session, error) {
	s, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.ExpiresAt = time.Now().UTC().Add(m.ttl)
	if err := m.store.Put(ctx, *s); err != nil {
		return nil, err
	}
	return s, nil
}

// Delete removes a session. If the session existed, the OnDeleted
// callback (if any) is invoked with the affected user's ID so live
// connections can be torn down.
func (m *SessionManager) Delete(ctx context.Context, id string) error {
	// Look up the user before deletion so we can notify even if the
	// store implementation does not return the deleted record.
	var userID string
	if s, err := m.store.Get(ctx, id); err == nil {
		userID = s.UserID
	}
	if err := m.store.Delete(ctx, id); err != nil {
		return err
	}
	if userID != "" && m.onDeleted != nil {
		m.onDeleted(userID)
	}
	return nil
}

// DeleteAllForUser removes every session for the given user and invokes
// the OnDeleted callback unconditionally — even when no sessions were
// found, it is harmless to ask the hub to disconnect a user with no
// open connections.
func (m *SessionManager) DeleteAllForUser(ctx context.Context, userID string) error {
	if err := m.store.DeleteAllForUser(ctx, userID); err != nil {
		return err
	}
	if m.onDeleted != nil {
		m.onDeleted(userID)
	}
	return nil
}

// SignCookieValue returns the HMAC-signed cookie payload for the given
// session ID. The format is `<id>.<base64url(hmac)>`.
func (m *SessionManager) SignCookieValue(sessionID string) string {
	mac := m.hmac(sessionID)
	return sessionID + "." + base64.RawURLEncoding.EncodeToString(mac)
}

// VerifyCookieValue parses a cookie payload generated by SignCookieValue
// and returns the embedded session ID. ErrSessionInvalid indicates a
// malformed or tampered cookie; callers should not distinguish between
// "no cookie" and "bad cookie" in their HTTP responses.
func (m *SessionManager) VerifyCookieValue(value string) (string, error) {
	dot := strings.IndexByte(value, '.')
	if dot <= 0 || dot == len(value)-1 {
		return "", ErrSessionInvalid
	}
	id := value[:dot]
	gotMAC, err := base64.RawURLEncoding.DecodeString(value[dot+1:])
	if err != nil {
		return "", ErrSessionInvalid
	}
	wantMAC := m.hmac(id)
	if subtle.ConstantTimeCompare(gotMAC, wantMAC) != 1 {
		return "", ErrSessionInvalid
	}
	return id, nil
}

func (m *SessionManager) hmac(id string) []byte {
	h := hmac.New(sha256.New, m.signKey)
	h.Write([]byte(id))
	return h.Sum(nil)
}
