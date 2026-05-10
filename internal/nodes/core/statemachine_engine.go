// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ── Machine Definition Types ─────────────────────────────────────────────────

// MachineDefinition is the top-level JSON structure for a state machine.
type MachineDefinition struct {
	ID      string                     `json:"id"`
	Initial string                     `json:"initial"`
	Context map[string]any             `json:"context,omitempty"`
	States  map[string]StateDefinition `json:"states"`
}

// StateDefinition describes a single state in the machine.
type StateDefinition struct {
	On      map[string]json.RawMessage `json:"on,omitempty"`      // event → target string or TransitionDef
	OnEntry []string                   `json:"onEntry,omitempty"` // action names to run on entry
	OnExit  []string                   `json:"onExit,omitempty"`  // action names to run on exit
	After   map[string]json.RawMessage `json:"after,omitempty"`   // delay(ms) → target string or TransitionDef
}

// TransitionDef is the expanded form of a transition with guard and actions.
type TransitionDef struct {
	Target  string   `json:"target"`
	Guard   string   `json:"guard,omitempty"`
	Actions []string `json:"actions,omitempty"`
}

// TransitionResult is returned by SMEngine.Send when a transition occurs.
type TransitionResult struct {
	From    string         `json:"from"`
	To      string         `json:"to"`
	Event   string         `json:"event"`
	Context map[string]any `json:"context"`
	Changed bool           `json:"changed"`
}

// ── Guard/Action Callbacks ───────────────────────────────────────────────────

// GuardFunc evaluates a named guard. Returns true if the transition is allowed.
type GuardFunc func(name string, ctx map[string]any, event map[string]any) bool

// ActionFunc executes a named action, potentially modifying context.
type ActionFunc func(name string, ctx map[string]any, event map[string]any)

// ── SMEngine ─────────────────────────────────────────────────────────────────

// SMEngine is a pure-Go finite state machine engine.
// It is deterministic and synchronous — timer management lives outside.
type SMEngine struct {
	def        MachineDefinition
	current    string
	context    map[string]any
	evalGuard  GuardFunc
	execAction ActionFunc
}

// NewSMEngine creates a new state machine engine from the given definition.
// The evalGuard and execAction callbacks are optional (can be nil).
func NewSMEngine(def MachineDefinition, evalGuard GuardFunc, execAction ActionFunc) *SMEngine {
	ctx := make(map[string]any)
	if def.Context != nil {
		for k, v := range def.Context {
			ctx[k] = v
		}
	}

	if evalGuard == nil {
		evalGuard = func(string, map[string]any, map[string]any) bool { return true }
	}
	if execAction == nil {
		execAction = func(string, map[string]any, map[string]any) {}
	}

	return &SMEngine{
		def:        def,
		current:    def.Initial,
		context:    ctx,
		evalGuard:  evalGuard,
		execAction: execAction,
	}
}

// CurrentState returns the name of the current state.
func (e *SMEngine) CurrentState() string {
	return e.current
}

// Context returns a shallow copy of the machine context.
func (e *SMEngine) Context() map[string]any {
	cp := make(map[string]any, len(e.context))
	for k, v := range e.context {
		cp[k] = v
	}
	return cp
}

// SetState forces the engine into a specific state (used for persistence restore).
func (e *SMEngine) SetState(state string) {
	e.current = state
}

// SetContext replaces the engine's context (used for persistence restore).
func (e *SMEngine) SetContext(ctx map[string]any) {
	e.context = ctx
}

// Send processes an event and returns the transition result.
// Returns nil if no matching transition was found or all guards blocked it.
//
// Transitions can be:
//   - a string:  "targetState"
//   - an object: { "target": "...", "guard": "...", "actions": [...] }
//   - an array:  [ { guard, target }, { target } ]  — first matching guard wins, last without guard is fallback
func (e *SMEngine) Send(eventName string, data map[string]any) *TransitionResult {
	stateDef, ok := e.def.States[e.current]
	if !ok {
		return nil
	}

	// Build the event data map for guards/actions.
	eventData := make(map[string]any)
	if data != nil {
		for k, v := range data {
			eventData[k] = v
		}
	}
	eventData["type"] = eventName

	// Check "on" transitions.
	raw, ok := stateDef.On[eventName]
	if !ok {
		return nil
	}

	candidates, err := parseTransitions(raw)
	if err != nil {
		return nil
	}

	// Try each candidate — first matching guard wins.
	for _, td := range candidates {
		if td.Guard != "" && !e.evalGuard(td.Guard, e.context, eventData) {
			continue // guard blocked, try next
		}
		return e.executeTransition(td, eventName, eventData)
	}

	// All guards blocked.
	return &TransitionResult{
		From:    e.current,
		To:      e.current,
		Event:   eventName,
		Context: e.Context(),
		Changed: false,
	}
}

// SendAfter processes a delayed transition event (__AFTER_<ms>).
// It looks up the matching "after" definition for the current state.
func (e *SMEngine) SendAfter(delayMs string) *TransitionResult {
	stateDef, ok := e.def.States[e.current]
	if !ok {
		return nil
	}

	raw, ok := stateDef.After[delayMs]
	if !ok {
		return nil
	}

	candidates, err := parseTransitions(raw)
	if err != nil || len(candidates) == 0 {
		return nil
	}

	eventName := "__AFTER_" + delayMs
	eventData := map[string]any{"type": eventName}

	// After-transitions: take the first candidate (guards not typical here, but supported).
	for _, td := range candidates {
		if td.Guard != "" && !e.evalGuard(td.Guard, e.context, eventData) {
			continue
		}
		return e.executeTransition(td, eventName, eventData)
	}
	return nil
}

