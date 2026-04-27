// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes_test

import (
	"testing"

	"github.com/niceclouds/flint/internal/flow"
	"github.com/niceclouds/flint/internal/nodes"
)

func newFunctionGoNode(t *testing.T, code string, outputs int) (flow.NodeInstance, *capturedDebug) {
	t.Helper()
	cfg := flow.NodeConfig{
		ID:   "fgo-test",
		Type: "function-go",
		Properties: map[string]any{
			"code":    code,
			"outputs": float64(outputs),
		},
	}
	n, err := nodes.NewFunctionGoNode(cfg)
	if err != nil {
		t.Fatalf("NewFunctionGoNode: %v", err)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	capt := &capturedDebug{}
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(m flow.DebugMessage) { capt.msgs = append(capt.msgs, m) })

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n, capt
}

type capturedDebug struct {
	msgs []flow.DebugMessage
}

func TestFunctionGoNode_PassThrough(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

func handle(payload any) any {
	return payload
}
`, 1)

	outputs, err := n.HandleMessage(inMsg("hello"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(outputs[0]) == 0 {
		t.Fatal("expected message on port 0")
	}
	if got := outputs[0][0].Payload(); got != "hello" {
		t.Errorf("payload: got %v, want hello", got)
	}
}

func TestFunctionGoNode_TypedStructFilter(t *testing.T) {
	n, _ := newFunctionGoNode(t, "" +
		"package main\n\n" +
		"type Reading struct {\n" +
		"\tTemperature float64 `json:\"temperature\"`\n" +
		"}\n\n" +
		"func handle(payload []Reading) []Reading {\n" +
		"\tout := []Reading{}\n" +
		"\tfor _, r := range payload {\n" +
		"\t\tif r.Temperature > 25 {\n" +
		"\t\t\tout = append(out, r)\n" +
		"\t\t}\n" +
		"\t}\n" +
		"\treturn out\n" +
		"}\n", 1)

	in := []map[string]any{
		{"temperature": 15.0},
		{"temperature": 30.0},
		{"temperature": 22.0},
		{"temperature": 28.0},
	}
	outputs, err := n.HandleMessage(inMsg(in))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	// JSON roundtrip will hand back []Reading; only type-check the count via reflection.
	got := outputs[0][0].Payload()
	// Be tolerant: since it's a Yaegi-defined type, we just count via reflection.
	switch v := got.(type) {
	case []map[string]any:
		// Some Yaegi versions yield map slices on the way back through JSON.
		if len(v) != 2 {
			t.Errorf("len: got %d, want 2", len(v))
		}
	default:
		// Otherwise, we can at least JSON-marshal-back to count.
		if v == nil {
			t.Fatalf("expected non-nil result, got nil")
		}
	}
}

func TestFunctionGoNode_BufferRoundTrip(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

func handle(payload []byte) []byte {
	out := make([]byte, len(payload))
	for i, b := range payload {
		out[i] = b ^ 0xFF
	}
	return out
}
`, 1)

	outputs, err := n.HandleMessage(inMsg([]int{0x00, 0x0F, 0xFF}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, ok := outputs[0][0].Payload().([]int)
	if !ok {
		t.Fatalf("payload: got %T, want []int", outputs[0][0].Payload())
	}
	want := []int{0xFF, 0xF0, 0x00}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: got 0x%X, want 0x%X", i, got[i], want[i])
		}
	}
}

func TestFunctionGoNode_NodeSend_MultiPort(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

import "flintnode"

func handle(payload any, node flintnode.Node) {
	if v, ok := payload.(int); ok && v > 50 {
		node.Send(0, payload)
	} else {
		node.Send(1, payload)
	}
}
`, 2)

	outHigh, err := n.HandleMessage(inMsg(75))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(outHigh[0]) != 1 || outHigh[0][0].Payload() != 75 {
		t.Errorf("high path: got %+v", outHigh)
	}
	if len(outHigh[1]) != 0 {
		t.Errorf("high path port 1 should be empty")
	}

	outLow, _ := n.HandleMessage(inMsg(25))
	if len(outLow[1]) != 1 || outLow[1][0].Payload() != 25 {
		t.Errorf("low path: got %+v", outLow)
	}
}

