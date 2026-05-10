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

// newFolderIn builds an initialised FolderInNode for tests.
func newFolderIn(t *testing.T, props map[string]any) (*FolderInNode, *struct {
	color string
	text  string
}) {
	t.Helper()

	node, err := NewFolderInNode(flow.NodeConfig{ID: "test-fld", Type: "folder-in", Properties: props})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	fi := node.(*FolderInNode)
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

// withFolderPers attaches the in-memory store for incremental mode.
func withFolderPers(fi *FolderInNode) *memStore {
	store := newMemStore()
	fi.SetContext(nil, nil, nil, store)
	return store
}

// seedFolder creates a folder with the given files; values are file content.
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

func TestFolderIn_ReadMode_Individual(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"a.txt": "a",
		"b.txt": "bb",
		"c.txt": "ccc",
	})

	fi, _ := newFolderIn(t, map[string]any{"path": dir, "mode": "read"})
	out, err := fi.HandleMessage(flow.NewMessage())
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 port, got %d", len(out))
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
		if msg.Get("index") != i {
			t.Errorf("msg %d index = %v, want %d", i, msg.Get("index"), i)
		}
		if msg.Get("total") != 3 {
			t.Errorf("msg %d total = %v, want 3", i, msg.Get("total"))
		}
	}
	if out[0][0].Get("isFirst") != true {
		t.Errorf("first isFirst = %v, want true", out[0][0].Get("isFirst"))
	}
	if out[0][2].Get("isLast") != true {
		t.Errorf("last isLast = %v, want true", out[0][2].Get("isLast"))
	}
}

func TestFolderIn_ReadMode_Array(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "1", "b.txt": "2"})

	fi, _ := newFolderIn(t, map[string]any{"path": dir, "mode": "read", "sendAs": "array"})
	out, _ := fi.HandleMessage(flow.NewMessage())
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

func TestFolderIn_ReadMode_GlobFilter(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"data.csv":  "x",
		"info.json": "y",
		"notes.csv": "z",
	})

	fi, _ := newFolderIn(t, map[string]any{"path": dir, "mode": "read", "glob": "*.csv"})
	out, _ := fi.HandleMessage(flow.NewMessage())
	if len(out[0]) != 2 {
		t.Errorf("expected 2 csv entries, got %d", len(out[0]))
	}
	for _, msg := range out[0] {
		entry := msg.Get("payload").(map[string]any)
		if !strings.HasSuffix(entry["name"].(string), ".csv") {
			t.Errorf("entry %v should be .csv", entry["name"])
		}
	}
}

func TestFolderIn_ReadMode_Recursive(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"top.txt":            "1",
		"sub/nested.txt":     "2",
		"sub/deep/leaf.txt":  "3",
	})

	fi, _ := newFolderIn(t, map[string]any{"path": dir, "mode": "read", "recursive": true})
	out, _ := fi.HandleMessage(flow.NewMessage())
	// 3 files + 2 directories
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

func TestFolderIn_ReadMode_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{
		"top.txt":        "1",
		"sub/nested.txt": "2",
	})

	fi, _ := newFolderIn(t, map[string]any{"path": dir, "mode": "read"})
	out, _ := fi.HandleMessage(flow.NewMessage())
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

func TestFolderIn_ReadMode_IncludeContent(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"hello.txt": "world"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":           dir,
		"mode":           "read",
		"includeContent": true,
	})
	out, _ := fi.HandleMessage(flow.NewMessage())
	entry := out[0][0].Get("payload").(map[string]any)
	if entry["content"] != "world" {
		t.Errorf("content = %v, want 'world'", entry["content"])
	}
}

func TestFolderIn_ReadMode_ContentSkippedWhenTooLarge(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 1024)
	seedFolder(t, dir, map[string]string{"big.txt": big})

	fi, _ := newFolderIn(t, map[string]any{
		"path":             dir,
		"mode":             "read",
		"includeContent":   true,
		"maxFileSizeBytes": 256,
	})
	out, _ := fi.HandleMessage(flow.NewMessage())
	entry := out[0][0].Get("payload").(map[string]any)
	if entry["content"] != nil {
		t.Errorf("content should be absent for oversized file, got %v", entry["content"])
	}
	if entry["contentSkipped"] != true {
		t.Errorf("contentSkipped = %v, want true", entry["contentSkipped"])
	}
}

