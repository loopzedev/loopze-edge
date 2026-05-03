// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow_test

import (
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/config"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// disabledTestRig wires a single source → capture pair and exposes hooks for
// disable/enable redeploys. It reuses sourceNode / captureNode from the package.
type disabledTestRig struct {
	t       *testing.T
	engine  *flow.Engine
	sinks   map[string]*captureNode
	sendMu  sync.Mutex
	send    flow.SendFunc
	ready   chan struct{}
	readyOK bool
}

func newDisabledRig(t *testing.T) *disabledTestRig {
	t.Helper()
	cfg := &config.Config{}
	engine := flow.NewEngine(cfg)
	rig := &disabledTestRig{
		t:      t,
		engine: engine,
		sinks:  make(map[string]*captureNode),
		ready:  make(chan struct{}),
	}

	engine.Registry().Register("test-source", func(_ flow.NodeConfig) (flow.NodeInstance, error) {
		return &sourceNode{
			onSendSet: func(fn flow.SendFunc) {
				rig.sendMu.Lock()
				rig.send = fn
				if !rig.readyOK {
					rig.readyOK = true
					close(rig.ready)
				}
				rig.sendMu.Unlock()
			},
		}, nil
	}, flow.NodeTypeInfo{Type: "test-source", Inputs: 0, Outputs: 1})

	engine.Registry().Register("test-capture", func(nc flow.NodeConfig) (flow.NodeInstance, error) {
		n := &captureNode{}
		rig.sinks[nc.ID] = n
		return n, nil
	}, flow.NodeTypeInfo{Type: "test-capture", Inputs: 1, Outputs: 0})

	if err := engine.Start(); err != nil {
		t.Fatalf("engine.Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })
	return rig
}

func (r *disabledTestRig) deploy(flows []flow.Flow, mode flow.DeployMode) {
	r.t.Helper()
	if err := r.engine.Deploy(flows, nil, mode); err != nil {
		r.t.Fatalf("engine.Deploy: %v", err)
	}
}

func (r *disabledTestRig) waitForSend() flow.SendFunc {
	r.t.Helper()
	select {
	case <-r.ready:
	case <-time.After(2 * time.Second):
		r.t.Fatal("timeout waiting for SendFunc injection")
	}
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	return r.send
}

func (r *disabledTestRig) currentSend() flow.SendFunc {
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	return r.send
}

// captureCount returns the number of messages captured by the sink with the
// given ID. Returns -1 if no such sink exists (i.e. node was never instantiated).
func (r *disabledTestRig) captureCount(id string) int {
	sink, ok := r.sinks[id]
	if !ok {
		return -1
	}
	return len(sink.snapshot())
}

func makeFlow(captureDisabled bool) []flow.Flow {
	return []flow.Flow{{
		ID:    "flow-d",
		Label: "Disabled Test Flow",
		Nodes: []flow.Node{
			{
				ID:    "src",
				Type:  "test-source",
				Z:     "flow-d",
				Wires: [][]string{{"cap"}},
			},
			{
				ID:       "cap",
				Type:     "test-capture",
				Z:        "flow-d",
				Wires:    [][]string{},
				Disabled: captureDisabled,
			},
		},
	}}
}

// TestDisabledNode_NotInstantiated verifies that a node marked Disabled is
// never instantiated by the engine: the factory is not called, and a wire
// targeting it drops messages silently without panicking.
func TestDisabledNode_NotInstantiated(t *testing.T) {
	rig := newDisabledRig(t)
	rig.deploy(makeFlow(true), flow.DeployFull)
	send := rig.waitForSend()

	if _, exists := rig.sinks["cap"]; exists {
		t.Fatalf("expected disabled capture node to NOT be instantiated, but factory was called")
	}

	// Wire-target points at a disabled (i.e. non-existent) node. Must not panic.
	send(0, flow.NewMessage())

	// Give the engine a moment; nothing should change.
	time.Sleep(50 * time.Millisecond)

	if got := rig.captureCount("cap"); got != -1 {
		t.Fatalf("disabled node received %d messages, want sink absent", got)
	}
}

// TestDisabledNode_PartialDeployDisableStopsNode verifies that toggling
// Disabled to true via DeployModifiedNodes stops the running node so it no
// longer receives messages.
func TestDisabledNode_PartialDeployDisableStopsNode(t *testing.T) {
	rig := newDisabledRig(t)

	// Initial deploy with capture enabled.
	rig.deploy(makeFlow(false), flow.DeployFull)
	send := rig.waitForSend()

	// Send first message — must arrive.
	send(0, flow.NewMessage())
	waitForCount(t, rig, "cap", 1)

	// Snapshot the sink instance so we can confirm it stays at 1 after disable.
	originalSink := rig.sinks["cap"]

	// Re-deploy with capture disabled, partial mode.
	rig.deploy(makeFlow(true), flow.DeployModifiedNodes)

	// Source is unchanged → its SendFunc gets re-wired (SetSend called again).
	// Use the latest captured send — it now routes to an empty targets map.
	send = rig.currentSend()
	send(0, flow.NewMessage())

	// Give the engine time; the disabled node must NOT pick up the message.
	time.Sleep(100 * time.Millisecond)

	if got := len(originalSink.snapshot()); got != 1 {
		t.Fatalf("after disable, original capture node received %d messages, want 1", got)
	}
}

// TestDisabledNode_PartialDeployEnableStartsNode verifies that toggling
// Disabled from true to false via DeployModifiedNodes instantiates and starts
// the node so it begins receiving messages.
func TestDisabledNode_PartialDeployEnableStartsNode(t *testing.T) {
	rig := newDisabledRig(t)

	// Initial deploy with capture disabled.
	rig.deploy(makeFlow(true), flow.DeployFull)
	send := rig.waitForSend()

	if _, exists := rig.sinks["cap"]; exists {
		t.Fatalf("expected disabled capture node to NOT be instantiated initially")
	}

	// Send a message — goes nowhere (capture not running).
	send(0, flow.NewMessage())
	time.Sleep(50 * time.Millisecond)

	// Re-deploy with capture enabled, partial mode.
	rig.deploy(makeFlow(false), flow.DeployModifiedNodes)

	if _, exists := rig.sinks["cap"]; !exists {
		t.Fatalf("expected capture node to be instantiated after enable")
	}

	// Source is unchanged but wireAllNodes refreshes its SendFunc to include
	// the now-running capture as a target.
	send = rig.currentSend()
	send(0, flow.NewMessage())

	waitForCount(t, rig, "cap", 1)
}

// waitForCount polls the named sink until it has captured at least n messages
// or fails the test on timeout.
func waitForCount(t *testing.T, rig *disabledTestRig, sinkID string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rig.captureCount(sinkID) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d messages on %q (got %d)", n, sinkID, rig.captureCount(sinkID))
}
