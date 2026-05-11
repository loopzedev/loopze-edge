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

func newFileWatch(t *testing.T, props map[string]any) (*FileWatchNode, *struct {
	color string
	text  string
}) {
	t.Helper()

	node, err := NewFileWatchNode(flow.NodeConfig{ID: "test-fw", Type: "file-watch", Properties: props})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	fw := node.(*FileWatchNode)
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	status := &struct {
		color string
		text  string
	}{}
	fw.SetStatus(func(color, text string) { status.color = color; status.text = text })
	return fw, status
}

func withWatchPers(fw *FileWatchNode) *memStore {
	store := newMemStore()
	fw.SetContext(nil, nil, nil, store)
	return store
}

func seedFolder(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ---------------- mode=read (folder listing) ----------------

func TestFileWatch_ReadMode_Individual(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "a", "b.txt": "bb", "c.txt": "ccc"})

	fw, _ := newFileWatch(t, map[string]any{"path": dir, "mode": "read"})
	out, err := fw.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out[0]) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(out[0]))
	}
	for i, msg := range out[0] {
		entry, ok := msg.Get("payload").(map[string]any)
		if !ok {
			t.Fatalf("msg %d payload type = %T", i, msg.Get("payload"))
		}
		if entry["isDir"] != false {
			t.Errorf("msg %d isDir = %v, want false", i, entry["isDir"])
		}
		if msg.Get("filename") != entry["path"] {
			t.Errorf("msg %d filename = %v, want %v", i, msg.Get("filename"), entry["path"])
		}
		if entry["content"] != nil {
			t.Errorf("msg %d should not include content (file-watch never reads), got %v", i, entry["content"])
		}
	}
	if out[0][0].Get("isFirst") != true {
		t.Errorf("first isFirst = %v, want true", out[0][0].Get("isFirst"))
	}
	if out[0][2].Get("isLast") != true {
		t.Errorf("last isLast = %v", out[0][2].Get("isLast"))
	}
}

func TestFileWatch_ReadMode_Array(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "1", "b.txt": "2"})

	fw, _ := newFileWatch(t, map[string]any{"path": dir, "mode": "read", "sendAs": "array"})
	out, _ := fw.HandleMessage(flow.NewMessage())
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out[0]))
	}
	arr, ok := out[0][0].Get("payload").([]any)
	if !ok {
		t.Fatalf("payload type = %T", out[0][0].Get("payload"))
	}
	if len(arr) != 2 {
		t.Errorf("array len = %d, want 2", len(arr))
	}
}

func TestFileWatch_ReadMode_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"data.csv": "x", "info.json": "y", "notes.csv": "z"})

	fw, _ := newFileWatch(t, map[string]any{"path": dir, "mode": "read", "glob": "*.csv"})
	out, _ := fw.HandleMessage(flow.NewMessage())
	if len(out[0]) != 2 {
		t.Errorf("expected 2 csv entries, got %d", len(out[0]))
	}
}

func TestFileWatch_ReadMode_Recursive(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"top.txt":           "1",
		"sub/nested.txt":    "2",
		"sub/deep/leaf.txt": "3",
	})

	fw, _ := newFileWatch(t, map[string]any{"path": dir, "mode": "read", "recursive": true})
	out, _ := fw.HandleMessage(flow.NewMessage())
	files := 0
	for _, msg := range out[0] {
		entry := msg.Get("payload").(map[string]any)
		if !entry["isDir"].(bool) {
			files++
		}
	}
	if files != 3 {
		t.Errorf("expected 3 files via recursion, got %d", files)
	}
}

func TestFileWatch_ReadMode_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"top.txt":        "1",
		"sub/nested.txt": "2",
	})

	fw, _ := newFileWatch(t, map[string]any{"path": dir, "mode": "read"})
	out, _ := fw.HandleMessage(flow.NewMessage())
	files := 0
	for _, msg := range out[0] {
		entry := msg.Get("payload").(map[string]any)
		if !entry["isDir"].(bool) {
			files++
		}
	}
	if files != 1 {
		t.Errorf("non-recursive: expected 1 file, got %d", files)
	}
}

