// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/auth"
)

// chi.Mount on the configured prefix must strip it before delegating to
// FlowEndpointMux's inner router so spec paths register as "/webhook"
// not "/endpoint/webhook".
func TestFlowEndpointMountStripsPrefix(t *testing.T) {
	mux := NewFlowEndpointMux()
	mux.Swap([]RouteSpec{{
		NodeID:  "n1",
		Method:  "GET",
		Path:    "/webhook",
		Handler: okHandler("seen"),
	}})

	r := chi.NewRouter()
	r.Mount("/endpoint", mux)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/endpoint/webhook", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "seen" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "seen")
	}
}

func TestFlowEndpointMountDoesNotShareCSRF(t *testing.T) {
	// Build a chi router that mirrors the production layout: the flow
	// endpoint mux is mounted BEFORE the /api/v1 group; only /api/v1
	// gets the CSRF middleware. A POST to /endpoint/* must succeed
	// without the X-CSRF-Token header.
	mux := NewFlowEndpointMux()
	mux.Swap([]RouteSpec{{
		NodeID:  "n1",
		Method:  "POST",
		Path:    "/hook",
		Handler: okHandler("ok"),
	}})

	r := chi.NewRouter()
	r.Mount("/endpoint", mux)

	csrfHit := false
	r.Route("/api/v1", func(sub chi.Router) {
		sub.Use(auth.CSRF())
		sub.Post("/anything", func(w http.ResponseWriter, _ *http.Request) {
			csrfHit = true
			w.WriteHeader(http.StatusOK)
		})
	})

	// POST /endpoint/hook with no CSRF header → must succeed.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/endpoint/hook", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flow endpoint POST without CSRF: got %d, want 200", rec.Code)
	}

	// POST /api/v1/anything with no CSRF header → must be rejected by CSRF.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/anything", nil)
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("api POST without CSRF should have been rejected, got 200")
	}
	if csrfHit {
		t.Errorf("CSRF-protected handler ran without a token")
	}
}

func TestFlowEndpointMountReplaceOnSwap(t *testing.T) {
	// Top-level mount must keep working after the inner router is
	// swapped. This is the core deploy-redeploy contract.
	mux := NewFlowEndpointMux()
	r := chi.NewRouter()
	r.Mount("/endpoint", mux)

	mux.Swap([]RouteSpec{{NodeID: "v1", Method: "GET", Path: "/x", Handler: okHandler("v1")}})
	if rec := serveTopLevel(r, "GET", "/endpoint/x"); rec.Body.String() != "v1" {
		t.Fatalf("v1: got %q", rec.Body.String())
	}

	mux.Swap([]RouteSpec{{NodeID: "v2", Method: "GET", Path: "/x", Handler: okHandler("v2")}})
	if rec := serveTopLevel(r, "GET", "/endpoint/x"); rec.Body.String() != "v2" {
		t.Fatalf("v2 after swap: got %q", rec.Body.String())
	}
}

func serveTopLevel(r chi.Router, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}
