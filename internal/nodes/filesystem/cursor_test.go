// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorKey_StableAndDistinct(t *testing.T) {
	a := cursorKey("node-1", "/var/log/app.log")
	b := cursorKey("node-1", "/var/log/app.log")
	if a != b {
		t.Errorf("cursorKey not deterministic: %q vs %q", a, b)
	}
	c := cursorKey("node-2", "/var/log/app.log")
	if a == c {
		t.Errorf("cursorKey collides between distinct node IDs: %q", a)
	}
	d := cursorKey("node-1", "/var/log/other.log")
	if a == d {
		t.Errorf("cursorKey collides between distinct paths: %q", a)
	}
	if !strings.HasPrefix(a, "_filecursor.") {
		t.Errorf("expected _filecursor. prefix, got %q", a)
	}
}

func TestTrimToLastLine_LF(t *testing.T) {
	cases := map[string]struct {
		in       string
		wantOut  string
		wantLine int
	}{
		"complete":           {"alice\nbob\n", "alice\nbob\n", 2},
		"trailing partial":   {"alice\nbob\nca", "alice\nbob\n", 2},
		"only one line":      {"alice\n", "alice\n", 1},
		"only partial":       {"alice", "", 0},
		"empty":              {"", "", 0},
		"multiple incl crlf": {"a\r\nb\nc", "a\r\nb\n", 2},
	}
	for name, tc := range cases {
		got, n, err := trimToLastLine([]byte(tc.in), DelimLF, 0)
		if err != nil {
			t.Errorf("%s: err = %v", name, err)
			continue
		}
		if string(got) != tc.wantOut {
			t.Errorf("%s: got %q, want %q", name, got, tc.wantOut)
		}
		if n != tc.wantLine {
			t.Errorf("%s: lineCount = %d, want %d", name, n, tc.wantLine)
		}
	}
}

func TestTrimToLastLine_CRLF(t *testing.T) {
	got, n, err := trimToLastLine([]byte("a\r\nb\r\nca"), DelimCRLF, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\r\nb\r\n" || n != 2 {
		t.Errorf("got %q (lines=%d), want \"a\\r\\nb\\r\\n\" (2)", got, n)
	}

	// Bare \n is NOT a CRLF boundary — entire buf is partial.
	got, n, err = trimToLastLine([]byte("a\nb\n"), DelimCRLF, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "" || n != 0 {
		t.Errorf("bare \\n should not match \\r\\n: got %q (lines=%d)", got, n)
	}
}

func TestTrimToLastLine_Auto(t *testing.T) {
	// Mixed line endings — auto accepts either as a boundary
	got, n, err := trimToLastLine([]byte("a\nb\r\nc\nd"), DelimAuto, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nb\r\nc\n" || n != 3 {
		t.Errorf("got %q (lines=%d), want \"a\\nb\\r\\nc\\n\" (3)", got, n)
	}
}

func TestTrimToLastLine_MaxBytesGuard(t *testing.T) {
	long := strings.Repeat("x", 1024) // no delimiter, 1024 bytes
	_, _, err := trimToLastLine([]byte(long), DelimLF, 1024)
	if err == nil {
		t.Fatal("expected maxLineBytes error")
	}
	if !strings.Contains(err.Error(), "exceeded maxLineBytes") {
		t.Errorf("error = %v, want 'exceeded maxLineBytes'", err)
	}

	// Below the limit — silent skip.
	out, _, err := trimToLastLine([]byte("xxx"), DelimLF, 1024)
	if err != nil || out != nil {
		t.Errorf("expected silent skip, got %q / %v", out, err)
	}
}

func TestReadIncremental_FromEOFFirstAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	res, err := readIncremental(store, IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Errorf("first access from EOF should skip, got data=%q", res.Data)
	}
	if res.Position != 9 {
		t.Errorf("position = %d, want 9", res.Position)
	}
}