func TestFileWatch_ReadMode_MsgPathOverride(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	seedFolder(t, dirA, map[string]string{"a.txt": "1"})
	seedFolder(t, dirB, map[string]string{"b.txt": "2", "c.txt": "3"})

	fw, _ := newFileWatch(t, map[string]any{"path": dirA, "mode": "read"})
	msg := flow.NewMessage()
	msg.Set("path", dirB)
	out, err := fw.HandleMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out[0]) != 2 {
		t.Errorf("expected 2 files from override path, got %d", len(out[0]))
	}
}

func TestFileWatch_ReadMode_NotFound(t *testing.T) {
	fw, status := newFileWatch(t, map[string]any{"path": "/does/not/exist", "mode": "read"})
	_, err := fw.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected error")
	}
	if status.color != "red" || status.text != "not found" {
		t.Errorf("status = (%s, %s)", status.color, status.text)
	}
}

// ---------------- Incremental (read mode) ----------------

func TestFileWatch_Incremental_Read_FromEOF(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"existing.txt": "x"})

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
	})
	withWatchPers(fw)

	out, _ := fw.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Errorf("first access from EOF should not emit, got %v", out)
	}
}

func TestFileWatch_Incremental_Read_FromStart(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "1", "b.txt": "2"})

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withWatchPers(fw)

	out, _ := fw.HandleMessage(flow.NewMessage())
	if len(out[0]) != 2 {
		t.Errorf("fromStart should emit 2 entries, got %d", len(out[0]))
	}
}

func TestFileWatch_Incremental_Read_DetectsChanges(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"existing.txt": "old"})

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
	})
	withWatchPers(fw)

	out, _ := fw.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Fatal("first call should be silent")
	}

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ = fw.HandleMessage(flow.NewMessage())
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 new entry, got %d", len(out[0]))
	}
	entry := out[0][0].Get("payload").(map[string]any)
	if entry["name"] != "new.txt" {
		t.Errorf("emitted entry = %v, want new.txt", entry["name"])
	}
}

func TestFileWatch_Incremental_Read_DetectsModification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withWatchPers(fw)

	first, _ := fw.HandleMessage(flow.NewMessage())
	if len(first[0]) != 1 {
		t.Fatal("expected initial emission")
	}

	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := fw.HandleMessage(flow.NewMessage())
	if len(out[0]) != 1 {
		t.Errorf("expected 1 changed entry, got %d", len(out[0]))
	}
}

func TestFileWatch_Incremental_Read_UnchangedSilent(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "x"})

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withWatchPers(fw)

	_, _ = fw.HandleMessage(flow.NewMessage())
	out, _ := fw.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Errorf("unchanged folder should be silent on second call, got %v", out)
	}
}

func TestFileWatch_Incremental_Read_ResetMsg(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "x"})

	fw, status := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	store := withWatchPers(fw)

	_, _ = fw.HandleMessage(flow.NewMessage())

	resetMsg := flow.NewMessage()
	resetMsg.Set("resetCursor", true)
	out, err := fw.HandleMessage(resetMsg)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("reset should not emit, got %v", out)
	}
	resolved, _ := fw.resolveFolderPath(flow.NewMessage())
	key := dirCursorKey(fw.cfg.ID, resolved)
	v, _ := store.Get(key)
	if v != nil {
		t.Errorf("cursor should be cleared, got %v", v)
	}
	if status.color != "blue" || !strings.Contains(status.text, "reset") {
		t.Errorf("status = (%s, %s)", status.color, status.text)
	}
}

// ---------------- Watch mode ----------------

func TestFileWatch_WatchMode_EmitsOnCreate(t *testing.T) {
	dir := t.TempDir()
	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"debounceMs":  20,
		"incremental": true, // exact change detection
	})
	withWatchPers(fw)
	col := newSendCollector()
	fw.SetSend(col.sendFn())
	if err := fw.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = fw.Stop() })

	time.Sleep(30 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "fresh.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)

	entry, _ := msg.Get("payload").(map[string]any)
	if entry == nil || entry["name"] != "fresh.txt" {
		t.Errorf("expected fresh.txt entry, got %v", entry)
	}
	if msg.Get("event") != "create" {
		t.Errorf("event = %v, want create", msg.Get("event"))
	}
	if msg.Get("changed") != true {
		t.Errorf("changed = %v, want true", msg.Get("changed"))
	}
	if msg.Get("filename") != entry["path"] {
		t.Errorf("filename should match entry path, got %v vs %v", msg.Get("filename"), entry["path"])
	}
}

