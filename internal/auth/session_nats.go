// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

// NATSSessionStore persists sessions in a JetStream KV bucket. The bucket
// itself enforces TTL based on its MaxAge setting (every Put resets the
// per-entry timer). We additionally re-check ExpiresAt on Get so a clock
// adjustment can never resurrect an expired session.
type NATSSessionStore struct {
	kv jetstream.KeyValue
}

// NewNATSSessionStore wraps an existing JetStream KV bucket. Use
// Broker.SetupSessionKV to create the bucket with the right configuration.
func NewNATSSessionStore(kv jetstream.KeyValue) *NATSSessionStore {
	return &NATSSessionStore{kv: kv}
}

// Put serialises the session as JSON and writes it under its ID. The
// bucket's MaxAge applies (TTL re-starts on every Put).
func (s *NATSSessionStore) Put(ctx context.Context, sess Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("auth: marshal session: %w", err)
	}
	if _, err := s.kv.Put(ctx, sess.ID, data); err != nil {
		return fmt.Errorf("auth: put session: %w", err)
	}
	return nil
}

// Get fetches and parses a session. Expired sessions are deleted and
// reported as ErrSessionNotFound.
func (s *NATSSessionStore) Get(ctx context.Context, id string) (*Session, error) {
	entry, err := s.kv.Get(ctx, id)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("auth: get session: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(entry.Value(), &sess); err != nil {
		return nil, fmt.Errorf("auth: parse session: %w", err)
	}
	if sess.IsExpired() {
		// Best-effort cleanup; a failed delete just means the bucket TTL
		// will catch it eventually.
		_ = s.kv.Delete(ctx, id)
		return nil, ErrSessionNotFound
	}
	return &sess, nil
}

// Delete removes a session. Missing keys are not treated as an error.
func (s *NATSSessionStore) Delete(ctx context.Context, id string) error {
	if err := s.kv.Delete(ctx, id); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// DeleteAllForUser iterates over every key in the bucket and removes the
// sessions whose UserID matches. Acceptable cost: we never expect more
// than a handful of concurrent sessions per user, and the operation runs
// only on logout-everywhere / disable / password-reset paths.
func (s *NATSSessionStore) DeleteAllForUser(ctx context.Context, userID string) error {
	lister, err := s.kv.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("auth: list session keys: %w", err)
	}
	defer lister.Stop()

	for key := range lister.Keys() {
		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			slog.Warn("auth: read session during user-wide delete", "key", key, "error", err)
			continue
		}
		var sess Session
		if err := json.Unmarshal(entry.Value(), &sess); err != nil {
			slog.Warn("auth: skipping unparseable session during user-wide delete", "key", key, "error", err)
			continue
		}
		if sess.UserID != userID {
			continue
		}
		if err := s.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.Warn("auth: delete session during user-wide delete", "key", key, "error", err)
		}
	}
	return nil
}
