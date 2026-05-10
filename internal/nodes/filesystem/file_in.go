// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// FileInNode reads a file on demand (mode=read), watches it for changes
// (mode=watch), or both (mode=read+watch). Optional incremental mode reads
// only bytes appended since the last read using a persistent cursor.
//
// Phase 1: skeleton only — palette registration and lifecycle stubs.
type FileInNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode
}

// NewFileInNode is the NodeFactory for the file-in node type.
func NewFileInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileInNode{cfg: cfg}, nil
}

// Init parses the node configuration. Phase 1: no-op.
func (n *FileInNode) Init() error { return nil }

// Start begins watcher goroutines (in watch / read+watch mode). Phase 1: no-op.
func (n *FileInNode) Start() error {
	slog.Info("file-in node started (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// Stop releases the watcher and drains background goroutines. Phase 1: no-op.
func (n *FileInNode) Stop() error {
	slog.Info("file-in node stopped (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage performs a one-shot read in mode=read. Phase 1: drops the
// message silently so flows wired to a placeholder file-in still execute.
func (n *FileInNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	return nil, nil
}

// FileInTypeInfo returns the NodeTypeInfo for registering the file-in node.
func FileInTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "file-in",
		Category:    "filesystem",
		Label:       "File In",
		Description: "Read file content and/or watch for changes",
		Icon:        "file-in",
		Defaults: map[string]any{
			"mode":         "read",
			"path":         "",
			"encoding":     "auto",
			"watchEvents":  []string{"write", "create"},
			"debounceMs":   50,
			"incremental":  false,
			"fromStart":    false,
			"delimiter":    "\n",
			"maxLineBytes": 1048576,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
