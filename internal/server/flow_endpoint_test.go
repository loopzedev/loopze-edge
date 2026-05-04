// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestFlowEndpointMuxEmptyIs404(t *testing.T) {
	m := NewFlowEndpointMux()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("empty mux: got %d, want 404", rec.Code)
	}
}

func TestFlowEndpointMuxSwapServesNewRoutes(t *testing.T) {
	m := NewFlowEndpointMux()

	conflicts := m.Swap([]RouteSpec{{
		NodeID:  "n1",
		Method:  "GET",
		Path:    "/webhook",
		Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) },
	}})
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/webhook", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("after swap: got %d %q, want 200 \"ok\"", rec.Code, rec.Body.String())
	}
}

func TestFlowEndpointMuxSwapReplacesPreviousRoutes(t *testing.T) {
	m := NewFlowEndpointMux()
	m.Swap([]RouteSpec{{NodeID: "n1", Method: "GET", Path: "/old", Handler: okHandler("old")}})

	// First deploy: /old answers, /new does not.
	if rec := serve(m, "GET", "/old"); rec.Code != http.StatusOK {
		t.Fatalf("first deploy /old: got %d, want 200", rec.Code)
	}
	if rec := serve(m, "GET", "/new"); rec.Code != http.StatusNotFound {
		t.Fatalf("first deploy /new: got %d, want 404", rec.Code)
	}

	// Redeploy with a different route set: /old must disappear.
	m.Swap([]RouteSpec{{NodeID: "n2", Method: "GET", Path: "/new", Handler: okHandler("new")}})
	if rec := serve(m, "GET", "/old"); rec.Code != http.StatusNotFound {
		t.Fatalf("after redeploy /old: got %d, want 404", rec.Code)
	}
	if rec := serve(m, "GET", "/new"); rec.Code != http.StatusOK || rec.Body.String() != "new" {
		t.Fatalf("after redeploy /new: got %d %q, want 200 \"new\"", rec.Code, rec.Body.String())
	}
}

func TestFlowEndpointMuxRouteConflict(t *testing.T) {
	m := NewFlowEndpointMux()
	conflicts := m.Swap([]RouteSpec{
		{NodeID: "a", Method: "POST", Path: "/hooks", Handler: okHandler("a")},
		{NodeID: "b", Method: "POST", Path: "/hooks", Handler: okHandler("b")},
		{NodeID: "c", Method: "GET", Path: "/other", Handler: okHandler("c")},
	})

	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts (both sides reported), got %d: %+v", len(conflicts), conflicts)
	}
	gotIDs := map[string]bool{}
	for _, c := range conflicts {
		gotIDs[c.Spec.NodeID] = true
		if !strings.Contains(c.Reason, "duplicate") {
			t.Errorf("conflict reason missing 'duplicate': %q", c.Reason)
		}
	}
	if !gotIDs["a"] || !gotIDs["b"] {
		t.Errorf("expected both a and b reported, got %v", gotIDs)
	}

	// /hooks must NOT serve (both sides skipped); /other still serves.
	if rec := serve(m, "POST", "/hooks"); rec.Code != http.StatusNotFound {
		t.Errorf("conflicting /hooks should not serve: got %d", rec.Code)
	}
	if rec := serve(m, "GET", "/other"); rec.Code != http.StatusOK {
		t.Errorf("non-conflicting /other should serve: got %d", rec.Code)
	}
}

func TestFlowEndpointMuxMethodIsolation(t *testing.T) {
	// Same path, different methods → not a conflict.
	m := NewFlowEndpointMux()
	conflicts := m.Swap([]RouteSpec{
		{NodeID: "g", Method: "GET", Path: "/x", Handler: okHandler("get")},
		{NodeID: "p", Method: "POST", Path: "/x", Handler: okHandler("post")},
	})
	if len(conflicts) != 0 {
		t.Fatalf("different methods on same path must not conflict, got: %+v", conflicts)
	}
	if rec := serve(m, "GET", "/x"); rec.Body.String() != "get" {
		t.Errorf("GET /x: got %q", rec.Body.String())
	}
	if rec := serve(m, "POST", "/x"); rec.Body.String() != "post" {
		t.Errorf("POST /x: got %q", rec.Body.String())
	}
}

func TestFlowEndpointMuxAnyVerbWildcard(t *testing.T) {
	m := NewFlowEndpointMux()
	m.Swap([]RouteSpec{{NodeID: "a", Method: "*", Path: "/any", Handler: okHandler("any")}})
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		if rec := serve(m, method, "/any"); rec.Code != http.StatusOK || rec.Body.String() != "any" {
			t.Errorf("%s /any: got %d %q", method, rec.Code, rec.Body.String())
		}
	}
}

// In-flight requests resolve through the router pointer they captured on
// entry. After a swap, that handler completes against the old router
// while new requests hit the new one. We model this with a handler that
// blocks until released, then assert that the old handler still served
// while a fresh request gets the new behaviour.
func TestFlowEndpointMuxSwapDoesNotAbortInflight(t *testing.T) {
	m := NewFlowEndpointMux()

	release := make(chan struct{})
	started := make(chan struct{})
	m.Swap([]RouteSpec{{
		NodeID: "old",
		Method: "GET",
		Path:   "/slow",
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-release
			_, _ = w.Write([]byte("old"))
		},
	}})

	var wg sync.WaitGroup
	wg.Add(1)
	rec := httptest.NewRecorder()
	go func() {
		defer wg.Done()
		m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()

	<-started // in-flight handler is past the atomic.Load.

	// Swap to a different route set. /slow disappears; /fast appears.
	m.Swap([]RouteSpec{{
		NodeID: "new",
		Method: "GET",
		Path:   "/fast",
		Handler: okHandler("new"),
	}})

	// New requests see the new router immediately.
	if r := serve(m, "GET", "/fast"); r.Body.String() != "new" {
		t.Errorf("new request after swap: got %q, want \"new\"", r.Body.String())
	}
	if r := serve(m, "GET", "/slow"); r.Code != http.StatusNotFound {
		t.Errorf("/slow after swap should be 404, got %d", r.Code)
	}

	// Release the in-flight handler — it completes against the captured
	// (old) router and writes "old".
	close(release)
	wg.Wait()
	if rec.Body.String() != "old" {
		t.Errorf("in-flight handler body: got %q, want \"old\"", rec.Body.String())
	}
}

func okHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func serve(m *FlowEndpointMux, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}
