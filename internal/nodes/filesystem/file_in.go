// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// File-in modes.
const (
	fileInModeRead      = "read"
	fileInModeWatch     = "watch"
	fileInModeReadWatch = "read+watch"
)

// FileInNode reads a file on demand (mode=read), watches it for changes
// (mode=watch), or both (mode=read+watch). Optional incremental mode reads
// only bytes appended since the last read using a persistent cursor.
//
// Phase 3: only mode=read is functional. Watch and incremental land in
// phases 4 and 5; their config fields are parsed and validated here so
// stored workspaces survive the upgrades.
type FileInNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode

	mode         string
	path         string
	encoding     string
	rootJail     string
	watchEvents  []string // parsed; unused until phase 4
	debounceMs   int      // parsed; unused until phase 4
	incremental  bool     // parsed; unused until phase 5
	fromStart    bool     // parsed; unused until phase 5
	delimiter    string   // parsed; unused until phase 5
	maxLineBytes int64    // parsed; unused until phase 5
}

// NewFileInNode is the NodeFactory for the file-in node type.
func NewFileInNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileInNode{cfg: cfg}, nil
}

// Init parses every config field, validates the mode and encoding, and
// returns a deploy-time error for invalid combinations.
func (n *FileInNode) Init() error {
	p := n.cfg.Properties
	n.mode = nodes.StringVal(p, "mode", fileInModeRead)
	n.path = nodes.StringVal(p, "path", "")
	n.encoding = nodes.StringVal(p, "encoding", "auto")
	n.rootJail = nodes.StringVal(p, "rootJail", "")
	n.watchEvents = stringSlice(p, "watchEvents", []string{"write", "create"})
	n.debounceMs = nodes.IntVal(p, "debounceMs", 50)
	n.incremental = nodes.BoolVal(p, "incremental", false)
	n.fromStart = nodes.BoolVal(p, "fromStart", false)
	n.delimiter = nodes.StringVal(p, "delimiter", "\n")
	n.maxLineBytes = int64(nodes.IntVal(p, "maxLineBytes", 1048576))

	switch n.mode {
	case fileInModeRead, fileInModeWatch, fileInModeReadWatch:
	default:
		return fmt.Errorf("file-in %s: invalid mode %q", n.cfg.ID, n.mode)
	}
	switch n.encoding {
	case "auto", "utf-8", "binary":
	default:
		return fmt.Errorf("file-in %s: invalid encoding %q", n.cfg.ID, n.encoding)
	}
	return nil
}

// Start logs deployment. Watch goroutines arrive in phase 4.
func (n *FileInNode) Start() error {
	slog.Info("file-in node started",
		"node_id", n.cfg.ID,
		"mode", n.mode,
		"path", n.path,
	)
	if n.mode != fileInModeRead {
		slog.Warn("file-in: watch not yet implemented (phase 4)",
			"node_id", n.cfg.ID,
			"mode", n.mode,
		)
	}
	if n.incremental {
		slog.Warn("file-in: incremental not yet implemented (phase 5)",
			"node_id", n.cfg.ID,
		)
	}
	return nil
}

// Stop releases the watcher in later phases. Phase 3: no-op.
func (n *FileInNode) Stop() error {
	slog.Info("file-in node stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage performs a one-shot file read in mode=read. In watch /
// read+watch mode the node has no input port (set by the workspace), so
// HandleMessage is normally not invoked; if it is, the message is dropped.
func (n *FileInNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	if n.mode != fileInModeRead {
		return nil, nil
	}

	resolved, err := resolvePath(n.path, msg, n.rootJail)
	if err != nil {
		n.statusError("path error")
		return nil, fmt.Errorf("file-in: %w", err)
	}

	data, info, err := readWholeFile(resolved)
	if err != nil {
		n.statusError(fileReadErrorLabel(err))
		return nil, fmt.Errorf("file-in: %w", err)
	}

	encoding := resolveEncoding(resolved, n.encoding)
	msg.SetPayload(decodePayload(data, encoding))
	msg.Set("filename", resolved)
	msg.Set("encoding", encoding)
	msg.Set("size", info.Size())

	if n.Status != nil {
		n.Status("blue", "read "+humanSize(int(info.Size())))
	}
	return [][]*flow.Message{{msg}}, nil
}

// readWholeFile reads the entire file into memory and returns its FileInfo
// alongside the bytes. A separate function so phase 4/5 can share the same
// not-found / permission-denied error mapping.
func readWholeFile(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return data, info, nil
}

// fileReadErrorLabel maps a stat / read error to a stable status label.
func fileReadErrorLabel(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "not found"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	default:
		return "read error"
	}
}

func (n *FileInNode) statusError(label string) {
	if n.Status != nil {
		n.Status("red", label)
	}
}

// stringSlice extracts a []string from the property bag. JSON-decoded values
// arrive as []any with string elements; native callers may pass []string
// directly. Falls back to the supplied default for missing or wrong-typed
// entries.
func stringSlice(m map[string]any, key string, fallback []string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return fallback
		}
		return out
	}
	return fallback
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
