// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRFCookieName is the cookie name used for the double-submit CSRF
// token. It is intentionally not HttpOnly so the SPA can read its value
// and echo it back via the X-CSRF-Token header on state-changing
// requests.
const CSRFCookieName = "loopze_csrf"

// CSRFHeaderName is the HTTP header the client must set on mutating
// requests; its value must equal the cookie value. Same-origin browser
// requests can set this header; cross-origin form submits cannot, which
// is what the protection exploits.
const CSRFHeaderName = "X-CSRF-Token"

// csrfTokenBytes is the raw entropy of the token before base64 encoding.
const csrfTokenBytes = 32

// CSRF returns middleware implementing the double-submit cookie CSRF
// pattern. On every request, if the CSRF cookie is missing it is set to
// a fresh random value. On state-changing methods (POST/PUT/PATCH/
// DELETE) the request is rejected with 403 unless X-CSRF-Token equals
// the cookie value. Safe methods (GET/HEAD/OPTIONS) are never rejected,
// which lets the SPA pick up the cookie via /auth/status before its
// first mutation.
//
// The Secure attribute on the issued cookie is decided per request
// (RequestIsSecure), so HTTPS callers get a Secure cookie and plain-
// HTTP callers do not — the same binary works on http://localhost, in
// a LAN over plain HTTP, and behind a TLS-terminating proxy without
// any boot-time flag.
func CSRF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, _ := r.Cookie(CSRFCookieName)
			token := ""
			if cookie != nil {
				token = cookie.Value
			}
			if token == "" {
				t, err := newCSRFToken()
				if err != nil {
					jsonError(w, http.StatusInternalServerError, "failed to issue CSRF token")
					return
				}
				token = t
				http.SetCookie(w, &http.Cookie{
					Name:     CSRFCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: false,
					Secure:   RequestIsSecure(r),
					SameSite: http.SameSiteLaxMode,
				})
			}

			if isMutatingMethod(r.Method) {
				header := r.Header.Get(CSRFHeaderName)
				if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(token)) != 1 {
					jsonError(w, http.StatusForbidden, "missing or invalid CSRF token")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// newCSRFToken returns a fresh URL-safe base64 token.
func newCSRFToken() (string, error) {
	buf := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// isMutatingMethod reports whether the HTTP method is one the CSRF
// middleware must guard. GET/HEAD/OPTIONS are considered safe per
// RFC 7231 and are never rejected.
func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
