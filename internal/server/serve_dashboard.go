// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/loopzedev/loopze-edge/web"
)

// serveDashboard mounts the embedded dashboard SPA at /dashboard/*.
// Mirrors serveFrontend but operates on a different embedded FS
// (web.dist-dashboard) and lives under a fixed prefix:
//
//   - GET /dashboard           → SPA index.html
//   - GET /dashboard/          → SPA index.html
//   - GET /dashboard/foo.js    → asset from dist-dashboard
//   - GET /dashboard/foo/bar   → SPA fallback to index.html (Vue Router)
//
// Mounted before the catch-all /* (which serves the editor SPA) so the
// dashboard prefix is not stolen by the editor's fallback.
//
// Auth: the SPA shell itself is served without authentication (so a
// login redirect can render). Live data flows through /api/dashboard/ws
// and /api/v1/dashboard/* which are session-gated.
func (s *Server) serveDashboard() {
	dashFS, err := web.GetDashboardFS()
	if err != nil {
		slog.Warn("failed to load dashboard filesystem, /dashboard will not be available", "error", err)
		return
	}

	indexBytes, err := readIndex(dashFS)
	if err != nil {
		slog.Warn("failed to load dashboard index.html", "error", err)
	}

	fileServer := http.FileServer(http.FS(dashFS))

	serveIndex := func(w http.ResponseWriter, _ *http.Request) {
		if len(indexBytes) == 0 {
			http.Error(w, "dashboard not built", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(indexBytes)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Strip the /dashboard prefix so we can open files in dashFS by
		// their bare name. Chi's wildcard handler hands us the full
		// r.URL.Path including the prefix.
		rel := strings.TrimPrefix(r.URL.Path, "/dashboard")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "index.html" {
			serveIndex(w, r)
			return
		}
		f, err := dashFS.Open(rel)
		if err != nil {
			if errFSNotExist(err) {
				serveIndex(w, r)
				return
			}
			fileServer.ServeHTTP(w, withRelativePath(r, rel))
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, withRelativePath(r, rel))
	}

	// Two routes so /dashboard and /dashboard/ both work, plus the
	// wildcard for everything below.
	s.router.Get("/dashboard", handler)
	s.router.Get("/dashboard/", handler)
	s.router.Get("/dashboard/*", handler)
}

// readIndex loads index.html out of the dashboard FS so the caller can
// serve it with the right Content-Type / Cache-Control without going
// through http.FileServer (which would set its own headers).
func readIndex(dashFS fs.FS) ([]byte, error) {
	f, err := dashFS.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, info.Size())
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// withRelativePath returns a shallow copy of r whose URL.Path is set to
// the prefix-stripped path. http.FileServer uses URL.Path to look up
// files; without rewriting it the request still carries /dashboard/foo
// and FileServer would 404.
func withRelativePath(r *http.Request, rel string) *http.Request {
	r2 := *r
	u := *r.URL
	u.Path = "/" + rel
	r2.URL = &u
	return &r2
}
