// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

// Package yaegi wraps github.com/traefik/yaegi as the function-go node's
// scripting engine. The user writes a `package main` with a `handle` function;
// the engine reflects on its signature to figure out how to bridge incoming
// flow.Message values into typed Go arguments and how to interpret the
// return value.
//
// Three handle signatures are supported (illustrative — the actual matching
// is structural, see Compile):
//
//	func handle(payload any) any
//	func handle(payload []map[string]any) []map[string]any
//	func handle(payload []SomeStruct) any
//	func handle(payload any, node flintnode.Node)   // multi-output via node.Send
//
// Buffer-shaped payloads (mqtt-in's []int wire format) are converted to []byte
// when the user declares a []byte input, and back to []int on the return path
// so downstream nodes (mqtt-out, debug viewer) see a well-formed payload.
//
// User code typed structs go through a JSON marshal/unmarshal at the boundary
// — driven by `json:"…"` tags on the struct fields. This costs reflection but
// keeps the hot loop inside handle running on real native types.
package yaegi

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/traefik/yaegi/interp"

	"github.com/niceclouds/flint/internal/scripting"
)

// Node is the interface the function-go user code receives as the optional
// second `handle` argument. The hosting node implements it (typically as an
// adapter struct that closes over the FunctionGoNode instance) and the engine
// passes the implementation to Run.
//
// Get/Set/Delete operate on a node-local map (volatile, lost on redeploy).
// FlowGet*/GlobalGet* hit the flow-/global-scoped context stores; the *P
// suffix (persistent) selects the file-backed store, otherwise the in-memory
// store is used. Returning nil for missing keys mirrors the Goja JS API.
type Node interface {
	Send(port int, msg any)
	Log(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Status(fill, text string)

	// Node-scoped in-memory context (private to this node instance).
	Get(key string) any
	Set(key string, val any)
	Delete(key string)

	// Flow-scoped context — memory store.
	FlowGet(key string) any
	FlowSet(key string, val any)
	FlowDelete(key string)
	// Flow-scoped context — persistent (file-backed) store.
	FlowGetP(key string) any
	FlowSetP(key string, val any)
	FlowDeleteP(key string)

	// Global context — memory store.
	GlobalGet(key string) any
	GlobalSet(key string, val any)
	GlobalDelete(key string)
	// Global context — persistent (file-backed) store.
	GlobalGetP(key string) any
	GlobalSetP(key string, val any)
	GlobalDeleteP(key string)
}

// Engine is reusable across nodes — it owns nothing per-instance other than
// the (large) symbol map. Each Compile creates a fresh interpreter.
type Engine struct {
	symbols interp.Exports
}

// New constructs an engine with the curated stdlib subset (see symbols.go).
func New() *Engine {
	return &Engine{symbols: SafeSymbols()}
}

// Program is the compiled form of a function-go user script. It bundles the
// reflected handle function with the metadata Run needs to coerce inputs and
// interpret the output. The mutex serialises Run calls because Yaegi
// interpreters are not documented as goroutine-safe — although the engine's
// per-node nodeLoop is single-threaded today, the lock is cheap insurance
// against future changes.
type Program struct {
	handleFn  reflect.Value
	inputType reflect.Type
	hasNode   bool
	returnsV  bool

	interp *interp.Interpreter
	mu     sync.Mutex
}

// Compile installs the stdlib subset, registers the Node interface so user
// code can `import "flintnode"` and reference `flintnode.Node`, evaluates the
// user source, and reflects on `main.handle` to validate the signature.
//
// Compile failures cover: forbidden imports (refusal to resolve), syntax /
// type errors in user code, missing or wrong-shaped `handle` symbol.
//
// The named return + deferred recover() guards against panics from Yaegi's
// internals — some malformed user code paths trigger `log.Panic` deep in the
// interpreter (nil reflect types, unknown symbols, …) which would otherwise
// kill the host process. We turn every such panic into a regular error so the
// node goes red instead of taking the engine down.
func (e *Engine) Compile(code string) (prog *Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			prog = nil
			err = fmt.Errorf("compile panic: %v", r)
		}
	}()

	i := interp.New(interp.Options{})

	if err := i.Use(e.symbols); err != nil {
		return nil, fmt.Errorf("install symbols: %w", err)
	}

	// Expose the Node interface under import path "flintnode".
	if err := i.Use(interp.Exports{
		"flintnode/flintnode": map[string]reflect.Value{
			"Node": reflect.ValueOf((*Node)(nil)),
		},
	}); err != nil {
		return nil, fmt.Errorf("install flintnode: %w", err)
	}

	if _, err := i.Eval(code); err != nil {
		return nil, fmt.Errorf("eval source: %w", err)
	}

	handleVal, err := i.Eval("main.handle")
	if err != nil {
		return nil, fmt.Errorf("lookup handle: %w", err)
	}
	fnType := handleVal.Type()
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("handle must be a function, got %v", fnType.Kind())
	}

	numIn := fnType.NumIn()
	if numIn < 1 || numIn > 2 {
		return nil, fmt.Errorf("handle must take 1 or 2 arguments (payload [, node]), got %d", numIn)
	}
	numOut := fnType.NumOut()
	if numOut > 1 {
		return nil, fmt.Errorf("handle must return 0 or 1 values, got %d", numOut)
	}

	return &Program{
		handleFn:  handleVal,
		inputType: fnType.In(0),
		hasNode:   numIn == 2,
		returnsV:  numOut == 1,
		interp:    i,
	}, nil
}

