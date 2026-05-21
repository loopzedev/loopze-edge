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

// ─── Cache ──────────────────────────────────────────────────────────────────

func TestCache_Put_StoresLatest(t *testing.T) {
	c := NewCache()
	ts := time.Now()
	c.Put("w1", 42, ts)
	got, ok := c.Get("w1")
	if !ok {
		t.Fatal("expected entry for w1")
	}
	if got.Value != 42 {
		t.Fatalf("value: want 42, got %v", got.Value)
	}
	if !got.TS.Equal(ts) {
		t.Fatalf("timestamp: want %v, got %v", ts, got.TS)
	}
}

func TestCache_Put_Replaces(t *testing.T) {
	c := NewCache()
	c.Put("w1", "first", time.Now())
	c.Put("w1", "second", time.Now())
	got, _ := c.Get("w1")
	if got.Value != "second" {
		t.Fatalf("want last write to win, got %v", got.Value)
	}
}

func TestCache_Retain_DropsMissing(t *testing.T) {
	c := NewCache()
	c.Put("keep", 1, time.Now())
	c.Put("drop", 2, time.Now())
	c.Retain(map[string]struct{}{"keep": {}})
	if _, ok := c.Get("keep"); !ok {
		t.Fatal("kept widget missing after Retain")
	}
	if _, ok := c.Get("drop"); ok {
		t.Fatal("expected dropped widget to be gone")
	}
}

// ─── Layout ────────────────────────────────────────────────────────────────

func TestLayout_NoBase_EmptySnapshot(t *testing.T) {
	snap := BuildLayout(flow.Workspace{})
	if snap == nil {
		t.Fatal("snapshot must never be nil")
	}
	if snap.Base != nil {
		t.Fatal("expected no base for an empty workspace")
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", snap.Errors)
	}
}

func TestLayout_HappyPath(t *testing.T) {
	ws := flow.Workspace{
		Flows: []flow.Flow{
			{
				ID: "f1",
				Nodes: []flow.Node{
					{
						ID:   "btn1",
						Type: "ui-button",
						Name: "Acknowledge",
						Config: map[string]any{
							"group": "g1",
							"order": float64(0),
							"label": "ACK",
						},
					},
				},
			},
		},
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base", Config: map[string]any{"name": "Test"}},
			{ID: "p1", Type: "ui-page", Config: map[string]any{"name": "Overview"}},
			{ID: "g1", Type: "ui-group", Config: map[string]any{"name": "Controls", "page": "p1", "width": float64(4)}},
		},
	}
	snap := BuildLayout(ws)
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	if snap.Base == nil || snap.Base.Name != "Test" {
		t.Fatalf("base: want name=Test, got %+v", snap.Base)
	}
	if len(snap.Pages) != 1 || snap.Pages[0].ID != "p1" {
		t.Fatalf("pages: want one page p1, got %+v", snap.Pages)
	}
	if len(snap.Groups) != 1 || snap.Groups[0].PageID != "p1" || snap.Groups[0].Width != 4 {
		t.Fatalf("groups: %+v", snap.Groups)
	}
	if len(snap.Widgets) != 1 || snap.Widgets[0].ID != "btn1" || snap.Widgets[0].GroupID != "g1" {
		t.Fatalf("widgets: %+v", snap.Widgets)
	}
	if snap.Widgets[0].Label != "ACK" {
		t.Fatalf("widget label: want ACK, got %q", snap.Widgets[0].Label)
	}
}

func TestLayout_DoubleBase_Error(t *testing.T) {
	ws := flow.Workspace{
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base"},
			{ID: "base2", Type: "ui-base"},
		},
	}
	snap := BuildLayout(ws)
	if snap.Base != nil {
		t.Fatal("expected no base when singleton violated")
	}
	if len(snap.Errors) != 1 {
		t.Fatalf("want one error, got %v", snap.Errors)
	}
}

func TestLayout_OrphanGroup_Error(t *testing.T) {
	ws := flow.Workspace{
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base"},
			{ID: "g1", Type: "ui-group", Config: map[string]any{"page": "missing"}},
		},
	}
	snap := BuildLayout(ws)
	if len(snap.Groups) != 0 {
		t.Fatalf("orphan group should not appear in layout, got %+v", snap.Groups)
	}
	if len(snap.Errors) != 1 || snap.Errors[0].NodeID != "g1" {
		t.Fatalf("want one error attached to g1, got %v", snap.Errors)
	}
}

