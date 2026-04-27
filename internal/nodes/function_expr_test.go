// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes_test

import (
	"testing"

	"github.com/niceclouds/flint/internal/flow"
	"github.com/niceclouds/flint/internal/nodes"
)

func newFunctionExprNode(t *testing.T, expression string, outputProperty string, passThrough bool) flow.NodeInstance {
	t.Helper()
	cfg := flow.NodeConfig{
		ID:   "fexpr-test",
		Type: "function-expr",
		Properties: map[string]any{
			"expression":     expression,
			"outputProperty": outputProperty,
			"passThrough":    passThrough,
		},
	}
	n, err := nodes.NewFunctionExprNode(cfg)
	if err != nil {
		t.Fatalf("NewFunctionExprNode: %v", err)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n
}

func TestFunctionExprNode_SimpleArithmetic(t *testing.T) {
	n := newFunctionExprNode(t, "payload * 2", "payload", true)
	outputs, err := n.HandleMessage(inMsg(21))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(outputs[0]) == 0 {
		t.Fatal("expected message on port 0")
	}
	if got := outputs[0][0].Payload(); got != 42 {
		t.Errorf("payload: got %v, want 42", got)
	}
}

func TestFunctionExprNode_PassThroughPreservesFields(t *testing.T) {
	n := newFunctionExprNode(t, "payload + 1", "payload", true)
	msg := inMsg(10)
	msg.Set("custom", "keep-me")
	originalID := msg.ID()

	outputs, _ := n.HandleMessage(msg)
	out := outputs[0][0]

	if out.ID() != originalID {
		t.Errorf("passThrough should preserve ID")
	}
	if out.Get("custom") != "keep-me" {
		t.Errorf("custom field: got %v, want keep-me", out.Get("custom"))
	}
	if out.Payload() != 11 {
		t.Errorf("payload: got %v, want 11", out.Payload())
	}
}

func TestFunctionExprNode_NoPassThroughDropsFields(t *testing.T) {
	n := newFunctionExprNode(t, "payload * 2", "payload", false)
	msg := inMsg(5)
	msg.SetTopic("sensor/x")
	msg.Set("custom", "drop-me")

	outputs, _ := n.HandleMessage(msg)
	out := outputs[0][0]

	if out.ID() == msg.ID() {
		t.Errorf("no-passThrough should produce a new ID")
	}
	if out.Topic() != "sensor/x" {
		t.Errorf("topic should carry over, got %v", out.Topic())
	}
	if got := out.Get("custom"); got != nil {
		t.Errorf("custom field should be dropped, got %v", got)
	}
	if out.Payload() != 10 {
		t.Errorf("payload: got %v, want 10", out.Payload())
	}
}

func TestFunctionExprNode_PipelineAggregate(t *testing.T) {
	n := newFunctionExprNode(t, `sum(map(filter(payload, .temperature > 20), .temperature))`, "payload", true)

	records := []any{
		map[string]any{"temperature": 15.0},
		map[string]any{"temperature": 25.0},
		map[string]any{"temperature": 30.0},
	}
	outputs, err := n.HandleMessage(inMsg(records))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := outputs[0][0].Payload(); got != 55.0 {
		t.Errorf("payload: got %v, want 55.0", got)
	}
}

func TestFunctionExprNode_ObjectConstruction(t *testing.T) {
	n := newFunctionExprNode(t, `{avg: 10, max: 20}`, "payload", false)
	outputs, _ := n.HandleMessage(inMsg(nil))
	got, ok := outputs[0][0].Payload().(map[string]any)
	if !ok {
		t.Fatalf("payload: got %T, want map", outputs[0][0].Payload())
	}
	if got["avg"] != 10 || got["max"] != 20 {
		t.Errorf("payload: got %v, want {avg:10 max:20}", got)
	}
}

func TestFunctionExprNode_TopicInExpression(t *testing.T) {
	n := newFunctionExprNode(t, `topic + "/converted"`, "topic", true)
	msg := inMsg("any")
	msg.SetTopic("sensor/raw")

	outputs, _ := n.HandleMessage(msg)
	if got := outputs[0][0].Topic(); got != "sensor/raw/converted" {
		t.Errorf("topic: got %v, want sensor/raw/converted", got)
	}
}

func TestFunctionExprNode_OutputPropertyDotPath(t *testing.T) {
	n := newFunctionExprNode(t, `42`, "payload.value", false)
	outputs, _ := n.HandleMessage(inMsg(nil))
	nested, _ := outputs[0][0].Get("payload").(map[string]any)
	if nested == nil || nested["value"] != 42 {
		t.Errorf("payload.value: got %v, want 42 (nested)", outputs[0][0].Get("payload"))
	}
}

func TestFunctionExprNode_CompileError(t *testing.T) {
	cfg := flow.NodeConfig{
		ID:   "fexpr-err",
		Type: "function-expr",
		Properties: map[string]any{
			"expression": "payload * * 2",
		},
	}
	n, _ := nodes.NewFunctionExprNode(cfg)
	_ = n.Init()

	var statusFill, statusText string
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(fill, text string) { statusFill, statusText = fill, text })
	n.SetDebug(func(flow.DebugMessage) {})

	if err := n.Start(); err == nil {
		t.Fatal("Start: expected compile error, got nil")
	}
	if statusFill != "red" {
		t.Errorf("status fill: got %q, want red", statusFill)
	}
	if statusText == "" {
		t.Error("expected non-empty status text")
	}
}

func TestFunctionExprNode_RuntimeError(t *testing.T) {
	n := newFunctionExprNode(t, `payload + 1`, "payload", true)
	// String + int → runtime error.
	_, err := n.HandleMessage(inMsg("abc"))
	if err == nil {
		t.Fatal("expected runtime error, got nil")
	}
}

func TestFunctionExprNode_EmptyExpressionPassThrough(t *testing.T) {
	n := newFunctionExprNode(t, "", "payload", false)
	msg := inMsg("untouched")
	outputs, _ := n.HandleMessage(msg)
	if outputs[0][0] != msg {
		t.Error("empty expression should pass message through unchanged")
	}
}

func TestFunctionExprNode_ConditionalRouting(t *testing.T) {
	n := newFunctionExprNode(t, `payload.value > 50 ? "high" : "low"`, "severity", true)
	outputs, _ := n.HandleMessage(inMsg(map[string]any{"value": 75}))
	if got := outputs[0][0].Get("severity"); got != "high" {
		t.Errorf("severity: got %v, want high", got)
	}
}
