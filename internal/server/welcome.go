// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/config"
)

// printWelcomeBanner writes a multi-line summary of the running runtime
// to w (typically stdout). It is called once after the HTTP listener
// has successfully bound, so the URLs printed are guaranteed to be
// reachable.
//
// The output is intentionally written via fmt.Fprintln (not slog) so
// the box drawing is not prefixed with timestamps and log levels.
func (s *Server) printWelcomeBanner(w io.Writer, addr string) {
	editorURL := buildBrowserURL(addr, s.cfg.BasePath)
	// buildBrowserURL already appends a trailing "/" after the prefix,
	// so the placeholder for flow-defined paths just needs the "…"
	// part — adding "/…" here would produce "/endpoint//…".
	endpointURL := buildBrowserURL(addr, s.cfg.HTTPNodeRoot) + "…"

	rows := []row{
		{"Editor", editorURL},
		{"Flow endpoints", endpointURL},
		{"Data directory", s.cfg.DataDir},
		{"NATS broker", fmt.Sprintf(":%d", s.cfg.NATSPort)},
		{"Log level", s.cfg.LogLevel},
	}
	if s.cfg.BasePath != "" {
		rows = append(rows, row{"URL prefix", s.cfg.BasePath})
	}

	notes := s.welcomeNotes()

	const indent = "  "
	const labelWidth = 16

	fmt.Fprintln(w)
	fmt.Fprintln(w, "  ────────────────────────────────────────────────────────────")
	fmt.Fprintf(w, "%s✓ LOOPZE %s is ready\n", indent, config.Version)
	fmt.Fprintln(w)
	for _, r := range rows {
		fmt.Fprintf(w, "%s%-*s  %s\n", indent, labelWidth, r.label, r.value)
	}
	if len(notes) > 0 {
		fmt.Fprintln(w)
		for _, note := range notes {
			fmt.Fprintf(w, "%s%s\n", indent, note)
		}
	}
	fmt.Fprintln(w, "  ────────────────────────────────────────────────────────────")
	fmt.Fprintln(w)
}

type row struct {
	label string
	value string
}

// welcomeNotes returns operationally-relevant hints to surface in the
// banner: first-run setup, dev-mode auth bypass, etc.
func (s *Server) welcomeNotes() []string {
	var notes []string

	if count, err := s.users.CountActiveAdmins(); err == nil && count == 0 {
		notes = append(notes, "First run — open the editor URL above to create the initial admin account.")
	}

	if s.authMW != nil && s.authMW.DevUser != nil {
		notes = append(notes, "⚠ Authentication is bypassed (LOOPZE_AUTH_DISABLE). Do not use in production.")
	}

	return notes
}

// buildBrowserURL composes the browser-facing URL for a server bound to
// addr ("0.0.0.0:1880" / "[::]:1880" / "127.0.0.1:1880" …) with the
// given base path. Wildcard binds are rewritten to "localhost" because
// 0.0.0.0 is not a valid browser destination.
func buildBrowserURL(addr, basePath string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = addr, ""
	}
	if isWildcardHost(host) {
		host = "localhost"
	}
	url := "http://" + host
	if port != "" {
		url += ":" + port
	}
	url += basePath + "/"
	return url
}

// isWildcardHost reports whether host is an "all interfaces" bind that
// shouldn't be shown as a browser destination.
func isWildcardHost(host string) bool {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return true
	}
	return strings.Contains(host, "::")
}
