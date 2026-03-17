// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dop251/goja"
	"github.com/niceclouds/flint/internal/flow"
)

// FunctionNode executes user-supplied JavaScript for each incoming message.
// It has 1 input and a configurable number of outputs (default 1).
//
// The user writes the function body (not the whole function):
//
//	msg.payload = msg.payload * 2;
//	return msg;
//
// JS API available inside the function:
//
//	msg                      – incoming message as a plain JS object
//	node.send(msg | [...])   – send to ports directly (alternative to return)
//	node.log(v)              – emit debug message (status "debug")
//	node.warn(v)             – emit debug message (status "warn")
//	node.error(v)            – emit debug message (status "error")
//	node.status(fill, text)  – update node status in the editor
//	console.log/warn/error   – alias for node.log/warn/error
type FunctionNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Parsed from Properties.
	code    string // JS function body
	outputs int    // number of output ports

	// Runtime state (set in Start, cleared in Stop).
	vm       *goja.Runtime
	compiled *goja.Program

	// Collects messages sent via node.send() during a HandleMessage call.
	// Reset at the start of each HandleMessage invocation.
	pendingSends [][]*flow.Message
}

// NewFunctionNode is the NodeFactory for the function node type.
func NewFunctionNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &FunctionNode{
		config:  config,
		outputs: 1,
	}, nil
}

// Init parses configuration properties.
func (n *FunctionNode) Init() error {
	props := n.config.Properties

	if v, ok := props["func"].(string); ok {
		n.code = v
	}
	if n.code == "" {
		n.code = "return msg;"
	}

	if v, ok := props["outputs"].(float64); ok && v >= 1 {
		n.outputs = int(v)
	}

	return nil
}

func (n *FunctionNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *FunctionNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *FunctionNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// Start creates the Goja runtime and compiles the user script.
func (n *FunctionNode) Start() error {
	n.vm = goja.New()
	n.registerGlobals()

	// Wrap the user's function body so they can write return statements.
	wrapped := fmt.Sprintf("(function(msg){ %s })", n.code)
	prog, err := goja.Compile("function.js", wrapped, false)
	if err != nil {
		return fmt.Errorf("function node %s: JS compile error: %w", n.config.ID, err)
	}
	n.compiled = prog

	slog.Info("function node started",
		"node_id", n.config.ID,
		"outputs", n.outputs,
	)
	return nil
}

// HandleMessage executes the compiled JS for the incoming message.
func (n *FunctionNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	// Reset per-call state.
	n.pendingSends = make([][]*flow.Message, n.outputs)

	// Convert Go message → JS object.
	jsMsg := n.messageToJS(msg)

	// Retrieve the compiled function and call it with the JS message.
	fn, err := n.vm.RunProgram(n.compiled)
	if err != nil {
		return nil, fmt.Errorf("function node %s: %w", n.config.ID, err)
	}

	callable, ok := goja.AssertFunction(fn)
	if !ok {
		return nil, fmt.Errorf("function node %s: compiled value is not callable", n.config.ID)
	}

	result, err := callable(goja.Undefined(), jsMsg)
	if err != nil {
		return nil, fmt.Errorf("function node %s: runtime error: %w", n.config.ID, err)
	}

	// If node.send() was called, use those; otherwise interpret return value.
	if n.hasPendingSends() {
		return n.pendingSends, nil
	}

	return n.resultToOutputs(result), nil
}

// Stop releases the Goja runtime.
func (n *FunctionNode) Stop() error {
	if n.vm != nil {
		n.vm.Interrupt("stop")
		n.vm = nil
	}
	slog.Info("function node stopped", "node_id", n.config.ID)
	return nil
}

// FunctionTypeInfo returns the NodeTypeInfo for the function node.
func FunctionTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "function",
		Category:    "function",
		Label:       "Function",
		Description: "Executes JavaScript code for each incoming message",
		Icon:        "mdi-function",
		Defaults: map[string]any{
			"func":    "return msg;",
			"outputs": 1,
		},
		Inputs:  1,
		Outputs: 1,
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// registerGlobals sets up the node and console global objects in the VM.
func (n *FunctionNode) registerGlobals() {
	nodeObj := n.vm.NewObject()

	// node.send(msg | [msg, ...])
	_ = nodeObj.Set("send", func(call goja.FunctionCall) goja.Value {
		n.jsSend(call.Argument(0))
		return goja.Undefined()
	})

	// node.log / warn / error
	_ = nodeObj.Set("log", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("debug", call.Argument(0))
		return goja.Undefined()
	})
	_ = nodeObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("warn", call.Argument(0))
		return goja.Undefined()
	})
	_ = nodeObj.Set("error", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("error", call.Argument(0))
		return goja.Undefined()
	})

	// node.status(fill, text)
	_ = nodeObj.Set("status", func(call goja.FunctionCall) goja.Value {
		fill := call.Argument(0).String()
		text := call.Argument(1).String()
		if n.status != nil {
			n.status(fill, text)
		}
		return goja.Undefined()
	})

	_ = n.vm.Set("node", nodeObj)

	// console.log / warn / error → node counterparts
	consoleObj := n.vm.NewObject()
	_ = consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("debug", call.Argument(0))
		return goja.Undefined()
	})
	_ = consoleObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("warn", call.Argument(0))
		return goja.Undefined()
	})
	_ = consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
		n.emitDebug("error", call.Argument(0))
		return goja.Undefined()
	})
	_ = n.vm.Set("console", consoleObj)
}

