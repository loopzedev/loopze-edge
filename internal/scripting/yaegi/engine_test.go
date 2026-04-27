// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package yaegi_test

import (
	"strings"
	"testing"

	scriptingyaegi "github.com/niceclouds/flint/internal/scripting/yaegi"
)

// fakeNode captures Send/Status/Log calls for assertion.
type fakeNode struct {
	sends      []sentMsg
	logs       []string
	statusFill string
	statusText string
	nodeCtx    map[string]any
}

type sentMsg struct {
	Port int
	Msg  any
}

func newFakeNode() *fakeNode {
	return &fakeNode{nodeCtx: map[string]any{}}
}

func (f *fakeNode) Send(port int, msg any)              { f.sends = append(f.sends, sentMsg{port, msg}) }
func (f *fakeNode) Log(args ...any)                     { f.logs = append(f.logs, "log") }
func (f *fakeNode) Warn(args ...any)                    { f.logs = append(f.logs, "warn") }
func (f *fakeNode) Error(args ...any)                   { f.logs = append(f.logs, "error") }
func (f *fakeNode) Status(fill, text string)            { f.statusFill, f.statusText = fill, text }
func (f *fakeNode) Get(key string) any                  { return f.nodeCtx[key] }
func (f *fakeNode) Set(key string, val any)             { f.nodeCtx[key] = val }
func (f *fakeNode) Delete(key string)                   { delete(f.nodeCtx, key) }
func (f *fakeNode) FlowGet(key string) any              { return nil }
func (f *fakeNode) FlowSet(key string, val any)         {}
func (f *fakeNode) FlowDelete(key string)               {}
func (f *fakeNode) FlowGetP(key string) any             { return nil }
func (f *fakeNode) FlowSetP(key string, val any)        {}
func (f *fakeNode) FlowDeleteP(key string)              {}
func (f *fakeNode) GlobalGet(key string) any            { return nil }
func (f *fakeNode) GlobalSet(key string, val any)       {}
func (f *fakeNode) GlobalDelete(key string)             {}
func (f *fakeNode) GlobalGetP(key string) any           { return nil }
func (f *fakeNode) GlobalSetP(key string, val any)      {}
func (f *fakeNode) GlobalDeleteP(key string)            {}

