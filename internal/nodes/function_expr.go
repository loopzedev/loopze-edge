// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"fmt"
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/scripting"
	scriptingexpr "github.com/loopzedev/loopze-edge/internal/scripting/expr"
)

// FunctionExprNode evaluates a single expr-lang expression for each incoming
// message. Idiomatic for pipeline transforms, aggregates, and conditional
// object construction — anywhere a single expression beats a JS function body
// in clarity and runtime cost.
//
// Env exposed to the expression:
//
//	payload  — msg.payload
//	topic    — msg.topic (string)
//	msg      — full message map (escape-hatch for less-common fields)
//
// Output behaviour is controlled by `passThrough`:
//
//	false (default) — emit a fresh message with topic + outputProperty=result
//	true            — keep the input message, overwrite outputProperty only
type FunctionExprNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Parsed from Properties.
	expression     string
	outputProperty string
	passThrough    bool

	// Compiled at Start; nil until then.
	program *scriptingexpr.Program
}

// NewFunctionExprNode is the NodeFactory for the function-expr node type.
func NewFunctionExprNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &FunctionExprNode{
		config:         config,
		outputProperty: "payload",
	}, nil
}

// Init parses configuration properties.
func (n *FunctionExprNode) Init() error {
	props := n.config.Properties

	if v, ok := props["expression"].(string); ok {
		n.expression = v
	}
	if v, ok := props["outputProperty"].(string); ok && v != "" {
		n.outputProperty = v
	}
	if v, ok := props["passThrough"].(bool); ok {
		n.passThrough = v
	}

	return nil
}

func (n *FunctionExprNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *FunctionExprNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *FunctionExprNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// Start compiles the expression. A compile failure puts the node in red status
// and propagates the error to the engine — the deploy reflects this in the UI.
func (n *FunctionExprNode) Start() error {
	if n.expression == "" {
		// Empty expression is permitted but emits a passthrough; tag the node
		// so the user can see that nothing meaningful is happening.
		if n.status != nil {
			n.status("yellow", "no expression")
		}
		return nil
	}

	envShape := map[string]any{
		"topic": "",
		"msg":   map[string]any{},
		// payload is left unset → AllowUndefinedVariables makes it dynamic
	}

	prog, err := scriptingexpr.Compile(n.expression, envShape)
	if err != nil {
		if n.status != nil {
			n.status("red", "compile: "+err.Error())
		}
		return fmt.Errorf("function-expr node %s: compile: %w", n.config.ID, err)
	}
	n.program = prog

	// Clear any leftover status from a previous failed deploy.
	if n.status != nil {
		n.status("", "")
	}

	slog.Info("function-expr node started", "node_id", n.config.ID)
	return nil
}

// HandleMessage evaluates the compiled expression for the message and emits
// the result on output port 0 according to the passThrough setting.
func (n *FunctionExprNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	if n.program == nil {
		// Empty expression — pass the message through unchanged.
		return [][]*flow.Message{{msg}}, nil
	}

	env := scripting.MessageEnv(msg)
	result, err := n.program.Run(env)
	if err != nil {
		return nil, fmt.Errorf("function-expr node %s: runtime: %w", n.config.ID, err)
	}

	out := scripting.ApplyResult(msg, result, n.outputProperty, n.passThrough)
	return [][]*flow.Message{{out}}, nil
}

// Stop is a no-op — the compiled program holds no resources beyond memory.
func (n *FunctionExprNode) Stop() error {
	slog.Info("function-expr node stopped", "node_id", n.config.ID)
	return nil
}

// FunctionExprTypeInfo returns the NodeTypeInfo for the function-expr node.
func FunctionExprTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "function-expr",
		Category:    "function",
		Label:       "Expr Function",
		Description: "Single expression — fast pipelines, transforms",
		Icon:        "mdi-function-variant",
		Defaults: map[string]any{
			"expression":     "payload",
			"outputProperty": "payload",
			"passThrough":    false,
		},
		Inputs:  1,
		Outputs: 1,
	}
}
