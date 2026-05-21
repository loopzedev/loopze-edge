// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// stubHub captures input-widget registrations and display-widget
// pushes without spinning up a real WebSocket hub.
type stubHub struct {
	mu        sync.Mutex
	callbacks map[string]func(flow.WidgetEvent)
	pushes    []stubPush
}

type stubPush struct {
	id    string
	value any
	ts    time.Time
}

func newStubHub() *stubHub {
	return &stubHub{callbacks: make(map[string]func(flow.WidgetEvent))}
}

func (h *stubHub) PushWidgetValue(id string, value any, ts time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pushes = append(h.pushes, stubPush{id: id, value: value, ts: ts})
}

func (h *stubHub) snapshotPushes() []stubPush {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]stubPush, len(h.pushes))
	copy(out, h.pushes)
	return out
}

func (h *stubHub) RegisterInputWidget(nodeID string, fn func(flow.WidgetEvent)) func() {
	h.mu.Lock()
	h.callbacks[nodeID] = fn
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.callbacks, nodeID)
		h.mu.Unlock()
	}
}

func (h *stubHub) fire(nodeID string, evt flow.WidgetEvent) bool {
	h.mu.Lock()
	fn, ok := h.callbacks[nodeID]
	h.mu.Unlock()
	if !ok {
		return false
	}
	fn(evt)
	return true
}

// newButtonForTest constructs a fully wired UIButtonNode against a stub
// hub and a slot-0 send channel, ready to receive simulated clicks.
func newButtonForTest(t *testing.T, props map[string]any) (*UIButtonNode, *stubHub, chan *flow.Message) {
	t.Helper()
	inst, err := NewUIButtonNode(flow.NodeConfig{
		ID:         "btn1",
		Type:       "ui-button",
		Properties: props,
	})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	btn := inst.(*UIButtonNode)
	if err := btn.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	sent := make(chan *flow.Message, 4)
	btn.SetSend(func(_ int, msg *flow.Message) { sent <- msg })
	btn.SetStatus(func(_, _ string) {})
	btn.SetDebug(func(_ flow.DebugMessage) {})
	hub := newStubHub()
	btn.SetDashboardHub(hub)
	if err := btn.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = btn.Stop() })
	return btn, hub, sent
}

func TestUIButton_Init_RejectsUnknownPayloadType(t *testing.T) {
	inst, _ := NewUIButtonNode(flow.NodeConfig{
		Properties: map[string]any{"payloadType": "msg"}, // not supported in Phase 1
	})
	if err := inst.(*UIButtonNode).Init(); err == nil {
		t.Fatal("expected error for unsupported payloadType")
	}
}

func TestUIButton_Click_EmitsBoolTrueByDefault(t *testing.T) {
	_, hub, sent := newButtonForTest(t, map[string]any{})
	if !hub.fire("btn1", flow.WidgetEvent{TS: time.Now()}) {
		t.Fatal("hub did not deliver click")
	}
	select {
	case msg := <-sent:
		if got := msg.Get("payload"); got != true {
			t.Fatalf("payload: want true, got %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no message emitted")
	}
}

func TestUIButton_Click_EmitsStringPayload(t *testing.T) {
	_, hub, sent := newButtonForTest(t, map[string]any{
		"payloadType": "string",
		"payload":     "ack",
	})
	hub.fire("btn1", flow.WidgetEvent{TS: time.Now()})
	msg := <-sent
	if got := msg.Get("payload"); got != "ack" {
		t.Fatalf("payload: want ack, got %v", got)
	}
}

func TestUIButton_Click_EmitsNumberPayload(t *testing.T) {
	_, hub, sent := newButtonForTest(t, map[string]any{
		"payloadType": "number",
		"payload":     "42.5",
	})
	hub.fire("btn1", flow.WidgetEvent{TS: time.Now()})
	msg := <-sent
	if got := msg.Get("payload"); got != 42.5 {
		t.Fatalf("payload: want 42.5, got %v", got)
	}
}

func TestUIButton_Click_SetsTopic(t *testing.T) {
	_, hub, sent := newButtonForTest(t, map[string]any{
		"topic": "alarms/ack",
	})
	hub.fire("btn1", flow.WidgetEvent{TS: time.Now()})
	msg := <-sent
	if got := msg.Get("topic"); got != "alarms/ack" {
		t.Fatalf("topic: want alarms/ack, got %v", got)
	}
}

func TestUIButton_Click_AttachesClientMetadata(t *testing.T) {
	_, hub, sent := newButtonForTest(t, map[string]any{})
	hub.fire("btn1", flow.WidgetEvent{
		TS: time.Now(),
		Client: flow.WidgetClient{
			UserID:   "alice",
			SocketID: "ws-7",
		},
	})
	msg := <-sent
	client, ok := msg.Get("_client").(map[string]any)
	if !ok {
		t.Fatalf("expected _client to be a map, got %T", msg.Get("_client"))
	}
	if client["userID"] != "alice" {
		t.Fatalf("userID: want alice, got %v", client["userID"])
	}
	if client["socketID"] != "ws-7" {
		t.Fatalf("socketID: want ws-7, got %v", client["socketID"])
	}
}

func TestUIButton_Stop_UnregistersCallback(t *testing.T) {
	btn, hub, _ := newButtonForTest(t, map[string]any{})
	if err := btn.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// After Stop the hub must no longer hold a callback for btn1.
	if hub.fire("btn1", flow.WidgetEvent{}) {
		t.Fatal("hub still has a callback after Stop")
	}
}

func TestUIButton_Start_WithoutHub_NoOp(t *testing.T) {
	inst, _ := NewUIButtonNode(flow.NodeConfig{ID: "btn1", Properties: map[string]any{}})
	btn := inst.(*UIButtonNode)
	if err := btn.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	btn.SetSend(func(_ int, _ *flow.Message) {})
	btn.SetStatus(func(_, _ string) {})
	btn.SetDebug(func(_ flow.DebugMessage) {})
	// Deliberately NOT calling SetDashboardHub.
	if err := btn.Start(); err != nil {
		t.Fatalf("start without hub should not error, got %v", err)
	}
	if err := btn.Stop(); err != nil {
		t.Fatalf("stop without hub should not error, got %v", err)
	}
}
