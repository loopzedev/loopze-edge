// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"github.com/loopzedev/loopze-edge/internal/nodes/filesystem"
)

// CSVOutNode serialises a payload as CSV and writes it to a file in one
// step. Owning both the CSV writer and the file handle removes the
// architectural friction of the csv (stringify) + file-out (append) chain:
// the header decision can read on-disk file state directly, so a redeploy
// or restart never produces a duplicate header in append mode.
//
// See specifications/issues/PARSER_CSV_NODE.md (Phase 2) for the contract.
type CSVOutNode struct {
	cfg flow.NodeConfig
	nodes.BaseNode

	// CSV format (mirrors CSVParserNode where applicable).
	property   string
	header     bool
	columns    []string
	delimiter  rune
	quoteChar  rune
	forceQuote bool
	newline    string

	// File destination.
	path       string
	mode       string // ModeAppend | ModeOverwrite | ModeCreate
	encoding   string
	createDirs bool
	rootJail   string

	inErrorState bool
}

// NewCSVOutNode is the NodeFactory for the csv-out node type.
func NewCSVOutNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
	return &CSVOutNode{cfg: cfg}, nil
}

func (n *CSVOutNode) Init() error {
	p := n.cfg.Properties

	n.property = nodes.StringVal(p, "property", "payload")
	n.header = nodes.BoolVal(p, "header", true)
	n.columns = parseCSVColumnsProp(p["columns"])
	n.delimiter = firstRune(p["delimiter"], ',')
	n.quoteChar = firstRune(p["quoteChar"], '"')
	n.forceQuote = nodes.BoolVal(p, "forceQuote", false)

	n.newline = nodes.StringVal(p, "newline", "\n")
	if n.newline != "\n" && n.newline != "\r\n" {
		slog.Warn("csv-out node: invalid newline, falling back to \\n",
			"node_id", n.cfg.ID, "newline", n.newline)
		n.newline = "\n"
	}

	n.path = nodes.StringVal(p, "path", "")
	n.mode = nodes.StringVal(p, "mode", filesystem.ModeAppend)
	switch n.mode {
	case filesystem.ModeAppend, filesystem.ModeOverwrite, filesystem.ModeCreate:
	default:
		return fmt.Errorf("csv-out %s: invalid mode %q", n.cfg.ID, n.mode)
	}

	n.encoding = nodes.StringVal(p, "encoding", "auto")
	switch n.encoding {
	case "auto", "utf-8":
	default:
		return fmt.Errorf("csv-out %s: invalid encoding %q (only auto and utf-8 supported)", n.cfg.ID, n.encoding)
	}

	n.createDirs = nodes.BoolVal(p, "createDirs", false)
	n.rootJail = nodes.StringVal(p, "rootJail", "")

	// Phase 1 quote-char restriction carries over here.
	if n.quoteChar != '"' {
		slog.Warn("csv-out node: custom quoteChar not supported in phase 1; using \"",
			"node_id", n.cfg.ID, "configured", string(n.quoteChar))
		n.quoteChar = '"'
	}
	return nil
}

func (n *CSVOutNode) Start() error {
	slog.Info("csv-out node started",
		"node_id", n.cfg.ID,
		"path", n.path,
		"mode", n.mode,
		"header", n.header,
		"delimiter", string(n.delimiter),
	)
	return nil
}

func (n *CSVOutNode) Stop() error {
	slog.Info("csv-out node stopped", "node_id", n.cfg.ID)
	return nil
}

// HandleMessage resolves the path, decides whether the header is needed
// based on on-disk state, serialises the payload, writes it atomically,
// and forwards the original message with write metadata attached.
func (n *CSVOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	resolved, err := filesystem.ResolvePath(n.path, msg, n.rootJail)
	if err != nil {
		return n.fail("path error", err)
	}

	if n.createDirs {
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return n.fail("mkdir error", err)
		}
	}

	needHeader, err := n.decideHeader(resolved)
	if err != nil {
		return n.fail(filesystem.FileWriteErrorLabel(err), err)
	}

	opts := csvWriteOpts{
		Columns:    n.columns,
		Header:     n.header,
		Delimiter:  n.delimiter,
		QuoteChar:  n.quoteChar,
		ForceQuote: n.forceQuote,
		Newline:    n.newline,
	}
	body, _, emittedHeader, err := stringifyCSVRaw(msg.Get(n.property), opts, needHeader)
	if err != nil {
		return n.fail("csv encode error", err)
	}

	if err := filesystem.WriteFile(resolved, body, n.mode); err != nil {
		return n.fail(filesystem.FileWriteErrorLabel(err), err)
	}

	n.clearError(len(body))

	info, statErr := os.Stat(resolved)
	var fileSize int64
	if statErr == nil {
		fileSize = info.Size()
	}

	msg.Set("filename", resolved)
	msg.Set("bytesWritten", len(body))
	msg.Set("fileSize", fileSize)
	msg.Set("headerWritten", emittedHeader)
	return [][]*flow.Message{{msg}}, nil
}

// decideHeader stats the resolved path and folds the configured mode into
// the boolean handed to stringifyCSVRaw. Single-writer semantics — see
// PARSER_CSV_NODE.md §8.
func (n *CSVOutNode) decideHeader(path string) (bool, error) {
	if !n.header {
		return false, nil
	}
	info, statErr := os.Stat(path)
	switch {
	case statErr == nil:
		switch n.mode {
		case filesystem.ModeAppend:
			// Empty existing file → still needs the header.
			return info.Size() == 0, nil
		case filesystem.ModeOverwrite:
			return true, nil
		case filesystem.ModeCreate:
			return false, fmt.Errorf("%w: %s", filesystem.ErrFileExists, path)
		}
	case errors.Is(statErr, os.ErrNotExist):
		return true, nil
	default:
		return false, statErr
	}
	return false, fmt.Errorf("unknown mode %q", n.mode)
}

func (n *CSVOutNode) fail(label string, err error) ([][]*flow.Message, error) {
	if n.Status != nil {
		n.Status("red", label)
	}
	n.inErrorState = true
	return nil, fmt.Errorf("csv-out: %w", err)
}

func (n *CSVOutNode) clearError(bytesWritten int) {
	if n.inErrorState && n.Status != nil {
		n.Status("blue", "wrote "+filesystem.HumanSize(bytesWritten))
	} else if n.Status != nil {
		n.Status("blue", "wrote "+filesystem.HumanSize(bytesWritten))
	}
	n.inErrorState = false
}

// CSVOutTypeInfo returns the NodeTypeInfo for the csv-out node.
func CSVOutTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "csv-out",
		Category:    "parser",
		Label:       "CSV Out",
		Description: "Serialise a payload as CSV and write it to a file",
		Icon:        "csv-out",
		Defaults: map[string]any{
			"property":   "payload",
			"header":     true,
			"columns":    "",
			"delimiter":  ",",
			"quoteChar":  "\"",
			"forceQuote": false,
			"newline":    "\n",
			"path":       "",
			"mode":       filesystem.ModeAppend,
			"encoding":   "auto",
			"createDirs": false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
