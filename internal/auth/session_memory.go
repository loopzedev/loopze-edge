// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import (
	"context"
	"sync"
)

// MemorySessionStore is an in-memory SessionStore. It is used by tests
// and by the LOOPZE_DISABLE_AUTH dev-bypass mode (where session state
// does not need to survive a restart). Not suitable for production —
// sessions are lost on shutdown and not shared across instances.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

// NewMemorySessionStore returns a ready-to-use in-memory store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{sessions: make(map[string]Session)}
}

// Put inserts or updates a session.
func (s *MemorySessionStore) Put(_ context.Context, sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return nil
}

// Get returns the session with the given ID. Expired sessions are
// removed on access and reported as ErrSessionNotFound.
func (s *MemorySessionStore) Get(_ context.Context, id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.IsExpired() {
		delete(s.sessions, id)
		return nil, ErrSessionNotFound
	}
	return &sess, nil
}

// Delete removes a session if present.
func (s *MemorySessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// DeleteAllForUser removes every session whose UserID matches.
func (s *MemorySessionStore) DeleteAllForUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, id)
		}
	}
	return nil
}