// Run invokes the user's handle function for one message. payload is the
// raw flow.Message payload (any-typed). node is the host implementation of
// Node — only used when the handle signature requested it.
//
// Recovers from panics in user code so a misbehaving script can't crash the
// engine; the recovered value is wrapped in an error.
func (p *Program) Run(payload any, node Node) (result any, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handle panic: %v", r)
		}
	}()

	in, err := coerceInput(payload, p.inputType)
	if err != nil {
		return nil, err
	}

	args := []reflect.Value{in}
	if p.hasNode {
		// reflect.ValueOf(nil) is invalid; pass a typed-nil interface to keep
		// reflect happy if the host hands us nil (test scenarios).
		if node == nil {
			args = append(args, reflect.Zero(reflect.TypeOf((*Node)(nil)).Elem()))
		} else {
			args = append(args, reflect.ValueOf(node))
		}
	}

	results := p.handleFn.Call(args)
	if !p.returnsV {
		return nil, nil
	}

	out := results[0].Interface()
	// Buffer auto-conversion on the way out: []byte → []int wire format,
	// matching what mqtt-out and the debug viewer expect.
	if b, ok := out.([]byte); ok {
		return scripting.BufferToInts(b), nil
	}
	return out, nil
}

// coerceInput turns the raw payload into a reflect.Value matching the user's
// declared input type. Fast paths handle the common cases without going
// through JSON; anything else falls back to a JSON marshal/unmarshal round
// trip driven by the user's `json:` tags.
func coerceInput(payload any, want reflect.Type) (reflect.Value, error) {
	// Plain interface{} (any) — pass through.
	if want.Kind() == reflect.Interface && want.NumMethod() == 0 {
		if payload == nil {
			return reflect.Zero(want), nil
		}
		return reflect.ValueOf(payload), nil
	}

	// []byte — buffer interop, accept any wire-format buffer shape.
	if want.Kind() == reflect.Slice && want.Elem().Kind() == reflect.Uint8 {
		buf := scripting.IntsToBuffer(payload)
		if buf == nil {
			// Not a buffer shape; an empty []byte is the safest fallback
			// because the user's handle expects a slice it can index.
			buf = []byte{}
		}
		return reflect.ValueOf(buf), nil
	}

	// []map[string]any — common Flint payload shape, handle without JSON.
	if want.Kind() == reflect.Slice && want.Elem().Kind() == reflect.Map {
		if v, ok := payload.([]map[string]any); ok {
			return reflect.ValueOf(v), nil
		}
		if arr, ok := payload.([]any); ok {
			out := make([]map[string]any, 0, len(arr))
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return reflect.ValueOf(out), nil
		}
		// Fall through to JSON below — gives the user a chance with []SomeStruct
		// even though Elem().Kind() is Map (e.g. when SomeStruct is a typed map).
	}

	// map[string]any — pass through if the shapes match.
	if want.Kind() == reflect.Map && want.Key().Kind() == reflect.String {
		if want.Elem().Kind() == reflect.Interface && want.Elem().NumMethod() == 0 {
			if m, ok := payload.(map[string]any); ok {
				return reflect.ValueOf(m), nil
			}
		}
	}

	// Fallback: JSON round trip. Drives both typed structs and slices of them.
	data, err := json.Marshal(payload)
	if err != nil {
		return reflect.Value{}, fmt.Errorf("marshal payload: %w", err)
	}
	target := reflect.New(want)
	if err := json.Unmarshal(data, target.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("unmarshal into %v: %w", want, err)
	}
	return target.Elem(), nil
}

// IsCompileError reports whether err originated from Compile (as opposed to
// runtime). Useful for callers that want to distinguish deploy-time problems
// from per-message ones — currently a thin marker, can grow if we wrap
// errors.
func IsCompileError(err error) bool {
	return errors.Is(err, errCompile)
}

var errCompile = errors.New("compile")
