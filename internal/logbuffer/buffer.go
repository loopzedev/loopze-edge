// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package logbuffer provides an in-memory ring buffer for application log
// entries plus a slog.Handler wrapper that captures every record into the
// buffer and optionally fans it out via a notify callback.
//
// The buffer is intended to back the Terminal Log panel in the editor: a
// REST endpoint serves the most recent N entries, while a notify callback
// pushes new entries to all connected WebSocket clients.
package logbuffer

import (
	"sync"
	"time"
)

// LogEntry is a single application log record captured by the slog handler.
//
// Seq is a monotonically increasing identifier assigned by the buffer when
// the entry is added. It is used by the frontend to deduplicate entries
// that arrive both via the REST snapshot and the live WebSocket stream.
type LogEntry struct {
	Seq     uint64         `json:"seq"`
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// Buffer is a thread-safe fixed-capacity ring buffer of LogEntry values.
type Buffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	head    int
	full    bool
	cap     int
	seq     uint64
}

// New creates a Buffer with the given capacity. A non-positive capacity is
// clamped to 1 so the buffer is always usable.
func New(capacity int) *Buffer {
	if capacity < 1 {
		capacity = 1
	}
	return &Buffer{
		entries: make([]LogEntry, capacity),
		cap:     capacity,
	}
}

// Add stamps the next sequence number on e, stores it in the ring, and
// returns the stamped entry so the caller can forward the same value (with
// the assigned Seq) to a notify callback.
func (b *Buffer) Add(e LogEntry) LogEntry {
	b.mu.Lock()
	b.seq++
	e.Seq = b.seq
	b.entries[b.head] = e
	b.head = (b.head + 1) % b.cap
	if b.head == 0 {
		b.full = true
	}
	b.mu.Unlock()
	return e
}

// Last returns the most recent min(n, len) entries in oldest-first order.
// A non-positive n returns an empty slice.
func (b *Buffer) Last(n int) []LogEntry {
	if n <= 0 {
		return []LogEntry{}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	size := b.head
	if b.full {
		size = b.cap
	}
	if n > size {
		n = size
	}
	out := make([]LogEntry, n)
	start := (b.head - n + b.cap) % b.cap
	for i := 0; i < n; i++ {
		out[i] = b.entries[(start+i)%b.cap]
	}
	return out
}

// Capacity returns the configured ring capacity.
func (b *Buffer) Capacity() int {
	return b.cap
}
