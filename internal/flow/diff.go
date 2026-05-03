// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import (
	"encoding/json"
	"reflect"
)

// DeployMode controls which nodes/flows are restarted during a deploy.
type DeployMode string

const (
	// DeployModifiedNodes restarts only nodes whose config, wires, or disabled
	// state have changed. Unchanged nodes keep running.
	DeployModifiedNodes DeployMode = "nodes"

	// DeployModifiedFlows restarts entire flows that contain at least one change.
	// Flows without changes keep running.
	DeployModifiedFlows DeployMode = "flows"

	// DeployFull stops all nodes and redeploys the entire workspace.
	DeployFull DeployMode = "full"

	// DeployRestart performs a full engine restart (Stop → Start → Deploy).
	// Resets all in-memory state including context stores.
	DeployRestart DeployMode = "restart"
)

// WorkspaceDiff describes the differences between two workspace versions.
type WorkspaceDiff struct {
	AddedFlows    []string // flow IDs present only in the new workspace
	RemovedFlows  []string // flow IDs present only in the old workspace
	ModifiedFlows []string // flow IDs with at least one change

	AddedNodes    []string // node IDs present only in the new workspace
	RemovedNodes  []string // node IDs present only in the old workspace
	ModifiedNodes []string // node IDs with changed config, wires, or disabled

	AddedConfigs    []string // config node IDs present only in the new workspace
	RemovedConfigs  []string // config node IDs present only in the old workspace
	ModifiedConfigs []string // config node IDs with changed config
}

// IsEmpty returns true if there are no differences between the workspaces.
func (d WorkspaceDiff) IsEmpty() bool {
	return len(d.AddedFlows) == 0 && len(d.RemovedFlows) == 0 && len(d.ModifiedFlows) == 0 &&
		len(d.AddedNodes) == 0 && len(d.RemovedNodes) == 0 && len(d.ModifiedNodes) == 0 &&
		len(d.AddedConfigs) == 0 && len(d.RemovedConfigs) == 0 && len(d.ModifiedConfigs) == 0
}

// DiffWorkspaces compares two workspace versions and returns a WorkspaceDiff
// describing what changed. Position (X/Y) changes are intentionally ignored
// because they are purely visual.
func DiffWorkspaces(old, new Workspace) WorkspaceDiff {
	var diff WorkspaceDiff

	// --- Flows ---
	oldFlows := indexFlows(old.Flows)
	newFlows := indexFlows(new.Flows)

	// --- Nodes ---
	oldNodes := indexNodes(old.Flows)
	newNodes := indexNodes(new.Flows)

	// Detect added/removed/modified nodes.
	modifiedNodeSet := make(map[string]bool)
	for id := range newNodes {
		if _, exists := oldNodes[id]; !exists {
			diff.AddedNodes = append(diff.AddedNodes, id)
			modifiedNodeSet[id] = true
		}
	}
	for id := range oldNodes {
		if _, exists := newNodes[id]; !exists {
			diff.RemovedNodes = append(diff.RemovedNodes, id)
			modifiedNodeSet[id] = true
		}
	}
	for id, newNode := range newNodes {
		oldNode, exists := oldNodes[id]
		if !exists {
			continue // already in AddedNodes
		}
		if nodeChanged(oldNode.Node, newNode.Node) {
			diff.ModifiedNodes = append(diff.ModifiedNodes, id)
			modifiedNodeSet[id] = true
		}
	}

	// Detect added/removed/modified flows.
	for id := range newFlows {
		if _, exists := oldFlows[id]; !exists {
			diff.AddedFlows = append(diff.AddedFlows, id)
		}
	}
	for id := range oldFlows {
		if _, exists := newFlows[id]; !exists {
			diff.RemovedFlows = append(diff.RemovedFlows, id)
		}
	}
	for id, newFlow := range newFlows {
		oldFlow, exists := oldFlows[id]
		if !exists {
			continue // already in AddedFlows
		}
		if flowChanged(oldFlow, newFlow, modifiedNodeSet) {
			diff.ModifiedFlows = append(diff.ModifiedFlows, id)
		}
	}

	// --- Config nodes ---
	oldConfigs := indexConfigs(old.Configs)
	newConfigs := indexConfigs(new.Configs)

	for id := range newConfigs {
		if _, exists := oldConfigs[id]; !exists {
			diff.AddedConfigs = append(diff.AddedConfigs, id)
		}
	}
	for id := range oldConfigs {
		if _, exists := newConfigs[id]; !exists {
			diff.RemovedConfigs = append(diff.RemovedConfigs, id)
		}
	}
	for id, newCfg := range newConfigs {
		oldCfg, exists := oldConfigs[id]
		if !exists {
			continue
		}
		if configNodeChanged(oldCfg, newCfg) {
			diff.ModifiedConfigs = append(diff.ModifiedConfigs, id)
		}
	}

	return diff
}

