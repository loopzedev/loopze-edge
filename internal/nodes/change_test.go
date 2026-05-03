// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// newChangeNode creates a ChangeNode with the given rules for testing.
func newChangeNode(t *testing.T, rules []map[string]any) *ChangeNode {
	t.Helper()

	config := flow.NodeConfig{
		ID:   "change-test",
		Type: "change",
		Properties: map[string]any{
			"rules": toAnySlice(rules),
		},
	}

	inst, err := NewChangeNode(config)
	if err != nil {
		t.Fatalf("NewChangeNode: %v", err)
	}

	n := inst.(*ChangeNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	n.SetSend(func(port int, msg *flow.Message) {})
	n.SetStatus(func(fill, text string) {})
	n.SetDebug(func(msg flow.DebugMessage) {})

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	return n
}

func toAnySlice(rules []map[string]any) []any {
	out := make([]any, len(rules))
	for i, r := range rules {
		out[i] = r
	}
	return out
}

func changeMsg(payload any) *flow.Message {
	msg := flow.NewMessage()
	msg.SetPayload(payload)
	return msg
}

// ── Set Operation ────────────────────────────────────────────────

func TestChangeNode_SetString(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "Hello World", "tot": "str"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != "Hello World" {
		t.Errorf("payload: want %q, got %v", "Hello World", got)
	}
}

func TestChangeNode_SetNumber(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "42.5", "tot": "num"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != 42.5 {
		t.Errorf("payload: want 42.5, got %v", got)
	}
}

func TestChangeNode_SetBoolean(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "true", "tot": "bool"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != true {
		t.Errorf("payload: want true, got %v", got)
	}
}

func TestChangeNode_SetJSON(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": `{"key":"value","num":42}`, "tot": "json"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got, ok := outputs[0][0].Payload().(map[string]any)
	if !ok {
		t.Fatalf("payload is not map, got %T", outputs[0][0].Payload())
	}
	if got["key"] != "value" {
		t.Errorf("key: want %q, got %v", "value", got["key"])
	}
}

func TestChangeNode_SetTimestamp(t *testing.T) {
	before := time.Now().UnixMilli()

	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "", "tot": "date"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got, ok := outputs[0][0].Payload().(float64)
	if !ok {
		t.Fatalf("payload is not float64, got %T", outputs[0][0].Payload())
	}

	after := time.Now().UnixMilli()
	if int64(got) < before || int64(got) > after {
		t.Errorf("timestamp %v not between %d and %d", got, before, after)
	}
}

func TestChangeNode_SetFromMsg(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "topic", "tot": "msg"},
	})

	msg := changeMsg("original")
	msg.SetTopic("my-topic")

	outputs, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != "my-topic" {
		t.Errorf("payload: want %q, got %v", "my-topic", got)
	}
}

func TestChangeNode_SetNested(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "data.nested.field", "pt": "msg", "to": "deep", "tot": "str"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Get("data.nested.field")
	if got != "deep" {
		t.Errorf("nested field: want %q, got %v", "deep", got)
	}
}

// ── Delete Operation ─────────────────────────────────────────────

func TestChangeNode_Delete(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "delete", "p": "topic", "pt": "msg"},
	})

	msg := changeMsg("hello")
	msg.SetTopic("to-delete")

	outputs, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Topic()
	if got != "" {
		t.Errorf("topic should be empty after delete, got %q", got)
	}
}

// ── Move Operation ───────────────────────────────────────────────

func TestChangeNode_Move(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "move", "p": "payload", "pt": "msg", "to": "data.original", "tot": "msg"},
	})

	outputs, err := n.HandleMessage(changeMsg("moved-value"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	out := outputs[0][0]

	// Source should be gone.
	if out.Payload() != nil {
		t.Errorf("source payload should be nil, got %v", out.Payload())
	}

	// Target should have the value.
	got := out.Get("data.original")
	if got != "moved-value" {
		t.Errorf("target: want %q, got %v", "moved-value", got)
	}
}

// ── Change (Search/Replace) Operation ────────────────────────────

func TestChangeNode_ChangeString(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "change", "p": "payload", "pt": "msg",
			"from": "foo", "fromt": "str",
			"to": "bar", "tot": "str"},
	})

	outputs, err := n.HandleMessage(changeMsg("foo is foo"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != "bar is bar" {
		t.Errorf("payload: want %q, got %v", "bar is bar", got)
	}
}

func TestChangeNode_ChangeRegex(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "change", "p": "payload", "pt": "msg",
			"from": `\d+`, "fromt": "re",
			"to": "NUM", "tot": "str"},
	})

	outputs, err := n.HandleMessage(changeMsg("item 42 and item 99"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != "item NUM and item NUM" {
		t.Errorf("payload: want %q, got %v", "item NUM and item NUM", got)
	}
}

// ── Multiple Rules ───────────────────────────────────────────────

func TestChangeNode_MultipleRules(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "step1", "tot": "str"},
		{"t": "set", "p": "step", "pt": "msg", "to": "2", "tot": "num"},
		{"t": "delete", "p": "topic", "pt": "msg"},
	})

	msg := changeMsg("original")
	msg.SetTopic("will-be-deleted")

	outputs, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	out := outputs[0][0]

	if out.Payload() != "step1" {
		t.Errorf("payload: want %q, got %v", "step1", out.Payload())
	}
	if out.Get("step") != float64(2) {
		t.Errorf("step: want 2, got %v", out.Get("step"))
	}
	if out.Topic() != "" {
		t.Errorf("topic should be deleted, got %q", out.Topic())
	}
}

