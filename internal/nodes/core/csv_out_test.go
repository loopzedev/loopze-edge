// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes/filesystem"
)

// newCSVOutNode boots a csv-out node with the given props and a temp file
// path. The path is returned so tests can read back what landed on disk.
func newCSVOutNode(t *testing.T, props map[string]any) (*CSVOutNode, string, *csvStores) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	full := map[string]any{"path": path}
	for k, v := range props {
		full[k] = v
	}

	cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out", Properties: full}
	inst, err := NewCSVOutNode(cfg)
	if err != nil {
		t.Fatalf("NewCSVOutNode: %v", err)
	}
	n := inst.(*CSVOutNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	stores := &csvStores{flowPers: newCSVJSONStore()}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(fill, text string) {
		stores.statusFill = fill
		stores.statusText = text
	})
	n.SetDebug(func(flow.DebugMessage) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n, path, stores
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// ─── Append mode: header lifecycle ─────────────────────────────────

func TestCSVOut_Append_FirstMessage_WritesHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "append", "columns": "a,b",
	})
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := readFile(t, path)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_Append_SecondMessage_NoHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "append", "columns": "a,b",
	})
	for i := 0; i < 2; i++ {
		_, err := n.HandleMessage(msgWith(map[string]any{
			"payload": []map[string]any{{"a": "1", "b": "2"}},
		}))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	got := readFile(t, path)
	want := "a,b\n1,2\n1,2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_Append_AcrossRestart_NoDuplicateHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")

	makeNode := func() *CSVOutNode {
		cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out",
			Properties: map[string]any{
				"path": path, "mode": "append", "columns": "a,b",
			}}
		inst, _ := NewCSVOutNode(cfg)
		n := inst.(*CSVOutNode)
		_ = n.Init()
		n.SetSend(func(int, *flow.Message) {})
		n.SetStatus(func(string, string) {})
		n.SetDebug(func(flow.DebugMessage) {})
		_ = n.Start()
		return n
	}

	// Node instance 1.
	n1 := makeNode()
	_, _ = n1.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	_ = n1.Stop()

	// Node instance 2 (simulated redeploy against the existing file).
	n2 := makeNode()
	_, _ = n2.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "3", "b": "4"}},
	}))
	_ = n2.Stop()

	got := readFile(t, path)
	want := "a,b\n1,2\n3,4\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_Append_EmptyFileExists_WritesHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "append", "columns": "a,b",
	})
	// Pre-create empty file.
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := readFile(t, path)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

// ─── Overwrite mode ─────────────────────────────────────────────────

func TestCSVOut_Overwrite_AlwaysWritesHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a,b",
	})
	for i := 0; i < 3; i++ {
		_, err := n.HandleMessage(msgWith(map[string]any{
			"payload": []map[string]any{{"a": "x", "b": "y"}},
		}))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	got := readFile(t, path)
	want := "a,b\nx,y\n"
	if got != want {
		t.Errorf("disk after 3 overwrites: want %q, got %q", want, got)
	}
}

// ─── Create mode ────────────────────────────────────────────────────

func TestCSVOut_Create_WritesHeaderOnNewFile(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "create", "columns": "a,b",
	})
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := readFile(t, path)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_Create_FailsIfExists(t *testing.T) {
	n, path, stores := newCSVOutNode(t, map[string]any{
		"mode": "create", "columns": "a,b",
	})
	if err := os.WriteFile(path, []byte("pre-existing\n"), 0o644); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	if err == nil {
		t.Fatal("expected error on create + existing file")
	}
	if !errors.Is(err, filesystem.ErrFileExists) {
		t.Errorf("expected ErrFileExists, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "file exists" {
		t.Errorf("status: got %q/%q", stores.statusFill, stores.statusText)
	}
}

// ─── Header policy ──────────────────────────────────────────────────

func TestCSVOut_HeaderFalse_NeverWritesHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a,b", "header": false,
	})
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": [][]any{{"1", "2"}, {"3", "4"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := readFile(t, path)
	want := "1,2\n3,4\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_ColumnsPinOrder(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "b,a",
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"a": "1", "b": "2"},
	}))
	got := readFile(t, path)
	want := "b,a\n2,1\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

// ─── Input shapes ───────────────────────────────────────────────────

func TestCSVOut_SingleObjectInput(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a,b",
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"a": "1", "b": "2"},
	}))
	got := readFile(t, path)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_ArrayArrayInput_NoHeader(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "header": false,
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": [][]any{{"1", "2"}, {"3", "4"}},
	}))
	got := readFile(t, path)
	want := "1,2\n3,4\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

// ─── Format options ─────────────────────────────────────────────────

func TestCSVOut_CRLFNewline(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a", "newline": "\r\n",
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1"}},
	}))
	got := readFile(t, path)
	if !strings.Contains(got, "\r\n") {
		t.Errorf("expected CRLF in output, got %q", got)
	}
}

func TestCSVOut_ForceQuote(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a,b", "forceQuote": true,
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "x", "b": "y"}},
	}))
	got := readFile(t, path)
	want := "\"a\",\"b\"\n\"x\",\"y\"\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

