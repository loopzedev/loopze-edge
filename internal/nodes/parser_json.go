// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// errJSONTypeMismatch marks errors caused by an input value that does not
// match the configured action (e.g. parse on a non-string, or stringify on
// a string). Used to derive the status label without parsing error strings.
var errJSONTypeMismatch = errors.New("json type error")

// JSONParserNode converts a message property between a JSON string/buffer
// representation and a structured Go value.
//
// Action modes:
//   - auto:      string/[]byte → parse, anything else → stringify
//   - parse:     forces parse, errors on non-string/[]byte
//   - stringify: forces marshal, errors on string/[]byte input
type JSONParserNode struct {
	config flow.NodeConfig
	BaseNode
	property string
	action   string
	indent   int

	// inErrorState tracks whether the last conversion failed. When the next
	// one succeeds we clear the status pill so the node visibly recovers.
	// Safe without locking: HandleMessage runs on a single goroutine per node.
	inErrorState bool
}

// NewJSONParserNode is the NodeFactory for the json node type.
func NewJSONParserNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &JSONParserNode{config: config}, nil
}

// Init reads configuration with sensible defaults and clamps the indent into
// a UI-aligned range.
func (n *JSONParserNode) Init() error {
	props := n.config.Properties

	n.property = stringVal(props, "property", "payload")
	n.action = stringVal(props, "action", "auto")
	switch n.action {
	case "auto", "parse", "stringify":
	default:
		slog.Warn("json node: unknown action, falling back to auto",
			"node_id", n.config.ID, "action", n.action)
		n.action = "auto"
	}

	n.indent = clampInt(intVal(props, "indent", 0), 0, 8)

	return nil
}

// Start logs the node startup.
func (n *JSONParserNode) Start() error {
	slog.Info("json parser node started",
		"node_id", n.config.ID,
		"property", n.property,
		"action", n.action,
		"indent", n.indent,
	)
	return nil
}

// Stop is a no-op for the json parser node.
func (n *JSONParserNode) Stop() error {
	slog.Info("json parser node stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage performs the configured conversion on msg.<property> and
// writes the result back to the same path. Type and parse errors are
// returned so the engine routes them to Catch nodes.
func (n *JSONParserNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	value := msg.Get(n.property)
	result, err := n.convert(value)
	if err != nil {
		if n.Status != nil {
			n.Status("red", n.errorLabel(err))
		}
		n.inErrorState = true
		return nil, fmt.Errorf("json: %w", err)
	}

	if n.inErrorState && n.Status != nil {
		n.Status("", "")
	}
	n.inErrorState = false

	msg.Set(n.property, result)
	return [][]*flow.Message{{msg}}, nil
}

// convert dispatches to the parse/stringify branches based on the action.
func (n *JSONParserNode) convert(value any) (any, error) {
	switch n.action {
	case "parse":
		return parseJSONValue(value)
	case "stringify":
		return stringifyJSONValue(value, n.indent)
	case "auto":
		if isStringish(value) {
			return parseJSONValue(value)
		}
		return stringifyJSONValue(value, n.indent)
	default:
		return nil, fmt.Errorf("unknown action %q", n.action)
	}
}

// errorLabel produces the status text shown on the node when conversion
// fails. Type errors get a dedicated label so users can distinguish "wrong
// shape of input" from "malformed JSON".
func (n *JSONParserNode) errorLabel(err error) string {
	if errors.Is(err, errJSONTypeMismatch) {
		return "json type error"
	}
	return "json parse error"
}

func isStringish(v any) bool {
	switch v.(type) {
	case string, []byte:
		return true
	default:
		return false
	}
}

func parseJSONValue(v any) (any, error) {
	var data []byte
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("parse: value is nil: %w", errJSONTypeMismatch)
	case string:
		data = []byte(x)
	case []byte:
		data = x
	default:
		return nil, fmt.Errorf("parse: expected string or []byte, got %T: %w", v, errJSONTypeMismatch)
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return parsed, nil
}

func stringifyJSONValue(v any, indent int) (string, error) {
	switch v.(type) {
	case string:
		return "", fmt.Errorf("stringify: value already a string: %w", errJSONTypeMismatch)
	case []byte:
		return "", fmt.Errorf("stringify: value is a byte buffer: %w", errJSONTypeMismatch)
	}
	var (
		b   []byte
		err error
	)
	if indent > 0 {
		b, err = json.MarshalIndent(v, "", strings.Repeat(" ", indent))
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return "", fmt.Errorf("stringify: %w", err)
	}
	return string(b), nil
}

// intVal extracts an int from a properties map, accepting both int and
// float64 (the latter is what JSON-decoded defaults arrive as).
func intVal(m map[string]any, key string, fallback int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return fallback
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// JSONParserTypeInfo returns the NodeTypeInfo for the json parser node.
func JSONParserTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "json",
		Category:    "parser",
		Label:       "JSON",
		Description: "Convert JSON strings to objects and back",
		Icon:        "json",
		Defaults: map[string]any{
			"property": "payload",
			"action":   "auto",
			"indent":   0,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
