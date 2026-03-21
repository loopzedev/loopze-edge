// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package flow

import (
	"testing"
)

func TestDiffWorkspaces_EmptyOld(t *testing.T) {
	old := Workspace{}
	new := Workspace{
		Flows: []Flow{{
			ID: "f1", Nodes: []Node{{ID: "n1", Type: "inject"}},
		}},
		Configs: []ConfigNode{{ID: "c1", Type: "mqtt-broker"}},
	}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.AddedFlows, "f1")
	assertContains(t, diff.AddedNodes, "n1")
	assertContains(t, diff.AddedConfigs, "c1")
	assertEmpty(t, "RemovedFlows", diff.RemovedFlows)
	assertEmpty(t, "ModifiedFlows", diff.ModifiedFlows)
}

func TestDiffWorkspaces_Identical(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "f1", Nodes: []Node{
				{ID: "n1", Type: "inject", Config: map[string]any{"interval": 1000}},
			},
		}},
	}

	diff := DiffWorkspaces(ws, ws)

	if !diff.IsEmpty() {
		t.Errorf("expected empty diff for identical workspaces, got %+v", diff)
	}
}

func TestDiffWorkspaces_PositionChangeIgnored(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", X: 100, Y: 200},
		},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", X: 300, Y: 400},
		},
	}}}

	diff := DiffWorkspaces(old, new)

	if !diff.IsEmpty() {
		t.Errorf("expected empty diff when only position changed, got %+v", diff)
	}
}

func TestDiffWorkspaces_NodeAdded(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{{ID: "n1", Type: "inject"}},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject"},
			{ID: "n2", Type: "debug"},
		},
	}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.AddedNodes, "n2")
	assertContains(t, diff.ModifiedFlows, "f1")
}

func TestDiffWorkspaces_NodeRemoved(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject"},
			{ID: "n2", Type: "debug"},
		},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{{ID: "n1", Type: "inject"}},
	}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.RemovedNodes, "n2")
	assertContains(t, diff.ModifiedFlows, "f1")
}

func TestDiffWorkspaces_NodeConfigModified(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", Config: map[string]any{"interval": float64(1000)}},
		},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", Config: map[string]any{"interval": float64(2000)}},
		},
	}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedNodes, "n1")
	assertContains(t, diff.ModifiedFlows, "f1")
}

func TestDiffWorkspaces_WireChanged(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", Wires: [][]string{{"n2"}}},
			{ID: "n2", Type: "debug"},
		},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{
			{ID: "n1", Type: "inject", Wires: [][]string{{"n3"}}},
			{ID: "n2", Type: "debug"},
			{ID: "n3", Type: "debug"},
		},
	}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedNodes, "n1")
	assertContains(t, diff.AddedNodes, "n3")
}

func TestDiffWorkspaces_DisabledToggle(t *testing.T) {
	old := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{{ID: "n1", Type: "inject", Disabled: false}},
	}}}
	new := Workspace{Flows: []Flow{{
		ID: "f1", Nodes: []Node{{ID: "n1", Type: "inject", Disabled: true}},
	}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedNodes, "n1")
}

func TestDiffWorkspaces_FlowAdded(t *testing.T) {
	old := Workspace{Flows: []Flow{{ID: "f1"}}}
	new := Workspace{Flows: []Flow{{ID: "f1"}, {ID: "f2"}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.AddedFlows, "f2")
	assertEmpty(t, "RemovedFlows", diff.RemovedFlows)
}

func TestDiffWorkspaces_FlowRemoved(t *testing.T) {
	old := Workspace{Flows: []Flow{{ID: "f1"}, {ID: "f2"}}}
	new := Workspace{Flows: []Flow{{ID: "f1"}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.RemovedFlows, "f2")
}

func TestDiffWorkspaces_FlowDisabledChanged(t *testing.T) {
	old := Workspace{Flows: []Flow{{ID: "f1", Disabled: false}}}
	new := Workspace{Flows: []Flow{{ID: "f1", Disabled: true}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedFlows, "f1")
}

func TestDiffWorkspaces_FlowEnvChanged(t *testing.T) {
	old := Workspace{Flows: []Flow{{ID: "f1", Env: map[string]string{"A": "1"}}}}
	new := Workspace{Flows: []Flow{{ID: "f1", Env: map[string]string{"A": "2"}}}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedFlows, "f1")
}

func TestDiffWorkspaces_ConfigNodeModified(t *testing.T) {
	old := Workspace{Configs: []ConfigNode{
		{ID: "c1", Type: "mqtt-broker", Config: map[string]any{"host": "localhost"}},
	}}
	new := Workspace{Configs: []ConfigNode{
		{ID: "c1", Type: "mqtt-broker", Config: map[string]any{"host": "remote.host"}},
	}}

	diff := DiffWorkspaces(old, new)

	assertContains(t, diff.ModifiedConfigs, "c1")
}

func TestExpandConfigDependents(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "f1", Nodes: []Node{
				{ID: "n1", Type: "mqtt-in", Config: map[string]any{"broker": "c1"}},
				{ID: "n2", Type: "mqtt-out", Config: map[string]any{"broker": "c1"}},
				{ID: "n3", Type: "debug"},
			},
		}},
		Configs: []ConfigNode{
			{ID: "c1", Type: "mqtt-broker", Config: map[string]any{"host": "new-host"}},
		},
	}

	diff := &WorkspaceDiff{
		ModifiedConfigs: []string{"c1"},
	}

	ExpandConfigDependents(diff, ws)

	assertContains(t, diff.ModifiedNodes, "n1")
	assertContains(t, diff.ModifiedNodes, "n2")
	// n3 does not reference c1
	if contains(diff.ModifiedNodes, "n3") {
		t.Errorf("n3 should not be marked as modified (does not reference config c1)")
	}
}

func TestExpandConfigDependents_SkipsAlreadyModified(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "f1", Nodes: []Node{
				{ID: "n1", Type: "mqtt-in", Config: map[string]any{"broker": "c1"}},
			},
		}},
	}

	diff := &WorkspaceDiff{
		ModifiedConfigs: []string{"c1"},
		ModifiedNodes:   []string{"n1"}, // already modified
	}

	ExpandConfigDependents(diff, ws)

	// n1 should appear only once
	count := 0
	for _, id := range diff.ModifiedNodes {
		if id == "n1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected n1 exactly once in ModifiedNodes, got %d", count)
	}
}

// --- Test helpers ---

func assertContains(t *testing.T, slice []string, id string) {
	t.Helper()
	if !contains(slice, id) {
		t.Errorf("expected slice to contain %q, got %v", id, slice)
	}
}

func assertEmpty(t *testing.T, name string, slice []string) {
	t.Helper()
	if len(slice) != 0 {
		t.Errorf("expected %s to be empty, got %v", name, slice)
	}
}

func contains(slice []string, id string) bool {
	for _, s := range slice {
		if s == id {
			return true
		}
	}
	return false
}
