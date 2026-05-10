// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// memStore is an in-memory flow.ContextStore used by cursor and incremental
// tests. Keys are strings, values are stored as-is. Concurrency-safe so it
// can be shared between the test goroutine and the file-in watch loop.
type memStore struct {
	mu sync.RWMutex
	kv map[string]any
}

func newMemStore() *memStore {
	return &memStore{kv: map[string]any{}}
}

func (s *memStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.kv[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s *memStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv[key] = value
	return nil
}

func (s *memStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.kv, key)
	return nil
}

func (s *memStore) Keys() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.kv))
	for k := range s.kv {
		out = append(out, k)
	}
	return out, nil
}

func TestResolvePath_Plain(t *testing.T) {
	got, err := resolvePath("/var/log/app.log", flow.NewMessage(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/var/log/app.log" {
		t.Errorf("path = %q, want %q", got, "/var/log/app.log")
	}
}

func TestResolvePath_Mustache(t *testing.T) {
	msg := flow.NewMessage()
	msg.Set("payload", map[string]any{"name": "alice"})

	got, err := resolvePath("/tmp/{{payload.name}}.log", msg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/alice.log" {
		t.Errorf("path = %q, want %q", got, "/tmp/alice.log")
	}
}

func TestResolvePath_MsgFilenameOverride(t *testing.T) {
	msg := flow.NewMessage()
	msg.Set("filename", "/tmp/override.log")

	got, err := resolvePath("/tmp/{{ignored}}.log", msg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/override.log" {
		t.Errorf("path = %q, want %q", got, "/tmp/override.log")
	}
}

func TestResolvePath_NotAbsolute(t *testing.T) {
	_, err := resolvePath("relative/path.log", flow.NewMessage(), "")
	if err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
	if !strings.Contains(err.Error(), "not absolute") {
		t.Errorf("error = %v, want 'not absolute'", err)
	}
}

func TestResolvePath_Empty(t *testing.T) {
	_, err := resolvePath("", flow.NewMessage(), "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestResolvePath_JailViolation(t *testing.T) {
	jail := t.TempDir()
	_, err := resolvePath("/etc/passwd", flow.NewMessage(), jail)
	if err == nil {
		t.Fatal("expected jail-escape error, got nil")
	}
	if !strings.Contains(err.Error(), "escapes jail") {
		t.Errorf("error = %v, want 'escapes jail'", err)
	}
}

func TestResolvePath_JailOK(t *testing.T) {
	jail := t.TempDir()
	target := filepath.Join(jail, "subdir", "file.log") // does not exist yet

	got, err := resolvePath(target, flow.NewMessage(), jail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != target {
		t.Errorf("path = %q, want %q", got, target)
	}
}

func TestResolveEncoding_Auto(t *testing.T) {
	cases := map[string]string{
		"/tmp/foo.log":  "utf-8",
		"/tmp/foo.json": "utf-8",
		"/tmp/foo.csv":  "utf-8",
		"/tmp/foo.bin":  "binary",
		"/tmp/foo":      "binary",
		"/tmp/IMG.PNG":  "binary",
	}
	for path, want := range cases {
		got := resolveEncoding(path, "auto")
		if got != want {
			t.Errorf("resolveEncoding(%q, auto) = %q, want %q", path, got, want)
		}
	}
}

func TestResolveEncoding_Explicit(t *testing.T) {
	if got := resolveEncoding("/tmp/foo.bin", "utf-8"); got != "utf-8" {
		t.Errorf("explicit utf-8 should pass through, got %q", got)
	}
	if got := resolveEncoding("/tmp/foo.log", "binary"); got != "binary" {
		t.Errorf("explicit binary should pass through, got %q", got)
	}
}

func TestEncodePayload_String(t *testing.T) {
	got, err := encodePayload("hello", "utf-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestEncodePayload_StructAsJSON(t *testing.T) {
	got, err := encodePayload(map[string]any{"a": 1}, "utf-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("got %q, want %q", got, `{"a":1}`)
	}
}

func TestEncodePayload_BinaryByteSlice(t *testing.T) {
	got, err := encodePayload([]byte{0xDE, 0xAD}, "binary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != 0xDE || got[1] != 0xAD {
		t.Errorf("got %v, want [0xDE 0xAD]", got)
	}
}

func TestEncodePayload_BinaryNumberArray(t *testing.T) {
	// JSON-decoded number arrays come through as []any with float64 elements.
	got, err := encodePayload([]any{float64(72), float64(105)}, "binary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "Hi" {
		t.Errorf("got %q, want %q", got, "Hi")
	}
}

func TestEncodePayload_BinaryIntSlice(t *testing.T) {
	got, err := encodePayload([]int{72, 105}, "binary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "Hi" {
		t.Errorf("got %q, want %q", got, "Hi")
	}
}

func TestEncodePayload_BinaryOutOfRange(t *testing.T) {
	_, err := encodePayload([]int{72, 256}, "binary")
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestEncodePayload_Nil(t *testing.T) {
	got, err := encodePayload(nil, "utf-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestApplyJail_Inside(t *testing.T) {
	jail := t.TempDir()
	target := filepath.Join(jail, "ok.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyJail(target, jail); err != nil {
		t.Errorf("expected jail OK, got %v", err)
	}
}

func TestApplyJail_Outside(t *testing.T) {
	jail := t.TempDir()
	if err := applyJail("/etc/passwd", jail); err == nil {
		t.Fatal("expected jail violation")
	}
}

func TestApplyJail_SymlinkEscape(t *testing.T) {
	jail := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(jail, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(link, "secret.txt") // resolves outside jail

	if err := applyJail(target, jail); err == nil {
		t.Fatal("expected jail violation via symlink, got nil")
	}
}

func TestApplyJail_NonexistentTargetInsideJail(t *testing.T) {
	jail := t.TempDir()
	target := filepath.Join(jail, "subdir", "nope.txt") // does not exist yet
	if err := applyJail(target, jail); err != nil {
		t.Errorf("expected lenient resolution to allow non-existent target, got %v", err)
	}
}
