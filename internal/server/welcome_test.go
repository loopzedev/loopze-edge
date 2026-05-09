// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import "testing"

func TestBuildBrowserURL(t *testing.T) {
	cases := []struct {
		name     string
		addr     string
		basePath string
		want     string
	}{
		{"localhost with port", "127.0.0.1:1880", "", "http://127.0.0.1:1880/"},
		{"wildcard rewrites to localhost", "0.0.0.0:1880", "", "http://localhost:1880/"},
		{"empty host rewrites to localhost", ":1880", "", "http://localhost:1880/"},
		{"ipv6 wildcard rewrites to localhost", "[::]:1880", "", "http://localhost:1880/"},
		{"with base path", "0.0.0.0:1880", "/loopze", "http://localhost:1880/loopze/"},
		{"hostname binding preserved", "broker.example.com:8443", "", "http://broker.example.com:8443/"},
	}
	for _, c := range cases {
		got := buildBrowserURL(c.addr, c.basePath)
		if got != c.want {
			t.Errorf("%s: buildBrowserURL(%q, %q) = %q, want %q",
				c.name, c.addr, c.basePath, got, c.want)
		}
	}
}
