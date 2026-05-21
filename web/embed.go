// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package web provides the embedded frontend assets for the LOOPZE editor
// and the dashboard SPA. Both Vue 3 apps are built into separate dist
// directories and embedded into the Go binary at compile time using
// go:embed.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// dist embeds the built editor Vue 3 assets from the dist/ directory.
// Build with:
//
//	cd frontend && npm run build:editor
//
//go:embed dist/*
var dist embed.FS

// distDashboard embeds the built dashboard SPA from dist-dashboard/.
// The dashboard is a separate Vue 3 app served at /dashboard/* — its
// bundle deliberately excludes Vue Flow, Monaco, and editor stores so
// operator endpoints (kiosks, mobile, shop-floor PCs) load fast.
// Build with:
//
//	cd frontend && npm run build:dashboard
//
//go:embed dist-dashboard/*
var distDashboard embed.FS

// GetFileSystem returns an http.FileSystem rooted at the dist/ subdirectory
// of the embedded filesystem. This strips the "dist" prefix so that files
// are served from the root path (e.g. /index.html instead of /dist/index.html).
func GetFileSystem() (http.FileSystem, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}

// GetFS returns the raw fs.FS rooted at the dist/ subdirectory.
// This is useful when a raw filesystem interface is needed instead of
// an http.FileSystem (e.g. for Chi's FileServer).
func GetFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

// GetDashboardFS returns the raw fs.FS rooted at the dist-dashboard/
// subdirectory. Counterpart to GetFS for the dashboard SPA.
func GetDashboardFS() (fs.FS, error) {
	return fs.Sub(distDashboard, "dist-dashboard")
}
