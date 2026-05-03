// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/config"
	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// ─── Test helpers ────────────────────────────────────────────────────────────

// asyncErrNode raises an error from a background goroutine via the engine-
// supplied ErrorFunc, simulating an async failure (e.g. MQTT publish callback).
// It implements ErrorProvider so the engine wires SetError automatically.
type asyncErrNode struct {
	send    flow.SendFunc
	errorFn flow.ErrorFunc
	fired   chan struct{}
}

func (n *asyncErrNode) Init() error                    { n.fired = make(chan struct{}); return nil }
func (n *asyncErrNode) SetSend(fn flow.SendFunc)        { n.send = fn }
func (n *asyncErrNode) SetStatus(_ flow.StatusFunc)     {}
func (n *asyncErrNode) SetDebug(_ flow.DebugFunc)       {}
func (n *asyncErrNode) SetError(fn flow.ErrorFunc)      { n.errorFn = fn }
func (n *asyncErrNode) Start() error                    { return nil }
func (n *asyncErrNode) Stop() error                     { return nil }
func (n *asyncErrNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	go func() {
		n.errorFn(errors.New("async boom"), msg)
		close(n.fired)
	}()
	return nil, nil
}

// captureNode is a sink that records every message it receives. It has 1 input
// and 0 outputs.
type captureNode struct {
	mu       sync.Mutex
	captured []*flow.Message
}

func (n *captureNode) Init() error                    { return nil }
func (n *captureNode) SetSend(_ flow.SendFunc)         {}
func (n *captureNode) SetStatus(_ flow.StatusFunc)     {}
func (n *captureNode) SetDebug(_ flow.DebugFunc)       {}
func (n *captureNode) Start() error                    { return nil }
func (n *captureNode) Stop() error                     { return nil }
func (n *captureNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	n.mu.Lock()
	n.captured = append(n.captured, msg)
	n.mu.Unlock()
	return nil, nil
}

func (n *captureNode) snapshot() []*flow.Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]*flow.Message, len(n.captured))
	copy(out, n.captured)
	return out
}

// catchTestRig encapsulates the deploy boilerplate: a source-err pair plus a
// configurable catch node wired to a capture sink. Returns the engine and the
// shared sourceNode.SendFunc once it's been injected.
type catchTestRig struct {
	t           *testing.T
	engine      *flow.Engine
	send        flow.SendFunc
	sinks       map[string]*captureNode // catchNodeID → sink
	asyncNodes  map[string]*asyncErrNode
	deploy      deployFn
}

type deployFn = func([]flow.Flow)

func newCatchRig(t *testing.T) *catchTestRig {
	t.Helper()
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)
	rig := &catchTestRig{
		t:          t,
		engine:     engine,
		sinks:      make(map[string]*captureNode),
		asyncNodes: make(map[string]*asyncErrNode),
	}

	var (
		sendMu    sync.Mutex
		sendReady = make(chan struct{})
		once      sync.Once
	)

	engine.Registry().Register("test-source", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &sourceNode{
			onSendSet: func(fn flow.SendFunc) {
				sendMu.Lock()
				rig.send = fn
				sendMu.Unlock()
				once.Do(func() { close(sendReady) })
			},
		}, nil
	}, flow.NodeTypeInfo{Type: "test-source", Inputs: 0, Outputs: 1})

	engine.Registry().Register("test-err", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &errNode{}, nil
	}, flow.NodeTypeInfo{Type: "test-err", Inputs: 1, Outputs: 0})

	engine.Registry().Register("test-async-err", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		n := &asyncErrNode{}
		return n, nil
	}, flow.NodeTypeInfo{Type: "test-async-err", Inputs: 1, Outputs: 0})

	engine.Registry().Register("test-capture", func(cfg flow.NodeConfig) (flow.NodeInstance, error) {
		n := &captureNode{}
		rig.sinks[cfg.ID] = n
		return n, nil
	}, flow.NodeTypeInfo{Type: "test-capture", Inputs: 1, Outputs: 0})

	engine.Registry().Register("catch", nodes.NewCatchNode, nodes.CatchTypeInfo())

	if err := engine.Start(); err != nil {
		t.Fatalf("engine.Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })

	t.Cleanup(func() {
		select {
		case <-sendReady:
		default:
		}
	})

	rig.deploy = func(flows []flow.Flow) {
		t.Helper()
		if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
			t.Fatalf("engine.Deploy: %v", err)
		}
		select {
		case <-sendReady:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for SendFunc injection")
		}
	}
	return rig
}

