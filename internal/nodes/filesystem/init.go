// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package filesystem implements the file-in / folder-in / file-out flow nodes.
// See specifications/issues/NODE_FILESYSTEM.md for the full design.
package filesystem

import "github.com/loopzedev/loopze-edge/internal/nodes"

// init registers the filesystem node group with the central registry hub.
// The blank import in cmd/loopze/groups.go triggers this side effect.
func init() {
	nodes.RegisterGroup(nodes.Group{
		Name:        "filesystem",
		Description: "File and folder read, watch, and write",
		Nodes: []nodes.FlowNodeRegistration{
			{Type: "file-read", Factory: NewFileReadNode, Info: FileReadTypeInfo()},
			{Type: "file-watch", Factory: NewFileWatchNode, Info: FileWatchTypeInfo()},
			{Type: "file-out", Factory: NewFileOutNode, Info: FileOutTypeInfo()},
		},
	})
}
