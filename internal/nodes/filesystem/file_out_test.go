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

// newFileOut builds a fully initialised FileOutNode for tests, skipping the
// engine. Returns the node and a captured-status helper that records the most
// recent (color, label) pair.
func newFileOut(t *testing.T, props map[string]any) (*FileOutNode, *struct {
	color string
	text  string
}) {
	t.Helper()

	node, err := NewFileOutNode(flow.NodeConfig{ID: "test-fo", Type: "file-out", Properties: props})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	fo := node.(*FileOutNode)
	if err := fo.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	status := &struct {
		color string
		text  string
	}{}
	fo.SetStatus(func(color, text string) { status.color = color; status.text = text })
	return fo, status
}

func TestFileOut_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	fo, _ := newFileOut(t, map[string]any{"path": path, "mode": "overwrite"})
	msg := flow.NewMessage()
	msg.SetPayload("new")

	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("file content = %q, want %q", got, "new")
	}
}

func TestFileOut_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	fo, _ := newFileOut(t, map[string]any{"path": path, "mode": "append"})

	for _, payload := range []string{"a", "b", "c"} {
		msg := flow.NewMessage()
		msg.SetPayload(payload)
		if _, err := fo.HandleMessage(msg); err != nil {
			t.Fatalf("HandleMessage: %v", err)
		}
	}
	got, _ := os.ReadFile(path)
	if string(got) != "abc" {
		t.Errorf("file content = %q, want %q", got, "abc")
	}
}

func TestFileOut_Create_FailsIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.log")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fo, status := newFileOut(t, map[string]any{"path": path, "mode": "create"})
	msg := flow.NewMessage()
	msg.SetPayload("y")

	_, err := fo.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "file already exists") {
		t.Errorf("error = %v, want 'file already exists'", err)
	}
	if status.color != "red" || status.text != "file exists" {
		t.Errorf("status = (%s, %s), want (red, file exists)", status.color, status.text)
	}
}

func TestFileOut_AppendNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	fo, _ := newFileOut(t, map[string]any{
		"path":          path,
		"mode":          "append",
		"appendNewline": true,
	})

	msg := flow.NewMessage()
	msg.SetPayload("hello")
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello\n" {
		t.Errorf("file content = %q, want %q", got, "hello\n")
	}
}

func TestFileOut_CreateDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deeply", "nested", "out.log")
	fo, _ := newFileOut(t, map[string]any{
		"path":       path,
		"mode":       "overwrite",
		"createDirs": true,
	})

	msg := flow.NewMessage()
	msg.SetPayload("hi")
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hi" {
		t.Errorf("file content = %q, want %q", got, "hi")
	}
}

func TestFileOut_CreateDirsDisabled_FailsOnMissingParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "out.log")
	fo, _ := newFileOut(t, map[string]any{"path": path, "mode": "overwrite"})

	msg := flow.NewMessage()
	msg.SetPayload("hi")
	if _, err := fo.HandleMessage(msg); err == nil {
		t.Fatal("expected error when parent dir missing, got nil")
	}
}

func TestFileOut_BinaryPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	fo, _ := newFileOut(t, map[string]any{"path": path, "mode": "overwrite"})

	msg := flow.NewMessage()
	msg.SetPayload([]any{float64(0xDE), float64(0xAD), float64(0xBE), float64(0xEF)})
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if string(got) != string(want) {
		t.Errorf("file content = %v, want %v", got, want)
	}
}

func TestFileOut_BinaryPayloadFromExtension(t *testing.T) {
	// .bin extension makes encoding=auto resolve to binary; payload is a
	// string of byte numbers in []any form.
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	fo, _ := newFileOut(t, map[string]any{"path": path, "mode": "overwrite"})

	msg := flow.NewMessage()
	msg.SetPayload([]int{72, 105}) // "Hi"
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "Hi" {
		t.Errorf("file content = %q, want 'Hi'", got)
	}
}

func TestFileOut_MsgFilenameOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.log")
	overridePath := filepath.Join(dir, "actual.log")
	fo, _ := newFileOut(t, map[string]any{"path": configPath, "mode": "overwrite"})

	msg := flow.NewMessage()
	msg.SetPayload("via override")
	msg.Set("filename", overridePath)
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		t.Errorf("config path should not have been written")
	}
	got, _ := os.ReadFile(overridePath)
	if string(got) != "via override" {
		t.Errorf("override content = %q, want 'via override'", got)
	}
}

func TestFileOut_MustachePath(t *testing.T) {
	dir := t.TempDir()
	fo, _ := newFileOut(t, map[string]any{
		"path": filepath.Join(dir, "{{topic}}.log"),
		"mode": "overwrite",
	})

	msg := flow.NewMessage()
	msg.SetPayload("x")
	msg.SetTopic("alice")
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "alice.log"))
	if string(got) != "x" {
		t.Errorf("file content = %q, want 'x'", got)
	}
}

func TestFileOut_Passthrough(t *testing.T) {
	dir := t.TempDir()
	fo, _ := newFileOut(t, map[string]any{
		"path": filepath.Join(dir, "out.log"),
		"mode": "overwrite",
	})

	msg := flow.NewMessage()
	msg.SetPayload("x")
	out, err := fo.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 1 || out[0][0] != msg {
		t.Errorf("expected pass-through of input message on port 0, got %v", out)
	}
}

func TestFileOut_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil { // r-x, no write
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	fo, status := newFileOut(t, map[string]any{
		"path": filepath.Join(dir, "blocked.log"),
		"mode": "overwrite",
	})

	msg := flow.NewMessage()
	msg.SetPayload("x")
	_, err := fo.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected permission error, got nil")
	}
	if status.color != "red" || status.text != "permission denied" {
		t.Errorf("status = (%s, %s), want (red, permission denied)", status.color, status.text)
	}
}

func TestFileOut_JailViolation(t *testing.T) {
	jail := t.TempDir()
	fo, status := newFileOut(t, map[string]any{
		"path":     "/etc/passwd",
		"mode":     "overwrite",
		"rootJail": jail,
	})

	msg := flow.NewMessage()
	msg.SetPayload("x")
	_, err := fo.HandleMessage(msg)
	if err == nil {
		t.Fatal("expected jail violation, got nil")
	}
	if !strings.Contains(err.Error(), "escapes jail") {
		t.Errorf("error = %v, want 'escapes jail'", err)
	}
	if status.color != "red" {
		t.Errorf("status color = %q, want red", status.color)
	}
}

func TestFileOut_InitInvalidMode(t *testing.T) {
	node, _ := NewFileOutNode(flow.NodeConfig{
		ID:         "x",
		Type:       "file-out",
		Properties: map[string]any{"mode": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestFileOut_SuccessStatus(t *testing.T) {
	dir := t.TempDir()
	fo, status := newFileOut(t, map[string]any{
		"path": filepath.Join(dir, "out.log"),
		"mode": "overwrite",
	})

	msg := flow.NewMessage()
	msg.SetPayload("hello world")
	if _, err := fo.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if status.color != "blue" {
		t.Errorf("status color = %q, want blue", status.color)
	}
	if !strings.Contains(status.text, "wrote") {
		t.Errorf("status text = %q, want 'wrote ...'", status.text)
	}
}

func TestFileOut_NilMessage(t *testing.T) {
	fo, _ := newFileOut(t, map[string]any{"path": "/tmp/x", "mode": "overwrite"})
	out, err := fo.HandleMessage(nil)
	if err != nil {
		t.Errorf("nil message should be ignored, got error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output for nil input, got %v", out)
	}
}
