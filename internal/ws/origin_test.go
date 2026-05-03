// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package ws

import (
	"net/http"
	"testing"
)

func mkReq(host, origin string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "http://"+host+"/ws", nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestOriginCheckerSameOriginFallback(t *testing.T) {
	check := buildOriginChecker(nil)

	tests := []struct {
		host, origin string
		want         bool
	}{
		{"app.example.com", "http://app.example.com", true},
		{"app.example.com", "https://app.example.com", true},
		{"app.example.com:8080", "http://app.example.com:8080", true},
		{"app.example.com", "http://evil.com", false},
		{"app.example.com", "http://app.example.com.evil.com", false},
	}
	for _, tc := range tests {
		got := check(mkReq(tc.host, tc.origin))
		if got != tc.want {
			t.Errorf("host=%s origin=%s: got %v, want %v", tc.host, tc.origin, got, tc.want)
		}
	}
}

func TestOriginCheckerExactAllowlist(t *testing.T) {
	check := buildOriginChecker([]string{"https://app.example.com", "http://localhost:1880"})

	cases := map[string]bool{
		"https://app.example.com": true,
		"http://localhost:1880":   true,
		"https://other.com":       false,
		"http://app.example.com":  false, // scheme/port matters in exact match
	}
	for origin, want := range cases {
		got := check(mkReq("anything", origin))
		if got != want {
			t.Errorf("origin=%s: got %v, want %v", origin, got, want)
		}
	}
}

func TestOriginCheckerWildcardSubdomain(t *testing.T) {
	check := buildOriginChecker([]string{"*.example.com"})

	cases := map[string]bool{
		"https://app.example.com":     true,
		"https://api.app.example.com": true,
		"https://example.com":         false, // bare apex not matched by *.x.com
		"https://example.com.evil":    false,
	}
	for origin, want := range cases {
		got := check(mkReq("anywhere", origin))
		if got != want {
			t.Errorf("origin=%s: got %v, want %v", origin, got, want)
		}
	}
}

func TestOriginCheckerEmptyOriginAllowed(t *testing.T) {
	// Non-browser clients omit Origin. Auth still gates the upgrade.
	check := buildOriginChecker([]string{"https://app.example.com"})
	if !check(mkReq("anywhere", "")) {
		t.Fatal("expected empty Origin to be allowed (non-browser client)")
	}
}

func TestOriginCheckerMalformedOriginRejected(t *testing.T) {
	check := buildOriginChecker(nil)
	if check(mkReq("anywhere", "not-a-url")) {
		t.Fatal("expected malformed Origin to be rejected")
	}
}
