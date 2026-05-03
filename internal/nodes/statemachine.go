// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/niceclouds/loopze/internal/flow"
)

// StateMachineNode runs a finite state machine defined via JSON with optional
// JS guards and actions. It has 1 input and 2 outputs:
//   - Port 0: state-change messages (state, previousState, event, context)
//   - Port 1: action-generated messages (via node.send() inside actions)
//
// Input: msg.topic = event name, msg.payload = event data.
//
// JS API in guards/actions:
//
//	node.send(msg)            – send a message to port 1
//	node.log/warn/error(v)    – emit debug messages
//	node.status(fill, text)   – update node status in the editor
//	global.get/set/delete/keys – global context KV
//	flow.get/set/delete/keys   – flow context KV
type StateMachineNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc
	errFn  flow.ErrorFunc

	// Context stores (injected via ContextProvider).
	ctxMem   flow.ContextStore
	ctxPers  flow.ContextStore
	flowMem  flow.ContextStore
	flowPers flow.ContextStore

	// Parsed from Properties.
	machineJSON string // raw JSON machine definition
	guardsCode  string // JS code for guards
	actionsCode string // JS code for actions
	persist     bool   // persist state across deploys

	// Runtime state.
	vm     *goja.Runtime
	engine *SMEngine
	mu     sync.Mutex
	timers map[string]*time.Timer
	done   chan struct{}

	// Collects messages sent via node.send() during action execution.
	pendingSends []*flow.Message

	// currentMsg is the message currently being processed in HandleMessage,
	// used so guard/action error reports carry the originating message.
	// nil for events triggered by after-timers.
	currentMsg *flow.Message
}

// NewStateMachineNode is the NodeFactory for the statemachine node type.
func NewStateMachineNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &StateMachineNode{
		config: config,
		timers: make(map[string]*time.Timer),
		done:   make(chan struct{}),
	}, nil
}

// Init parses configuration properties.
func (n *StateMachineNode) Init() error {
	props := n.config.Properties

	if v, ok := props["machine"].(string); ok {
		n.machineJSON = v
	}
	if n.machineJSON == "" {
		return fmt.Errorf("statemachine node %s: missing 'machine' property", n.config.ID)
	}

	if v, ok := props["guards"].(string); ok {
		n.guardsCode = v
	}
	if v, ok := props["actions"].(string); ok {
		n.actionsCode = v
	}
	if v, ok := props["persist"].(bool); ok {
		n.persist = v
	}

	// Validate the machine definition can be parsed.
	var def MachineDefinition
	if err := json.Unmarshal([]byte(n.machineJSON), &def); err != nil {
		return fmt.Errorf("statemachine node %s: invalid machine JSON: %w", n.config.ID, err)
	}

	return nil
}

