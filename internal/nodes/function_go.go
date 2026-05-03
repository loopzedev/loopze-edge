// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
	scriptingyaegi "github.com/loopzedev/loopze-edge/internal/scripting/yaegi"
)

// FunctionGoNode runs user-supplied Go code (interpreted by Yaegi) for each
// incoming message. The user writes a `package main` containing a `handle`
// function whose signature the engine reflects on.
//
// Three signature shapes are supported (illustrative):
//
//	func handle(payload any) any
//	func handle(payload []map[string]any) []map[string]any
//	func handle(payload []SomeStruct) any                  // typed, JSON boundary
//	func handle(payload any, node loopzenode.Node)          // multi-output via node.Send
//
// See internal/scripting/yaegi for the full Node interface and supported
// stdlib subset.
type FunctionGoNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	ctxMem   flow.ContextStore
	ctxPers  flow.ContextStore
	flowMem  flow.ContextStore
	flowPers flow.ContextStore

	// Parsed from Properties.
	code    string
	outputs int

	// Compiled at Start; nil until then.
	program *scriptingyaegi.Program

	// Node-scoped in-memory context (private to this node instance).
	nodeCtx map[string]any

	// Collects messages sent via node.Send() during a single HandleMessage.
	// Reset at the start of each invocation.
	pendingSends [][]*flow.Message
}

// NewFunctionGoNode is the NodeFactory for the function-go node type.
func NewFunctionGoNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &FunctionGoNode{
		config:  config,
		outputs: 1,
		nodeCtx: make(map[string]any),
	}, nil
}

// Init parses configuration properties.
func (n *FunctionGoNode) Init() error {
	props := n.config.Properties

	if v, ok := props["code"].(string); ok {
		n.code = v
	}
	if v, ok := props["outputs"].(float64); ok && v >= 1 {
		n.outputs = int(v)
	}

	return nil
}

func (n *FunctionGoNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *FunctionGoNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *FunctionGoNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// SetContext implements flow.ContextProvider. Called by the engine after
// wiring, before Start().
func (n *FunctionGoNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start compiles the user code. A compile failure puts the node in red status
// and propagates the error to the engine — the deploy reflects this in the UI.
func (n *FunctionGoNode) Start() error {
	if n.code == "" {
		if n.status != nil {
			n.status("yellow", "no code")
		}
		return nil
	}

	prog, err := scriptingyaegi.New().Compile(n.code)
	if err != nil {
		if n.status != nil {
			n.status("red", "compile: "+err.Error())
		}
		return fmt.Errorf("function-go node %s: compile: %w", n.config.ID, err)
	}
	n.program = prog

	// Clear any leftover status from a previous failed deploy.
	if n.status != nil {
		n.status("", "")
	}

	slog.Info("function-go node started", "node_id", n.config.ID, "outputs", n.outputs)
	return nil
}

// HandleMessage runs the compiled handle function for the message. If handle
// returns a value it goes out on port 0; node.Send() calls collect into
// pendingSends and override the return value.
func (n *FunctionGoNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}
	if n.program == nil {
		return [][]*flow.Message{{msg}}, nil
	}

	// Reset per-call state.
	n.pendingSends = make([][]*flow.Message, n.outputs)

	api := &goNodeAPI{node: n}
	result, err := n.program.Run(msg.Payload(), api)
	if err != nil {
		return nil, fmt.Errorf("function-go node %s: runtime: %w", n.config.ID, err)
	}

	if n.hasPendingSends() {
		return n.pendingSends, nil
	}
	if result == nil {
		return nil, nil
	}

	// Single return value → port 0, payload-overwrite, preserve other fields.
	out := msg
	out.SetPayload(result)
	return [][]*flow.Message{{out}}, nil
}

// Stop is a no-op — the Yaegi interpreter holds no resources beyond memory.
func (n *FunctionGoNode) Stop() error {
	slog.Info("function-go node stopped", "node_id", n.config.ID)
	return nil
}

// FunctionGoTypeInfo returns the NodeTypeInfo for the function-go node.
func FunctionGoTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "function-go",
		Category:    "function",
		Label:       "Go Function",
		Description: "Go code — fast batch processing",
		Icon:        "mdi-language-go",
		Defaults: map[string]any{
			"code":    "package main\n\nfunc handle(payload any) any {\n    return payload\n}\n",
			"outputs": 1,
		},
		Inputs:  1,
		Outputs: 1,
	}
}

// hasPendingSends returns true if node.Send() was called at least once.
func (n *FunctionGoNode) hasPendingSends() bool {
	for _, msgs := range n.pendingSends {
		if len(msgs) > 0 {
			return true
		}
	}
	return false
}

// payloadToMessage builds an output flow.Message from a value passed to
// node.Send(). If the value is already a *flow.Message it's used as-is;
// otherwise a fresh message is created with the value as payload.
func (n *FunctionGoNode) payloadToMessage(v any) *flow.Message {
	if v == nil {
		return nil
	}
	if m, ok := v.(*flow.Message); ok {
		return m
	}
	out := flow.NewMessage()
	out.SetPayload(v)
	return out
}