func TestCompile_AnyPayload(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

func handle(payload any) any {
	return payload
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run("hello", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %v, want hello", got)
	}
}

func TestCompile_MapSlicePayload(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

func handle(payload []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range payload {
		if t, ok := r["temperature"].(float64); ok && t > 25 {
			out = append(out, r)
		}
	}
	return out
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	in := []map[string]any{
		{"temperature": 15.0},
		{"temperature": 30.0},
		{"temperature": 22.0},
		{"temperature": 28.0},
	}
	got, err := prog.Run(in, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, ok := got.([]map[string]any)
	if !ok {
		t.Fatalf("type: got %T, want []map[string]any", got)
	}
	if len(out) != 2 {
		t.Errorf("len: got %d, want 2", len(out))
	}
}

func TestCompile_MapSlicePayload_FromAnyArray(t *testing.T) {
	// Realistic input from JSON-decoded payload: []any of map[string]any.
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

func handle(payload []map[string]any) int {
	return len(payload)
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	in := []any{
		map[string]any{"x": 1},
		map[string]any{"x": 2},
	}
	got, err := prog.Run(in, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 2 {
		t.Errorf("got %v, want 2", got)
	}
}

func TestCompile_TypedStructSlice_JSONBoundary(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile("" +
		"package main\n\n" +
		"type Reading struct {\n" +
		"\tTemperature float64 `json:\"temperature\"`\n" +
		"\tHumidity    float64 `json:\"humidity\"`\n" +
		"}\n\n" +
		"func handle(payload []Reading) float64 {\n" +
		"\tsum := 0.0\n" +
		"\tfor _, r := range payload {\n" +
		"\t\tif r.Temperature > 20 {\n" +
		"\t\t\tsum += r.Temperature\n" +
		"\t\t}\n" +
		"\t}\n" +
		"\treturn sum\n" +
		"}\n")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	in := []map[string]any{
		{"temperature": 15.0, "humidity": 50.0},
		{"temperature": 25.0, "humidity": 60.0},
		{"temperature": 30.0, "humidity": 55.0},
	}
	got, err := prog.Run(in, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 55.0 {
		t.Errorf("got %v, want 55.0", got)
	}
}

func TestCompile_BufferIn_ByteOut(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

func handle(payload []byte) []byte {
	out := make([]byte, len(payload))
	for i, b := range payload {
		out[i] = b ^ 0xFF
	}
	return out
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// mqtt-in wire format is []int.
	got, err := prog.Run([]int{0x00, 0x0F, 0xFF}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Returned []byte should be auto-converted to []int wire format.
	out, ok := got.([]int)
	if !ok {
		t.Fatalf("type: got %T, want []int", got)
	}
	want := []int{0xFF, 0xF0, 0x00}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("byte[%d]: got 0x%X, want 0x%X", i, out[i], want[i])
		}
	}
}

func TestCompile_BufferIn_BinaryParsing(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

import "encoding/binary"

func handle(payload []byte) uint64 {
	return binary.BigEndian.Uint64(payload[:8])
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run([]int{0, 0, 0, 0, 0, 0, 0xCA, 0xFE}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != uint64(0xCAFE) {
		t.Errorf("got %v, want 0xCAFE", got)
	}
}

func TestCompile_NodeArgument_MultiSend(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

import "flintnode"

func handle(payload any, node flintnode.Node) {
	if v, ok := payload.(int); ok && v > 50 {
		node.Send(0, payload)
	} else {
		node.Send(1, payload)
	}
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	node := newFakeNode()
	if _, err := prog.Run(75, node); err != nil {
		t.Fatalf("Run high: %v", err)
	}
	if _, err := prog.Run(25, node); err != nil {
		t.Fatalf("Run low: %v", err)
	}
	if len(node.sends) != 2 {
		t.Fatalf("sends: got %d, want 2", len(node.sends))
	}
	if node.sends[0].Port != 0 || node.sends[0].Msg != 75 {
		t.Errorf("first send: %+v, want port=0 msg=75", node.sends[0])
	}
	if node.sends[1].Port != 1 || node.sends[1].Msg != 25 {
		t.Errorf("second send: %+v, want port=1 msg=25", node.sends[1])
	}
}

func TestCompile_NodeArgument_Status(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

import "flintnode"

func handle(payload any, node flintnode.Node) any {
	node.Status("green", "ok")
	return payload
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	node := newFakeNode()
	_, err = prog.Run("x", node)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if node.statusFill != "green" || node.statusText != "ok" {
		t.Errorf("status: got %q/%q, want green/ok", node.statusFill, node.statusText)
	}
}

func TestCompile_NodeArgument_NodeContext(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
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
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	node := newFakeNode()
	for i := 1; i <= 3; i++ {
		got, err := prog.Run("ignored", node)
		if err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
		if got != i {
			t.Errorf("call %d: got %v, want %d", i, got, i)
		}
	}
}

func TestCompile_VoidReturn_NoOutput(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

import "flintnode"

func handle(payload any, node flintnode.Node) {
	node.Send(0, payload)
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run("x", newFakeNode())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != nil {
		t.Errorf("void return should yield nil result, got %v", got)
	}
}

func TestCompile_Sandbox_RejectsForbiddenImport(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

import "os"

func handle(payload any) any {
	return os.Getenv("HOME")
}
`)
	if err == nil {
		t.Fatal("expected compile error for os import, got nil")
	}
}

func TestCompile_Sandbox_RejectsTimeSleep(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

import "time"

func handle(payload any) any {
	time.Sleep(time.Second)
	return payload
}
`)
	if err == nil {
		t.Fatal("expected compile error for time.Sleep, got nil")
	}
}

func TestCompile_AllowsTimeFormat(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

import "time"

func handle(payload any) any {
	return time.Now().Format(time.RFC3339)
}
`)
	if err != nil {
		t.Fatalf("expected time.Now/Format to be allowed, got: %v", err)
	}
}

func TestCompile_SyntaxError_ReturnsError(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

func handle(payload any) any {
	this is not valid go
}
`)
	if err == nil {
		t.Fatal("expected syntax error, got nil")
	}
}

func TestCompile_MissingHandle(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

func somethingElse() {}
`)
	if err == nil {
		t.Fatal("expected error for missing handle, got nil")
	}
	if !strings.Contains(err.Error(), "handle") {
		t.Errorf("error should mention handle, got: %v", err)
	}
}

func TestCompile_HandleWithWrongArgCount(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

func handle() any {
	return nil
}
`)
	if err == nil {
		t.Fatal("expected error for zero-arg handle, got nil")
	}
}

// Reproduces the user-reported crash: handle refers to an undeclared `node`
// (forgot the second parameter). Yaegi's internals raise log.Panic("nil
// reflect type") which would kill the process — we expect Compile to surface
// the panic as a regular error instead.
func TestCompile_UndefinedSymbolDoesNotCrashHost(t *testing.T) {
	engine := scriptingyaegi.New()
	_, err := engine.Compile(`
package main

import "flintnode"

var _ = flintnode.Node(nil)

func handle(payload any) any {
	node.FlowSet("key", "value")
	return payload
}
`)
	if err == nil {
		t.Fatal("expected error for undefined node symbol, got nil")
	}
}

// Belt-and-suspenders: a wider set of malformed inputs that historically
// triggered Yaegi-internal panics. None should kill the host; all should
// return errors.
func TestCompile_MalformedInputsAreRecovered(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{
			name: "undefined identifier in body",
			code: `package main
func handle(payload any) any { return undefinedSymbol }`,
		},
		{
			name: "method call on undefined receiver",
			code: `package main
func handle(payload any) any { foo.Bar(); return payload }`,
		},
		{
			name: "type assertion to undefined type",
			code: `package main
func handle(payload any) any { return payload.(NotAType) }`,
		},
	}
	engine := scriptingyaegi.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Compile(tc.code)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestRun_PanicRecovered(t *testing.T) {
	engine := scriptingyaegi.New()
	prog, err := engine.Compile(`
package main

func handle(payload any) any {
	var s []int
	return s[5]   // out of range
}
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = prog.Run(nil, nil)
	if err == nil {
		t.Fatal("expected panic to surface as error, got nil")
	}
}