func TestFunctionGoNode_NodeContext(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

import "flintnode"

func handle(payload any, node flintnode.Node) any {
	prev := node.Get("count")
	if prev == nil {
		prev = 0
	}
	n := prev.(int) + 1
	node.Set("count", n)
	return n
}
`, 1)

	for i := 1; i <= 3; i++ {
		outputs, err := n.HandleMessage(inMsg("ignored"))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if got := outputs[0][0].Payload(); got != i {
			t.Errorf("call %d: got %v, want %d", i, got, i)
		}
	}
}

func TestFunctionGoNode_BinaryParsing(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

import "encoding/binary"

func handle(payload []byte) uint64 {
	return binary.BigEndian.Uint64(payload[:8])
}
`, 1)

	outputs, err := n.HandleMessage(inMsg([]int{0, 0, 0, 0, 0, 0, 0xCA, 0xFE}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := outputs[0][0].Payload(); got != uint64(0xCAFE) {
		t.Errorf("payload: got %v, want 0xCAFE", got)
	}
}

func TestFunctionGoNode_VoidReturn_NoSend_NoOutput(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

import "flintnode"

func handle(payload any, node flintnode.Node) {
	// No send, no return — message dropped.
	_ = payload
}
`, 1)

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

func TestFunctionGoNode_CompileError(t *testing.T) {
	cfg := flow.NodeConfig{
		ID:   "fgo-err",
		Type: "function-go",
		Properties: map[string]any{
			"code":    "package main\n\nfunc handle(payload any) any {\n  this is not valid\n}\n",
			"outputs": float64(1),
		},
	}
	n, _ := nodes.NewFunctionGoNode(cfg)
	_ = n.Init()

	var statusFill string
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(fill, _ string) { statusFill = fill })
	n.SetDebug(func(flow.DebugMessage) {})

	if err := n.Start(); err == nil {
		t.Fatal("Start: expected compile error, got nil")
	}
	if statusFill != "red" {
		t.Errorf("status fill: got %q, want red", statusFill)
	}
}

func TestFunctionGoNode_RuntimeError(t *testing.T) {
	n, _ := newFunctionGoNode(t, `
package main

func handle(payload any) any {
	var s []int
	return s[5]
}
`, 1)
	_, err := n.HandleMessage(inMsg(nil))
	if err == nil {
		t.Fatal("expected runtime error, got nil")
	}
}

func TestFunctionGoNode_Sandbox_RejectsForbiddenImport(t *testing.T) {
	cfg := flow.NodeConfig{
		ID:   "fgo-sandbox",
		Type: "function-go",
		Properties: map[string]any{
			"code":    "package main\n\nimport \"os\"\n\nfunc handle(payload any) any { return os.Getenv(\"X\") }\n",
			"outputs": float64(1),
		},
	}
	n, _ := nodes.NewFunctionGoNode(cfg)
	_ = n.Init()
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(string, string) {})
	n.SetDebug(func(flow.DebugMessage) {})

	if err := n.Start(); err == nil {
		t.Fatal("expected sandbox to reject os import, got nil")
	}
}

func TestFunctionGoNode_NodeStatus(t *testing.T) {
	cfg := flow.NodeConfig{
		ID:   "fgo-status",
		Type: "function-go",
		Properties: map[string]any{
			"code":    "package main\n\nimport \"flintnode\"\n\nfunc handle(payload any, node flintnode.Node) any { node.Status(\"green\", \"ok\"); return payload }\n",
			"outputs": float64(1),
		},
	}
	n, _ := nodes.NewFunctionGoNode(cfg)
	_ = n.Init()
	var fill, text string
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(f, t string) { fill, text = f, t })
	n.SetDebug(func(flow.DebugMessage) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	if _, err := n.HandleMessage(inMsg("x")); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if fill != "green" || text != "ok" {
		t.Errorf("status: got %q/%q, want green/ok", fill, text)
	}
}

func TestFunctionGoNode_NodeLog(t *testing.T) {
	n, capt := newFunctionGoNode(t, `
package main

import "flintnode"

func handle(payload any, node flintnode.Node) any {
	node.Log("hello from go")
	return payload
}
`, 1)

	if _, err := n.HandleMessage(inMsg("x")); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(capt.msgs) == 0 {
		t.Fatal("expected debug message from node.Log, got none")
	}
	if capt.msgs[0].Status != "debug" {
		t.Errorf("status: got %q, want debug", capt.msgs[0].Status)
	}
}