func TestLayout_OrphanWidget_Error(t *testing.T) {
	ws := flow.Workspace{
		Flows: []flow.Flow{
			{
				ID: "f1",
				Nodes: []flow.Node{
					{ID: "btn1", Type: "ui-button", Config: map[string]any{"group": "missing"}},
				},
			},
		},
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base"},
		},
	}
	snap := BuildLayout(ws)
	if len(snap.Widgets) != 0 {
		t.Fatalf("orphan widget should not appear, got %+v", snap.Widgets)
	}
	if len(snap.Errors) != 1 || snap.Errors[0].NodeID != "btn1" {
		t.Fatalf("want one error attached to btn1, got %v", snap.Errors)
	}
}

func TestLayout_WidgetIDs_ContainsOnlyValid(t *testing.T) {
	ws := flow.Workspace{
		Flows: []flow.Flow{
			{
				ID: "f1",
				Nodes: []flow.Node{
					{ID: "good", Type: "ui-button", Config: map[string]any{"group": "g1"}},
					{ID: "bad", Type: "ui-button", Config: map[string]any{"group": "missing"}},
				},
			},
		},
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base"},
			{ID: "p1", Type: "ui-page"},
			{ID: "g1", Type: "ui-group", Config: map[string]any{"page": "p1"}},
		},
	}
	ids := BuildLayout(ws).WidgetIDs()
	if _, ok := ids["good"]; !ok {
		t.Fatal("WidgetIDs should contain valid widget")
	}
	if _, ok := ids["bad"]; ok {
		t.Fatal("WidgetIDs should not contain orphan widget")
	}
}

// ─── Hub ────────────────────────────────────────────────────────────────────

func TestHub_AuthMode_DefaultsToSession(t *testing.T) {
	h := NewHub(nil)
	if got := h.AuthMode(); got != "session" {
		t.Fatalf("default auth: want session, got %q", got)
	}
}

func TestHub_AuthMode_NoneAfterDeploy(t *testing.T) {
	h := NewHub(nil)
	h.RebuildLayout(flow.Workspace{
		Configs: []flow.ConfigNode{
			{ID: "base1", Type: "ui-base", Config: map[string]any{"auth": "none"}},
		},
	})
	if got := h.AuthMode(); got != "none" {
		t.Fatalf("auth=none: want none, got %q", got)
	}
}

func TestHub_Snapshot_EmptyBeforeDeploy(t *testing.T) {
	h := NewHub(nil)
	snap := h.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot must never be nil")
	}
	if snap.Base != nil {
		t.Fatal("Snapshot before deploy should have no base")
	}
}

func TestHub_PushWidgetValue_UpdatesCache(t *testing.T) {
	h := NewHub(nil)
	h.PushWidgetValue("w1", 42, time.Now())
	got, ok := h.cache.Get("w1")
	if !ok || got.Value != 42 {
		t.Fatalf("cache not updated: ok=%v val=%v", ok, got.Value)
	}
}

func TestHub_RegisterInput_DispatchEvent(t *testing.T) {
	h := NewHub(nil)
	var (
		gotMu sync.Mutex
		got   flow.WidgetEvent
		fired = make(chan struct{}, 1)
	)
	unregister := h.RegisterInputWidget("btn1", func(ev flow.WidgetEvent) {
		gotMu.Lock()
		defer gotMu.Unlock()
		got = ev
		fired <- struct{}{}
	})
	defer unregister()

	dispatched := h.dispatchEvent(&client{userID: "alice", socketID: "ws-1"}, "btn1", "click")
	if !dispatched {
		t.Fatal("dispatchEvent returned false for registered widget")
	}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("callback never fired")
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	if got.Value != "click" {
		t.Fatalf("value: want click, got %v", got.Value)
	}
	if got.Client.UserID != "alice" {
		t.Fatalf("user: want alice, got %q", got.Client.UserID)
	}
}

func TestHub_DispatchEvent_UnknownWidget(t *testing.T) {
	h := NewHub(nil)
	if h.dispatchEvent(&client{}, "unknown", nil) {
		t.Fatal("dispatchEvent should return false for unknown widget")
	}
}

func TestHub_RegisterInput_UnregisterIsGenerationAware(t *testing.T) {
	h := NewHub(nil)
	calls := make(chan string, 4)
	unregOld := h.RegisterInputWidget("btn1", func(ev flow.WidgetEvent) { calls <- "old" })
	// Re-register before unregistering the old one (simulating a deploy
	// race where the new registration lands first).
	_ = h.RegisterInputWidget("btn1", func(ev flow.WidgetEvent) { calls <- "new" })
	// Calling the old unregister now must NOT remove the new callback.
	unregOld()
	if !h.dispatchEvent(&client{}, "btn1", nil) {
		t.Fatal("new callback was deleted by stale unregister")
	}
	select {
	case which := <-calls:
		if which != "new" {
			t.Fatalf("want new callback, got %q", which)
		}
	case <-time.After(time.Second):
		t.Fatal("new callback never fired")
	}
}