// ExpandConfigDependents adds nodes that reference a modified config node
// to the ModifiedNodes list (cascading restart).
func ExpandConfigDependents(diff *WorkspaceDiff, ws Workspace) {
	if len(diff.ModifiedConfigs) == 0 {
		return
	}

	changedConfigs := make(map[string]bool, len(diff.ModifiedConfigs))
	for _, id := range diff.ModifiedConfigs {
		changedConfigs[id] = true
	}

	alreadyModified := make(map[string]bool, len(diff.ModifiedNodes)+len(diff.AddedNodes))
	for _, id := range diff.ModifiedNodes {
		alreadyModified[id] = true
	}
	for _, id := range diff.AddedNodes {
		alreadyModified[id] = true
	}

	for _, f := range ws.Flows {
		for _, n := range f.Nodes {
			if alreadyModified[n.ID] {
				continue
			}
			if nodeReferencesConfig(n, changedConfigs) {
				diff.ModifiedNodes = append(diff.ModifiedNodes, n.ID)
				alreadyModified[n.ID] = true
			}
		}
	}
}

// nodeReferencesConfig checks if any string value in the node's config map
// matches a changed config node ID.
func nodeReferencesConfig(n Node, changedConfigs map[string]bool) bool {
	for _, v := range n.Config {
		if s, ok := v.(string); ok && changedConfigs[s] {
			return true
		}
	}
	return false
}

// nodeChanged returns true if a node's functional properties have changed.
// Position (X/Y) is intentionally excluded — it's purely visual.
func nodeChanged(old, new Node) bool {
	if old.Disabled != new.Disabled {
		return true
	}
	if old.Name != new.Name {
		return true
	}
	if !wiresEqual(old.Wires, new.Wires) {
		return true
	}
	if !mapEqual(old.Config, new.Config) {
		return true
	}
	return false
}

// flowChanged returns true if a flow's properties or node composition changed.
func flowChanged(old, new Flow, modifiedNodes map[string]bool) bool {
	if old.Disabled != new.Disabled {
		return true
	}
	if !reflect.DeepEqual(old.Env, new.Env) {
		return true
	}
	// Check if any node in this flow was added/removed/modified.
	oldNodeIDs := make(map[string]bool, len(old.Nodes))
	for _, n := range old.Nodes {
		oldNodeIDs[n.ID] = true
	}
	newNodeIDs := make(map[string]bool, len(new.Nodes))
	for _, n := range new.Nodes {
		newNodeIDs[n.ID] = true
	}
	// Node added or removed from this flow.
	for id := range newNodeIDs {
		if !oldNodeIDs[id] {
			return true
		}
	}
	for id := range oldNodeIDs {
		if !newNodeIDs[id] {
			return true
		}
	}
	// Any node in this flow was modified.
	for _, n := range new.Nodes {
		if modifiedNodes[n.ID] {
			return true
		}
	}
	return false
}

// configNodeChanged returns true if a config node's properties have changed.
func configNodeChanged(old, new ConfigNode) bool {
	return !mapEqual(old.Config, new.Config)
}

// wiresEqual compares two wire configurations for equality.
func wiresEqual(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// mapEqual compares two map[string]any for deep equality.
// Uses JSON serialization for reliable comparison of nested structures.
func mapEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

// --- Index helpers ---

type indexedNode struct {
	Node   Node
	FlowID string
}

func indexFlows(flows []Flow) map[string]Flow {
	m := make(map[string]Flow, len(flows))
	for _, f := range flows {
		m[f.ID] = f
	}
	return m
}

func indexNodes(flows []Flow) map[string]indexedNode {
	m := make(map[string]indexedNode)
	for _, f := range flows {
		for _, n := range f.Nodes {
			m[n.ID] = indexedNode{Node: n, FlowID: f.ID}
		}
	}
	return m
}

func indexConfigs(configs []ConfigNode) map[string]ConfigNode {
	m := make(map[string]ConfigNode, len(configs))
	for _, c := range configs {
		m[c.ID] = c
	}
	return m
}
