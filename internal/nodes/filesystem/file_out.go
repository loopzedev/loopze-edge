// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// FileOutNode writes msg.payload to a file. Modes: overwrite, append,
// or create-if-missing. Optional pass-through emits the original msg
// downstream after the write completes.
//
// Phase 1: skeleton only — palette registration and lifecycle stubs.
type FileOutNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode
}

// NewFileOutNode is the NodeFactory for the file-out node type.
func NewFileOutNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileOutNode{cfg: cfg}, nil
}

// Init parses the node configuration. Phase 1: no-op.
func (n *FileOutNode) Init() error { return nil }

// Start logs node startup. Phase 1: no-op.
func (n *FileOutNode) Start() error {
	slog.Info("file-out node started (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// Stop logs shutdown. Phase 1: no-op.
func (n *FileOutNode) Stop() error {
	slog.Info("file-out node stopped (skeleton)", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage writes msg.payload to disk. Phase 1: forwards msg unchanged
// to the pass-through output port (if wired) without writing anything.
func (n *FileOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	return [][]*flow.Message{{msg}}, nil
}

// FileOutTypeInfo returns the NodeTypeInfo for registering the file-out node.
func FileOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "file-out",
		Category:    "filesystem",
		Label:       "File Write",
		Description: "Write or append payload to a file",
		Icon:        "file-out",
		Defaults: map[string]any{
			"path":          "",
			"mode":          "overwrite",
			"encoding":      "auto",
			"createDirs":    false,
			"appendNewline": false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