func TestCSVOut_CustomDelimiter(t *testing.T) {
	n, path, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a,b", "delimiter": ";",
	})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1", "b": "2"}},
	}))
	got := readFile(t, path)
	want := "a;b\n1;2\n"
	if got != want {
		t.Errorf("disk: want %q, got %q", want, got)
	}
}

// ─── Path resolution ────────────────────────────────────────────────

func TestCSVOut_PathMustacheTemplate(t *testing.T) {
	dir := t.TempDir()
	pathTmpl := filepath.Join(dir, "{{date}}.csv")
	cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out",
		Properties: map[string]any{
			"path": pathTmpl, "mode": "overwrite", "columns": "a",
		}}
	inst, _ := NewCSVOutNode(cfg)
	n := inst.(*CSVOutNode)
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})
	_ = n.Start()
	t.Cleanup(func() { _ = n.Stop() })

	_, err := n.HandleMessage(msgWith(map[string]any{
		"date":    "2026-05-19",
		"payload": []map[string]any{{"a": "1"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	resolved := filepath.Join(dir, "2026-05-19.csv")
	if _, err := os.Stat(resolved); err != nil {
		t.Fatalf("expected file at %s: %v", resolved, err)
	}
}

func TestCSVOut_MsgFilenameOverride(t *testing.T) {
	dir := t.TempDir()
	configured := filepath.Join(dir, "configured.csv")
	override := filepath.Join(dir, "override.csv")
	cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out",
		Properties: map[string]any{
			"path": configured, "mode": "overwrite", "columns": "a",
		}}
	inst, _ := NewCSVOutNode(cfg)
	n := inst.(*CSVOutNode)
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})
	_ = n.Start()
	t.Cleanup(func() { _ = n.Stop() })

	_, _ = n.HandleMessage(msgWith(map[string]any{
		"filename": override,
		"payload":  []map[string]any{{"a": "1"}},
	}))
	if _, err := os.Stat(override); err != nil {
		t.Fatalf("override path not written: %v", err)
	}
	if _, err := os.Stat(configured); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("configured path should not have been written, got err=%v", err)
	}
}

func TestCSVOut_CreateDirs_True(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "out.csv")
	cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out",
		Properties: map[string]any{
			"path": nested, "mode": "overwrite", "columns": "a",
			"createDirs": true,
		}}
	inst, _ := NewCSVOutNode(cfg)
	n := inst.(*CSVOutNode)
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})
	_ = n.Start()
	t.Cleanup(func() { _ = n.Stop() })

	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("nested file: %v", err)
	}
}

func TestCSVOut_CreateDirs_False_NotFound_Error(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "missing", "out.csv")
	cfg := flow.NodeConfig{ID: "csv-out-test", Type: "csv-out",
		Properties: map[string]any{
			"path": nested, "mode": "overwrite", "columns": "a",
			"createDirs": false,
		}}
	inst, _ := NewCSVOutNode(cfg)
	n := inst.(*CSVOutNode)
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})
	_ = n.Start()
	t.Cleanup(func() { _ = n.Stop() })

	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1"}},
	}))
	if err == nil {
		t.Fatal("expected error for missing parent dir without createDirs")
	}
}

// ─── Pass-through and metadata ──────────────────────────────────────

func TestCSVOut_Passthrough_PreservesMsg(t *testing.T) {
	n, _, _ := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a",
	})
	in := msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1"}},
		"topic":   "test-topic",
	})
	out, _ := n.HandleMessage(in)
	if out[0][0].Get("topic") != "test-topic" {
		t.Errorf("topic not preserved")
	}
	if out[0][0].Get("bytesWritten") == nil {
		t.Errorf("bytesWritten missing on output")
	}
	if out[0][0].Get("fileSize") == nil {
		t.Errorf("fileSize missing on output")
	}
	if out[0][0].Get("filename") == nil {
		t.Errorf("filename missing on output")
	}
}

func TestCSVOut_HeaderWrittenField(t *testing.T) {
	n, _, _ := newCSVOutNode(t, map[string]any{
		"mode": "append", "columns": "a",
	})
	out1, _ := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "1"}},
	}))
	if out1[0][0].Get("headerWritten") != true {
		t.Errorf("first call: headerWritten should be true, got %v", out1[0][0].Get("headerWritten"))
	}
	out2, _ := n.HandleMessage(msgWith(map[string]any{
		"payload": []map[string]any{{"a": "2"}},
	}))
	if out2[0][0].Get("headerWritten") != false {
		t.Errorf("second call: headerWritten should be false, got %v", out2[0][0].Get("headerWritten"))
	}
}

// ─── Error path ─────────────────────────────────────────────────────

func TestCSVOut_EncodeError_UnsupportedPayload(t *testing.T) {
	n, _, stores := newCSVOutNode(t, map[string]any{
		"mode": "overwrite", "columns": "a",
	})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if err == nil {
		t.Fatal("expected encode error for scalar payload")
	}
	if stores.statusFill != "red" {
		t.Errorf("status: want red, got %q", stores.statusFill)
	}
}