func (n *StateMachineNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *StateMachineNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *StateMachineNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// SetError implements flow.ErrorProvider so guard/action errors raised inside
// the JS runtime can be surfaced to Catch Nodes via the engine's error fan-out.
// Without this, runtime errors in guards or actions would only appear in the
// server log and never reach the flow.
func (n *StateMachineNode) SetError(fn flow.ErrorFunc) { n.errFn = fn }

// SetContext implements flow.ContextProvider.
func (n *StateMachineNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start creates the Goja runtime, compiles guards/actions, and initializes the engine.
func (n *StateMachineNode) Start() error {
	var def MachineDefinition
	if err := json.Unmarshal([]byte(n.machineJSON), &def); err != nil {
		return fmt.Errorf("statemachine node %s: %w", n.config.ID, err)
	}

	// Set up Goja VM for guards/actions.
	n.vm = goja.New()
	n.registerGlobals()

	// Compile guards and actions separately.
	var guardsObj, actionsObj *goja.Object

	if n.guardsCode != "" {
		wrapped := fmt.Sprintf("(function(){ %s })()", n.guardsCode)
		prog, err := goja.Compile("guards.js", wrapped, false)
		if err != nil {
			return fmt.Errorf("statemachine node %s: guards compile error: %w", n.config.ID, err)
		}
		result, err := n.vm.RunProgram(prog)
		if err != nil {
			return fmt.Errorf("statemachine node %s: guards runtime error: %w", n.config.ID, err)
		}
		if result != nil && !goja.IsNull(result) && !goja.IsUndefined(result) {
			guardsObj = result.ToObject(n.vm)
		}
	}

	if n.actionsCode != "" {
		wrapped := fmt.Sprintf("(function(){ %s })()", n.actionsCode)
		prog, err := goja.Compile("actions.js", wrapped, false)
		if err != nil {
			return fmt.Errorf("statemachine node %s: actions compile error: %w", n.config.ID, err)
		}
		result, err := n.vm.RunProgram(prog)
		if err != nil {
			return fmt.Errorf("statemachine node %s: actions runtime error: %w", n.config.ID, err)
		}
		if result != nil && !goja.IsNull(result) && !goja.IsUndefined(result) {
			actionsObj = result.ToObject(n.vm)
		}
	}

	// Build guard callback.
	evalGuard := func(name string, ctx map[string]any, event map[string]any) bool {
		if guardsObj == nil {
			return true
		}
		fnVal := guardsObj.Get(name)
		if fnVal == nil || goja.IsUndefined(fnVal) {
			slog.Warn("statemachine: guard not found", "guard", name, "node_id", n.config.ID)
			return true
		}
		callable, ok := goja.AssertFunction(fnVal)
		if !ok {
			slog.Warn("statemachine: guard is not a function", "guard", name, "node_id", n.config.ID)
			return true
		}
		result, err := callable(goja.Undefined(), n.vm.ToValue(ctx), n.vm.ToValue(event))
		if err != nil {
			slog.Warn("statemachine: guard error", "guard", name, "error", err, "node_id", n.config.ID)
			if n.errFn != nil {
				n.errFn(fmt.Errorf("guard %q: %w", name, err), n.currentMsg)
			}
			return false
		}
		return result.ToBoolean()
	}

	// Build action callback.
	execAction := func(name string, ctx map[string]any, event map[string]any) {
		if actionsObj == nil {
			return
		}
		fnVal := actionsObj.Get(name)
		if fnVal == nil || goja.IsUndefined(fnVal) {
			slog.Warn("statemachine: action not found", "action", name, "node_id", n.config.ID)
			return
		}
		callable, ok := goja.AssertFunction(fnVal)
		if !ok {
			slog.Warn("statemachine: action is not a function", "action", name, "node_id", n.config.ID)
			return
		}
		if _, err := callable(goja.Undefined(), n.vm.ToValue(ctx), n.vm.ToValue(event)); err != nil {
			slog.Warn("statemachine: action error", "action", name, "error", err, "node_id", n.config.ID)
			if n.errFn != nil {
				n.errFn(fmt.Errorf("action %q: %w", name, err), n.currentMsg)
			}
		}
	}

	n.engine = NewSMEngine(def, evalGuard, execAction)

	// Validate the machine.
	if err := n.engine.Validate(); err != nil {
		return fmt.Errorf("statemachine node %s: %w", n.config.ID, err)
	}

	// Restore persisted state if available.
	if n.persist && n.flowPers != nil {
		n.restoreState()
	}

	// Set initial status.
	if n.status != nil {
		n.status("green", "State: "+n.engine.CurrentState())
	}

	// Start after-timers for the initial state.
	n.startDelayTimers()

	slog.Info("statemachine node started",
		"node_id", n.config.ID,
		"state", n.engine.CurrentState(),
	)
	return nil
}

// HandleMessage processes incoming messages as state machine events.
func (n *StateMachineNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Reset per-call pending sends.
	n.pendingSends = nil

	// Track the originating message so guard/action error reports can
	// preserve it on the catch output. Cleared on return.
	n.currentMsg = msg
	defer func() { n.currentMsg = nil }()

	topic := msg.Topic()
	if topic == "" {
		return nil, nil
	}

	// Extract event data from payload.
	var eventData map[string]any
	if payload, ok := msg.Payload().(map[string]any); ok {
		eventData = payload
	} else if msg.Payload() != nil {
		eventData = map[string]any{"payload": msg.Payload()}
	}

	result := n.engine.Send(topic, eventData)

	if result == nil {
		return nil, nil
	}

	outputs := make([][]*flow.Message, 2)

	if result.Changed {
		// Cancel old timers and start new ones.
		n.cancelDelayTimers()
		n.startDelayTimers()

		// Update node status.
		if n.status != nil {
			n.status("green", "State: "+result.To)
		}

		// Persist state if enabled.
		if n.persist && n.flowPers != nil {
			n.persistState()
		}
	}

	// Port 0: state change message.
	outMsg := flow.NewMessage()
	outMsg.SetTopic("statemachine")
	outMsg.SetPayload(map[string]any{
		"state":         result.To,
		"previousState": result.From,
		"event": map[string]any{
			"topic":   msg.Topic(),
			"payload": msg.Payload(),
		},
		"context":       result.Context,
		"machineId":     n.engine.def.ID,
		"changed":       result.Changed,
	})
	outputs[0] = []*flow.Message{outMsg}

	// Port 1: action-generated messages.
	if len(n.pendingSends) > 0 {
		outputs[1] = n.pendingSends
	}

	return outputs, nil
}

// Stop cancels all timers and releases the Goja runtime.
func (n *StateMachineNode) Stop() error {
	close(n.done)
	n.mu.Lock()
	n.cancelDelayTimers()
	n.mu.Unlock()

	if n.vm != nil {
		n.vm.Interrupt("stop")
		n.vm = nil
	}
	slog.Info("statemachine node stopped", "node_id", n.config.ID)
	return nil
}

// StateMachineTypeInfo returns the NodeTypeInfo for the statemachine node.
func StateMachineTypeInfo() flow.NodeTypeInfo {
	defaultMachine := `{` +
		`"id":"doorLock",` +
		`"initial":"locked",` +
		`"context":{"failedAttempts":0,"maxAttempts":3},` +
		`"states":{` +
		`"locked":{` +
		`"onEntry":["logState"],` +
		`"on":{` +
		`"UNLOCK":[{"target":"unlocked","guard":"pinCorrect"},{"target":"locked","actions":["countFailure"]}],` +
		`"TRIGGER_ALARM":"alarm"` +
		`}` +
		`},` +
		`"unlocked":{` +
		`"onEntry":["resetFailures","logState"],` +
		`"after":{"10000":"locked"},` +
		`"on":{"LOCK":"locked"}` +
		`},` +
		`"alarm":{` +
		`"onEntry":["triggerAlarm","logState"],` +
		`"on":{"RESET":{"target":"locked","actions":["resetFailures"]}}` +
		`}` +
		`}` +
		`}`

	defaultGuards := "return {\n" +
		"  pinCorrect: function(ctx, event) {\n" +
		"    return event.pin === \"1234\";\n" +
		"  }\n" +
		"}"

	defaultActions := "return {\n" +
		"  countFailure: function(ctx, event) {\n" +
		"    ctx.failedAttempts = (ctx.failedAttempts || 0) + 1;\n" +
		"    node.log(\"Failed attempt #\" + ctx.failedAttempts);\n" +
		"    if (ctx.failedAttempts >= ctx.maxAttempts) {\n" +
		"      node.warn(\"Too many attempts\");\n" +
		"      node.send({ topic: \"TRIGGER_ALARM\", payload: {} });\n" +
		"    }\n" +
		"  },\n" +
		"  resetFailures: function(ctx, event) {\n" +
		"    ctx.failedAttempts = 0;\n" +
		"  },\n" +
		"  triggerAlarm: function(ctx, event) {\n" +
		"    node.warn(\"ALARM: Too many failed attempts!\");\n" +
		"    node.send({ topic: \"alarm/door\", payload: { reason: \"too_many_attempts\", attempts: ctx.failedAttempts } });\n" +
		"  },\n" +
		"  logState: function(ctx, event) {\n" +
		"    node.log(\"State entered\");\n" +
		"  }\n" +
		"}"

	return flow.NodeTypeInfo{
		Type:        "statemachine",
		Category:    "function",
		Label:       "State Machine",
		Description: "Finite state machine with guards, actions, and delayed transitions",
		Icon:        "mdi-state-machine",
		Defaults: map[string]any{
			"machine": defaultMachine,
			"guards":  defaultGuards,
			"actions": defaultActions,
			"persist": false,
		},
		Inputs:  1,
		Outputs: 2,
	}
}

// ── Timer Management ─────────────────────────────────────────────────────────

// startDelayTimers starts after-timers for the current state.
func (n *StateMachineNode) startDelayTimers() {
	delays := n.engine.ActiveDelays()
	for msStr := range delays {
		ms, err := ParseDelayMs(msStr)
		if err != nil {
			slog.Warn("statemachine: invalid delay", "delay", msStr, "node_id", n.config.ID)
			continue
		}
		dur := time.Duration(ms) * time.Millisecond
		delayKey := msStr // capture for closure

		timer := time.AfterFunc(dur, func() {
			select {
			case <-n.done:
				return
			default:
			}
			n.handleAfterTimer(delayKey)
		})
		n.timers[msStr] = timer
	}
}

// handleAfterTimer executes a delayed transition directly from the timer goroutine.
// It performs the state change internally and sends outputs downstream.
func (n *StateMachineNode) handleAfterTimer(delayMs string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Reset per-call pending sends.
	n.pendingSends = nil

	result := n.engine.SendAfter(delayMs)
	if result == nil || !result.Changed {
		return
	}

	// Cancel old timers and start new ones for the new state.
	n.cancelDelayTimers()
	n.startDelayTimers()

	// Update node status.
	if n.status != nil {
		n.status("green", "State: "+result.To)
	}

	// Persist state if enabled.
	if n.persist && n.flowPers != nil {
		n.persistState()
	}

	// Send state change on port 0.
	outMsg := flow.NewMessage()
	outMsg.SetTopic("statemachine")
	outMsg.SetPayload(map[string]any{
		"state":         result.To,
		"previousState": result.From,
		"event": map[string]any{
			"topic":   result.Event,
			"payload": nil,
		},
		"context":       result.Context,
		"machineId":     n.engine.def.ID,
		"changed":       result.Changed,
	})
	if n.send != nil {
		n.send(0, outMsg)
	}

	// Send action-generated messages on port 1.
	for _, msg := range n.pendingSends {
		if n.send != nil {
			n.send(1, msg)
		}
	}
}

// cancelDelayTimers stops all active timers.
func (n *StateMachineNode) cancelDelayTimers() {
	for k, t := range n.timers {
		t.Stop()
		delete(n.timers, k)
	}
}

// ── Persistence ──────────────────────────────────────────────────────────────

func (n *StateMachineNode) persistState() {
	state := map[string]any{
		"current": n.engine.CurrentState(),
		"context": n.engine.Context(),
	}
	key := "sm_" + n.config.ID
	if err := n.flowPers.Set(key, state); err != nil {
		slog.Warn("statemachine: persist error", "error", err, "node_id", n.config.ID)
	}
}

func (n *StateMachineNode) restoreState() {
	key := "sm_" + n.config.ID
	val, err := n.flowPers.Get(key)
	if err != nil || val == nil {
		return
	}
	state, ok := val.(map[string]any)
	if !ok {
		return
	}
	if current, ok := state["current"].(string); ok {
		// Only restore if the state still exists in the definition.
		if _, exists := n.engine.def.States[current]; exists {
			n.engine.SetState(current)
		}
	}
	if ctx, ok := state["context"].(map[string]any); ok {
		n.engine.SetContext(ctx)
	}
	slog.Info("statemachine: restored persisted state",
		"node_id", n.config.ID,
		"state", n.engine.CurrentState(),
	)
}

// ── JS Globals ───────────────────────────────────────────────────────────────

// registerGlobals sets up node, console, global, and flow objects in the VM.
func (n *StateMachineNode) registerGlobals() {
	nodeObj := n.vm.NewObject()

	// node.send(msg) — sends to port 1 (action output).
	_ = nodeObj.Set("send", func(call goja.FunctionCall) goja.Value {
		n.jsSend(call.Argument(0))
		return goja.Undefined()
	})

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

	_ = nodeObj.Set("status", func(call goja.FunctionCall) goja.Value {
		fill := call.Argument(0).String()
		text := call.Argument(1).String()
		if n.status != nil {
			n.status(fill, text)
		}
		return goja.Undefined()
	})

	_ = n.vm.Set("node", nodeObj)

	// console.log / warn / error
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
	globalObj := n.vm.NewObject()
	_ = globalObj.Set("get", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(1))
		if store == nil {
			return goja.Null()
		}
		key := call.Argument(0).String()
		val, err := store.Get(key)
		if err != nil {
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
		_ = store.Set(key, val)
		return goja.Undefined()
	})
	_ = globalObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(1))
		if store == nil {
			return goja.Undefined()
		}
		_ = store.Delete(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = globalObj.Set("keys", func(call goja.FunctionCall) goja.Value {
		store := n.pickStore(call.Argument(0))
		if store == nil {
			return n.vm.ToValue([]string{})
		}
		keys, err := store.Keys()
		if err != nil || keys == nil {
			return n.vm.ToValue([]string{})
		}
		return n.vm.ToValue(keys)
	})
	_ = n.vm.Set("global", globalObj)

	// flow.get/set/delete/keys
	flowObj := n.vm.NewObject()
	_ = flowObj.Set("get", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(1))
		if store == nil {
			return goja.Null()
		}
		key := call.Argument(0).String()
		val, err := store.Get(key)
		if err != nil || val == nil {
			return goja.Null()
		}
		return n.vm.ToValue(val)
	})
	_ = flowObj.Set("set", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(2))
		if store == nil {
			return goja.Undefined()
		}
		_ = store.Set(call.Argument(0).String(), call.Argument(1).Export())
		return goja.Undefined()
	})
	_ = flowObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(1))
		if store == nil {
			return goja.Undefined()
		}
		_ = store.Delete(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = flowObj.Set("keys", func(call goja.FunctionCall) goja.Value {
		store := n.pickFlowStore(call.Argument(0))
		if store == nil {
			return n.vm.ToValue([]string{})
		}
		keys, err := store.Keys()
		if err != nil || keys == nil {
			return n.vm.ToValue([]string{})
		}
		return n.vm.ToValue(keys)
	})
	_ = n.vm.Set("flow", flowObj)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func (n *StateMachineNode) jsSend(val goja.Value) {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return
	}
	obj := val.ToObject(n.vm)
	if obj == nil {
		return
	}
	data := make(map[string]any)
	for _, key := range obj.Keys() {
		data[key] = obj.Get(key).Export()
	}
	msg := flow.NewMessageFromData(data)
	n.pendingSends = append(n.pendingSends, msg)
}

func (n *StateMachineNode) emitDebug(status string, val goja.Value) {
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
		Property:  "statemachine",
		NodeName:  n.config.Name,
	})
}

func (n *StateMachineNode) pickStore(persistArg goja.Value) flow.ContextStore {
	if persistArg != nil && !goja.IsUndefined(persistArg) && !goja.IsNull(persistArg) {
		if persistArg.ToBoolean() {
			return n.ctxPers
		}
	}
	return n.ctxMem
}

func (n *StateMachineNode) pickFlowStore(persistArg goja.Value) flow.ContextStore {
	if persistArg != nil && !goja.IsUndefined(persistArg) && !goja.IsNull(persistArg) {
		if persistArg.ToBoolean() {
			return n.flowPers
		}
	}
	return n.flowMem
}
