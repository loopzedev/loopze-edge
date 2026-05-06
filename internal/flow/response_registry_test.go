// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResponseHandleMarshalJSON(t *testing.T) {
	// json.Marshal HTML-escapes "<" and ">" in strings by default
	// (< / >), so we round-trip through Unmarshal and assert
	// the semantic value, not the byte form.
	h := &ResponseHandle{id: "abc123"}
	out, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var s string
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, out)
	}
	if s != "<response handle>" {
		t.Fatalf("got %q, want \"<response handle>\"", s)
	}
}

func TestResponseHandleInsideMessage(t *testing.T) {
	// Carrying a *ResponseHandle on a Message must not break MarshalJSON.
	msg := NewMessage()
	msg.Set("res", &ResponseHandle{id: "h1"})
	msg.Set("payload", "hello")

	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["res"] != "<response handle>" {
		t.Errorf("res in message JSON: got %v, want \"<response handle>\"", got["res"])
	}
	if got["payload"] != "hello" {
		t.Errorf("payload preserved: got %v", got["payload"])
	}
}

func TestResponseHandleSurvivesCOWClone(t *testing.T) {
	// COWClone must carry the handle by reference (identity preserved)
	// — that's what makes Link-Out → Link-In carry the handle across
	// flows correctly.
	orig := NewMessage()
	h := &ResponseHandle{id: "carried"}
	orig.Set("res", h)

	clone := orig.COWClone()
	gotPtr, ok := clone.Get("res").(*ResponseHandle)
	if !ok {
		t.Fatalf("clone.res type: got %T, want *ResponseHandle", clone.Get("res"))
	}
	if gotPtr != h {
		t.Errorf("handle identity not preserved across COW clone")
	}
}

func TestResponseRegistryRegisterAndComplete(t *testing.T) {
	rg := NewResponseRegistry()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)

	handle, done := rg.Register(rec, req, 0, "node-1", "flow-1")
	if handle.ID() == "" {
		t.Fatal("empty handle ID")
	}
	if rg.Len() != 1 {
		t.Fatalf("register: registry len = %d, want 1", rg.Len())
	}

	completed, err := rg.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("done"))
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !completed {
		t.Fatal("expected completed=true")
	}
	select {
	case <-done:
	default:
		t.Fatal("done channel not closed after Complete")
	}
	if rec.Code != http.StatusCreated || rec.Body.String() != "done" {
		t.Errorf("response: got %d %q", rec.Code, rec.Body.String())
	}
	if rg.Len() != 0 {
		t.Errorf("post-complete len = %d, want 0", rg.Len())
	}
}

func TestResponseRegistryUnknownHandle(t *testing.T) {
	rg := NewResponseRegistry()
	completed, err := rg.Complete("not-a-real-id", func(w http.ResponseWriter, _ *http.Request) {})
	if completed {
		t.Error("completed should be false")
	}
	if !errors.Is(err, ErrUnknownHandle) {
		t.Errorf("err = %v, want ErrUnknownHandle", err)
	}
}

func TestResponseRegistryDoubleComplete(t *testing.T) {
	rg := NewResponseRegistry()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	handle, _ := rg.Register(rec, req, 0, "n", "f")

	first, err := rg.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("first"))
	})
	if !first || err != nil {
		t.Fatalf("first complete: completed=%v err=%v", first, err)
	}

	// Second call: registry already removed the slot, so unknown-handle.
	second, err := rg.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("second"))
	})
	if second {
		t.Error("second complete should not write")
	}
	if !errors.Is(err, ErrUnknownHandle) {
		t.Errorf("second complete err = %v, want ErrUnknownHandle", err)
	}
	if rec.Body.String() != "first" {
		t.Errorf("body should still be \"first\", got %q", rec.Body.String())
	}
}

func TestResponseRegistrySlotConcurrentCompleteIsAtMostOnce(t *testing.T) {
	// Direct test of slot.Complete under concurrent invocation: only
	// one writer fires.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rg := NewResponseRegistry()
	_, _ = rg.Register(rec, req, 0, "n", "f")

	// Pull the slot out for direct concurrent calls.
	var slot *ResponseSlot
	for _, s := range rg.slots {
		slot = s
	}

	const n = 20
	var fired atomic.Int32
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			if slot.Complete(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(200)
			}) {
				fired.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := fired.Load(); got != 1 {
		t.Errorf("fired = %d, want 1 (sync.Once should serialise)", got)
	}
	if !slot.IsCompleted() {
		t.Error("IsCompleted false after concurrent completes")
	}
}

func TestResponseRegistrySweeperExpiresSlot(t *testing.T) {
	rg := NewResponseRegistry()
	rg.SweepInterval = 5 * time.Millisecond

	// Inject a controllable clock so we can advance time without sleeping
	// past the deadline ourselves.
	var nowVal atomic.Int64
	nowVal.Store(time.Now().UnixNano())
	rg.SetNowForTest(func() time.Time { return time.Unix(0, nowVal.Load()) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	_, done := rg.Register(rec, req, 50*time.Millisecond, "n", "f")

	rg.Run()
	defer rg.Stop()

	// Advance the clock past the deadline and wait for the sweeper to
	// fire.
	nowVal.Store(time.Now().Add(time.Second).UnixNano())
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweeper did not expire slot within 500ms")
	}

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("expected 504 fallback, got %d", rec.Code)
	}
	if rg.Len() != 0 {
		t.Errorf("expired slot not removed: len=%d", rg.Len())
	}
}

func TestResponseRegistryDrainAll(t *testing.T) {
	rg := NewResponseRegistry()

	rec1, rec2 := httptest.NewRecorder(), httptest.NewRecorder()
	_, d1 := rg.Register(rec1, httptest.NewRequest(http.MethodGet, "/a", nil), 0, "n1", "f")
	_, d2 := rg.Register(rec2, httptest.NewRequest(http.MethodGet, "/b", nil), 0, "n2", "f")

	rg.DrainAll()

	for i, d := range []<-chan struct{}{d1, d2} {
		select {
		case <-d:
		case <-time.After(50 * time.Millisecond):
			t.Errorf("done[%d] not closed after DrainAll", i)
		}
	}
	if rec1.Code != http.StatusServiceUnavailable || rec2.Code != http.StatusServiceUnavailable {
		t.Errorf("DrainAll: codes %d / %d, want 503/503", rec1.Code, rec2.Code)
	}
	if rg.Len() != 0 {
		t.Errorf("DrainAll: len=%d, want 0", rg.Len())
	}
}

func TestResponseRegistryZeroTimeoutNoExpire(t *testing.T) {
	rg := NewResponseRegistry()
	rg.SweepInterval = 5 * time.Millisecond

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	handle, done := rg.Register(rec, req, 0, "n", "f")

	rg.Run()
	defer rg.Stop()

	// Give the sweeper plenty of ticks; the slot must survive.
	time.Sleep(40 * time.Millisecond)

	if rg.Len() != 1 {
		t.Errorf("zero-timeout slot was reaped: len=%d", rg.Len())
	}
	select {
	case <-done:
		t.Error("done closed without an explicit Complete")
	default:
	}

	// Manual complete still works.
	if _, err := rg.Complete(handle.ID(), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}); err != nil {
		t.Errorf("manual complete failed: %v", err)
	}
}

func TestResponseRegistryStopIsIdempotent(t *testing.T) {
	rg := NewResponseRegistry()
	rg.SweepInterval = 5 * time.Millisecond
	rg.Run()
	rg.Stop()
	rg.Stop() // must not panic / hang
}