func TestFolderIn_ReadMode_MsgPathOverride(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	seedFolder(t, dirA, map[string]string{"a.txt": "1"})
	seedFolder(t, dirB, map[string]string{"b.txt": "2", "c.txt": "3"})

	fi, _ := newFolderIn(t, map[string]any{"path": dirA, "mode": "read"})
	msg := flow.NewMessage()
	msg.Set("path", dirB)
	out, err := fi.HandleMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out[0]) != 2 {
		t.Errorf("expected 2 files from override path, got %d", len(out[0]))
	}
}

func TestFolderIn_ReadMode_NotFound(t *testing.T) {
	fi, status := newFolderIn(t, map[string]any{"path": "/does/not/exist", "mode": "read"})
	_, err := fi.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected error")
	}
	if status.color != "red" || status.text != "not found" {
		t.Errorf("status = (%s, %s), want (red, not found)", status.color, status.text)
	}
}

func TestFolderIn_Incremental_Read_FromEOF(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"existing.txt": "x"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
	})
	withFolderPers(fi)

	out, _ := fi.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Errorf("first access from EOF should not emit, got %v", out)
	}
}

func TestFolderIn_Incremental_Read_FromStart(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "1", "b.txt": "2"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withFolderPers(fi)

	out, _ := fi.HandleMessage(flow.NewMessage())
	if len(out[0]) != 2 {
		t.Errorf("fromStart should emit 2 entries, got %d", len(out[0]))
	}
	entry := out[0][0].Get("payload").(map[string]any)
	if entry["isDir"] == nil { // sanity
		t.Errorf("entry shape unexpected")
	}
}

func TestFolderIn_Incremental_Read_DetectsChanges(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"existing.txt": "old"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
	})
	withFolderPers(fi)

	// First call records baseline, no emit
	out, _ := fi.HandleMessage(flow.NewMessage())
	if out != nil {
		t.Fatal("first call should be silent")
	}

	// New file
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ = fi.HandleMessage(flow.NewMessage())
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 new entry, got %d", len(out[0]))
	}
	entry := out[0][0].Get("payload").(map[string]any)
	if entry["name"] != "new.txt" {
		t.Errorf("emitted entry = %v, want new.txt", entry["name"])
	}
}

func TestFolderIn_Incremental_Read_DetectsModification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withFolderPers(fi)

	first, _ := fi.HandleMessage(flow.NewMessage())
	if len(first[0]) != 1 {
		t.Fatal("expected initial emission of existing file")
	}

	// Modify (sleep guarantees modTime advances on FAT/exFAT-style filesystems too)
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := fi.HandleMessage(flow.NewMessage())
	if len(out[0]) != 1 {
		t.Errorf("expected 1 changed entry, got %d", len(out[0]))
	}
}

func TestFolderIn_Incremental_Read_UnchangedSilent(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "x"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	withFolderPers(fi)

	_, _ = fi.HandleMessage(flow.NewMessage())     // baseline + emit
	out, _ := fi.HandleMessage(flow.NewMessage()) // no changes
	if out != nil {
		t.Errorf("unchanged folder should be silent on second call, got %v", out)
	}
}

func TestFolderIn_Incremental_Read_ResetMsg(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"a.txt": "x"})

	fi, status := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read",
		"incremental": true,
		"fromStart":   true,
	})
	store := withFolderPers(fi)

	_, _ = fi.HandleMessage(flow.NewMessage())

	resetMsg := flow.NewMessage()
	resetMsg.Set("resetCursor", true)
	out, err := fi.HandleMessage(resetMsg)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("reset should not emit, got %v", out)
	}
	resolved, _ := fi.resolveFolderPath(flow.NewMessage())
	key := dirCursorKey(fi.cfg.ID, resolved)
	v, _ := store.Get(key)
	if v != nil {
		t.Errorf("cursor should be cleared, got %v", v)
	}
	if status.color != "blue" || !strings.Contains(status.text, "reset") {
		t.Errorf("status = (%s, %s)", status.color, status.text)
	}
}

func TestFolderIn_WatchMode_EmitsOnCreate(t *testing.T) {
	dir := t.TempDir()
	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"debounceMs":  20,
		"incremental": true, // exact change detection
	})
	withFolderPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = fi.Stop() })

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
}

