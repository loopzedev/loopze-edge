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

// withPers attaches an in-memory persistent store to the node so incremental
// mode tests have somewhere to persist the cursor.
func withPers(fi *FileInNode) *memStore {
	store := newMemStore()
	fi.SetContext(nil, nil, nil, store)
	return store
}

func TestFileIn_Incremental_Read_FromStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("alice\nbob\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withPers(fi)

	out, err := fi.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	msg := out[0][0]
	if got := msg.Get("payload"); got != "alice\nbob\n" {
		t.Errorf("payload = %v, want 'alice\\nbob\\n'", got)
	}
	if got := msg.Get("lineCount"); got != 2 {
		t.Errorf("lineCount = %v, want 2", got)
	}
	if got, _ := msg.Get("position").(int64); got != 10 {
		t.Errorf("position = %v, want 10", msg.Get("position"))
	}
}

func TestFileIn_Incremental_Read_FromEOF_NoEmitOnFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, status := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   false,
	})
	withPers(fi)

	out, err := fi.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out != nil {
		t.Errorf("expected no emission on first access from EOF, got %v", out)
	}
	if status.color == "red" {
		t.Errorf("first access should not error: status %s/%s", status.color, status.text)
	}
}

func TestFileIn_Incremental_Read_AppendsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withPers(fi)

	// First trigger — file empty, nothing to emit.
	out, _ := fi.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Fatalf("expected no emission on empty file, got %v", out)
	}

	// Append two lines and trigger again.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("first\nsecond\n")
	_ = f.Close()

	out, _ = fi.HandleMessage(flow.NewMessage())
	msg := out[0][0]
	if got := msg.Get("payload"); got != "first\nsecond\n" {
		t.Errorf("after append: payload = %v, want 'first\\nsecond\\n'", got)
	}

	// Append one more — only the new line should come through.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("third\n")
	_ = f.Close()

	out, _ = fi.HandleMessage(flow.NewMessage())
	msg = out[0][0]
	if got := msg.Get("payload"); got != "third\n" {
		t.Errorf("delta read: payload = %v, want 'third\\n'", got)
	}
}

func TestFileIn_Incremental_Read_PartialLineNotEmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("incomplete"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withPers(fi)

	out, _ := fi.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Errorf("partial line should not emit, got %v", out)
	}

	// Complete the line — now it should come through.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("\n")
	_ = f.Close()

	out, _ = fi.HandleMessage(flow.NewMessage())
	msg := out[0][0]
	if got := msg.Get("payload"); got != "incomplete\n" {
		t.Errorf("after newline: payload = %v, want 'incomplete\\n'", got)
	}
}

func TestFileIn_Incremental_Read_ResetMsg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, status := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	store := withPers(fi)

	_, _ = fi.HandleMessage(flow.NewMessage()) // cursor advances

	// Verify cursor is set, then send resetCursor.
	key := cursorKey(fi.cfg.ID, path)
	if v, _ := store.Get(key); v == nil {
		t.Fatal("cursor should be persisted after first read")
	}

	resetMsg := flow.NewMessage()
	resetMsg.Set("resetCursor", true)
	out, err := fi.HandleMessage(resetMsg)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if out != nil {
		t.Errorf("reset should not emit content, got %v", out)
	}
	if v, _ := store.Get(key); v != nil {
		t.Errorf("cursor should be cleared after reset, got %v", v)
	}
	if status.color != "blue" || !strings.Contains(status.text, "reset") {
		t.Errorf("status = (%s, %s), want (blue, ...reset)", status.color, status.text)
	}
}

func TestFileIn_Incremental_Read_TruncationEmitsReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("old1\nold2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withPers(fi)

	_, _ = fi.HandleMessage(flow.NewMessage())

	// Truncate (rotation) and write a fresh line.
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := fi.HandleMessage(flow.NewMessage())
	msg := out[0][0]
	if got := msg.Get("payload"); got != "new\n" {
		t.Errorf("after truncation: payload = %v, want 'new\\n'", got)
	}
	if got, _ := msg.Get("reset").(bool); !got {
		t.Errorf("expected reset=true on truncation, got %v", msg.Get("reset"))
	}
}

func TestFileIn_Incremental_Read_DelimiterNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.bin")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"encoding":    "binary",
		"incremental": true,
		"fromStart":   true,
		"delimiter":   "none",
	})
	withPers(fi)

	out, _ := fi.HandleMessage(flow.NewMessage())
	msg := out[0][0]
	bytes, _ := msg.Get("payload").([]int)
	if len(bytes) != 3 {
		t.Errorf("payload len = %d, want 3", len(bytes))
	}
	if got := msg.Get("lineCount"); got != nil {
		t.Errorf("lineCount should be absent for DelimNone, got %v", got)
	}
}

func TestFileIn_Incremental_Read_DefaultDelimiterByEncoding(t *testing.T) {
	// Binary encoding with default delimiter (\n) → should auto-fall through to none
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(path, []byte{0xFF, 0xFE, 0xFD}, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read",
		"encoding":    "auto",
		"incremental": true,
		"fromStart":   true,
		// delimiter left at default "\n"
	})
	withPers(fi)

	out, _ := fi.HandleMessage(flow.NewMessage())
	if out == nil {
		t.Fatal("expected emission for binary file with default delimiter, got nil")
	}
	bytes, ok := out[0][0].Get("payload").([]int)
	if !ok || len(bytes) != 3 {
		t.Errorf("expected 3-byte []int payload, got %T %v", out[0][0].Get("payload"), out[0][0].Get("payload"))
	}
}

func TestFileIn_Incremental_StartFailsWithoutFlowPers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "watch",
		"incremental": true,
	})
	// Note: NOT calling withPers — flowPers stays nil.
	col := newSendCollector()
	fi.SetSend(col.sendFn())

	if err := fi.Start(); err == nil {
		_ = fi.Stop()
		t.Fatal("expected Start to fail without flowPers, got nil")
	}
}

func TestFileIn_Incremental_ReadPlusWatch_TailLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "growing.log")
	if err := os.WriteFile(path, []byte("seed line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, status := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read+watch",
		"incremental": true,
		"fromStart":   false, // skip seed
		"debounceMs":  20,
	})
	withPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = fi.Stop() })
	if status.color != "green" {
		t.Errorf("status color after start = %q, want green", status.color)
	}

	time.Sleep(30 * time.Millisecond)

	// Append a complete line — should appear.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("event one\n")
	_ = f.Close()

	msg := col.wait(t, 2*time.Second)
	if got := msg.Get("payload"); got != "event one\n" {
		t.Errorf("first emit: payload = %v, want 'event one\\n'", got)
	}

	// Append a second line.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("event two\n")
	_ = f.Close()

	msg = col.wait(t, 2*time.Second)
	if got := msg.Get("payload"); got != "event two\n" {
		t.Errorf("second emit: payload = %v, want 'event two\\n'", got)
	}
}

func TestFileIn_Incremental_ReadPlusWatch_PartialLineSilent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "growing.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFileIn(t, map[string]any{
		"path":        path,
		"mode":        "read+watch",
		"incremental": true,
		"fromStart":   true,
		"debounceMs":  20,
	})
	withPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = fi.Stop() })

	time.Sleep(30 * time.Millisecond)

	// Write a partial line (no newline).
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("partial")
	_ = f.Close()

	col.waitNone(t, 200*time.Millisecond)

	// Complete it — should now emit the whole line.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("\n")
	_ = f.Close()

	msg := col.wait(t, 2*time.Second)
	if got := msg.Get("payload"); got != "partial\n" {
		t.Errorf("after newline: payload = %v, want 'partial\\n'", got)
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
