// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// sendCollector captures messages pushed by source nodes via SendFunc so
// watch-mode tests can assert on emitted events with a timeout.
type sendCollector struct {
	ch chan *flow.Message
}

func newSendCollector() *sendCollector {
	return &sendCollector{ch: make(chan *flow.Message, 16)}
}

func (c *sendCollector) sendFn() flow.SendFunc {
	return func(_ int, msg *flow.Message) { c.ch <- msg }
}

// wait blocks until one message arrives or the timeout elapses.
func (c *sendCollector) wait(t *testing.T, d time.Duration) *flow.Message {
	t.Helper()
	select {
	case msg := <-c.ch:
		return msg
	case <-time.After(d):
		t.Fatalf("timed out after %s waiting for message", d)
		return nil
	}
}

// waitNone asserts no message arrives within the window.
func (c *sendCollector) waitNone(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case msg := <-c.ch:
		t.Fatalf("expected no message in %s, got payload=%v event=%v",
			d, msg.Get("payload"), msg.Get("event"))
	case <-time.After(d):
	}
}

// drain returns all messages received within the window.
func (c *sendCollector) drain(t *testing.T, d time.Duration) []*flow.Message {
	t.Helper()
	deadline := time.After(d)
	var out []*flow.Message
	for {
		select {
		case msg := <-c.ch:
			out = append(out, msg)
		case <-deadline:
			return out
		}
	}
}

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

// startWatchFi builds a watch-mode file-in node, attaches a send collector,
// and starts it. Returns the node, collector, and status capture.
func startWatchFi(t *testing.T, props map[string]any) (*FileInNode, *sendCollector, *struct {
	color string
	text  string
}) {
	t.Helper()
	fi, status := newFileIn(t, props)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = fi.Stop() })
	return fi, col, status
}

func TestFileIn_WatchMode_EmitsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.log")
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, col, _ := startWatchFi(t, map[string]any{
		"path":       path,
		"mode":       "watch",
		"debounceMs": 0,
	})
	// Give the watcher a moment to register before writing.
	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)

	if got := msg.Get("event"); got != "write" {
		t.Errorf("event = %v, want write", got)
	}
	if got := msg.Get("filename"); got != path {
		t.Errorf("filename = %v, want %v", got, path)
	}
	if msg.Get("payload") != nil {
		t.Errorf("watch mode should not emit payload, got %v", msg.Get("payload"))
	}
}

func TestFileIn_WatchMode_EventFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.log")
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Only listen for "remove" — writes should be silently filtered.
	_, col, _ := startWatchFi(t, map[string]any{
		"path":        path,
		"mode":        "watch",
		"watchEvents": []string{"remove"},
		"debounceMs":  0,
	})
	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	col.waitNone(t, 200*time.Millisecond)
}

func TestFileIn_WatchMode_Debounce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.log")
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, col, _ := startWatchFi(t, map[string]any{
		"path":       path,
		"mode":       "watch",
		"debounceMs": 200,
	})
	time.Sleep(20 * time.Millisecond)

	// Three rapid writes inside the 200ms debounce window.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte("burst"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}

	// Wait for debounce to elapse plus slack.
	got := col.drain(t, 500*time.Millisecond)
	if len(got) != 1 {
		t.Errorf("expected 1 coalesced message, got %d", len(got))
	}
}

func TestFileIn_ReadPlusWatch_EmitsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tail.log")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, col, _ := startWatchFi(t, map[string]any{
		"path":       path,
		"mode":       "read+watch",
		"debounceMs": 0,
	})
	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(path, []byte("updated content"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)

	if got := msg.Get("payload"); got != "updated content" {
		t.Errorf("payload = %v, want 'updated content'", got)
	}
	if got := msg.Get("event"); got != "write" {
		t.Errorf("event = %v, want write", got)
	}
	if got := msg.Get("encoding"); got != "utf-8" {
		t.Errorf("encoding = %v, want utf-8", got)
	}
}

func TestFileIn_WatchMode_Status(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, status := startWatchFi(t, map[string]any{
		"path": path,
		"mode": "watch",
	})
	if status.color != "green" {
		t.Errorf("status color = %q, want green", status.color)
	}
	if !strings.Contains(status.text, "watching") {
		t.Errorf("status text = %q, want 'watching ...'", status.text)
	}
}

func TestFileIn_WatchMode_StartFailsOnMissingFile(t *testing.T) {
	fi, status := newFileIn(t, map[string]any{
		"path": "/does/not/exist/file.log",
		"mode": "watch",
	})
	col := newSendCollector()
	fi.SetSend(col.sendFn())

	if err := fi.Start(); err == nil {
		_ = fi.Stop()
		t.Fatal("expected Start error for missing file, got nil")
	}
	if status.color != "red" {
		t.Errorf("status color = %q, want red", status.color)
	}
}

func TestFileIn_WatchMode_InvalidWatchEvent(t *testing.T) {
	node, _ := NewFileInNode(flow.NodeConfig{
		ID:   "x",
		Type: "file-in",
		Properties: map[string]any{
			"mode":        "watch",
			"path":        "/tmp/x",
			"watchEvents": []string{"chmod"}, // not in whitelist
		},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid watch event")
	}
}

func TestFileIn_WatchMode_StopDrains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, status := newFileIn(t, map[string]any{"path": path, "mode": "watch"})
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- fi.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete within 2s")
	}
	_ = status
}
