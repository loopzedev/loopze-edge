// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestSessionHandleMarshalJSON(t *testing.T) {
	h := &SessionHandle{id: "abcdef0123"}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s != "<tcp session>" {
		t.Errorf("got %q, want %q", s, "<tcp session>")
	}
}

func TestSessionHandleStringNil(t *testing.T) {
	var h *SessionHandle
	if got := h.ID(); got != "" {
		t.Errorf("nil handle ID = %q, want empty", got)
	}
	if got := (&SessionHandle{}).String(); got != "<tcp session>" {
		t.Errorf("String() = %q", got)
	}
}

func TestSessionRegistryRegisterAndResolve(t *testing.T) {
	rg := NewSessionRegistry()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	handle, slot := rg.Register(a, "tcp-in-1")
	if handle.ID() == "" {
		t.Fatal("empty handle ID")
	}
	if slot.Owner() != "tcp-in-1" {
		t.Errorf("Owner = %q", slot.Owner())
	}
	if slot.ID() != handle.ID() {
		t.Errorf("slot.ID = %q, handle.ID = %q", slot.ID(), handle.ID())
	}

	got, err := rg.Resolve(handle.ID())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != slot {
		t.Errorf("Resolve returned different slot")
	}
	if rg.Len() != 1 || rg.LenByOwner("tcp-in-1") != 1 {
		t.Errorf("Len=%d LenByOwner=%d", rg.Len(), rg.LenByOwner("tcp-in-1"))
	}
}

func TestSessionRegistryResolveUnknown(t *testing.T) {
	rg := NewSessionRegistry()
	_, err := rg.Resolve("nonexistent")
	if !errors.Is(err, ErrUnknownSession) {
		t.Errorf("got %v, want ErrUnknownSession", err)
	}
}

func TestSessionRegistryCloseRemovesSlot(t *testing.T) {
	rg := NewSessionRegistry()
	a, b := net.Pipe()
	defer b.Close()

	h, slot := rg.Register(a, "tcp-in-1")
	rg.Close(h.ID())

	if !slot.Closed.Load() {
		t.Error("Closed flag not set")
	}
	select {
	case <-slot.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed")
	}
	if _, err := rg.Resolve(h.ID()); !errors.Is(err, ErrUnknownSession) {
		t.Errorf("Resolve after Close: got %v, want ErrUnknownSession", err)
	}
	if rg.Len() != 0 {
		t.Errorf("Len = %d after Close", rg.Len())
	}
	// Underlying conn should be closed; reading from the peer should
	// observe EOF or a closed-pipe error.
	one := make([]byte, 1)
	_ = a.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, err := b.Read(one); err == nil {
		t.Error("expected error reading from closed peer pipe")
	}
}

func TestSessionRegistryCloseIdempotent(t *testing.T) {
	rg := NewSessionRegistry()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	h, _ := rg.Register(a, "owner-1")
	rg.Close(h.ID())
	rg.Close(h.ID()) // should not panic / double-close
}

func TestSessionRegistryCloseByOwner(t *testing.T) {
	rg := NewSessionRegistry()
	pipes := make([][2]net.Conn, 3)
	for i := range pipes {
		pipes[i][0], pipes[i][1] = net.Pipe()
		defer pipes[i][1].Close()
	}

	rg.Register(pipes[0][0], "owner-A")
	rg.Register(pipes[1][0], "owner-A")
	rg.Register(pipes[2][0], "owner-B")
	if rg.Len() != 3 {
		t.Fatalf("Len before = %d", rg.Len())
	}

	rg.CloseByOwner("owner-A")

	if rg.Len() != 1 {
		t.Errorf("Len after CloseByOwner = %d, want 1", rg.Len())
	}
	if rg.LenByOwner("owner-A") != 0 {
		t.Errorf("owner-A still has %d", rg.LenByOwner("owner-A"))
	}
	if rg.LenByOwner("owner-B") != 1 {
		t.Errorf("owner-B has %d, want 1", rg.LenByOwner("owner-B"))
	}
}

func TestSessionRegistryCloseAll(t *testing.T) {
	rg := NewSessionRegistry()
	pipes := make([][2]net.Conn, 4)
	for i := range pipes {
		pipes[i][0], pipes[i][1] = net.Pipe()
		defer pipes[i][1].Close()
	}
	rg.Register(pipes[0][0], "owner-A")
	rg.Register(pipes[1][0], "owner-A")
	rg.Register(pipes[2][0], "owner-B")
	rg.Register(pipes[3][0], "owner-C")

	rg.CloseAll()

	if rg.Len() != 0 {
		t.Errorf("Len after CloseAll = %d", rg.Len())
	}
	if rg.LenByOwner("owner-A") != 0 || rg.LenByOwner("owner-B") != 0 {
		t.Error("byOwner map not cleared")
	}
}

func TestSessionRegistrySessionsByOwner(t *testing.T) {
	rg := NewSessionRegistry()
	a1, b1 := net.Pipe()
	a2, b2 := net.Pipe()
	a3, b3 := net.Pipe()
	defer b1.Close()
	defer b2.Close()
	defer b3.Close()

	_, s1 := rg.Register(a1, "broadcast-source")
	_, s2 := rg.Register(a2, "broadcast-source")
	_, _ = rg.Register(a3, "other-source")

	got := rg.SessionsByOwner("broadcast-source")
	if len(got) != 2 {
		t.Fatalf("SessionsByOwner = %d slots, want 2", len(got))
	}
	seen := map[*SessionSlot]bool{}
	for _, s := range got {
		seen[s] = true
	}
	if !seen[s1] || !seen[s2] {
		t.Errorf("expected both s1 and s2; got %v", got)
	}

	if got := rg.SessionsByOwner("nonexistent"); got != nil {
		t.Errorf("SessionsByOwner(nonexistent) = %v, want nil", got)
	}
}

func TestSessionRegistryResolveAfterClose(t *testing.T) {
	rg := NewSessionRegistry()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	h, _ := rg.Register(a, "owner")
	rg.Close(h.ID())

	if _, err := rg.Resolve(h.ID()); !errors.Is(err, ErrUnknownSession) {
		t.Errorf("got %v, want ErrUnknownSession", err)
	}
}

func TestSessionRegistryConcurrentRegisterAndClose(t *testing.T) {
	rg := NewSessionRegistry()
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			a, b := net.Pipe()
			defer b.Close()
			h, _ := rg.Register(a, "owner")
			rg.Close(h.ID())
		}()
	}
	wg.Wait()
	if rg.Len() != 0 {
		t.Errorf("after N register/close: Len = %d", rg.Len())
	}
}
