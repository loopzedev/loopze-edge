// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dop251/goja"
	"github.com/niceclouds/flint/internal/flow"
	scriptingoja "github.com/niceclouds/flint/internal/scripting/goja"
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
//	msg                       – incoming message as a plain JS object
//	node.send(msg | [...])    – send to ports directly (alternative to return)
//	node.log(v)               – emit debug message (status "debug")
//	node.warn(v)              – emit debug message (status "warn")
//	node.error(v)             – emit debug message (status "error")
//	node.status(fill, text)   – update node status in the editor
//	console.log/warn/error    – alias for node.log/warn/error
//	global.get(key)           – read from global context KV
//	global.set(key, value)    – write to global context KV
//	global.delete(key)        – remove from global context KV
//	global.keys()             – list all global context keys
type FunctionNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc
	ctxMem  flow.ContextStore // global volatile memory store
	ctxPers flow.ContextStore // global persistent file-backed store
	flowMem  flow.ContextStore // flow-scoped volatile memory store
	flowPers flow.ContextStore // flow-scoped persistent file-backed store

	// Parsed from Properties.
	code    string // JS function body
	outputs int    // number of output ports

	// Runtime state (set in Start, cleared in Stop).
	vm        *goja.Runtime
	compiled  *goja.Program
	callable  goja.Callable // cached function reference from compiled program
	bufferWrp *scriptingoja.BufferWrapper

	// Node-scoped in-memory context (private to this node instance).
	// Survives across HandleMessage calls but lost on redeploy.
	nodeCtx map[string]any

	// Collects messages sent via node.send() during a HandleMessage call.
	// Reset at the start of each HandleMessage invocation.
	pendingSends [][]*flow.Message
}

// NewFunctionNode is the NodeFactory for the function node type.
func NewFunctionNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &FunctionNode{
		config:  config,
		outputs: 1,
		nodeCtx: make(map[string]any),
	}, nil
}

// Init parses configuration properties.
func (n *FunctionNode) Init() error {
	props := n.config.Properties

	if v, ok := props["func"].(string); ok {
		n.code = v
	}

	if v, ok := props["outputs"].(float64); ok && v >= 1 {
		n.outputs = int(v)
	}

	return nil
}