// emitDebug publishes a debug message via the debug callback.
func (n *FunctionGoNode) emitDebug(status string, args []any) {
	if n.debug == nil {
		return
	}
	var payload any
	if len(args) == 1 {
		payload = args[0]
	} else if len(args) > 1 {
		payload = args
	}
	n.debug(flow.DebugMessage{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    status,
		Payload:   payload,
		Format:    detectFormat(payload),
		Property:  "function-go",
		NodeName:  n.config.Name,
	})
}

// pickGlobalStore returns the persistent store when persistent is true,
// otherwise the in-memory store. Returns nil if the chosen store isn't
// configured (which makes Get/Set/Delete no-ops).
func (n *FunctionGoNode) pickGlobalStore(persistent bool) flow.ContextStore {
	if persistent {
		return n.ctxPers
	}
	return n.ctxMem
}

func (n *FunctionGoNode) pickFlowStoreP(persistent bool) flow.ContextStore {
	if persistent {
		return n.flowPers
	}
	return n.flowMem
}

// goNodeAPI adapts FunctionGoNode to the yaegi.Node interface that user code
// receives as its second argument. Kept as a thin pass-through so the node
// struct stays focused on lifecycle rather than the wide API surface.
type goNodeAPI struct {
	node *FunctionGoNode
}

func (a *goNodeAPI) Send(port int, v any) {
	if port < 0 || port >= a.node.outputs {
		return
	}
	if msg := a.node.payloadToMessage(v); msg != nil {
		a.node.pendingSends[port] = append(a.node.pendingSends[port], msg)
	}
}

func (a *goNodeAPI) Log(args ...any)   { a.node.emitDebug("debug", args) }
func (a *goNodeAPI) Warn(args ...any)  { a.node.emitDebug("warn", args) }
func (a *goNodeAPI) Error(args ...any) { a.node.emitDebug("error", args) }

func (a *goNodeAPI) Status(fill, text string) {
	if a.node.status != nil {
		a.node.status(fill, text)
	}
}

func (a *goNodeAPI) Get(key string) any { return a.node.nodeCtx[key] }
func (a *goNodeAPI) Set(key string, val any) {
	a.node.nodeCtx[key] = val
}
func (a *goNodeAPI) Delete(key string) { delete(a.node.nodeCtx, key) }

func (a *goNodeAPI) FlowGet(key string) any        { return ctxGet(a.node.pickFlowStoreP(false), key) }
func (a *goNodeAPI) FlowSet(key string, val any)   { ctxSet(a.node.pickFlowStoreP(false), key, val) }
func (a *goNodeAPI) FlowDelete(key string)         { ctxDelete(a.node.pickFlowStoreP(false), key) }
func (a *goNodeAPI) FlowGetP(key string) any       { return ctxGet(a.node.pickFlowStoreP(true), key) }
func (a *goNodeAPI) FlowSetP(key string, val any)  { ctxSet(a.node.pickFlowStoreP(true), key, val) }
func (a *goNodeAPI) FlowDeleteP(key string)        { ctxDelete(a.node.pickFlowStoreP(true), key) }

func (a *goNodeAPI) GlobalGet(key string) any       { return ctxGet(a.node.pickGlobalStore(false), key) }
func (a *goNodeAPI) GlobalSet(key string, val any)  { ctxSet(a.node.pickGlobalStore(false), key, val) }
func (a *goNodeAPI) GlobalDelete(key string)        { ctxDelete(a.node.pickGlobalStore(false), key) }
func (a *goNodeAPI) GlobalGetP(key string) any      { return ctxGet(a.node.pickGlobalStore(true), key) }
func (a *goNodeAPI) GlobalSetP(key string, val any) { ctxSet(a.node.pickGlobalStore(true), key, val) }
func (a *goNodeAPI) GlobalDeleteP(key string)       { ctxDelete(a.node.pickGlobalStore(true), key) }

// ctxGet/ctxSet/ctxDelete are nil-store-tolerant helpers — when context wiring
// is missing (e.g. in unit tests) they degrade to no-ops rather than panicking.
func ctxGet(store flow.ContextStore, key string) any {
	if store == nil {
		return nil
	}
	val, err := store.Get(key)
	if err != nil {
		slog.Warn("function-go context get error", "key", key, "error", err)
		return nil
	}
	return val
}

func ctxSet(store flow.ContextStore, key string, val any) {
	if store == nil {
		return
	}
	if err := store.Set(key, val); err != nil {
		slog.Warn("function-go context set error", "key", key, "error", err)
	}
}

func ctxDelete(store flow.ContextStore, key string) {
	if store == nil {
		return
	}
	if err := store.Delete(key); err != nil {
		slog.Warn("function-go context delete error", "key", key, "error", err)
	}
}
