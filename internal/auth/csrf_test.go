// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newCSRFTestHandler builds a handler chain with the CSRF middleware
// applied. The Secure attribute on issued cookies is decided per
// request (RequestIsSecure), so individual tests set TLS / X-Forwarded-
// Proto on the request as needed.
func newCSRFTestHandler() http.Handler {
	mw := CSRF()
	return mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCSRFIssuesCookieOnFirstRequest(t *testing.T) {
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == CSRFCookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("expected loopze_csrf cookie to be set")
	}
	if found.Value == "" {
		t.Fatal("loopze_csrf cookie has empty value")
	}
	if found.HttpOnly {
		t.Error("CSRF cookie must NOT be HttpOnly (SPA needs to read it)")
	}
}

func TestCSRFCookieSecureFromXForwardedProto(t *testing.T) {
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName && !c.Secure {
			t.Fatal("expected Secure when X-Forwarded-Proto=https")
		}
	}
}

func TestCSRFCookieNotSecureOverPlainHTTP(t *testing.T) {
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no TLS, no XFP
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName && c.Secure {
			t.Fatal("expected NOT Secure on plain HTTP")
		}
	}
}

func TestCSRFAllowsSafeMethodsWithoutHeader(t *testing.T) {
	h := newCSRFTestHandler()
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(m, "/", nil)
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("method %s: got %d, want 200", m, rec.Code)
		}
	}
}

func TestCSRFRejectsMutatingWithoutHeader(t *testing.T) {
	h := newCSRFTestHandler()
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(m, "/", nil)
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("method %s without header: got %d, want 403", m, rec.Code)
		}
	}
}

func TestCSRFRejectsMutatingWithMismatchedHeader(t *testing.T) {
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "abc"})
	req.Header.Set(CSRFHeaderName, "different-value")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestCSRFAcceptsMatchingHeader(t *testing.T) {
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "matching-token"})
	req.Header.Set(CSRFHeaderName, "matching-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestCSRFFreshlyIssuedTokenAcceptsImmediateMutation(t *testing.T) {
	// First request issues the cookie. The same handler re-runs with
	// the issued cookie + matching header — this is the SPA's flow
	// when the very first call is a POST and the response cookie is
	// captured by the cookie jar before the next request.
	h := newCSRFTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var token string
	for _, c := range rec.Result().Cookies() {
		if c.Name == CSRFCookieName {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("no CSRF token issued")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})
	req2.Header.Set(CSRFHeaderName, token)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request: got %d, want 200", rec2.Code)
	}
}

func TestCSRFTokenIsRandom(t *testing.T) {
	t1, err := newCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	t2, err := newCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if t1 == t2 {
		t.Fatal("two consecutive tokens were equal — entropy source broken")
	}
	if len(t1) < 32 {
		t.Fatalf("token shorter than expected: %d chars", len(t1))
	}
}