func TestFileWatch_WatchMode_RemoveEvent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doomed.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fw, _ := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"incremental": true,
		"fromStart":   true,
		"debounceMs":  20,
	})
	store := newMemStore()
	fw.SetContext(nil, nil, nil, store)
	col := newSendCollector()
	fw.SetSend(col.sendFn())
	if err := fw.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Stop() })

	// Pre-load the cursor with the target so the remove dispatches as remove.
	resolved, _ := fw.resolveFolderPath(flow.NewMessage())
	info, _ := os.Stat(target)
	key := dirCursorKey(fw.cfg.ID, resolved)
	_ = store.Set(key, map[string]any{
		target: info.ModTime().UTC().Format(time.RFC3339Nano),
	})

	time.Sleep(30 * time.Millisecond)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)
	if msg.Get("event") != "remove" {
		t.Errorf("event = %v, want remove", msg.Get("event"))
	}
	entry, _ := msg.Get("payload").(map[string]any)
	if entry == nil || entry["path"] != target {
		t.Errorf("entry path = %v, want %v", entry, target)
	}
}

func TestFileWatch_ReadPlusWatch_OnlyChanged(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"old.txt": "x"})

	fw, _ := newFileWatch(t, map[string]any{
		"path":       dir,
		"mode":       "read+watch",
		"debounceMs": 20,
	})
	withWatchPers(fw)
	col := newSendCollector()
	fw.SetSend(col.sendFn())
	if err := fw.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Stop() })

	time.Sleep(30 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)
	entry, _ := msg.Get("payload").(map[string]any)
	if entry["name"] != "new.txt" {
		t.Errorf("expected new.txt, got %v", entry["name"])
	}
	if entry["content"] != nil {
		t.Errorf("file-watch must not include content, got %v", entry["content"])
	}

	// old.txt was in the baseline → should not re-emit
	col.waitNone(t, 200*time.Millisecond)
}

func TestFileWatch_WatchMode_Status(t *testing.T) {
	dir := t.TempDir()
	fw, status := newFileWatch(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"incremental": true,
	})
	withWatchPers(fw)
	col := newSendCollector()
	fw.SetSend(col.sendFn())
	if err := fw.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Stop() })

	if status.color != "green" {
		t.Errorf("status = %s, want green", status.color)
	}
	if !strings.Contains(status.text, "watching") {
		t.Errorf("status text = %q", status.text)
	}
}

func TestFileWatch_WatchMode_StartFailsWithoutPers(t *testing.T) {
	dir := t.TempDir()
	fw, _ := newFileWatch(t, map[string]any{
		"path": dir,
		"mode": "read+watch", // always-incremental
	})
	col := newSendCollector()
	fw.SetSend(col.sendFn())
	if err := fw.Start(); err == nil {
		_ = fw.Stop()
		t.Fatal("expected Start to fail without flowPers for read+watch")
	}
}

func TestFileWatch_InitInvalidMode(t *testing.T) {
	node, _ := NewFileWatchNode(flow.NodeConfig{
		ID: "x", Type: "file-watch",
		Properties: map[string]any{"mode": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestFileWatch_InitInvalidSendAs(t *testing.T) {
	node, _ := NewFileWatchNode(flow.NodeConfig{
		ID: "x", Type: "file-watch",
		Properties: map[string]any{"sendAs": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid sendAs")
	}
}

func TestFileWatch_NilMessage(t *testing.T) {
	fw, _ := newFileWatch(t, map[string]any{"path": "/tmp/x", "mode": "read"})
	out, err := fw.HandleMessage(nil)
	if err != nil {
		t.Errorf("nil message should be ignored, got %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got %v", out)
	}
}

func TestFileWatch_JailViolation(t *testing.T) {
	jail := t.TempDir()
	fw, _ := newFileWatch(t, map[string]any{
		"path":     "/etc",
		"mode":     "read",
		"rootJail": jail,
	})
	_, err := fw.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected jail violation")
	}
	if !strings.Contains(err.Error(), "escapes jail") {
		t.Errorf("error = %v", err)
	}
}
