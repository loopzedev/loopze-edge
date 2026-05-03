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
)

// ─── errNode ────────────────────────────────────────────────────────────────

// errNode always returns an error from HandleMessage.
// It has 1 input so the engine starts a nodeLoop for it.
type errNode struct {
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc
}

func (n *errNode) Init() error                                            { return nil }
func (n *errNode) SetSend(fn flow.SendFunc)                               { n.send = fn }
func (n *errNode) SetStatus(fn flow.StatusFunc)                           { n.status = fn }
func (n *errNode) SetDebug(fn flow.DebugFunc)                             { n.debug = fn }
func (n *errNode) Start() error                                           { return nil }
func (n *errNode) Stop() error                                            { return nil }
func (n *errNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, errors.New("intentional test error")
}

// ─── sourceNode ─────────────────────────────────────────────────────────────

// sourceNode has 0 inputs and 1 output. It captures its SendFunc so the test
// can inject messages into the wired downstream node.
type sourceNode struct {
	send flow.SendFunc
	// onSendSet is called after the engine injects the SendFunc.
	onSendSet func(fn flow.SendFunc)
}

func (n *sourceNode) Init() error  { return nil }
func (n *sourceNode) SetSend(fn flow.SendFunc) {
	n.send = fn
	if n.onSendSet != nil {
		n.onSendSet(fn)
	}
}
func (n *sourceNode) SetStatus(_ flow.StatusFunc)                           {}
func (n *sourceNode) SetDebug(_ flow.DebugFunc)                             {}
func (n *sourceNode) Start() error                                          { return nil }
func (n *sourceNode) Stop() error                                           { return nil }
func (n *sourceNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) { return nil, nil }

// ─── startFailNode ───────────────────────────────────────────────────────────

type startFailNode struct{ errNode }

func (n *startFailNode) Start() error { return errors.New("node failed to start") }

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestEngineErrorChannel_HandleMessage verifies that when a node's HandleMessage
// returns an error, the engine publishes an error-status DebugMessage via
// publishDebug. The message must travel through nodeLoop to trigger publishNodeError.
func TestEngineErrorChannel_HandleMessage(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)

	// capturedSend is set by the sourceNode's SetSend; used to inject a message.
	var (
		sendMu       sync.Mutex
		capturedSend flow.SendFunc
		sendReady    = make(chan struct{})
	)
	once := sync.Once{}

	engine.Registry().Register("test-source", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &sourceNode{
			onSendSet: func(fn flow.SendFunc) {
				sendMu.Lock()
				capturedSend = fn
				sendMu.Unlock()
				once.Do(func() { close(sendReady) })
			},
		}, nil
	}, flow.NodeTypeInfo{Type: "test-source", Inputs: 0, Outputs: 1})

	engine.Registry().Register("test-err", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &errNode{}, nil
	}, flow.NodeTypeInfo{Type: "test-err", Inputs: 1, Outputs: 0})

	var mu sync.Mutex
	var captured []flow.DebugMessage

	engine.SetPublishDebug(func(_ string, msg flow.DebugMessage) {
		mu.Lock()
		captured = append(captured, msg)
		mu.Unlock()
	})

	if err := engine.Start(); err != nil {
		t.Fatalf("engine.Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })

	flows := []flow.Flow{{
		ID:    "flow-1",
		Label: "Test Flow",
		Nodes: []flow.Node{
			{
				ID:    "src-1",
				Type:  "test-source",
				Z:     "flow-1",
				Wires: [][]string{{"err-1"}}, // port 0 → err-1
			},
			{
				ID:    "err-1",
				Type:  "test-err",
				Z:     "flow-1",
				Wires: [][]string{},
			},
		},
	}}

	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatalf("engine.Deploy: %v", err)
	}

	// Wait until the engine has injected the SendFunc into our source node.
	select {
	case <-sendReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SendFunc injection")
	}

	// Inject a message into the source's output port 0, wired to err-1.
	// The engine's SendFunc routes it to err-1.inputCh.
	// nodeLoop picks it up, calls HandleMessage → error → publishNodeError.
	sendMu.Lock()
	send := capturedSend
	sendMu.Unlock()

	msg := flow.NewMessage()
	send(0, msg)

	// Give nodeLoop time to process the message and publish the error.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(captured)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(captured) == 0 {
		t.Fatal("expected at least one debug message to be published, got none")
	}

	got := captured[0]
	if got.Status != "error" {
		t.Errorf("status: want %q, got %q", "error", got.Status)
	}
	if got.NodeID != "err-1" {
		t.Errorf("nodeId: want %q, got %q", "err-1", got.NodeID)
	}
	if got.FlowID != "flow-1" {
		t.Errorf("flowId: want %q, got %q", "flow-1", got.FlowID)
	}
	payload, _ := got.Payload.(string)
	if payload != "intentional test error" {
		t.Errorf("payload: want %q, got %q", "intentional test error", payload)
	}
	if got.Format != "string" {
		t.Errorf("format: want %q, got %q", "string", got.Format)
	}
}

// TestEngineErrorChannel_StartError verifies that when a node's Start()
// returns an error, the engine publishes an error-status DebugMessage.
func TestEngineErrorChannel_StartError(t *testing.T) {
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)

	engine.Registry().Register("start-fail", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &startFailNode{}, nil
	}, flow.NodeTypeInfo{Type: "start-fail", Inputs: 1, Outputs: 0})

	var mu sync.Mutex
	var captured []flow.DebugMessage

	engine.SetPublishDebug(func(_ string, msg flow.DebugMessage) {
		mu.Lock()
		captured = append(captured, msg)
		mu.Unlock()
	})

	if err := engine.Start(); err != nil {
		t.Fatalf("engine.Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })

	flows := []flow.Flow{{
		ID:    "flow-2",
		Nodes: []flow.Node{
			{ID: "fail-1", Type: "start-fail", Z: "flow-2", Wires: [][]string{}},
		},
	}}

	// Deploy triggers Start() on each node. start-fail returns an error.
	if err := engine.Deploy(flows, nil, flow.DeployFull); err != nil {
		t.Fatalf("engine.Deploy: %v", err)
	}

	// Give the engine a moment to publish the error synchronously during Deploy.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(captured) == 0 {
		t.Fatal("expected a start-error debug message, got none")
	}
	if captured[0].Status != "error" {
		t.Errorf("status: want %q, got %q", "error", captured[0].Status)
	}
}
