// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func newXMLNode(t *testing.T, props map[string]any) (*XMLParserNode, *jsonStores) {
	t.Helper()

	cfg := flow.NodeConfig{
		ID:         "xml-test",
		Type:       "xml",
		Properties: props,
	}
	inst, err := NewXMLParserNode(cfg)
	if err != nil {
		t.Fatalf("NewXMLParserNode: %v", err)
	}
	n := inst.(*XMLParserNode)
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

func TestXML_AutoParseString(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": `<sensor><value>25.4</value></sensor>`,
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(map[string]any)
	if !ok {
		t.Fatalf("payload type: want map[string]any, got %T", out[0][0].Get("payload"))
	}
	sensor, ok := got["sensor"].(map[string]any)
	if !ok {
		t.Fatalf("sensor type: want map[string]any, got %T", got["sensor"])
	}
	if sensor["value"] != "25.4" {
		t.Errorf("sensor.value: want \"25.4\", got %v", sensor["value"])
	}
}

func TestXML_AutoParseBytes(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "auto"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": []byte(`<status ok="true"/>`),
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(map[string]any)
	if !ok {
		t.Fatalf("payload type: want map[string]any, got %T", out[0][0].Get("payload"))
	}
	status, ok := got["status"].(map[string]any)
	if !ok {
		t.Fatalf("status type: want map[string]any, got %T", got["status"])
	}
	if status["-ok"] != "true" {
		t.Errorf("status.-ok: want \"true\", got %v", status["-ok"])
	}
}

func TestXML_AutoStringifyMap(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "auto", "root": "data"})
	in := map[string]any{"data": map[string]any{"name": "Alice"}}
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("payload").(string)
	if !ok {
		t.Fatalf("payload type: want string, got %T", out[0][0].Get("payload"))
	}
	if !strings.Contains(got, "Alice") {
		t.Errorf("payload should contain Alice, got %q", got)
	}
}

// ── attributes ───────────────────────────────────────────────────

func TestXML_ParseAttributes(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "parse"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": `<sensor id="42" unit="C"><value>25.4</value></sensor>`,
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := out[0][0].Get("payload").(map[string]any)
	sensor := got["sensor"].(map[string]any)
	if sensor["-id"] != "42" {
		t.Errorf("sensor.-id: want \"42\", got %v", sensor["-id"])
	}
	if sensor["-unit"] != "C" {
		t.Errorf("sensor.-unit: want \"C\", got %v", sensor["-unit"])
	}
}

// ── repeated elements ─────────────────────────────────────────────

func TestXML_ParseRepeatedElements(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "parse"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": `<list><item>a</item><item>b</item></list>`,
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := out[0][0].Get("payload").(map[string]any)
	list := got["list"].(map[string]any)
	items, ok := list["item"].([]any)
	if !ok {
		t.Fatalf("list.item type: want []any, got %T", list["item"])
	}
	if len(items) != 2 {
		t.Errorf("list.item length: want 2, got %d", len(items))
	}
}

// ── strict mode errors ──────────────────────────────────────────

