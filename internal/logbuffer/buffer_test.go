// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package logbuffer

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestNew_MinCapacity(t *testing.T) {
	if got := New(0).Capacity(); got != 1 {
		t.Errorf("New(0) capacity = %d, want 1", got)
	}
	if got := New(-5).Capacity(); got != 1 {
		t.Errorf("New(-5) capacity = %d, want 1", got)
	}
	if got := New(7).Capacity(); got != 7 {
		t.Errorf("New(7) capacity = %d, want 7", got)
	}
}

func TestAdd_AssignsMonotonicSeq(t *testing.T) {
	b := New(10)
	for i := 1; i <= 5; i++ {
		got := b.Add(LogEntry{Message: "x"})
		if got.Seq != uint64(i) {
			t.Errorf("Add #%d returned Seq %d, want %d", i, got.Seq, i)
		}
	}
	last := b.Last(5)
	for i, e := range last {
		if e.Seq != uint64(i+1) {
			t.Errorf("Last[%d].Seq = %d, want %d", i, e.Seq, i+1)
		}
	}
}

func TestLast_PartialFill(t *testing.T) {
	b := New(10)
	b.Add(LogEntry{Message: "a"})
	b.Add(LogEntry{Message: "b"})
	b.Add(LogEntry{Message: "c"})
	got := b.Last(5)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Message != want {
			t.Errorf("Last[%d].Message = %q, want %q", i, got[i].Message, want)
		}
	}
}

func TestLast_Wraparound(t *testing.T) {
	b := New(3)
	for i := 1; i <= 5; i++ {
		b.Add(LogEntry{Message: "x"})
	}
	got := b.Last(3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantSeqs := []uint64{3, 4, 5}
	for i, w := range wantSeqs {
		if got[i].Seq != w {
			t.Errorf("Last[%d].Seq = %d, want %d", i, got[i].Seq, w)
		}
	}
}

func TestLast_LimitedByCap(t *testing.T) {
	b := New(3)
	for i := 1; i <= 5; i++ {
		b.Add(LogEntry{Message: "x"})
	}
	got := b.Last(100)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

func TestLast_NonPositive(t *testing.T) {
	b := New(3)
	b.Add(LogEntry{Message: "a"})
	if got := b.Last(0); len(got) != 0 {
		t.Errorf("Last(0) len = %d, want 0", len(got))
	}
	if got := b.Last(-1); len(got) != 0 {
		t.Errorf("Last(-1) len = %d, want 0", len(got))
	}
}

func TestConcurrent_Add(t *testing.T) {
	const goroutines = 10
	const perGoroutine = 100
	const total = goroutines * perGoroutine
	b := New(total)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				b.Add(LogEntry{Message: "x"})
			}
		}()
	}
	wg.Wait()

	got := b.Last(total)
	if len(got) != total {
		t.Fatalf("len = %d, want %d", len(got), total)
	}
	seqs := make([]uint64, len(got))
	for i, e := range got {
		seqs[i] = e.Seq
	}
	if !sort.SliceIsSorted(seqs, func(i, j int) bool { return seqs[i] < seqs[j] }) {
		t.Fatal("Last() did not return entries in seq order")
	}
	if seqs[0] != 1 || seqs[len(seqs)-1] != total {
		t.Errorf("seqs span = [%d,%d], want [1,%d]", seqs[0], seqs[len(seqs)-1], total)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("seq gap at index %d: %d -> %d", i, seqs[i-1], seqs[i])
		}
	}
}

// --- Handler tests ---

func newTestHandler(buf *Buffer) *Handler {
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	return NewHandler(inner, buf)
}

func TestHandler_AttrsIteration(t *testing.T) {
	buf := New(10)
	h := newTestHandler(buf)
	logger := slog.New(h)

	logger.Info("hi", "k1", "v1", "k2", 42)

	got := buf.Last(1)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Message != "hi" || got[0].Level != "INFO" {
		t.Errorf("entry = %+v, want INFO/hi", got[0])
	}
	if got[0].Attrs["k1"] != "v1" {
		t.Errorf("Attrs[k1] = %v, want v1", got[0].Attrs["k1"])
	}
	if got[0].Attrs["k2"] != int64(42) {
		t.Errorf("Attrs[k2] = %v (%T), want 42", got[0].Attrs["k2"], got[0].Attrs["k2"])
	}
}

func TestHandler_WithAttrs_PassesThrough(t *testing.T) {
	buf := New(10)
	h := newTestHandler(buf)
	logger := slog.New(h).With("flow", "f1")

	logger.Info("hi", "node", "n1")

	got := buf.Last(1)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Attrs["flow"] != "f1" {
		t.Errorf("Attrs[flow] = %v, want f1 (lost from With chain)", got[0].Attrs["flow"])
	}
	if got[0].Attrs["node"] != "n1" {
		t.Errorf("Attrs[node] = %v, want n1", got[0].Attrs["node"])
	}
}

func TestHandler_LevelMapping(t *testing.T) {
	buf := New(10)
	h := newTestHandler(buf)
	logger := slog.New(h)

	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")
	logger.Error("e")

	got := buf.Last(4)
	want := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	for i, w := range want {
		if got[i].Level != w {
			t.Errorf("Last[%d].Level = %q, want %q", i, got[i].Level, w)
		}
	}
}

func TestHandler_NotifyCallback(t *testing.T) {
	buf := New(10)
	h := newTestHandler(buf)

	var received []LogEntry
	var mu sync.Mutex
	h.SetNotify(func(e LogEntry) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	logger := slog.New(h)
	logger.Info("a")
	logger.Info("b")

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("notify called %d times, want 2", len(received))
	}
	if received[0].Seq != 1 || received[1].Seq != 2 {
		t.Errorf("notify seqs = [%d,%d], want [1,2]", received[0].Seq, received[1].Seq)
	}
	if received[0].Message != "a" || received[1].Message != "b" {
		t.Errorf("notify messages = [%q,%q], want [a,b]", received[0].Message, received[1].Message)
	}
}

func TestHandler_NotifyReentrancy(t *testing.T) {
	// If the notify callback itself logs, the wrapping handler must not
	// recurse infinitely. We rely on the caller (e.g. ws.Hub.Broadcast)
	// to be non-blocking; here we simulate that by having notify trigger
	// at most one additional log without recursing further.
	buf := New(10)
	h := newTestHandler(buf)
	logger := slog.New(h)

	var depth int32
	var mu sync.Mutex
	maxDepth := int32(0)
	h.SetNotify(func(e LogEntry) {
		mu.Lock()
		depth++
		if depth > maxDepth {
			maxDepth = depth
		}
		d := depth
		mu.Unlock()
		if d <= 2 {
			logger.Warn("notify-loop", "from_seq", e.Seq)
		}
		mu.Lock()
		depth--
		mu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		logger.Info("trigger")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("notify-reentrancy did not terminate within 500ms")
	}
}

func TestHandler_Enabled(t *testing.T) {
	buf := New(10)
	inner := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewHandler(inner, buf)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(Info) = true, want false (inner level is Warn)")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(Error) = false, want true")
	}
}
