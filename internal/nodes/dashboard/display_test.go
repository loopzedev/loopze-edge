// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// newDisplayForTest constructs a display-widget node, injects a stub
// hub, and returns both so the caller can deliver a message and assert
// the resulting push.
type factoryFn func(flow.NodeConfig) (flow.NodeInstance, error)

func newDisplayForTest(
	t *testing.T,
	factory factoryFn,
	nodeID string,
	props map[string]any,
) (flow.NodeInstance, *stubHub) {
	t.Helper()
	inst, err := factory(flow.NodeConfig{
		ID:         nodeID,
		Properties: props,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := inst.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	inst.SetSend(func(_ int, _ *flow.Message) {})
	inst.SetStatus(func(_, _ string) {})
	inst.SetDebug(func(_ flow.DebugMessage) {})

	hub := newStubHub()
	provider, ok := inst.(flow.DashboardHubProvider)
	if !ok {
		t.Fatalf("%T does not implement DashboardHubProvider", inst)
	}
	provider.SetDashboardHub(hub)
	if err := inst.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = inst.Stop() })
	return inst, hub
}

func deliver(t *testing.T, inst flow.NodeInstance, msg *flow.Message) {
	t.Helper()
	out, err := inst.HandleMessage(msg)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if out != nil {
		t.Fatalf("display widget must return nil outputs, got %d ports", len(out))
	}
}

// ─── ui-text ───────────────────────────────────────────────────────────────

func TestUIText_HandleMessage_PushesPayloadValue(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUITextNode, "t1", map[string]any{})
	msg := flow.NewMessage()
	msg.SetPayload("hello")
	deliver(t, inst, msg)

	pushes := hub.snapshotPushes()
	if len(pushes) != 1 {
		t.Fatalf("want 1 push, got %d", len(pushes))
	}
	if pushes[0].id != "t1" || pushes[0].value != "hello" {
		t.Fatalf("push: %+v", pushes[0])
	}
}

func TestUIText_HandleMessage_PushesConfiguredProperty(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUITextNode, "t1", map[string]any{
		"property": "temperature",
	})
	msg := flow.NewMessage()
	msg.Set("temperature", 42.5)
	deliver(t, inst, msg)

	pushes := hub.snapshotPushes()
	if len(pushes) != 1 || pushes[0].value != 42.5 {
		t.Fatalf("want value=42.5, got %+v", pushes)
	}
}

func TestUIText_HandleMessage_NilMsg_NoPush(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUITextNode, "t1", map[string]any{})
	deliver(t, inst, nil)
	if len(hub.snapshotPushes()) != 0 {
		t.Fatal("nil message should not produce a push")
	}
}

func TestUIText_Start_WithoutHub_NoOp(t *testing.T) {
	inst, _ := NewUITextNode(flow.NodeConfig{ID: "t1", Properties: map[string]any{}})
	if err := inst.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	inst.SetSend(func(_ int, _ *flow.Message) {})
	inst.SetStatus(func(_, _ string) {})
	inst.SetDebug(func(_ flow.DebugMessage) {})
	if err := inst.Start(); err != nil {
		t.Fatalf("start without hub: %v", err)
	}
	// HandleMessage with no hub is a no-op, not a panic.
	out, err := inst.HandleMessage(flow.NewMessage())
	if err != nil || out != nil {
		t.Fatalf("handle without hub: out=%v err=%v", out, err)
	}
}

// ─── ui-led ────────────────────────────────────────────────────────────────

func TestUILed_HandleMessage_PushesBool(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUILedNode, "l1", map[string]any{})
	msg := flow.NewMessage()
	msg.SetPayload(true)
	deliver(t, inst, msg)

	pushes := hub.snapshotPushes()
	if len(pushes) != 1 || pushes[0].value != true {
		t.Fatalf("want bool true, got %+v", pushes)
	}
}

func TestUILed_HandleMessage_PushesString(t *testing.T) {
	// Spec: states like {when:"alarm"}; we push the raw value, the
	// frontend evaluates rules. Verify the raw value reaches the hub.
	inst, hub := newDisplayForTest(t, NewUILedNode, "l1", map[string]any{})
	msg := flow.NewMessage()
	msg.SetPayload("alarm")
	deliver(t, inst, msg)
	pushes := hub.snapshotPushes()
	if pushes[0].value != "alarm" {
		t.Fatalf("want \"alarm\", got %+v", pushes[0])
	}
}

// ─── ui-gauge ──────────────────────────────────────────────────────────────

func TestUIGauge_HandleMessage_PushesNumber(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUIGaugeNode, "g1", map[string]any{})
	msg := flow.NewMessage()
	msg.SetPayload(73.4)
	deliver(t, inst, msg)
	if hub.snapshotPushes()[0].value != 73.4 {
		t.Fatalf("want 73.4, got %+v", hub.snapshotPushes()[0])
	}
}

func TestUIGauge_HandleMessage_PushesObject(t *testing.T) {
	// Spec also accepts {value: N}. The node pushes the raw payload —
	// the frontend reads .value if it's an object.
	inst, hub := newDisplayForTest(t, NewUIGaugeNode, "g1", map[string]any{})
	msg := flow.NewMessage()
	msg.SetPayload(map[string]any{"value": 5})
	deliver(t, inst, msg)
	obj, ok := hub.snapshotPushes()[0].value.(map[string]any)
	if !ok || obj["value"] != 5 {
		t.Fatalf("want {value:5}, got %+v", hub.snapshotPushes()[0])
	}
}

// ─── shared: hub must replace not accumulate ──────────────────────────────

func TestDisplay_RepeatedPushes_AllRecordedInOrder(t *testing.T) {
	inst, hub := newDisplayForTest(t, NewUITextNode, "t1", map[string]any{})
	for _, v := range []any{1, 2, 3} {
		msg := flow.NewMessage()
		msg.SetPayload(v)
		deliver(t, inst, msg)
	}
	pushes := hub.snapshotPushes()
	if len(pushes) != 3 {
		t.Fatalf("want 3 pushes, got %d", len(pushes))
	}
	for i, want := range []any{1, 2, 3} {
		if pushes[i].value != want {
			t.Fatalf("push %d: want %v, got %v", i, want, pushes[i].value)
		}
	}
}
