// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newFakeIndexFS(body string) fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(body)},
	}
}

func TestIndexInjectorRoot(t *testing.T) {
	fsys := newFakeIndexFS(`<html><head><title>x</title></head></html>`)
	inj, err := newIndexInjector(fsys, "")
	if err != nil {
		t.Fatal(err)
	}
	body := string(inj.render())
	if !strings.Contains(body, `<base href="/">`) {
		t.Errorf("expected <base href=\"/\">, got: %s", body)
	}
	if !strings.Contains(body, `window.__LOOPZE_BASE__='/'`) {
		t.Errorf("expected window.__LOOPZE_BASE__='/', got: %s", body)
	}
}

func TestIndexInjectorSubpath(t *testing.T) {
	fsys := newFakeIndexFS(`<html><head><title>x</title></head></html>`)
	inj, err := newIndexInjector(fsys, "/loopze")
	if err != nil {
		t.Fatal(err)
	}
	body := string(inj.render())
	if !strings.Contains(body, `<base href="/loopze/">`) {
		t.Errorf("expected <base href=\"/loopze/\">, got: %s", body)
	}
	if !strings.Contains(body, `window.__LOOPZE_BASE__='/loopze/'`) {
		t.Errorf("expected window.__LOOPZE_BASE__='/loopze/', got: %s", body)
	}
}

func TestIndexInjectorEscapesScriptCloseInPath(t *testing.T) {
	// A pathological base path containing "</" must not break out of
	// the injected <script> tag. (We never set this from config but the
	// guard is cheap and prevents nasty surprises.)
	fsys := newFakeIndexFS(`<html><head></head></html>`)
	inj, err := newIndexInjector(fsys, "/x</script>")
	if err != nil {
		t.Fatal(err)
	}
	body := string(inj.render())
	// Inside the script tag, </ must be escaped to <\/.
	if bytes.Contains([]byte(body[:strings.Index(body, "</script>")+9]), []byte(`</script>x`)) {
		t.Fatal("script close was not escaped inside JS literal")
	}
}

func TestStripBasePathRedirectsBareSegment(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	mw := stripBasePath("/loopze")(inner)

	req := httptest.NewRequest(http.MethodGet, "/loopze", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("got %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/loopze/" {
		t.Errorf("redirect location: got %q, want %q", loc, "/loopze/")
	}
	if called {
		t.Error("inner handler should not have been called on redirect")
	}
}

func TestStripBasePathRewritesPath(t *testing.T) {
	var seen string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.URL.Path })
	mw := stripBasePath("/loopze")(inner)

	req := httptest.NewRequest(http.MethodGet, "/loopze/api/v1/foo", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "/api/v1/foo" {
		t.Fatalf("inner saw path %q, want /api/v1/foo", seen)
	}
}

func TestStripBasePath404OutsidePrefix(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := stripBasePath("/loopze")(inner)

	req := httptest.NewRequest(http.MethodGet, "/somewhere/else", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestStripBasePathEmptyIsNoop(t *testing.T) {
	var seen string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.URL.Path })
	mw := stripBasePath("")(inner)

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "/anything" {
		t.Fatalf("expected path unchanged, got %q", seen)
	}
}
