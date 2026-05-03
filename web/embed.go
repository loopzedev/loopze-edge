// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package web provides the embedded frontend assets for the LOOPZE editor.
// The Vue 3 frontend is built into the dist/ directory and embedded into
// the Go binary at compile time using go:embed.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// dist embeds the built Vue 3 frontend assets from the dist/ directory.
// The frontend must be built before compiling the Go binary:
//
//	cd frontend && npm run build
//
// The build output is placed in web/dist/ and embedded via go:embed.
//
//go:embed dist/*
var dist embed.FS

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