// ── No Rules ─────────────────────────────────────────────────────

func TestChangeNode_NoRules(t *testing.T) {
	config := flow.NodeConfig{
		ID:         "change-empty",
		Type:       "change",
		Properties: map[string]any{},
	}

	inst, err := NewChangeNode(config)
	if err != nil {
		t.Fatalf("NewChangeNode: %v", err)
	}

	n := inst.(*ChangeNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	n.SetSend(func(port int, msg *flow.Message) {})

	outputs, err := n.HandleMessage(changeMsg("passthrough"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if outputs[0][0].Payload() != "passthrough" {
		t.Errorf("message should pass through unchanged")
	}
}

func TestChangeNode_EnvVar(t *testing.T) {
	t.Setenv("LOOPZE_TEST_VAR", "from-env")

	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "LOOPZE_TEST_VAR", "tot": "env"},
	})

	outputs, err := n.HandleMessage(changeMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	got := outputs[0][0].Payload()
	if got != "from-env" {
		t.Errorf("payload: want %q, got %v", "from-env", got)
	}
}

// ── Expr Value Type ──────────────────────────────────────────────

func TestChangeNode_SetExpr_Numeric(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "payload * 1.8 + 32", "tot": "expr"},
	})

	outputs, err := n.HandleMessage(changeMsg(20.0))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := outputs[0][0].Payload(); got != 68.0 {
		t.Errorf("payload: want 68, got %v", got)
	}
}

func TestChangeNode_SetExpr_Conditional(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "severity", "pt": "msg",
			"to": `payload.value > 50 ? "high" : "low"`, "tot": "expr"},
	})

	msg := changeMsg(map[string]any{"value": 75})
	outputs, _ := n.HandleMessage(msg)
	if got := outputs[0][0].Get("severity"); got != "high" {
		t.Errorf("severity: want high, got %v", got)
	}

	msg2 := changeMsg(map[string]any{"value": 30})
	outputs2, _ := n.HandleMessage(msg2)
	if got := outputs2[0][0].Get("severity"); got != "low" {
		t.Errorf("severity: want low, got %v", got)
	}
}

func TestChangeNode_SetExpr_TopicRewrite(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "topic", "pt": "msg",
			"to": `topic + "/converted"`, "tot": "expr"},
	})

	msg := changeMsg("payload")
	msg.SetTopic("sensor/raw")
	outputs, _ := n.HandleMessage(msg)
	if got := outputs[0][0].Topic(); got != "sensor/raw/converted" {
		t.Errorf("topic: want sensor/raw/converted, got %q", got)
	}
}

func TestChangeNode_SetExpr_FullMsgEscapeHatch(t *testing.T) {
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg",
			"to": `msg["customField"]`, "tot": "expr"},
	})

	msg := changeMsg("ignored")
	msg.Set("customField", "extracted")
	outputs, _ := n.HandleMessage(msg)
	if got := outputs[0][0].Payload(); got != "extracted" {
		t.Errorf("payload: want extracted, got %v", got)
	}
}

func TestChangeNode_SetExpr_CompileError_PinpointsRuleIndex(t *testing.T) {
	config := flow.NodeConfig{
		ID:   "change-expr-err",
		Type: "change",
		Properties: map[string]any{
			"rules": toAnySlice([]map[string]any{
				{"t": "set", "p": "payload", "pt": "msg", "to": "10", "tot": "num"},
				{"t": "set", "p": "payload", "pt": "msg", "to": "payload * * 2", "tot": "expr"}, // rule index 1
			}),
		},
	}
	inst, err := NewChangeNode(config)
	if err != nil {
		t.Fatalf("NewChangeNode: %v", err)
	}
	err = inst.(*ChangeNode).Init()
	if err == nil {
		t.Fatal("Init: expected compile error, got nil")
	}
	if !strings.Contains(err.Error(), "rule 1") {
		t.Errorf("error should pinpoint rule 1, got: %v", err)
	}
}

func TestChangeNode_SetExpr_RuntimeErrorIsLoggedNotFatal(t *testing.T) {
	// Runtime error in an expr rule logs and continues — matches existing
	// rule-error behaviour (see HandleMessage's slog.Warn path).
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg",
			"to": `payload + 1`, "tot": "expr"},
	})

	// String + int → runtime error; the rule swallows and continues.
	outputs, err := n.HandleMessage(changeMsg("not-a-number"))
	if err != nil {
		t.Fatalf("HandleMessage should not return error from a single bad rule: %v", err)
	}
	// Original payload survives because the bad rule didn't write anything.
	if got := outputs[0][0].Payload(); got != "not-a-number" {
		t.Errorf("payload should be unchanged after rule error, got %v", got)
	}
}

func TestChangeNode_SetExpr_EmptyExpressionYieldsNil(t *testing.T) {
	// An expr rule with an empty value-string is allowed (Init tolerance for
	// half-configured rules) and resolves to nil, mirroring resolveValue's
	// permissive contract.
	n := newChangeNode(t, []map[string]any{
		{"t": "set", "p": "payload", "pt": "msg", "to": "", "tot": "expr"},
	})

	outputs, _ := n.HandleMessage(changeMsg("anything"))
	if got := outputs[0][0].Payload(); got != nil {
		t.Errorf("payload: want nil, got %v", got)
	}
}
