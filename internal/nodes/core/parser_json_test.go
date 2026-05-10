// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

type jsonStores struct {
	statusFill string
	statusText string
}

func newJSONNode(t *testing.T, props map[string]any) (*JSONParserNode, *jsonStores) {
	t.Helper()

	cfg := flow.NodeConfig{
		ID:         "json-test",
		Type:       "json",
		Properties: props,
	}
	inst, err := NewJSONParserNode(cfg)
	if err != nil {
		t.Fatalf("NewJSONParserNode: %v", err)
	}
	n := inst.(*JSONParserNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	stores := &jsonStores{}
	n.SetSend(func(port int, msg *flow.Message) {})
	n.SetStatus(func(fill, text string) {
		stores.statusFill = fill
		stores.statusText = text
	})
	n.SetDebug(func(msg flow.DebugMessage) {})

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n, stores
}

// ── auto / parse / stringify branches ────────────────────────────

func TestJSON_AutoParseString(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": `{"v":1}`}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(map[string]any)
	if !ok {
		t.Fatalf("payload type: want map[string]any, got %T", out[0][0].Get("payload"))
	}
	if got["v"] != float64(1) {
		t.Errorf("payload.v: want 1, got %v", got["v"])
	}
}

func TestJSON_AutoParseBytes(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": []byte(`{"v":2}`)}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(map[string]any)
	if !ok {
		t.Fatalf("payload type: want map[string]any, got %T", out[0][0].Get("payload"))
	}
	if got["v"] != float64(2) {
		t.Errorf("payload.v: want 2, got %v", got["v"])
	}
}

func TestJSON_AutoStringifyObject(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	in := map[string]any{"v": float64(3)}
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(string)
	if !ok {
		t.Fatalf("payload type: want string, got %T", out[0][0].Get("payload"))
	}
	var roundtrip map[string]any
	if err := json.Unmarshal([]byte(got), &roundtrip); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if roundtrip["v"] != float64(3) {
		t.Errorf("roundtrip.v: want 3, got %v", roundtrip["v"])
	}
}

func TestJSON_AutoStringifyArray(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": []any{1, 2, 3}}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Get("payload"); got != "[1,2,3]" {
		t.Errorf("payload: want [1,2,3], got %v", got)
	}
}

func TestJSON_AutoStringifyNumber(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Get("payload"); got != "42" {
		t.Errorf("payload: want \"42\", got %v", got)
	}
}

// ── strict mode errors ──────────────────────────────────────────

func TestJSON_ParseForcesString(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "parse"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if err == nil {
		t.Fatal("expected error for parse on number, got nil")
	}
	if !errors.Is(err, errJSONTypeMismatch) {
		t.Errorf("expected errJSONTypeMismatch, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "json type error" {
		t.Errorf("status: want red/json type error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestJSON_StringifyForcesNonString(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "stringify"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": "hello"}))
	if err == nil {
		t.Fatal("expected error for stringify on string, got nil")
	}
	if !errors.Is(err, errJSONTypeMismatch) {
		t.Errorf("expected errJSONTypeMismatch, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "json type error" {
		t.Errorf("status: want red/json type error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

// ── pretty-print ────────────────────────────────────────────────

func TestJSON_StringifyPretty(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{
		"action": "stringify",
		"indent": 2,
	})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"a": float64(1)},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	if !strings.Contains(got, "\n  \"a\"") {
		t.Errorf("payload should contain pretty 2-space indent, got %q", got)
	}
}

func TestJSON_StringifyCompactDefault(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "stringify"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"a": float64(1)},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	if strings.Contains(got, "\n") {
		t.Errorf("payload should be compact, got %q", got)
	}
}

// ── parse error vs type error ───────────────────────────────────

func TestJSON_ParseInvalidJSON(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "parse"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": "not json {"}))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if errors.Is(err, errJSONTypeMismatch) {
		t.Errorf("expected non-type error, got type mismatch: %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "json parse error" {
		t.Errorf("status: want red/json parse error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

// ── property / defaults / clamping ──────────────────────────────

func TestJSON_CustomProperty(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{
		"action":   "parse",
		"property": "data.payload",
	})
	msg := flow.NewMessage()
	msg.Set("data.payload", `{"v":1}`)
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("data.payload").(map[string]any)
	if !ok {
		t.Fatalf("data.payload type: want map[string]any, got %T", out[0][0].Get("data.payload"))
	}
	if got["v"] != float64(1) {
		t.Errorf("data.payload.v: want 1, got %v", got["v"])
	}
}

func TestJSON_NilValueAuto(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "auto"})
	msg := flow.NewMessage()
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Get("payload"); got != "null" {
		t.Errorf("nil payload via auto: want \"null\", got %v", got)
	}
}

func TestJSON_DefaultActionIsAuto(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{})
	if n.action != "auto" {
		t.Errorf("default action: want auto, got %q", n.action)
	}
	if n.property != "payload" {
		t.Errorf("default property: want payload, got %q", n.property)
	}
}

func TestJSON_IndentClamp(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "stringify", "indent": 99})
	if n.indent != 8 {
		t.Errorf("indent should clamp to 8, got %d", n.indent)
	}
}

func TestJSON_StatusUnchangedOnSuccess(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "auto"})
	if _, err := n.HandleMessage(msgWith(map[string]any{"payload": `{"v":1}`})); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if stores.statusFill != "" || stores.statusText != "" {
		t.Errorf("status should remain empty on success, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestJSON_StatusClearsAfterRecovery(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "parse"})

	if _, err := n.HandleMessage(msgWith(map[string]any{"payload": "not json {"})); err == nil {
		t.Fatal("expected parse error on first message")
	}
	if stores.statusFill != "red" {
		t.Fatalf("status after error: want red, got %q", stores.statusFill)
	}

	if _, err := n.HandleMessage(msgWith(map[string]any{"payload": `{"v":1}`})); err != nil {
		t.Fatalf("HandleMessage on recovery: %v", err)
	}
	if stores.statusFill != "" || stores.statusText != "" {
		t.Errorf("status should be cleared after recovery, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestJSON_StatusNotTouchedOnRepeatedSuccess(t *testing.T) {
	n, stores := newJSONNode(t, map[string]any{"action": "auto"})
	calls := 0
	n.SetStatus(func(fill, text string) {
		calls++
		stores.statusFill = fill
		stores.statusText = text
	})

	for i := 0; i < 3; i++ {
		if _, err := n.HandleMessage(msgWith(map[string]any{"payload": `{"v":1}`})); err != nil {
			t.Fatalf("HandleMessage[%d]: %v", i, err)
		}
	}
	if calls != 0 {
		t.Errorf("status callback should not fire on uneventful success, got %d calls", calls)
	}
}

func TestJSON_UnknownActionFallsBackToAuto(t *testing.T) {
	n, _ := newJSONNode(t, map[string]any{"action": "wibble"})
	if n.action != "auto" {
		t.Errorf("unknown action should fall back to auto, got %q", n.action)
	}
}
