// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes_test

import (
	"testing"

	"github.com/niceclouds/flint/internal/flow"
	"github.com/niceclouds/flint/internal/nodes"
)

// newFunctionNode is a test helper that creates, inits, and starts a FunctionNode.
func newFunctionNode(t *testing.T, code string, outputs int) (flow.NodeInstance, [][]* flow.Message) {
	t.Helper()

	cfg := flow.NodeConfig{
		ID:   "fn-test",
		Type: "function",
		Name: "Test Function",
		Properties: map[string]any{
			"func":    code,
			"outputs": float64(outputs),
		},
	}

	n, err := nodes.NewFunctionNode(cfg)
	if err != nil {
		t.Fatalf("NewFunctionNode: %v", err)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Capture messages sent via node.send().
	sent := make([][]*flow.Message, outputs)
	n.SetSend(func(port int, msg *flow.Message) {
		if port < outputs {
			sent[port] = append(sent[port], msg)
		}
	})
	n.SetStatus(func(fill, text string) {})
	n.SetDebug(func(flow.DebugMessage) {})

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	return n, sent
}

// inMsg creates a test message with a given payload.
func inMsg(payload any) *flow.Message {
	m := flow.NewMessage()
	m.SetPayload(payload)
	return m
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestFunctionNode_PassThrough(t *testing.T) {
	n, _ := newFunctionNode(t, `return msg;`, 1)

	msg := inMsg("hello")
	outputs, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs) == 0 || len(outputs[0]) == 0 {
		t.Fatal("expected message on port 0, got none")
	}
	got := outputs[0][0].Payload()
	if got != "hello" {
		t.Errorf("payload: want %q, got %v", "hello", got)
	}
}

func TestFunctionNode_ModifyPayload(t *testing.T) {
	n, _ := newFunctionNode(t, `msg.payload = "modified"; return msg;`, 1)

	outputs, err := n.HandleMessage(inMsg("original"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs) == 0 || len(outputs[0]) == 0 {
		t.Fatal("expected message on port 0, got none")
	}
	got := outputs[0][0].Payload()
	if got != "modified" {
		t.Errorf("payload: want %q, got %v", "modified", got)
	}
}

func TestFunctionNode_ReturnNull(t *testing.T) {
	n, _ := newFunctionNode(t, `return null;`, 1)

	outputs, err := n.HandleMessage(inMsg("x"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	for i, port := range outputs {
		if len(port) > 0 {
			t.Errorf("port %d: expected no messages, got %d", i, len(port))
		}
	}
}

func TestFunctionNode_ReturnNothing(t *testing.T) {
	n, _ := newFunctionNode(t, `/* no return */`, 1)

	outputs, err := n.HandleMessage(inMsg("x"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	// nil or empty outputs are both acceptable.
	for i, port := range outputs {
		if len(port) > 0 {
			t.Errorf("port %d: expected no messages, got %d", i, len(port))
		}
	}
}

func TestFunctionNode_MultiPort(t *testing.T) {
	n, _ := newFunctionNode(t, `return [msg, msg];`, 2)

	msg := inMsg("multi")
	outputs, err := n.HandleMessage(msg)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs) < 2 {
		t.Fatalf("expected 2 output ports, got %d", len(outputs))
	}
	if len(outputs[0]) == 0 {
		t.Error("port 0: expected message, got none")
	}
	if len(outputs[1]) == 0 {
		t.Error("port 1: expected message, got none")
	}
}

func TestFunctionNode_MultiPort_NullPort(t *testing.T) {
	n, _ := newFunctionNode(t, `return [msg, null];`, 2)

	outputs, err := n.HandleMessage(inMsg("x"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs[0]) == 0 {
		t.Error("port 0: expected message")
	}
	if len(outputs[1]) > 0 {
		t.Errorf("port 1: expected no message, got %d", len(outputs[1]))
	}
}

func TestFunctionNode_NodeSend(t *testing.T) {
	n, _ := newFunctionNode(t, `node.send(msg); return null;`, 1)

	outputs, err := n.HandleMessage(inMsg("via-send"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs) == 0 || len(outputs[0]) == 0 {
		t.Fatal("expected message on port 0 via node.send(), got none")
	}
	if outputs[0][0].Payload() != "via-send" {
		t.Errorf("payload: want %q, got %v", "via-send", outputs[0][0].Payload())
	}
}

func TestFunctionNode_PreservesMessageID(t *testing.T) {
	n, _ := newFunctionNode(t, `return msg;`, 1)

	original := inMsg("id-check")
	outputs, err := n.HandleMessage(original)
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(outputs) == 0 || len(outputs[0]) == 0 {
		t.Fatal("no output message")
	}
	if outputs[0][0].ID() != original.ID() {
		t.Errorf("message ID: want %q, got %q", original.ID(), outputs[0][0].ID())
	}
}

func TestFunctionNode_SyntaxError(t *testing.T) {
	cfg := flow.NodeConfig{
		ID:   "fn-err",
		Type: "function",
		Properties: map[string]any{
			"func":    `this is not valid javascript }{{{`,
			"outputs": float64(1),
		},
	}
	n, err := nodes.NewFunctionNode(cfg)
	if err != nil {
		t.Fatalf("NewFunctionNode: %v", err)
	}
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})

	if err := n.Start(); err == nil {
		t.Fatal("Start: expected compile error, got nil")
	}
}

func TestFunctionNode_RuntimeError(t *testing.T) {
	n, _ := newFunctionNode(t, `throw new Error("boom");`, 1)

	_, err := n.HandleMessage(inMsg("x"))
	if err == nil {
		t.Fatal("HandleMessage: expected runtime error, got nil")
	}
}

func TestFunctionNode_ConsoleLog(t *testing.T) {
	var captured []flow.DebugMessage
	cfg := flow.NodeConfig{
		ID:   "fn-console",
		Type: "function",
		Properties: map[string]any{
			"func":    `console.log("hello from console"); return msg;`,
			"outputs": float64(1),
		},
	}
	n, err := nodes.NewFunctionNode(cfg)
	if err != nil {
		t.Fatalf("NewFunctionNode: %v", err)
	}
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(msg flow.DebugMessage) { captured = append(captured, msg) })
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	if _, err := n.HandleMessage(inMsg("x")); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(captured) == 0 {
		t.Fatal("expected a debug message from console.log, got none")
	}
	if captured[0].Status != "debug" {
		t.Errorf("status: want %q, got %q", "debug", captured[0].Status)
	}
}
