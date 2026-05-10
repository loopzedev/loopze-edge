// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// FolderInNode lists folder entries on demand, watches a folder for changes,
// or both. Optional incremental mode emits only entries whose modTime has
// advanced since the last scan, using a persistent per-file modTime map.
//
// Phase 1: skeleton only — palette registration and lifecycle stubs.
type FolderInNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode
}

// NewFolderInNode is the NodeFactory for the folder-in node type.
func NewFolderInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FolderInNode{cfg: cfg}, nil
}

// Init parses the node configuration. Phase 1: no-op.
func (n *FolderInNode) Init() error { return nil }

// Start begins watcher goroutines. Phase 1: no-op.
func (n *FolderInNode) Start() error {
	slog.Info("folder-in node started (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// Stop releases the watcher and drains background goroutines. Phase 1: no-op.
func (n *FolderInNode) Stop() error {
	slog.Info("folder-in node stopped (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage performs a one-shot listing in mode=read. Phase 1: drops.
func (n *FolderInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// FolderInTypeInfo returns the NodeTypeInfo for registering the folder-in node.
func FolderInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "folder-in",
		Category:    "filesystem",
		Label:       "Folder In",
		Description: "List folder entries and/or watch for changes",
		Icon:        "folder-in",
		Defaults: map[string]any{
			"mode":             "read",
			"path":             "",
			"recursive":        false,
			"glob":             "*",
			"watchEvents":      []string{"create", "write", "remove", "rename"},
			"sendAs":           "individual",
			"includeContent":   false,
			"contentEncoding":  "auto",
			"maxFileSizeBytes": 1048576,
			"debounceMs":       100,
			"incremental":      false,
			"fromStart":        false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
