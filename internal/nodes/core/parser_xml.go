// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	mxj "github.com/clbanning/mxj/v2"
	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/nodes"
)

func init() {
	// Ensure mxj escapes &, <, >, ', " in text values so generated XML is
	// well-formed. Without this flag mxj leaves bare & in output, which
	// causes xml.Decoder to reject it on the next parse.
	mxj.XMLEscapeChars(true)
}

var (
	errXMLTypeMismatch = errors.New("xml type error")
	errXMLRootMissing  = errors.New("xml root required")
)

// XMLParserNode converts a message property between an XML string/buffer
// representation and a structured Go value (map[string]any).
//
// Action modes:
//   - auto:      string/[]byte → parse, anything else → stringify
//   - parse:     forces parse, errors on non-string/[]byte
//   - stringify: forces marshal, errors on string/[]byte input
//
// Map representation follows the mxj convention:
//   - attributes are prefixed with "-" (e.g. "-id")
//   - text content of mixed elements is stored under "#text"
//   - repeated sibling elements become []any slices
type XMLParserNode struct {
	config flow.NodeConfig
	nodes.BaseNode
	property    string
	action      string
	root        string
	indent      int
	declaration bool

	inErrorState bool
}

// NewXMLParserNode is the NodeFactory for the xml node type.
func NewXMLParserNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &XMLParserNode{config: config}, nil
}

func (n *XMLParserNode) Init() error {
	props := n.config.Properties

	n.property = nodes.StringVal(props, "property", "payload")
	n.action = nodes.StringVal(props, "action", "auto")
	switch n.action {
	case "auto", "parse", "stringify":
	default:
		slog.Warn("xml node: unknown action, falling back to auto",
			"node_id", n.config.ID, "action", n.action)
		n.action = "auto"
	}

	// Read root directly so that an explicitly blank value is preserved and
	// caught as an error at stringify time — StringVal would replace "" with
	// the fallback, masking the misconfiguration.
	if v, ok := props["root"].(string); ok {
		n.root = v
	} else {
		n.root = "root"
	}
	n.indent = clampInt(nodes.IntVal(props, "indent", 0), 0, 8)
	n.declaration = nodes.BoolVal(props, "declaration", true)

	return nil
}

func (n *XMLParserNode) Start() error {
	slog.Info("xml parser node started",
		"node_id", n.config.ID,
		"property", n.property,
		"action", n.action,
		"root", n.root,
		"indent", n.indent,
		"declaration", n.declaration,
	)
	return nil
}

func (n *XMLParserNode) Stop() error {
	slog.Info("xml parser node stopped", "node_id", n.config.ID)
	return nil
}

func (n *XMLParserNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
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
		return nil, fmt.Errorf("xml: %w", err)
	}

	if n.inErrorState && n.Status != nil {
		n.Status("", "")
	}
	n.inErrorState = false

	msg.Set(n.property, result)
	return [][]*flow.Message{{msg}}, nil
}

func (n *XMLParserNode) convert(value any) (any, error) {
	switch n.action {
	case "parse":
		return parseXMLValue(value)
	case "stringify":
		return stringifyXMLValue(value, n.root, n.indent, n.declaration)
	case "auto":
		if isStringish(value) {
			return parseXMLValue(value)
		}
		return stringifyXMLValue(value, n.root, n.indent, n.declaration)
	default:
		return nil, fmt.Errorf("unknown action %q", n.action)
	}
}

func (n *XMLParserNode) errorLabel(err error) string {
	switch {
	case errors.Is(err, errXMLTypeMismatch):
		return "xml type error"
	case errors.Is(err, errXMLRootMissing):
		return "xml root required"
	default:
		return "xml parse error"
	}
}

func parseXMLValue(v any) (any, error) {
	var data []byte
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("parse: value is nil: %w", errXMLTypeMismatch)
	case string:
		data = []byte(x)
	case []byte:
		data = x
	default:
		return nil, fmt.Errorf("parse: expected string or []byte, got %T: %w", v, errXMLTypeMismatch)
	}
	m, err := mxj.NewMapXml(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return m.Old(), nil
}

func stringifyXMLValue(v any, root string, indent int, declaration bool) (string, error) {
	switch v.(type) {
	case string:
		return "", fmt.Errorf("stringify: value already a string: %w", errXMLTypeMismatch)
	case []byte:
		return "", fmt.Errorf("stringify: value is a byte buffer: %w", errXMLTypeMismatch)
	}
	if root == "" {
		return "", fmt.Errorf("stringify: root element name required: %w", errXMLRootMissing)
	}

	mv, err := mxj.NewMapXml([]byte("<" + root + "/>"))
	if err != nil {
		return "", fmt.Errorf("stringify: build map: %w", err)
	}
	// Replace the empty root with the actual value.
	inputMap, ok := toStringKeyedMap(v)
	if !ok {
		// Wrap non-map values under the root element as text content.
		inputMap = map[string]any{root: v}
	} else {
		inputMap = map[string]any{root: inputMap}
	}
	_ = mv

	m := mxj.Map(inputMap)
	var b []byte
	if indent > 0 {
		b, err = m.XmlIndent("", strings.Repeat(" ", indent))
	} else {
		b, err = m.Xml()
	}
	if err != nil {
		return "", fmt.Errorf("stringify: %w", err)
	}

	result := string(b)
	if declaration {
		result = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + result
	}
	return result, nil
}

// toStringKeyedMap converts v to map[string]any if possible.
func toStringKeyedMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	default:
		return nil, false
	}
}

// XMLParserTypeInfo returns the NodeTypeInfo for the xml parser node.
func XMLParserTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "xml",
		Category:    "parser",
		Label:       "XML",
		Description: "Convert XML strings to objects and back",
		Icon:        "xml",
		Defaults: map[string]any{
			"property":    "payload",
			"action":      "auto",
			"root":        "root",
			"indent":      0,
			"declaration": true,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