func TestXML_ParseForcesString(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "parse"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if err == nil {
		t.Fatal("expected error for parse on number, got nil")
	}
	if !errors.Is(err, errXMLTypeMismatch) {
		t.Errorf("expected errXMLTypeMismatch, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "xml type error" {
		t.Errorf("status: want red/xml type error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestXML_StringifyForcesNonString(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "stringify", "root": "r"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": "already xml"}))
	if err == nil {
		t.Fatal("expected error for stringify on string, got nil")
	}
	if !errors.Is(err, errXMLTypeMismatch) {
		t.Errorf("expected errXMLTypeMismatch, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "xml type error" {
		t.Errorf("status: want red/xml type error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestXML_StringifyMissingRoot(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "stringify", "root": ""})
	_, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"key": "val"},
	}))
	if err == nil {
		t.Fatal("expected error for missing root, got nil")
	}
	if !errors.Is(err, errXMLRootMissing) {
		t.Errorf("expected errXMLRootMissing, got %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "xml root required" {
		t.Errorf("status: want red/xml root required, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

// ── parse error ──────────────────────────────────────────────────

func TestXML_ParseInvalidXML(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "parse"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": "not xml <"}))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if errors.Is(err, errXMLTypeMismatch) {
		t.Errorf("expected non-type error, got type mismatch: %v", err)
	}
	if stores.statusFill != "red" || stores.statusText != "xml parse error" {
		t.Errorf("status: want red/xml parse error, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

// ── pretty-print ────────────────────────────────────────────────

func TestXML_StringifyPretty(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{
		"action": "stringify",
		"root":   "data",
		"indent": 2,
	})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"data": map[string]any{"name": "Alice"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	if !strings.Contains(got, "\n") {
		t.Errorf("payload should contain newlines for pretty-print, got %q", got)
	}
}

func TestXML_StringifyCompactDefault(t *testing.T) {
	// indent=0 means the XML body itself is compact (no element indentation),
	// but the declaration line still ends with a newline separator.
	n, _ := newXMLNode(t, map[string]any{"action": "stringify", "root": "data"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"data": map[string]any{"name": "Bob"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	// Strip the declaration line and verify the remainder has no extra indentation.
	body := got
	if idx := strings.Index(got, "\n"); idx >= 0 {
		body = got[idx+1:]
	}
	if strings.Contains(body, "\n") {
		t.Errorf("xml body should be compact (no element indentation), got %q", got)
	}
}

// ── XML declaration ──────────────────────────────────────────────

func TestXML_StringifyDeclaration(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{
		"action":      "stringify",
		"root":        "data",
		"declaration": true,
	})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"data": map[string]any{"v": "1"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("payload should start with XML declaration, got %q", got)
	}
}

func TestXML_StringifyDeclarationByDefault(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "stringify", "root": "data"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload": map[string]any{"data": map[string]any{"v": "1"}},
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	if !strings.HasPrefix(got, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("payload should start with XML declaration by default, got %q", got)
	}
}

// ── special characters / roundtrip ───────────────────────────────

func TestXML_RoundtripAmpersand(t *testing.T) {
	// Values containing & must survive a stringify → parse roundtrip.
	// mxj must escape & to &amp; on stringify, otherwise parse fails.
	nStringify, _ := newXMLNode(t, map[string]any{"action": "stringify", "root": "root"})
	nParse, _ := newXMLNode(t, map[string]any{"action": "parse"})

	// Stringify: {university: "William & Mary"} → <root><university>William &amp; Mary</university></root>
	in := map[string]any{"university": "William & Mary"}
	out, err := nStringify.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("stringify: %v", err)
	}
	xmlStr, ok := out[0][0].Get("payload").(string)
	if !ok {
		t.Fatalf("stringify result type: want string, got %T", out[0][0].Get("payload"))
	}
	if !strings.Contains(xmlStr, "&amp;") {
		t.Errorf("stringify should escape & to &amp;, got %q", xmlStr)
	}

	// Parse back: <root><university>William &amp; Mary</university></root> → {root: {university: "William & Mary"}}
	out2, err := nParse.HandleMessage(msgWith(map[string]any{"payload": xmlStr}))
	if err != nil {
		t.Fatalf("parse after roundtrip: %v", err)
	}
	got := out2[0][0].Get("payload").(map[string]any)
	rootMap := got["root"].(map[string]any)
	if rootMap["university"] != "William & Mary" {
		t.Errorf("roundtrip university: want \"William & Mary\", got %v", rootMap["university"])
	}
}

// ── defaults / config ────────────────────────────────────────────

func TestXML_DefaultActionIsAuto(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{})
	if n.action != "auto" {
		t.Errorf("default action: want auto, got %q", n.action)
	}
	if n.property != "payload" {
		t.Errorf("default property: want payload, got %q", n.property)
	}
	if n.root != "root" {
		t.Errorf("default root: want root, got %q", n.root)
	}
}

func TestXML_IndentClamp(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "stringify", "indent": 99})
	if n.indent != 8 {
		t.Errorf("indent should clamp to 8, got %d", n.indent)
	}
}

func TestXML_UnknownActionFallsBackToAuto(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{"action": "wibble"})
	if n.action != "auto" {
		t.Errorf("unknown action should fall back to auto, got %q", n.action)
	}
}

// ── custom property ──────────────────────────────────────────────

func TestXML_CustomProperty(t *testing.T) {
	n, _ := newXMLNode(t, map[string]any{
		"action":   "parse",
		"property": "data.xml",
	})
	msg := flow.NewMessage()
	msg.Set("data.xml", `<x><v>1</v></x>`)
	out, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := out[0][0].Get("data.xml").(map[string]any)
	if !ok {
		t.Fatalf("data.xml type: want map[string]any, got %T", out[0][0].Get("data.xml"))
	}
	x := got["x"].(map[string]any)
	if x["v"] != "1" {
		t.Errorf("x.v: want \"1\", got %v", x["v"])
	}
}

// ── status ───────────────────────────────────────────────────────

func TestXML_StatusUnchangedOnSuccess(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "auto"})
	if _, err := n.HandleMessage(msgWith(map[string]any{
		"payload": `<a><b>1</b></a>`,
	})); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if stores.statusFill != "" || stores.statusText != "" {
		t.Errorf("status should remain empty on success, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}

func TestXML_StatusClearsAfterRecovery(t *testing.T) {
	n, stores := newXMLNode(t, map[string]any{"action": "parse"})

	if _, err := n.HandleMessage(msgWith(map[string]any{"payload": "not xml <"})); err == nil {
		t.Fatal("expected parse error on first message")
	}
	if stores.statusFill != "red" {
		t.Fatalf("status after error: want red, got %q", stores.statusFill)
	}

	if _, err := n.HandleMessage(msgWith(map[string]any{
		"payload": `<a><b>1</b></a>`,
	})); err != nil {
		t.Fatalf("HandleMessage on recovery: %v", err)
	}
	if stores.statusFill != "" || stores.statusText != "" {
		t.Errorf("status should be cleared after recovery, got %q/%q",
			stores.statusFill, stores.statusText)
	}
}
