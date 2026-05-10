// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// newFileIn builds an initialised FileInNode for tests, capturing the most
// recent (color, label) pair pushed to the status callback.
func newFileIn(t *testing.T, props map[string]any) (*FileInNode, *struct {
	color string
	text  string
}) {
	t.Helper()

	node, err := NewFileInNode(flow.NodeConfig{ID: "test-fi", Type: "file-in", Properties: props})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	fi := node.(*FileInNode)
	if err := fi.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	status := &struct {
		color string
		text  string
	}{}
	fi.SetStatus(func(color, text string) { status.color = color; status.text = text })
	return fi, status
}

func TestFileIn_ReadMode_UTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{"path": path, "mode": "read"})
	out, err := fi.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 1 {
		t.Fatalf("expected 1x1 output, got %v", out)
	}
	msg := out[0][0]
	if got := msg.Get("payload"); got != "hello world" {
		t.Errorf("payload = %v, want %q", got, "hello world")
	}
	if got := msg.Get("encoding"); got != "utf-8" {
		t.Errorf("encoding = %v, want utf-8", got)
	}
	if got := msg.Get("filename"); got != path {
		t.Errorf("filename = %v, want %q", got, path)
	}
	if got, _ := msg.Get("size").(int64); got != 11 {
		t.Errorf("size = %v, want 11", msg.Get("size"))
	}
}

func TestFileIn_ReadMode_Binary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte{0xDE, 0xAD, 0xBE, 0xEF}, 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{"path": path, "mode": "read"})
	out, _ := fi.HandleMessage(flow.NewMessage())
	msg := out[0][0]

	bytes, ok := msg.Get("payload").([]int)
	if !ok {
		t.Fatalf("payload type = %T, want []int", msg.Get("payload"))
	}
	want := []int{0xDE, 0xAD, 0xBE, 0xEF}
	if len(bytes) != len(want) {
		t.Fatalf("len = %d, want %d", len(bytes), len(want))
	}
	for i, b := range bytes {
		if b != want[i] {
			t.Errorf("byte[%d] = %d, want %d", i, b, want[i])
		}
	}
	if got := msg.Get("encoding"); got != "binary" {
		t.Errorf("encoding = %v, want binary", got)
	}
}

func TestFileIn_ReadMode_AutoEncoding(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"data.json": "utf-8",
		"app.log":   "utf-8",
		"img.png":   "binary",
		"raw.bin":   "binary",
	}
	for name, wantEnc := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("xyz"), 0o644); err != nil {
			t.Fatal(err)
		}
		fi, _ := newFileIn(t, map[string]any{"path": path, "mode": "read", "encoding": "auto"})
		out, err := fi.HandleMessage(flow.NewMessage())
		if err != nil {
			t.Fatalf("%s: HandleMessage: %v", name, err)
		}
		if got := out[0][0].Get("encoding"); got != wantEnc {
			t.Errorf("%s: encoding = %v, want %v", name, got, wantEnc)
		}
	}
}

func TestFileIn_ReadMode_MsgFilenameOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.txt")
	overridePath := filepath.Join(dir, "override.txt")
	if err := os.WriteFile(overridePath, []byte("from override"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{"path": configPath, "mode": "read"})
	msg := flow.NewMessage()
	msg.Set("filename", overridePath)

	out, err := fi.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Get("payload"); got != "from override" {
		t.Errorf("payload = %v, want 'from override'", got)
	}
}

func TestFileIn_ReadMode_MustachePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alice.log")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{
		"path": filepath.Join(dir, "{{topic}}.log"),
		"mode": "read",
	})
	msg := flow.NewMessage()
	msg.SetTopic("alice")

	out, err := fi.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Get("payload"); got != "ok" {
		t.Errorf("payload = %v, want 'ok'", got)
	}
}

func TestFileIn_ReadMode_NotFound(t *testing.T) {
	fi, status := newFileIn(t, map[string]any{
		"path": "/does/not/exist/at/all.log",
		"mode": "read",
	})

	_, err := fi.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if status.color != "red" || status.text != "not found" {
		t.Errorf("status = (%s, %s), want (red, not found)", status.color, status.text)
	}
}

func TestFileIn_ReadMode_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	fi, status := newFileIn(t, map[string]any{"path": path, "mode": "read"})
	_, err := fi.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected permission error")
	}
	if status.color != "red" || status.text != "permission denied" {
		t.Errorf("status = (%s, %s), want (red, permission denied)", status.color, status.text)
	}
}

func TestFileIn_JailViolation(t *testing.T) {
	jail := t.TempDir()
	fi, _ := newFileIn(t, map[string]any{
		"path":     "/etc/passwd",
		"mode":     "read",
		"rootJail": jail,
	})
	_, err := fi.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected jail violation")
	}
	if !strings.Contains(err.Error(), "escapes jail") {
		t.Errorf("error = %v, want 'escapes jail'", err)
	}
}

func TestFileIn_PreservesExistingMsgFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hi.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{"path": path, "mode": "read"})
	msg := flow.NewMessage()
	msg.SetTopic("kept")
	msg.Set("custom", 42)

	out, _ := fi.HandleMessage(msg)
	got := out[0][0]
	if got.Get("topic") != "kept" {
		t.Errorf("topic was overwritten: %v", got.Get("topic"))
	}
	if got.Get("custom") != 42 {
		t.Errorf("custom field lost: %v", got.Get("custom"))
	}
}

func TestFileIn_NonReadMode_DropsMessage(t *testing.T) {
	// In watch / read+watch mode the node has no input wired by the engine,
	// but defensively we should also drop incoming messages if one arrives.
	for _, mode := range []string{"watch", "read+watch"} {
		fi, _ := newFileIn(t, map[string]any{"path": "/tmp/x", "mode": mode})
		out, err := fi.HandleMessage(flow.NewMessage())
		if err != nil {
			t.Errorf("mode=%s: unexpected error %v", mode, err)
		}
		if out != nil {
			t.Errorf("mode=%s: expected nil output, got %v", mode, out)
		}
	}
}

func TestFileIn_InitInvalidMode(t *testing.T) {
	node, _ := NewFileInNode(flow.NodeConfig{
		ID:         "x",
		Type:       "file-in",
		Properties: map[string]any{"mode": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestFileIn_InitInvalidEncoding(t *testing.T) {
	node, _ := NewFileInNode(flow.NodeConfig{
		ID:         "x",
		Type:       "file-in",
		Properties: map[string]any{"encoding": "rot13"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid encoding")
	}
}

func TestFileIn_NilMessage(t *testing.T) {
	fi, _ := newFileIn(t, map[string]any{"path": "/tmp/x", "mode": "read"})
	out, err := fi.HandleMessage(nil)
	if err != nil {
		t.Errorf("nil message should be ignored, got error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for nil input, got %v", out)
	}
}