func TestFolderIn_WatchMode_RemoveEvent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doomed.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"incremental": true,
		"fromStart":   true, // include doomed.txt in baseline
		"debounceMs":  20,
	})
	withFolderPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fi.Stop() })

	// initDirCursor with fromStart=true seeds an empty map; the existing
	// file will be reported as "create" on the first event. To get a
	// remove event we have to first let the create event fire and update
	// the map, then delete.
	// Simpler: pre-load the cursor with the file in it.
	store := newMemStore()
	fi.SetContext(nil, nil, nil, store)

	// Already started — we'll just trigger the cursor pre-load via a
	// dummy event. But easier: use the in-memory store's set:
	resolved, _ := fi.resolveFolderPath(flow.NewMessage())
	info, _ := os.Stat(target)
	key := dirCursorKey(fi.cfg.ID, resolved)
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

func TestFolderIn_ReadPlusWatch_OnlyChanged(t *testing.T) {
	dir := t.TempDir()
	seedFolder(t, dir, map[string]string{"old.txt": "x"})

	fi, _ := newFolderIn(t, map[string]any{
		"path":           dir,
		"mode":           "read+watch",
		"includeContent": true,
		"debounceMs":     20,
	})
	withFolderPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fi.Stop() })

	time.Sleep(30 * time.Millisecond)

	// Add a brand-new file — should trigger one emission
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := col.wait(t, 2*time.Second)
	entry, _ := msg.Get("payload").(map[string]any)
	if entry["name"] != "new.txt" {
		t.Errorf("expected new.txt, got %v", entry["name"])
	}
	if entry["content"] != "hi" {
		t.Errorf("content = %v, want 'hi'", entry["content"])
	}

	// Touch the existing file — would it be emitted as a no-op? The first
	// scan after Start should have updated the baseline already. With
	// init pinning fromStart=false, old.txt was in the baseline from
	// initDirCursor, so it should NOT emit a create on it.
	col.waitNone(t, 200*time.Millisecond)
}

func TestFolderIn_WatchMode_Status(t *testing.T) {
	dir := t.TempDir()
	fi, status := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "watch",
		"incremental": true,
	})
	withFolderPers(fi)
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fi.Stop() })

	if status.color != "green" {
		t.Errorf("status = %s, want green", status.color)
	}
	if !strings.Contains(status.text, "watching") {
		t.Errorf("status text = %q", status.text)
	}
}

func TestFolderIn_WatchMode_StartFailsWithoutPers(t *testing.T) {
	dir := t.TempDir()
	fi, _ := newFolderIn(t, map[string]any{
		"path":        dir,
		"mode":        "read+watch", // always-incremental
	})
	col := newSendCollector()
	fi.SetSend(col.sendFn())
	if err := fi.Start(); err == nil {
		_ = fi.Stop()
		t.Fatal("expected Start to fail without flowPers for read+watch")
	}
}

func TestFolderIn_InitInvalidMode(t *testing.T) {
	node, _ := NewFolderInNode(flow.NodeConfig{
		ID: "x", Type: "folder-in",
		Properties: map[string]any{"mode": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestFolderIn_InitInvalidSendAs(t *testing.T) {
	node, _ := NewFolderInNode(flow.NodeConfig{
		ID: "x", Type: "folder-in",
		Properties: map[string]any{"sendAs": "wrong"},
	})
	if err := node.Init(); err == nil {
		t.Fatal("expected error for invalid sendAs")
	}
}

func TestFolderIn_NilMessage(t *testing.T) {
	fi, _ := newFolderIn(t, map[string]any{"path": "/tmp/x", "mode": "read"})
	out, err := fi.HandleMessage(nil)
	if err != nil {
		t.Errorf("nil message should be ignored, got %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got %v", out)
	}
}

func TestFolderIn_JailViolation(t *testing.T) {
	jail := t.TempDir()
	fi, _ := newFolderIn(t, map[string]any{
		"path":     "/etc",
		"mode":     "read",
		"rootJail": jail,
	})
	_, err := fi.HandleMessage(flow.NewMessage())
	if err == nil {
		t.Fatal("expected jail violation")
	}
	if !strings.Contains(err.Error(), "escapes jail") {
		t.Errorf("error = %v", err)
	}
}
