// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nats

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// KVContextStore implements flow.KVContextStore backed by a NATS JetStream KV bucket.
// Values are serialised as JSON so any type representable in JSON can be stored
// and round-tripped back to JavaScript correctly.
type KVContextStore struct {
	kv jetstream.KeyValue
}

// NewKVContextStore wraps a NATS KV bucket as a flow.KVContextStore.
func NewKVContextStore(kv jetstream.KeyValue) *KVContextStore {
	return &KVContextStore{kv: kv}
}

// Get retrieves the value stored under key. Returns nil (no error) when the
// key does not exist.
func (s *KVContextStore) Get(key string) (any, error) {
	entry, err := s.kv.Get(context.Background(), key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var val any
	if err := json.Unmarshal(entry.Value(), &val); err != nil {
		// Fallback: return raw string if not valid JSON.
		return string(entry.Value()), nil
	}
	return val, nil
}

// Set serialises value as JSON and stores it under key.
func (s *KVContextStore) Set(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(context.Background(), key, data)
	return err
}

// Delete removes key from the store. A missing key is silently ignored.
func (s *KVContextStore) Delete(key string) error {
	err := s.kv.Delete(context.Background(), key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	return err
}

// KeyValue returns the underlying NATS JetStream KeyValue bucket
// for advanced operations like Watch.
func (s *KVContextStore) KeyValue() jetstream.KeyValue {
	return s.kv
}

// Keys returns all active keys in the store.
func (s *KVContextStore) Keys() ([]string, error) {
	keys, err := s.kv.Keys(context.Background())
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	return keys, nil
}
