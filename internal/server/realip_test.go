// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func captureHandler(captured *string) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		*captured = r.RemoteAddr
	})
}

func TestRealIPNoTrustedProxiesIsNoop(t *testing.T) {
	checker, err := newTrustedProxyChecker(nil)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := realIPMiddleware(checker)(captureHandler(&got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:55555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.10:55555" {
		t.Fatalf("expected unchanged RemoteAddr when no proxies trusted, got %q", got)
	}
}

func TestRealIPHonoursXForwardedForFromTrustedPeer(t *testing.T) {
	checker, err := newTrustedProxyChecker([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := realIPMiddleware(checker)(captureHandler(&got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.5.5.5:443"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.10" {
		t.Fatalf("expected RemoteAddr rewritten to leftmost X-Forwarded-For, got %q", got)
	}
}

func TestRealIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	checker, err := newTrustedProxyChecker([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := realIPMiddleware(checker)(captureHandler(&got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.99:33333" // public IP, not trusted
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "203.0.113.99:33333" {
		t.Fatalf("expected RemoteAddr unchanged when peer is untrusted, got %q", got)
	}
}

func TestRealIPSingleIPAllowlist(t *testing.T) {
	checker, err := newTrustedProxyChecker([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := realIPMiddleware(checker)(captureHandler(&got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("X-Real-IP", "198.51.100.7")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "198.51.100.7" {
		t.Fatalf("expected X-Real-IP honoured, got %q", got)
	}
}

func TestRealIPRFC7239Forwarded(t *testing.T) {
	checker, err := newTrustedProxyChecker([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := realIPMiddleware(checker)(captureHandler(&got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("Forwarded", `for="192.0.2.43:47011";proto=https;by=10.0.0.1`)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "192.0.2.43" {
		t.Fatalf("expected RFC 7239 for= honoured, got %q", got)
	}
}

func TestNewTrustedProxyCheckerInvalidEntry(t *testing.T) {
	if _, err := newTrustedProxyChecker([]string{"not an ip"}); err == nil {
		t.Fatal("expected error for malformed entry")
	}
	if _, err := newTrustedProxyChecker([]string{"10.0.0.0/notacidr"}); err == nil {
		t.Fatal("expected error for malformed CIDR")
	}
}