// messageToJS converts a *flow.Message to a Goja JS object.
func (n *FunctionNode) messageToJS(msg *flow.Message) goja.Value {
	obj := n.vm.NewObject()
	for k, v := range msg.Data() {
		_ = obj.Set(k, v)
	}
	_ = obj.Set("_id", msg.ID())
	return obj
}

// jsValueToMessage converts a Goja value (JS object) back to a *flow.Message.
// Returns nil if the value is null or undefined.
func (n *FunctionNode) jsValueToMessage(val goja.Value) *flow.Message {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return nil
	}
	obj := val.ToObject(n.vm)
	if obj == nil {
		return nil
	}
	data := make(map[string]any)
	for _, key := range obj.Keys() {
		data[key] = obj.Get(key).Export()
	}
	return flow.NewMessageFromData(data)
}

// resultToOutputs converts the return value of the user function to [][]*Message.
//
//   - null / undefined → nil (send nothing)
//   - JS object → port 0 gets the message
//   - JS array → each element maps to the corresponding output port
func (n *FunctionNode) resultToOutputs(val goja.Value) [][]*flow.Message {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return nil
	}

	exported := val.Export()

	// Check if the exported value is a slice (JS array).
	if arr, ok := exported.([]interface{}); ok {
		outputs := make([][]*flow.Message, n.outputs)
		for i, item := range arr {
			if i >= n.outputs {
				break
			}
			if item == nil {
				continue
			}
			// Convert each array element back to a JS value, then to a message.
			jsVal := n.vm.ToValue(item)
			if msg := n.jsValueToMessage(jsVal); msg != nil {
				outputs[i] = []*flow.Message{msg}
			}
		}
		return outputs
	}

	// Single object → port 0.
	if msg := n.jsValueToMessage(val); msg != nil {
		outputs := make([][]*flow.Message, n.outputs)
		outputs[0] = []*flow.Message{msg}
		return outputs
	}

	return nil
}

// jsSend handles calls to node.send() from within the user script.
func (n *FunctionNode) jsSend(val goja.Value) {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return
	}

	exported := val.Export()

	if arr, ok := exported.([]interface{}); ok {
		for i, item := range arr {
			if i >= n.outputs || item == nil {
				continue
			}
			jsVal := n.vm.ToValue(item)
			if msg := n.jsValueToMessage(jsVal); msg != nil {
				n.pendingSends[i] = append(n.pendingSends[i], msg)
			}
		}
		return
	}

	// Single message → port 0.
	if msg := n.jsValueToMessage(val); msg != nil {
		n.pendingSends[0] = append(n.pendingSends[0], msg)
	}
}

// hasPendingSends returns true if node.send() was called at least once.
func (n *FunctionNode) hasPendingSends() bool {
	for _, msgs := range n.pendingSends {
		if len(msgs) > 0 {
			return true
		}
	}
	return false
}

// emitDebug publishes a debug message via the debug callback.
func (n *FunctionNode) emitDebug(status string, val goja.Value) {
	if n.debug == nil {
		return
	}
	var payload any
	if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		payload = val.Export()
	}
	n.debug(flow.DebugMessage{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    status,
		Payload:   payload,
		Format:    detectFormat(payload),
		Property:  "function",
		NodeName:  n.config.Name,
	})
}