func TestReadIncremental_FromStartFirstAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	res, err := readIncremental(store, IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != "hello\nworld\n" {
		t.Errorf("got %q, want %q", res.Data, "hello\nworld\n")
	}
	if res.LineCount != 2 {
		t.Errorf("lineCount = %d, want 2", res.LineCount)
	}
	if res.Position != 12 {
		t.Errorf("position = %d, want 12", res.Position)
	}
}

func TestReadIncremental_AppendThenRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	opts := IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	}
	first, _ := readIncremental(store, opts)
	if string(first.Data) != "first\n" {
		t.Fatalf("first read: got %q", first.Data)
	}

	// Append a second complete line.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("second\n")
	_ = f.Close()

	second, _ := readIncremental(store, opts)
	if string(second.Data) != "second\n" {
		t.Errorf("second read: got %q, want %q", second.Data, "second\n")
	}
	if second.Position != 13 {
		t.Errorf("position = %d, want 13", second.Position)
	}
}

func TestReadIncremental_PartialLineNotEmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("incomplete"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	opts := IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	}
	res, _ := readIncremental(store, opts)
	if !res.Skipped || len(res.Data) != 0 {
		t.Errorf("partial-only line should be skipped, got data=%q skipped=%v", res.Data, res.Skipped)
	}

	// Cursor should remain at 0 — append the missing newline and verify the
	// whole line comes through on the second read.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("\n")
	_ = f.Close()

	res2, _ := readIncremental(store, opts)
	if string(res2.Data) != "incomplete\n" {
		t.Errorf("after newline, got %q, want %q", res2.Data, "incomplete\n")
	}
}

func TestReadIncremental_TruncationReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("1\n2\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newMemStore()
	opts := IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	}
	_, _ = readIncremental(store, opts) // advances cursor to 6

	// Truncate (rotation) and write a fresh line.
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, _ := readIncremental(store, opts)
	if !res.Reset {
		t.Errorf("expected Reset=true after truncation")
	}
	if string(res.Data) != "new\n" {
		t.Errorf("after truncation: got %q, want %q", res.Data, "new\n")
	}
	if res.Position != 4 {
		t.Errorf("position = %d, want 4", res.Position)
	}
}

func TestReadIncremental_DelimiterNoneEmitsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	res, err := readIncremental(store, IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimNone, FromStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) != 4 {
		t.Errorf("got %d bytes, want 4", len(res.Data))
	}
	if res.LineCount != 0 {
		t.Errorf("lineCount = %d, want 0 for DelimNone", res.LineCount)
	}
}

func TestReadIncremental_NoNewBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	opts := IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	}
	_, _ = readIncremental(store, opts) // cursor → 2

	res, _ := readIncremental(store, opts)
	if !res.Skipped {
		t.Errorf("no new bytes should skip, got data=%q", res.Data)
	}
}

func TestReadIncremental_CursorPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	opts := IncrementalOpts{
		NodeID: "n1", AbsPath: path, Delimiter: DelimLF, MaxLineBytes: 1024, FromStart: true,
	}
	_, _ = readIncremental(store, opts)

	key := cursorKey(opts.NodeID, opts.AbsPath)
	v, _ := store.Get(key)
	if toInt64(v) != 6 {
		t.Errorf("persisted cursor = %v, want 6", v)
	}
}

func TestReadIncremental_NotFound(t *testing.T) {
	store := newMemStore()
	_, err := readIncremental(store, IncrementalOpts{
		NodeID: "n1", AbsPath: "/does/not/exist", Delimiter: DelimLF, FromStart: true,
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestResetCursorKey(t *testing.T) {
	store := newMemStore()
	key := cursorKey("n1", "/x")
	_ = store.Set(key, int64(42))

	if err := resetCursorKey(store, key); err != nil {
		t.Fatal(err)
	}
	v, _ := store.Get(key)
	if v != nil {
		t.Errorf("after reset, value = %v, want nil", v)
	}
}
