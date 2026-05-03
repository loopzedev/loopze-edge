// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"sync"
	"testing"

	"github.com/niceclouds/loopze/internal/flow"
)

// memStore is a minimal in-memory ContextStore used in template node tests.
type memStore struct {
	mu   sync.RWMutex
	data map[string]any
}

func newMemStore() *memStore {
	return &memStore{data: map[string]any{}}
}

func (s *memStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key], nil
}

func (s *memStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *memStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *memStore) Keys() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys, nil
}

type templateStores struct {
	flowMem    *memStore
	flowPers   *memStore
	globalMem  *memStore
	globalPers *memStore
	statusFill string
	statusText string
}

func newTemplateNode(t *testing.T, props map[string]any) (*TemplateNode, *templateStores) {
	t.Helper()

	config := flow.NodeConfig{
		ID:         "template-test",
		Type:       "template",
		Properties: props,
	}

	inst, err := NewTemplateNode(config)
	if err != nil {
		t.Fatalf("NewTemplateNode: %v", err)
	}
	n := inst.(*TemplateNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	stores := &templateStores{
		flowMem:    newMemStore(),
		flowPers:   newMemStore(),
		globalMem:  newMemStore(),
		globalPers: newMemStore(),
	}
	n.SetContext(stores.globalMem, stores.globalPers, stores.flowMem, stores.flowPers)
	n.SetSend(func(port int, msg *flow.Message) {})
	n.SetStatus(func(fill, text string) {
		stores.statusFill = fill
		stores.statusText = text
	})
	n.SetDebug(func(msg flow.DebugMessage) {})

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	return n, stores
}

func msgWith(fields map[string]any) *flow.Message {
	msg := flow.NewMessage()
	for k, v := range fields {
		msg.Set(k, v)
	}
	return msg
}

// ── Mustache rendering ────────────────────────────────────────────

func TestTemplateNode_RendersPayload(t *testing.T) {
	n, _ := newTemplateNode(t, map[string]any{
		"template":  "value is {{payload}}",
		"field":     "payload",
		"fieldType": "msg",
		"format":    "plain",
		"syntax":    "mustache",
	})

	out, err := n.HandleMessage(msgWith(map[string]any{"payload": "42"}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got := out[0][0].Payload()
	if got != "value is 42" {
		t.Errorf("payload: want %q, got %v", "value is 42", got)
	}
}

func TestTemplateNode_ReadsFlowContext(t *testing.T) {
	n, stores := newTemplateNode(t, map[string]any{
		"template":  "device {{flow.deviceId}}",
		"field":     "payload",
		"fieldType": "msg",
		"format":    "plain",
		"syntax":    "mustache",
	})
	_ = stores.flowMem.Set("deviceId", "ABC")

	out, err := n.HandleMessage(msgWith(nil))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "device ABC" {
		t.Errorf("payload: want %q, got %v", "device ABC", got)
	}
}

func TestTemplateNode_ReadsGlobalContext(t *testing.T) {
	n, stores := newTemplateNode(t, map[string]any{
		"template":  "tenant={{global.tenant}}",
		"field":     "payload",
		"fieldType": "msg",
		"format":    "plain",
		"syntax":    "mustache",
	})
	_ = stores.globalMem.Set("tenant", "acme")

	out, err := n.HandleMessage(msgWith(nil))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "tenant=acme" {
		t.Errorf("payload: want %q, got %v", "tenant=acme", got)
	}
}

// ── Output target ────────────────────────────────────────────────

func TestTemplateNode_WritesToFlowPersistent(t *testing.T) {
	n, stores := newTemplateNode(t, map[string]any{
		"template":     "hello {{payload}}",
		"field":        "greeting",
		"fieldType":    "flow",
		"fieldStorage": "persistent",
		"format":       "plain",
		"syntax":       "mustache",
	})

	if _, err := n.HandleMessage(msgWith(map[string]any{"payload": "world"})); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := stores.flowPers.Get("greeting")
	if got != "hello world" {
		t.Errorf("flowPers[greeting]: want %q, got %v", "hello world", got)
	}
	if v, _ := stores.flowMem.Get("greeting"); v != nil {
		t.Errorf("flowMem must remain empty when storage=persistent, got %v", v)
	}
}

// ── Format handling ──────────────────────────────────────────────

func TestTemplateNode_JSONFormatParsesObject(t *testing.T) {
	n, _ := newTemplateNode(t, map[string]any{
		"template":  `{"id":"{{id}}","v":{{v}}}`,
		"field":     "payload",
		"fieldType": "msg",
		"format":    "json",
		"syntax":    "mustache",
	})

	out, err := n.HandleMessage(msgWith(map[string]any{"id": "abc", "v": 7}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	parsed, ok := out[0][0].Payload().(map[string]any)
	if !ok {
		t.Fatalf("payload type: want map[string]any, got %T", out[0][0].Payload())
	}
	if parsed["id"] != "abc" {
		t.Errorf("id: want %q, got %v", "abc", parsed["id"])
	}
	if parsed["v"] != float64(7) {
		t.Errorf("v: want 7, got %v", parsed["v"])
	}
}

func TestTemplateNode_JSONFormatErrorSetsStatus(t *testing.T) {
	n, stores := newTemplateNode(t, map[string]any{
		"template":  "not json",
		"field":     "payload",
		"fieldType": "msg",
		"format":    "json",
		"syntax":    "mustache",
	})

	if _, err := n.HandleMessage(msgWith(nil)); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if stores.statusFill != "red" {
		t.Errorf("status fill: want red, got %q", stores.statusFill)
	}
}

// ── Syntax modes ─────────────────────────────────────────────────

func TestTemplateNode_PlainSyntaxPassesThrough(t *testing.T) {
	n, _ := newTemplateNode(t, map[string]any{
		"template":  "literal {{payload}} with braces",
		"field":     "payload",
		"fieldType": "msg",
		"format":    "plain",
		"syntax":    "plain",
	})

	out, err := n.HandleMessage(msgWith(map[string]any{"payload": "ignored"}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if got := out[0][0].Payload(); got != "literal {{payload}} with braces" {
		t.Errorf("payload: want literal pass-through, got %v", got)
	}
}

func TestTemplateNode_InitFailsOnInvalidMustache(t *testing.T) {
	config := flow.NodeConfig{
		ID:   "template-test",
		Type: "template",
		Properties: map[string]any{
			"template": "broken {{#unclosed",
			"syntax":   "mustache",
		},
	}
	inst, err := NewTemplateNode(config)
	if err != nil {
		t.Fatalf("NewTemplateNode: %v", err)
	}
	if err := inst.(*TemplateNode).Init(); err == nil {
		t.Fatal("expected Init to fail on invalid mustache template")
	}
}
