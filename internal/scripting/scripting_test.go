// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package scripting_test

import (
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/scripting"
)

func TestMessageEnv(t *testing.T) {
	msg := flow.NewMessage()
	msg.SetPayload(42)
	msg.SetTopic("sensor/temp")
	msg.Set("custom", "value")

	env := scripting.MessageEnv(msg)

	if env["payload"] != 42 {
		t.Errorf("payload: got %v, want 42", env["payload"])
	}
	if env["topic"] != "sensor/temp" {
		t.Errorf("topic: got %v, want sensor/temp", env["topic"])
	}
	full, ok := env["msg"].(map[string]any)
	if !ok {
		t.Fatalf("msg: got %T, want map[string]any", env["msg"])
	}
	if full["custom"] != "value" {
		t.Errorf("msg.custom: got %v, want value", full["custom"])
	}
}

func TestApplyResult_PassThrough(t *testing.T) {
	msg := flow.NewMessage()
	msg.SetPayload("original")
	msg.SetTopic("t")
	msg.Set("keep", "me")
	originalID := msg.ID()

	out := scripting.ApplyResult(msg, "modified", "payload", true)

	if out.ID() != originalID {
		t.Errorf("passThrough should preserve ID: got %s, want %s", out.ID(), originalID)
	}
	if out.Payload() != "modified" {
		t.Errorf("payload: got %v, want modified", out.Payload())
	}
	if out.Get("keep") != "me" {
		t.Errorf("keep: got %v, want me (other fields should survive)", out.Get("keep"))
	}
}

func TestApplyResult_NoPassThrough(t *testing.T) {
	msg := flow.NewMessage()
	msg.SetPayload("original")
	msg.SetTopic("t")
	msg.Set("custom", "x")

	out := scripting.ApplyResult(msg, 99, "payload", false)

	if out.ID() == msg.ID() {
		t.Error("no-passThrough should produce a new ID")
	}
	if out.Payload() != 99 {
		t.Errorf("payload: got %v, want 99", out.Payload())
	}
	if out.Topic() != "t" {
		t.Errorf("topic: got %v, want t (topic should carry over)", out.Topic())
	}
	if got := out.Get("custom"); got != nil {
		t.Errorf("custom field should be dropped, got %v", got)
	}
}

func TestApplyResult_DotPath(t *testing.T) {
	msg := flow.NewMessage()
	out := scripting.ApplyResult(msg, 42, "payload.value", false)
	nested, _ := out.Get("payload").(map[string]any)
	if nested == nil {
		t.Fatalf("expected nested map, got %T", out.Get("payload"))
	}
	if nested["value"] != 42 {
		t.Errorf("payload.value: got %v, want 42", nested["value"])
	}
}

func TestApplyResult_EmptyOutputProperty(t *testing.T) {
	msg := flow.NewMessage()
	out := scripting.ApplyResult(msg, "x", "", true)
	if out.Payload() != "x" {
		t.Errorf("default to payload: got %v", out.Payload())
	}
}

func TestIntsToBuffer(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []byte
	}{
		{"[]byte passthrough", []byte{0xCA, 0xFE}, []byte{0xCA, 0xFE}},
		{"[]int", []int{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0x03}},
		{"[]int64", []int64{255, 0, 128}, []byte{0xFF, 0x00, 0x80}},
		{"[]any with float64 (JSON shape)", []any{float64(10), float64(20)}, []byte{10, 20}},
		{"[]any with int64", []any{int64(1), int64(2)}, []byte{1, 2}},
		{"unrelated type returns nil", "string", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scripting.IntsToBuffer(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("byte[%d]: got %d, want %d", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestBufferToInts(t *testing.T) {
	got := scripting.BufferToInts([]byte{0xCA, 0xFE, 0xBA, 0xBE})
	want := []int{0xCA, 0xFE, 0xBA, 0xBE}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestIsBufferShape(t *testing.T) {
	tests := []struct {
		in   any
		want bool
	}{
		{[]byte{1}, true},
		{[]int{1}, true},
		{[]int64{1}, true},
		{[]any{float64(1)}, true},
		{[]any{int64(1)}, true},
		{[]any{}, false},
		{"string", false},
		{42, false},
		{map[string]any{}, false},
	}
	for _, tc := range tests {
		if got := scripting.IsBufferShape(tc.in); got != tc.want {
			t.Errorf("IsBufferShape(%T %v): got %v, want %v", tc.in, tc.in, got, tc.want)
		}
	}
}
