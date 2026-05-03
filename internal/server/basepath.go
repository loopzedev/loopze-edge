// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"bytes"
	"errors"
	"html"
	"io/fs"
	"net/http"
	"strings"
)

// indexInjector wraps an embedded index.html so it is served with a
// runtime-configured <base href> and a window.__LOOPZE_BASE__ global.
// This lets a single binary be mounted at the root or at a subpath
// (e.g. /loopze) without rebuilding the frontend.
type indexInjector struct {
	raw      []byte
	basePath string // "" or "/segment"
}

// newIndexInjector reads index.html from the embedded filesystem and
// preprocesses it for runtime base-path injection. The raw bytes are
// scanned once; per-request work is just two byte replacements.
func newIndexInjector(fsys fs.FS, basePath string) (*indexInjector, error) {
	f, err := fsys.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, err
	}

	return &indexInjector{
		raw:      buf.Bytes(),
		basePath: basePath,
	}, nil
}

// render returns the index.html bytes with the runtime base path baked
// into a <base> tag and a global JS variable. The tags are inserted
// right after the opening <head> so they precede any module script.
//
// basePath is normalised to either "/" (root) or "/segment/" (with
// trailing slash, as required by the HTML <base> element).
func (i *indexInjector) render() []byte {
	hrefVal := "/"
	if i.basePath != "" {
		hrefVal = i.basePath + "/"
	}
	escaped := html.EscapeString(hrefVal)
	jsVal := jsString(hrefVal)

	inject := []byte(
		`<base href="` + escaped + `">` +
			`<script>window.__LOOPZE_BASE__=` + jsVal + `;</script>`,
	)

	out := bytes.Replace(i.raw, []byte("<head>"), append([]byte("<head>"), inject...), 1)
	if !bytes.Contains(out, inject) {
		// Fallback: prepend if <head> was not found verbatim.
		out = append(inject, i.raw...)
	}
	return out
}

// jsString quotes a string so it is safe to embed inside a JS literal.
// Only escapes backslash, single quote and the </ sequence that would
// otherwise close the surrounding <script> tag prematurely.
func jsString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `</`, `<\/`)
	return "'" + s + "'"
}

// stripBasePath returns middleware that removes the configured base
// prefix from r.URL.Path before handing the request to the inner
// handler. Requests that do not start with the prefix get a 404. When
// basePath is empty the middleware is a no-op.
func stripBasePath(basePath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if basePath == "" {
			return next
		}
		prefix := basePath
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			switch {
			case p == prefix:
				// Redirect /prefix → /prefix/ so the SPA's <base href>
				// resolves relative URLs correctly in the browser.
				http.Redirect(w, r, prefix+"/", http.StatusMovedPermanently)
				return
			case strings.HasPrefix(p, prefix+"/"):
				r2 := *r
				u := *r.URL
				u.Path = strings.TrimPrefix(p, prefix)
				if u.Path == "" {
					u.Path = "/"
				}
				r2.URL = &u
				next.ServeHTTP(w, &r2)
				return
			default:
				http.NotFound(w, r)
			}
		})
	}
}

// errFSNotExist matches fs.ErrNotExist, kept here as a tiny helper so
// callers don't have to import "io/fs" alongside their own imports.
func errFSNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
