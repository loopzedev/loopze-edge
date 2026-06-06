// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"sync"
	"time"
)

// Sample is one point in a stat widget's sparkline history.
type Sample struct {
	V float64 `json:"v"`
	T int64   `json:"t"` // unix milliseconds
}

// StatSampleStore holds a per-widget ring buffer of sparkline samples.
// Separate from Cache (which holds single last-values) because the
// access pattern is fundamentally different: append-with-trim vs.
// overwrite-last. Future ui-chart widgets will get their own store
// with potentially different semantics — keeping these orthogonal
// avoids merging them prematurely.
type StatSampleStore struct {
	mu      sync.RWMutex
	buffers map[string][]Sample
}

func NewStatSampleStore() *StatSampleStore {
	return &StatSampleStore{buffers: make(map[string][]Sample)}
}

// Append adds a sample to the widget's ring buffer and trims from the
// front if the buffer exceeds windowSize. windowSize is read on every
// append so a redeploy with a smaller window takes effect immediately.
// Returns the appended sample so callers can broadcast it without
// re-locking.
func (s *StatSampleStore) Append(widgetID string, value float64, ts time.Time, windowSize int) Sample {
	if windowSize < 1 {
		windowSize = 1
	}
	sample := Sample{V: value, T: ts.UnixMilli()}

	s.mu.Lock()
	defer s.mu.Unlock()

	buf := s.buffers[widgetID]
	buf = append(buf, sample)
	if len(buf) > windowSize {
		// Trim the oldest entries. A new backing array is allocated
		// instead of using slice tricks so the trimmed-off elements
		// can be garbage-collected (no reference leak).
		drop := len(buf) - windowSize
		trimmed := make([]Sample, windowSize)
		copy(trimmed, buf[drop:])
		buf = trimmed
	}
	s.buffers[widgetID] = buf
	return sample
}

// Get returns a copy of the widget's current ring buffer (never shares
// the internal slice with callers).
func (s *StatSampleStore) Get(widgetID string) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	buf := s.buffers[widgetID]
	if len(buf) == 0 {
		return nil
	}
	out := make([]Sample, len(buf))
	copy(out, buf)
	return out
}

// Snapshot returns a copy of every ring buffer. Used by the snapshot
// handshake to replay sparkline history to fresh clients.
func (s *StatSampleStore) Snapshot() map[string][]Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]Sample, len(s.buffers))
	for id, buf := range s.buffers {
		if len(buf) == 0 {
			continue
		}
		cp := make([]Sample, len(buf))
		copy(cp, buf)
		out[id] = cp
	}
	return out
}

// Drop removes a widget's ring buffer entirely.
func (s *StatSampleStore) Drop(widgetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buffers, widgetID)
}

// Retain drops ring buffers for widgets not present in the keep set.
// Called after a deploy when nodes are removed.
func (s *StatSampleStore) Retain(keep map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.buffers {
		if _, ok := keep[id]; !ok {
			delete(s.buffers, id)
		}
	}
}
