// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// trustedProxyChecker resolves CIDR/IP entries from config into a fast
// matcher that decides whether a TCP peer is allowed to set forwarded
// headers on our behalf.
type trustedProxyChecker struct {
	nets []*net.IPNet
	ips  map[string]struct{}
}

// newTrustedProxyChecker compiles the configured CIDR/IP entries. Single
// IPs are stored in a set; CIDR blocks are stored as parsed networks.
// Returns an error on the first malformed entry so misconfiguration is
// surfaced at startup rather than silently ignored.
func newTrustedProxyChecker(entries []string) (*trustedProxyChecker, error) {
	c := &trustedProxyChecker{ips: make(map[string]struct{})}
	for _, raw := range entries {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			_, n, err := net.ParseCIDR(s)
			if err != nil {
				return nil, fmt.Errorf("trusted-proxies: invalid CIDR %q: %w", s, err)
			}
			c.nets = append(c.nets, n)
			continue
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("trusted-proxies: invalid IP %q", s)
		}
		c.ips[ip.String()] = struct{}{}
	}
	return c, nil
}

// trusts reports whether a peer IP is in the configured allowlist.
func (c *trustedProxyChecker) trusts(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if _, ok := c.ips[ip.String()]; ok {
		return true
	}
	for _, n := range c.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// empty reports whether no proxies are trusted at all. Used to skip the
// middleware entirely when the operator has not configured any.
func (c *trustedProxyChecker) empty() bool {
	return len(c.nets) == 0 && len(c.ips) == 0
}

// realIPMiddleware rewrites r.RemoteAddr from forwarded headers, but only
// when the direct TCP peer is in the trusted-proxy allowlist. This is a
// safer replacement for chi's middleware.RealIP, which trusts every hop.
//
// Headers consulted, in order: Forwarded (RFC 7239), X-Forwarded-For,
// X-Real-IP. The leftmost public-looking address wins; we do not walk the
// chain past the first untrusted hop because everything beyond a trusted
// proxy may have been forged by the actual client.
func realIPMiddleware(checker *trustedProxyChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if checker == nil || checker.empty() {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peerIP := peerIPFromRemoteAddr(r.RemoteAddr)
			if peerIP != nil && checker.trusts(peerIP) {
				if ip := clientIPFromHeaders(r); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// peerIPFromRemoteAddr extracts the bare IP from an "ip:port" form. Falls
// back to ParseIP on the whole string for callers that pass a bare IP.
func peerIPFromRemoteAddr(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.ParseIP(host)
}

// clientIPFromHeaders returns the leftmost IP from the configured
// forwarded-header set. Returns "" when no recognised header is present.
func clientIPFromHeaders(r *http.Request) string {
	if v := r.Header.Get("Forwarded"); v != "" {
		if ip := parseForwardedFor(v); ip != "" {
			return ip
		}
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		if ip := strings.TrimSpace(v); ip != "" {
			return ip
		}
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

// parseForwardedFor extracts the first for= parameter from an RFC 7239
// Forwarded header. Strips surrounding quotes and brackets if present.
func parseForwardedFor(header string) string {
	first := header
	if i := strings.IndexByte(header, ','); i >= 0 {
		first = header[:i]
	}
	for _, part := range strings.Split(first, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(kv[0]), "for") {
			continue
		}
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		val = strings.TrimPrefix(val, "[")
		val = strings.TrimSuffix(val, "]")
		if i := strings.LastIndexByte(val, ':'); i >= 0 && strings.Count(val, ":") == 1 {
			val = val[:i]
		}
		return val
	}
	return ""
}