func (r *catchTestRig) waitForCaptures(sinkID string, n int) {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sink, ok := r.sinks[sinkID]; ok {
			if len(sink.snapshot()) >= n {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestCatch_ScopeFlow_TriggeredOnError(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-1", Wires: [][]string{{"err-1"}}},
			{ID: "err-1", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{"scope": "flow"},
				Wires:      [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	rig.send(0, flow.NewMessage())
	rig.waitForCaptures("sink-1", 1)

	got := rig.sinks["sink-1"].snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 captured message, got %d", len(got))
	}
	errBlock, _ := got[0].Get("_error").(map[string]any)
	if errBlock == nil {
		t.Fatalf("missing _error on captured message")
	}
	if errBlock["message"] != "intentional test error" {
		t.Errorf("_error.message: want %q, got %v", "intentional test error", errBlock["message"])
	}
	source, _ := errBlock["source"].(map[string]any)
	if source["id"] != "err-1" {
		t.Errorf("_error.source.id: want %q, got %v", "err-1", source["id"])
	}
	if source["flowId"] != "flow-1" {
		t.Errorf("_error.source.flowId: want %q, got %v", "flow-1", source["flowId"])
	}
	if source["type"] != "test-err" {
		t.Errorf("_error.source.type: want %q, got %v", "test-err", source["type"])
	}
}

func TestCatch_ScopeFlow_IgnoresOtherFlows(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{
		{
			ID: "flow-A",
			Nodes: []flow.Node{
				{ID: "src-A", Type: "test-source", Z: "flow-A", Wires: [][]string{{"err-A"}}},
				{ID: "err-A", Type: "test-err", Z: "flow-A", Wires: [][]string{}},
			},
		},
		{
			ID: "flow-B",
			Nodes: []flow.Node{
				{ID: "catch-B", Type: "catch", Z: "flow-B",
					Config: map[string]any{"scope": "flow"},
					Wires:      [][]string{{"sink-B"}}},
				{ID: "sink-B", Type: "test-capture", Z: "flow-B", Wires: [][]string{}},
			},
		},
	})

	rig.send(0, flow.NewMessage())
	time.Sleep(150 * time.Millisecond)

	if got := rig.sinks["sink-B"].snapshot(); len(got) != 0 {
		t.Fatalf("expected no captures across flows, got %d", len(got))
	}
}

func TestCatch_ScopeSelected_OnlyMatching(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-A", Type: "test-source", Z: "flow-1", Wires: [][]string{{"err-A"}}},
			{ID: "err-A", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "err-B", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{
					"scope":       "selected",
					"targetNodes": []any{"err-B"},
				},
				Wires: [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	// Only err-A is wired and reachable; err-B never receives a message.
	// Catch is configured for err-B → no fan-out.
	rig.send(0, flow.NewMessage())
	time.Sleep(150 * time.Millisecond)

	if got := rig.sinks["sink-1"].snapshot(); len(got) != 0 {
		t.Fatalf("catch should not fire for err-A, got %d captures", len(got))
	}
}

func TestCatch_ScopeAll_AcrossFlows(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{
		{
			ID: "flow-A",
			Nodes: []flow.Node{
				{ID: "src-A", Type: "test-source", Z: "flow-A", Wires: [][]string{{"err-A"}}},
				{ID: "err-A", Type: "test-err", Z: "flow-A", Wires: [][]string{}},
			},
		},
		{
			ID: "flow-B",
			Nodes: []flow.Node{
				{ID: "catch-B", Type: "catch", Z: "flow-B",
					Config: map[string]any{"scope": "all"},
					Wires:      [][]string{{"sink-B"}}},
				{ID: "sink-B", Type: "test-capture", Z: "flow-B", Wires: [][]string{}},
			},
		},
	})

	rig.send(0, flow.NewMessage())
	rig.waitForCaptures("sink-B", 1)

	if got := rig.sinks["sink-B"].snapshot(); len(got) != 1 {
		t.Fatalf("scope=all: want 1 capture across flows, got %d", len(got))
	}
}

func TestCatch_MultipleCatch_BothTriggered(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-1", Wires: [][]string{{"err-1"}}},
			{ID: "err-1", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{"scope": "flow"},
				Wires:      [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-2", Type: "catch", Z: "flow-1",
				Config: map[string]any{"scope": "flow"},
				Wires:      [][]string{{"sink-2"}}},
			{ID: "sink-2", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	rig.send(0, flow.NewMessage())
	rig.waitForCaptures("sink-1", 1)
	rig.waitForCaptures("sink-2", 1)

	if got := len(rig.sinks["sink-1"].snapshot()); got != 1 {
		t.Errorf("sink-1: want 1, got %d", got)
	}
	if got := len(rig.sinks["sink-2"].snapshot()); got != 1 {
		t.Errorf("sink-2: want 1, got %d", got)
	}
}

func TestCatch_LoopGuard_CaughtMsgNotRefired(t *testing.T) {
	rig := newCatchRig(t)
	// catch-1 is wired into err-2: when catch-1 emits its error message into
	// err-2, err-2 raises another error. The error-message engine must NOT
	// fan that second error out to any catch node (neither catch-1 nor
	// catch-2), because the message carries _caught=true. Without this
	// guard, catch-1 would re-catch err-2's error and feed it back into
	// err-2 in an infinite loop.
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-1", Wires: [][]string{{"err-1"}}},
			{ID: "err-1", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{"scope": "flow"},
				Wires:  [][]string{{"err-2"}}},
			{ID: "err-2", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-2", Type: "catch", Z: "flow-1",
				Config: map[string]any{
					"scope":       "selected",
					"targetNodes": []any{"err-2"},
				},
				Wires: [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	rig.send(0, flow.NewMessage())
	time.Sleep(200 * time.Millisecond)

	if got := len(rig.sinks["sink-1"].snapshot()); got != 0 {
		t.Errorf("loop guard breached: catch-2 must not see a re-fired error, got %d captures", got)
	}
}

func TestCatch_AsyncError_TriggersCatch(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-1", Wires: [][]string{{"async-1"}}},
			{ID: "async-1", Type: "test-async-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{"scope": "flow"},
				Wires:      [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	rig.send(0, flow.NewMessage())
	rig.waitForCaptures("sink-1", 1)

	got := rig.sinks["sink-1"].snapshot()
	if len(got) != 1 {
		t.Fatalf("async catch: want 1 capture, got %d", len(got))
	}
	errBlock, _ := got[0].Get("_error").(map[string]any)
	if errBlock == nil || errBlock["message"] != "async boom" {
		t.Errorf("expected async error payload, got %v", errBlock)
	}
}

// TestCatch_StateMachineGuardError verifies the user-reported scenario: a JS
// guard inside a State Machine references a non-existent property (event.pin
// → undefined → TypeError on .pin). The guard returns false (transition
// blocked, machine stays in the current state), but the error must surface
// to a Catch Node via the ErrorProvider hook on StateMachineNode.
func TestCatch_StateMachineGuardError(t *testing.T) {
	rig := newCatchRig(t)
	rig.engine.Registry().Register("statemachine", nodes.NewStateMachineNode, nodes.StateMachineTypeInfo())

	machine := `{"id":"m","initial":"locked","context":{},"states":{` +
		`"locked":{"on":{"UNLOCK":[{"target":"unlocked","guard":"pinCorrect"}]}},` +
		`"unlocked":{}` +
		`}}`
	guards := `return { pinCorrect: function(ctx, event) { return event.pin.pin === "1234"; } }`

	rig.deploy([]flow.Flow{{
		ID: "flow-sm",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-sm", Wires: [][]string{{"sm-1"}}},
			{ID: "sm-1", Type: "statemachine", Z: "flow-sm",
				Config: map[string]any{"machine": machine, "guards": guards},
				Wires:  [][]string{{}, {}}},
			{ID: "catch-1", Type: "catch", Z: "flow-sm",
				Config: map[string]any{"scope": "flow"},
				Wires:  [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-sm", Wires: [][]string{}},
		},
	}})

	// Send an UNLOCK event without an event.pin object — guard will throw.
	msg := flow.NewMessage()
	msg.SetTopic("UNLOCK")
	msg.SetPayload(map[string]any{}) // no "pin" → event.pin is undefined
	rig.send(0, msg)

	rig.waitForCaptures("sink-1", 1)
	got := rig.sinks["sink-1"].snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 catch capture from guard error, got %d", len(got))
	}
	errBlock, _ := got[0].Get("_error").(map[string]any)
	if errBlock == nil {
		t.Fatalf("missing _error on captured message")
	}
	source, _ := errBlock["source"].(map[string]any)
	if source["id"] != "sm-1" {
		t.Errorf("_error.source.id: want sm-1, got %v", source["id"])
	}
	if source["type"] != "statemachine" {
		t.Errorf("_error.source.type: want statemachine, got %v", source["type"])
	}
	gotMsg, _ := errBlock["message"].(string)
	if gotMsg == "" || !contains(gotMsg, "pinCorrect") {
		t.Errorf("_error.message should mention guard name, got %q", gotMsg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestCatch_SelectedNodes_UnknownIDIgnored(t *testing.T) {
	rig := newCatchRig(t)
	rig.deploy([]flow.Flow{{
		ID: "flow-1",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-1", Wires: [][]string{{"err-1"}}},
			{ID: "err-1", Type: "test-err", Z: "flow-1", Wires: [][]string{}},
			{ID: "catch-1", Type: "catch", Z: "flow-1",
				Config: map[string]any{
					"scope":       "selected",
					"targetNodes": []any{"non-existent"},
				},
				Wires: [][]string{{"sink-1"}}},
			{ID: "sink-1", Type: "test-capture", Z: "flow-1", Wires: [][]string{}},
		},
	}})

	rig.send(0, flow.NewMessage())
	time.Sleep(150 * time.Millisecond)

	if got := rig.sinks["sink-1"].snapshot(); len(got) != 0 {
		t.Fatalf("unknown selected id must not match, got %d captures", len(got))
	}
}