// ActiveDelays returns the after-delays defined for the current state.
// Keys are delay durations in ms (strings), values are target states.
// The caller is responsible for starting timers.
func (e *SMEngine) ActiveDelays() map[string]string {
	stateDef, ok := e.def.States[e.current]
	if !ok {
		return nil
	}
	if len(stateDef.After) == 0 {
		return nil
	}

	delays := make(map[string]string, len(stateDef.After))
	for ms, raw := range stateDef.After {
		candidates, err := parseTransitions(raw)
		if err != nil || len(candidates) == 0 {
			continue
		}
		delays[ms] = candidates[0].Target
	}
	return delays
}

// Validate checks that the machine definition is internally consistent:
// - initial state exists
// - all transition targets refer to existing states
func (e *SMEngine) Validate() error {
	if _, ok := e.def.States[e.def.Initial]; !ok {
		return fmt.Errorf("initial state %q does not exist", e.def.Initial)
	}

	for name, state := range e.def.States {
		for event, raw := range state.On {
			candidates, err := parseTransitions(raw)
			if err != nil {
				return fmt.Errorf("state %q event %q: %w", name, event, err)
			}
			for _, td := range candidates {
				if _, ok := e.def.States[td.Target]; !ok {
					return fmt.Errorf("state %q event %q: target %q does not exist", name, event, td.Target)
				}
			}
		}
		for ms, raw := range state.After {
			candidates, err := parseTransitions(raw)
			if err != nil {
				return fmt.Errorf("state %q after %s: %w", name, ms, err)
			}
			for _, td := range candidates {
				if _, ok := e.def.States[td.Target]; !ok {
					return fmt.Errorf("state %q after %s: target %q does not exist", name, ms, td.Target)
				}
			}
		}
	}
	return nil
}

// EntryActions returns the sorted delay keys for the current state.
// This is used to determine which after-timers to start deterministically.
func (e *SMEngine) SortedDelayKeys() []string {
	stateDef, ok := e.def.States[e.current]
	if !ok || len(stateDef.After) == 0 {
		return nil
	}
	keys := make([]string, 0, len(stateDef.After))
	for k := range stateDef.After {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ── Internal ─────────────────────────────────────────────────────────────────

// executeTransition performs exit actions, transition actions, state change,
// and entry actions. Guard evaluation has already happened in Send().
func (e *SMEngine) executeTransition(td TransitionDef, eventName string, eventData map[string]any) *TransitionResult {
	if _, ok := e.def.States[td.Target]; !ok {
		return nil
	}

	from := e.current

	// Execute exit actions for the current state.
	if fromDef, ok := e.def.States[from]; ok {
		for _, action := range fromDef.OnExit {
			e.execAction(action, e.context, eventData)
		}
	}

	// Execute transition actions.
	for _, action := range td.Actions {
		e.execAction(action, e.context, eventData)
	}

	// Move to the new state.
	e.current = td.Target

	// Execute entry actions for the new state.
	if toDef, ok := e.def.States[td.Target]; ok {
		for _, action := range toDef.OnEntry {
			e.execAction(action, e.context, eventData)
		}
	}

	return &TransitionResult{
		From:    from,
		To:      td.Target,
		Event:   eventName,
		Context: e.Context(),
		Changed: from != td.Target,
	}
}

// parseTransitions parses a raw JSON value into one or more TransitionDefs.
// Supported forms:
//   - string:  "targetState"
//   - object:  { "target": "...", "guard": "...", "actions": [...] }
//   - array:   [ { "target": "...", "guard": "..." }, { "target": "..." } ]
//
// For arrays, order matters: first guard that matches wins, entry without
// guard acts as fallback (put it last).
func parseTransitions(raw json.RawMessage) ([]TransitionDef, error) {
	// Try string first (short form): "targetState"
	var target string
	if err := json.Unmarshal(raw, &target); err == nil {
		return []TransitionDef{{Target: target}}, nil
	}

	// Try array form: [ { ... }, { ... } ]
	var arr []TransitionDef
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		for i, td := range arr {
			if td.Target == "" {
				return nil, fmt.Errorf("transition[%d] missing target: %s", i, string(raw))
			}
		}
		return arr, nil
	}

	// Try single object form: { "target": "..." }
	var td TransitionDef
	if err := json.Unmarshal(raw, &td); err != nil {
		return nil, fmt.Errorf("invalid transition definition: %s", string(raw))
	}
	if td.Target == "" {
		return nil, fmt.Errorf("transition missing target: %s", string(raw))
	}
	return []TransitionDef{td}, nil
}

// ParseDelayMs converts a delay key (string) to milliseconds.
func ParseDelayMs(s string) (int, error) {
	s = strings.TrimSpace(s)
	ms, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid delay %q: %w", s, err)
	}
	if ms < 0 {
		return 0, fmt.Errorf("delay must be non-negative: %d", ms)
	}
	return ms, nil
}