func (n *FunctionNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *FunctionNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *FunctionNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// SetContext implements flow.ContextProvider.
// Called by the engine after wiring, before Start().
func (n *FunctionNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start creates the Goja runtime and compiles the user script.
func (n *FunctionNode) Start() error {
	n.vm = goja.New()
	n.bufferWrp = scriptingoja.NewBufferWrapper(n.vm)
	n.registerGlobals()
	n.bufferWrp.Register()

	// Wrap the user's function body so they can write return statements.
	wrapped := fmt.Sprintf("(function(msg){ %s })", n.code)
	prog, err := goja.Compile("function.js", wrapped, false)
	if err != nil {
		return fmt.Errorf("function node %s: JS compile error: %w", n.config.ID, err)
	}
	n.compiled = prog

	// Run once to get the function value, then cache it.
	fn, err := n.vm.RunProgram(n.compiled)
	if err != nil {
		return fmt.Errorf("function node %s: %w", n.config.ID, err)
	}
	callable, ok := goja.AssertFunction(fn)
	if !ok {
		return fmt.Errorf("function node %s: compiled value is not callable", n.config.ID)
	}
	n.callable = callable

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

	// Call the cached function directly — no RunProgram on every message.
	result, err := n.callable(goja.Undefined(), jsMsg)
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

	// node.get(key) — read from node-scoped in-memory context
	_ = nodeObj.Set("get", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		val, ok := n.nodeCtx[key]
		if !ok {
			return goja.Undefined()
		}
		return n.vm.ToValue(val)
	})

	// node.set(key, value) — write to node-scoped in-memory context
	_ = nodeObj.Set("set", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		val := call.Argument(1).Export()
		n.nodeCtx[key] = val
		return goja.Undefined()
	})

	// node.delete(key) — remove from node-scoped in-memory context
	_ = nodeObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		delete(n.nodeCtx, key)
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

	// global.get/set/delete/keys
	//   default (no boolean arg) → memory store (volatile, fast)
	//   third arg true           → persistent store (file-backed, survives restarts)
	globalObj := n.vm.NewObject()

	_ = globalObj.Set("get", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(1))
		if store == nil {
			return goja.Null()
		}
		key := call.Argument(0).String()
		val, err := store.Get(key)
		if err != nil {
			slog.Warn("global.get error", "key", key, "error", err)
			return goja.Null()
		}
		if val == nil {
			return goja.Null()
		}
		return n.vm.ToValue(val)
	})

	_ = globalObj.Set("set", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(2))
		if store == nil {
			return goja.Undefined()
		}
		key := call.Argument(0).String()
		val := call.Argument(1).Export()
		if err := store.Set(key, val); err != nil {
			slog.Warn("global.set error", "key", key, "error", err)
		}
		return goja.Undefined()
	})

	_ = globalObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(1))
		if store == nil {
			return goja.Undefined()
		}
		key := call.Argument(0).String()
		if err := store.Delete(key); err != nil {
			slog.Warn("global.delete error", "key", key, "error", err)
		}
		return goja.Undefined()
	})

	_ = globalObj.Set("keys", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(0))
		if store == nil {
			return n.vm.ToValue([]string{})
		}
		keys, err := store.Keys()
		if err != nil {
			slog.Warn("global.keys error", "error", err)
			return n.vm.ToValue([]string{})
		}
		if keys == nil {
			return n.vm.ToValue([]string{})
		}
		return n.vm.ToValue(keys)
	})

	_ = n.vm.Set("global", globalObj)

	// flow.get/set/delete/keys — identical API to global but scoped to the current flow.
	//   flow.get(key)              → memory store
	//   flow.get(key, true)        → persistent store
	//   flow.set(key, value)       → memory store
	//   flow.set(key, value, true) → persistent store
	flowObj := n.vm.NewObject()

	_ = flowObj.Set("get", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(1))
		if store == nil {
			return goja.Null()
		}
		key := call.Argument(0).String()
		val, err := store.Get(key)
		if err != nil {
			slog.Warn("flow.get error", "key", key, "error", err)
			return goja.Null()
		}
		if val == nil {
			return goja.Null()
		}
		return n.vm.ToValue(val)
	})

	_ = flowObj.Set("set", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(2))
		if store == nil {
			return goja.Undefined()
		}
		key := call.Argument(0).String()
		val := call.Argument(1).Export()
		if err := store.Set(key, val); err != nil {
			slog.Warn("flow.set error", "key", key, "error", err)
		}
		return goja.Undefined()
	})

	_ = flowObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(1))
		if store == nil {
			return goja.Undefined()
		}
		key := call.Argument(0).String()
		if err := store.Delete(key); err != nil {
			slog.Warn("flow.delete error", "key", key, "error", err)
		}
		return goja.Undefined()
	})

	_ = flowObj.Set("keys", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(0))
		if store == nil {
			return n.vm.ToValue([]string{})
		}
		keys, err := store.Keys()
		if err != nil {
			slog.Warn("flow.keys error", "error", err)
			return n.vm.ToValue([]string{})
		}
		if keys == nil {
			return n.vm.ToValue([]string{})
		}
		return n.vm.ToValue(keys)
	})

	_ = n.vm.Set("flow", flowObj)
}

// messageToJS converts a *flow.Message to a Goja JS object.
func (n *FunctionNode) messageToJS(msg *flow.Message) goja.Value {
	obj := n.vm.NewObject()
	for k, v := range msg.DataView() {
		_ = obj.Set(k, v)
	}
	_ = obj.Set("_id", msg.ID())
	return obj
}

// jsValueToMessage converts a Goja value (JS object) back to a *flow.Message.
// Returns nil if the value is null or undefined.
//
// Buffer-shaped values (created via the JS Buffer.* API in this VM) are
// detected per top-level key and converted to []int, matching the wire format
// produced by mqtt-in's buffer mode. Without this, JS Buffer objects would
// leak across the message boundary as a map of internal fields (__bufferData,
// length, plus all method functions) — invalid as a payload for mqtt-out and
// cluttering the debug viewer.
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
		data[key] = scriptingoja.ExportValue(obj.Get(key))
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

// pickStore returns the global persistent store if the Goja argument is boolean
// true, otherwise the global memory store. Returns nil if the chosen store is not set.
func (n *FunctionNode) pickStore(persistArg goja.Value) flow.ContextStore {
	if persistArg != nil && !goja.IsUndefined(persistArg) && !goja.IsNull(persistArg) {
		if persistArg.ToBoolean() {
			return n.ctxPers
		}
	}
	return n.ctxMem
}

// pickFlowStore is the flow-scoped equivalent of pickStore.
func (n *FunctionNode) pickFlowStore(persistArg goja.Value) flow.ContextStore {
	if persistArg != nil && !goja.IsUndefined(persistArg) && !goja.IsNull(persistArg) {
		if persistArg.ToBoolean() {
			return n.flowPers
		}
	}
	return n.flowMem
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
