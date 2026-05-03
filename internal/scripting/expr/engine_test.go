// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package expr_test

import (
	"errors"
	"testing"

	scriptingexpr "github.com/niceclouds/loopze/internal/scripting/expr"
)

var defaultEnv = map[string]any{
	"payload": any(nil),
	"topic":   "",
	"msg":     map[string]any{},
}

func TestCompile_SimpleExpression(t *testing.T) {
	prog, err := scriptingexpr.Compile("payload * 2", defaultEnv)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run(map[string]any{"payload": 5, "topic": "", "msg": map[string]any{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 10 {
		t.Errorf("got %v, want 10", got)
	}
}

func TestCompile_PipelineWithMap(t *testing.T) {
	prog, err := scriptingexpr.Compile(`sum(map(payload, .x))`, map[string]any{
		"payload": []any{},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run(map[string]any{
		"payload": []any{
			map[string]any{"x": 1},
			map[string]any{"x": 2},
			map[string]any{"x": 3},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 6 {
		t.Errorf("got %v, want 6", got)
	}
}

func TestCompile_SyntaxError_ReturnsPosition(t *testing.T) {
	_, err := scriptingexpr.Compile("payload * * 2", defaultEnv)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	var ce *scriptingexpr.CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("error type: got %T, want *CompileError", err)
	}
	if ce.Line == 0 {
		t.Errorf("expected non-zero line, got %v", ce.Line)
	}
}

func TestRun_RuntimeError_DoesNotPanic(t *testing.T) {
	prog, err := scriptingexpr.Compile(`payload + 1`, defaultEnv)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Pass incompatible types — expr returns an error rather than panicking.
	_, err = prog.Run(map[string]any{"payload": "abc", "topic": "", "msg": map[string]any{}})
	if err == nil {
		t.Error("expected runtime error from string + 1, got nil")
	}
}

func TestRun_TopicAccess(t *testing.T) {
	prog, err := scriptingexpr.Compile(`topic + "/converted"`, defaultEnv)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run(map[string]any{
		"payload": nil,
		"topic":   "sensor/raw",
		"msg":     map[string]any{},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "sensor/raw/converted" {
		t.Errorf("got %v, want sensor/raw/converted", got)
	}
}

func TestRun_MsgEscapeHatch(t *testing.T) {
	prog, err := scriptingexpr.Compile(`msg["custom"]`, defaultEnv)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Run(map[string]any{
		"payload": nil,
		"topic":   "",
		"msg":     map[string]any{"custom": "value"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "value" {
		t.Errorf("got %v, want value", got)
	}
}
