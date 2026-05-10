// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// File-out modes.
const (
	fileOutModeOverwrite = "overwrite"
	fileOutModeAppend    = "append"
	fileOutModeCreate    = "create"
)

// errFileExists is the sentinel returned when create mode hits an existing
// file. Used by the status mapper to derive a stable error label.
var errFileExists = errors.New("file already exists")

// FileOutNode writes msg.payload to a file. Modes:
//   - overwrite: replace existing content (or create if missing)
//   - append:    append to existing content (or create if missing)
//   - create:    create a new file; fail if it already exists
//
// Pass-through is implicit: the input message is forwarded unchanged on
// the output port after a successful write so downstream nodes can chain.
type FileOutNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode

	path          string
	mode          string
	encoding      string
	createDirs    bool
	appendNewline bool
	rootJail      string
}

// NewFileOutNode is the NodeFactory for the file-out node type.
func NewFileOutNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &FileOutNode{cfg: cfg}, nil
}

// Init parses configuration and validates the mode + encoding choices.
// Path validation happens per-message because of {{mustache}} resolution.
func (n *FileOutNode) Init() error {
	p := n.cfg.Properties
	n.path = nodes.StringVal(p, "path", "")
	n.mode = nodes.StringVal(p, "mode", fileOutModeOverwrite)
	n.encoding = nodes.StringVal(p, "encoding", "auto")
	n.createDirs = nodes.BoolVal(p, "createDirs", false)
	n.appendNewline = nodes.BoolVal(p, "appendNewline", false)
	n.rootJail = nodes.StringVal(p, "rootJail", "")

	switch n.mode {
	case fileOutModeOverwrite, fileOutModeAppend, fileOutModeCreate:
	default:
		return fmt.Errorf("file-out %s: invalid mode %q", n.cfg.ID, n.mode)
	}
	switch n.encoding {
	case "auto", "utf-8", "binary":
	default:
		return fmt.Errorf("file-out %s: invalid encoding %q", n.cfg.ID, n.encoding)
	}
	return nil
}

// Start logs deployment.
func (n *FileOutNode) Start() error {
	slog.Info("file-out node started",
		"node_id", n.cfg.ID,
		"mode", n.mode,
		"path", n.path,
	)
	return nil
}

// Stop is a no-op — files open per-message and close before HandleMessage returns.
func (n *FileOutNode) Stop() error {
	slog.Info("file-out node stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage resolves the path, encodes the payload, writes it, and
// forwards the original message on output 0.
func (n *FileOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	resolved, err := resolvePath(n.path, msg, n.rootJail)
	if err != nil {
		n.statusError("path error")
		return nil, fmt.Errorf("file-out: %w", err)
	}

	encoding := resolveEncoding(resolved, n.encoding)
	data, err := encodePayload(msg.Get("payload"), encoding)
	if err != nil {
		n.statusError("encode error")
		return nil, fmt.Errorf("file-out: %w", err)
	}
	if n.appendNewline {
		data = append(data, '\n')
	}

	if n.createDirs {
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			n.statusError("mkdir error")
			return nil, fmt.Errorf("file-out: mkdir: %w", err)
		}
	}

	if err := writeFile(resolved, data, n.mode); err != nil {
		n.statusError(fileWriteErrorLabel(err))
		return nil, fmt.Errorf("file-out: %w", err)
	}

	if n.Status != nil {
		n.Status("blue", "wrote "+humanSize(len(data)))
	}
	return [][]*flow.Message{{msg}}, nil
}

// writeFile opens the file with the right flags for the configured mode and
// writes data in a single call.
func writeFile(path string, data []byte, mode string) error {
	var flag int
	switch mode {
	case fileOutModeAppend:
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case fileOutModeCreate:
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	default: // overwrite
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", errFileExists, path)
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// fileWriteErrorLabel maps an OS error to a stable status string.
func fileWriteErrorLabel(err error) string {
	switch {
	case errors.Is(err, errFileExists):
		return "file exists"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	case strings.Contains(err.Error(), "no space left"):
		return "disk full"
	default:
		return "write error"
	}
}

func (n *FileOutNode) statusError(label string) {
	if n.Status != nil {
		n.Status("red", label)
	}
}

// humanSize renders a byte count like "1.5 kB" / "2.3 MB" for status text.
func humanSize(b int) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(b)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
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
