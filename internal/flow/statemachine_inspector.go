// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import "errors"

// Sentinel errors returned by Engine lookup helpers used by the State Machine
// inspector API. Callers map these to HTTP status codes.
var (
	ErrFlowNotFound    = errors.New("flow not found")
	ErrNodeNotFound    = errors.New("node not found")
	ErrNotStateMachine = errors.New("node is not a state machine")
)

// StateMachineInspector is an optional capability implemented by node instances
// that expose a runtime snapshot of their state machine. The State Machine
// Inspector in the UI uses this to render the current state, context, and
// recent transitions of a running statemachine node.
type StateMachineInspector interface {
	StateMachineSnapshot() StateMachineSnapshot
}

// StateMachineSnapshot is the read-only view of a running state machine node
// returned over the REST API.
type StateMachineSnapshot struct {
	// MachineID is the id from the machine definition (e.g. "doorLock").
	MachineID string `json:"machineId"`

	// CurrentState is the name of the state the machine is currently in.
	CurrentState string `json:"currentState"`

	// States lists every state declared in the machine definition. Order is
	// not guaranteed across snapshots — the UI sorts as needed.
	States []string `json:"states"`

	// Initial is the initial state name from the definition.
	Initial string `json:"initial"`

	// Context is a shallow copy of the machine context (mutable JSON object
	// that guards/actions read and write).
	Context map[string]any `json:"context"`

	// AvailableEvents lists the event names accepted in the current state
	// (keys of states[current].on plus any "after" timer pseudo-events).
	AvailableEvents []string `json:"availableEvents"`

	// History is a rolling buffer of the most recent transitions (newest last).
	// Empty when no transitions have occurred yet.
	History []StateMachineTransition `json:"history"`
}

// StateMachineTransition is one entry in the snapshot history buffer.
type StateMachineTransition struct {
	// Timestamp is RFC3339Nano UTC.
	Timestamp string `json:"ts"`
	// From is the state name before the transition.
	From string `json:"from"`
	// To is the state name after the transition.
	To string `json:"to"`
	// Event is the event name that triggered the transition. For after-timers
	// this is "__AFTER_<ms>".
	Event string `json:"event"`
}
