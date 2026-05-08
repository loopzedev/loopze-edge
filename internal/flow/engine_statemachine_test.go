// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow_test

import (
	"errors"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

const inspectorMachineJSON = `{` +
	`"id":"door","initial":"locked","context":{"failedAttempts":0},` +
	`"states":{` +
	`"locked":{"on":{"UNLOCK":"unlocked","TRIGGER_ALARM":"alarm"}},` +
	`"unlocked":{"on":{"LOCK":"locked"}},` +
	`"alarm":{"on":{"RESET":"locked"}}` +
	`}}`

func TestEngine_ListStateMachines(t *testing.T) {
	rig := newCatchRig(t)
	rig.engine.Registry().Register("statemachine", nodes.NewStateMachineNode, nodes.StateMachineTypeInfo())

	rig.deploy([]flow.Flow{{
		ID: "flow-sm",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-sm", Wires: [][]string{{"sm-a"}}},
			{ID: "sm-a", Type: "statemachine", Z: "flow-sm", Name: "Door A",
				Config: map[string]any{"machine": inspectorMachineJSON},
				Wires:  [][]string{{}, {}}},
			{ID: "sm-b", Type: "statemachine", Z: "flow-sm",
				Config: map[string]any{"machine": inspectorMachineJSON},
				Wires:  [][]string{{}, {}}},
		},
	}})

	got, err := rig.engine.ListStateMachines("flow-sm")
	if err != nil {
		t.Fatalf("ListStateMachines: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	// Find sm-a and sm-b regardless of order.
	byID := map[string]flow.StateMachineList{}
	for _, m := range got {
		byID[m.NodeID] = m
	}
	if a := byID["sm-a"]; a.Label != "Door A" {
		t.Errorf("sm-a label: got %q, want %q", a.Label, "Door A")
	}
	if a := byID["sm-a"]; a.CurrentState != "locked" {
		t.Errorf("sm-a state: got %q, want locked", a.CurrentState)
	}
	// sm-b has no Name — falls back to MachineID "door".
	if b := byID["sm-b"]; b.Label != "door" {
		t.Errorf("sm-b label: got %q, want %q", b.Label, "door")
	}
}

func TestEngine_ListStateMachines_UnknownFlow(t *testing.T) {
	rig := newCatchRig(t)
	rig.engine.Registry().Register("statemachine", nodes.NewStateMachineNode, nodes.StateMachineTypeInfo())

	rig.deploy([]flow.Flow{{
		ID: "flow-sm",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-sm", Wires: [][]string{}},
		},
	}})

	if _, err := rig.engine.ListStateMachines("nope"); !errors.Is(err, flow.ErrFlowNotFound) {
		t.Fatalf("err: got %v, want ErrFlowNotFound", err)
	}
}

func TestEngine_StateMachineSnapshot(t *testing.T) {
	rig := newCatchRig(t)
	rig.engine.Registry().Register("statemachine", nodes.NewStateMachineNode, nodes.StateMachineTypeInfo())

	rig.deploy([]flow.Flow{{
		ID: "flow-sm",
		Nodes: []flow.Node{
			{ID: "src-1", Type: "test-source", Z: "flow-sm", Wires: [][]string{{"sm-1"}}},
			{ID: "sm-1", Type: "statemachine", Z: "flow-sm",
				Config: map[string]any{"machine": inspectorMachineJSON},
				Wires:  [][]string{{}, {}}},
			{ID: "other-1", Type: "test-source", Z: "flow-sm", Wires: [][]string{}},
		},
	}})

	snap, err := rig.engine.StateMachineSnapshot("flow-sm", "sm-1")
	if err != nil {
		t.Fatalf("StateMachineSnapshot: %v", err)
	}
	if snap.MachineID != "door" {
		t.Errorf("MachineID: got %q, want door", snap.MachineID)
	}
	if snap.CurrentState != "locked" {
		t.Errorf("CurrentState: got %q, want locked", snap.CurrentState)
	}
	if len(snap.States) != 3 {
		t.Errorf("States: got %d entries, want 3", len(snap.States))
	}

	// Errors: unknown flow / unknown node / wrong type.
	cases := []struct {
		name      string
		flowID    string
		nodeID    string
		wantErr   error
		wantSnap  bool
	}{
		{"unknown flow", "nope", "sm-1", flow.ErrFlowNotFound, false},
		{"unknown node", "flow-sm", "nope", flow.ErrNodeNotFound, false},
		{"wrong type", "flow-sm", "other-1", flow.ErrNotStateMachine, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rig.engine.StateMachineSnapshot(tc.flowID, tc.nodeID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err: got %v, want %v", err, tc.wantErr)
			}
		})
	}
}
