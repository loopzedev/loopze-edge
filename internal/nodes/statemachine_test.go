// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes_test

import (
	"encoding/json"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/nodes"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func mustParseMachine(t *testing.T, js string) nodes.MachineDefinition {
	t.Helper()
	var def nodes.MachineDefinition
	if err := json.Unmarshal([]byte(js), &def); err != nil {
		t.Fatalf("parse machine: %v", err)
	}
	return def
}

const simpleMachineJSON = `{
	"id": "test",
	"initial": "idle",
	"context": { "count": 0 },
	"states": {
		"idle": {
			"on": {
				"START": "running"
			}
		},
		"running": {
			"on": {
				"STOP": "idle",
				"PAUSE": "paused"
			}
		},
		"paused": {
			"on": {
				"RESUME": "running",
				"STOP": "idle"
			}
		}
	}
}`

// ── Engine Tests ─────────────────────────────────────────────────────────────

func TestSMEngine_BasicTransition(t *testing.T) {
	def := mustParseMachine(t, simpleMachineJSON)
	eng := nodes.NewSMEngine(def, nil, nil)

	if eng.CurrentState() != "idle" {
		t.Fatalf("expected initial state 'idle', got %q", eng.CurrentState())
	}

	result := eng.Send("START", nil)
	if result == nil {
		t.Fatal("expected transition result, got nil")
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}
	if result.From != "idle" || result.To != "running" {
		t.Fatalf("expected idle→running, got %s→%s", result.From, result.To)
	}
	if eng.CurrentState() != "running" {
		t.Fatalf("expected state 'running', got %q", eng.CurrentState())
	}
}

func TestSMEngine_UnknownEvent(t *testing.T) {
	def := mustParseMachine(t, simpleMachineJSON)
	eng := nodes.NewSMEngine(def, nil, nil)

	result := eng.Send("UNKNOWN", nil)
	if result != nil {
		t.Fatalf("expected nil for unknown event, got %+v", result)
	}
	if eng.CurrentState() != "idle" {
		t.Fatalf("state should remain 'idle', got %q", eng.CurrentState())
	}
}

func TestSMEngine_MultipleTransitions(t *testing.T) {
	def := mustParseMachine(t, simpleMachineJSON)
	eng := nodes.NewSMEngine(def, nil, nil)

	eng.Send("START", nil)
	eng.Send("PAUSE", nil)
	if eng.CurrentState() != "paused" {
		t.Fatalf("expected 'paused', got %q", eng.CurrentState())
	}

	eng.Send("RESUME", nil)
	if eng.CurrentState() != "running" {
		t.Fatalf("expected 'running', got %q", eng.CurrentState())
	}

	eng.Send("STOP", nil)
	if eng.CurrentState() != "idle" {
		t.Fatalf("expected 'idle', got %q", eng.CurrentState())
	}
}

func TestSMEngine_Guard(t *testing.T) {
	machineJSON := `{
		"id": "guarded",
		"initial": "locked",
		"context": { "pin": "1234" },
		"states": {
			"locked": {
				"on": {
					"UNLOCK": { "target": "unlocked", "guard": "pinCorrect" }
				}
			},
			"unlocked": {
				"on": {
					"LOCK": "locked"
				}
			}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	guard := func(name string, ctx map[string]any, event map[string]any) bool {
		if name == "pinCorrect" {
			return event["pin"] == ctx["pin"]
		}
		return false
	}

	eng := nodes.NewSMEngine(def, guard, nil)

	// Wrong pin — should be blocked.
	result := eng.Send("UNLOCK", map[string]any{"pin": "0000"})
	if result == nil {
		t.Fatal("expected result for blocked guard")
	}
	if result.Changed {
		t.Fatal("guard should have blocked transition")
	}
	if eng.CurrentState() != "locked" {
		t.Fatalf("expected 'locked', got %q", eng.CurrentState())
	}

	// Correct pin — should transition.
	result = eng.Send("UNLOCK", map[string]any{"pin": "1234"})
	if result == nil || !result.Changed {
		t.Fatal("expected successful transition")
	}
	if eng.CurrentState() != "unlocked" {
		t.Fatalf("expected 'unlocked', got %q", eng.CurrentState())
	}
}

func TestSMEngine_Actions(t *testing.T) {
	machineJSON := `{
		"id": "counter",
		"initial": "counting",
		"context": { "count": 0 },
		"states": {
			"counting": {
				"on": {
					"INCREMENT": { "target": "counting", "actions": ["addOne"] },
					"RESET": { "target": "counting", "actions": ["resetCount"] }
				}
			}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	action := func(name string, ctx map[string]any, event map[string]any) {
		switch name {
		case "addOne":
			count, _ := ctx["count"].(float64)
			ctx["count"] = count + 1
		case "resetCount":
			ctx["count"] = float64(0)
		}
	}

	eng := nodes.NewSMEngine(def, nil, action)

	eng.Send("INCREMENT", nil)
	eng.Send("INCREMENT", nil)
	eng.Send("INCREMENT", nil)

	ctx := eng.Context()
	if ctx["count"] != float64(3) {
		t.Fatalf("expected count=3, got %v", ctx["count"])
	}

	eng.Send("RESET", nil)
	ctx = eng.Context()
	if ctx["count"] != float64(0) {
		t.Fatalf("expected count=0 after reset, got %v", ctx["count"])
	}
}

func TestSMEngine_EntryExitActions(t *testing.T) {
	machineJSON := `{
		"id": "entryexit",
		"initial": "off",
		"context": {},
		"states": {
			"off": {
				"onExit": ["leavingOff"],
				"on": { "TURN_ON": "on" }
			},
			"on": {
				"onEntry": ["enteringOn"],
				"onExit": ["leavingOn"],
				"on": { "TURN_OFF": "off" }
			}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	var log []string
	action := func(name string, ctx map[string]any, event map[string]any) {
		log = append(log, name)
	}

	eng := nodes.NewSMEngine(def, nil, action)

	eng.Send("TURN_ON", nil)
	if len(log) != 2 || log[0] != "leavingOff" || log[1] != "enteringOn" {
		t.Fatalf("expected [leavingOff, enteringOn], got %v", log)
	}

	log = nil
	eng.Send("TURN_OFF", nil)
	if len(log) != 1 || log[0] != "leavingOn" {
		t.Fatalf("expected [leavingOn], got %v", log)
	}
}

func TestSMEngine_DelayedTransition(t *testing.T) {
	machineJSON := `{
		"id": "delayed",
		"initial": "waiting",
		"context": {},
		"states": {
			"waiting": {
				"after": { "5000": "done" },
				"on": { "CANCEL": "cancelled" }
			},
			"done": {},
			"cancelled": {}
		}
	}`

	def := mustParseMachine(t, machineJSON)
	eng := nodes.NewSMEngine(def, nil, nil)

	// Check active delays.
	delays := eng.ActiveDelays()
	if delays == nil || delays["5000"] != "done" {
		t.Fatalf("expected delay 5000→done, got %v", delays)
	}

	// Simulate timer fire.
	result := eng.SendAfter("5000")
	if result == nil || !result.Changed {
		t.Fatal("expected transition on timer fire")
	}
	if eng.CurrentState() != "done" {
		t.Fatalf("expected 'done', got %q", eng.CurrentState())
	}
}

func TestSMEngine_SelfTransition(t *testing.T) {
	machineJSON := `{
		"id": "self",
		"initial": "active",
		"context": { "ticks": 0 },
		"states": {
			"active": {
				"onEntry": ["tick"],
				"on": { "TICK": "active" }
			}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	action := func(name string, ctx map[string]any, event map[string]any) {
		if name == "tick" {
			count, _ := ctx["ticks"].(float64)
			ctx["ticks"] = count + 1
		}
	}

	eng := nodes.NewSMEngine(def, nil, action)

	// Initial entry action is not called by engine (only on transitions).
	// First TICK: exit→transition→entry on "active".
	eng.Send("TICK", nil)
	eng.Send("TICK", nil)

	ctx := eng.Context()
	// 2 self-transitions, each triggers onEntry once.
	if ctx["ticks"] != float64(2) {
		t.Fatalf("expected ticks=2, got %v", ctx["ticks"])
	}
}

func TestSMEngine_Validate(t *testing.T) {
	// Invalid: initial state does not exist.
	badJSON := `{
		"id": "bad",
		"initial": "nonexistent",
		"states": { "a": {} }
	}`

	def := mustParseMachine(t, badJSON)
	eng := nodes.NewSMEngine(def, nil, nil)
	if err := eng.Validate(); err == nil {
		t.Fatal("expected validation error for missing initial state")
	}

	// Invalid: transition target does not exist.
	badTargetJSON := `{
		"id": "bad",
		"initial": "a",
		"states": {
			"a": { "on": { "GO": "nonexistent" } }
		}
	}`

	def2 := mustParseMachine(t, badTargetJSON)
	eng2 := nodes.NewSMEngine(def2, nil, nil)
	if err := eng2.Validate(); err == nil {
		t.Fatal("expected validation error for missing target state")
	}
}

func TestSMEngine_TransitionWithActions(t *testing.T) {
	machineJSON := `{
		"id": "withactions",
		"initial": "a",
		"context": { "log": "" },
		"states": {
			"a": {
				"onExit": ["exitA"],
				"on": {
					"GO": { "target": "b", "actions": ["transAction"] }
				}
			},
			"b": {
				"onEntry": ["enterB"]
			}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	var order []string
	action := func(name string, ctx map[string]any, event map[string]any) {
		order = append(order, name)
	}

	eng := nodes.NewSMEngine(def, nil, action)
	eng.Send("GO", nil)

	// Order: exit A → transition action → enter B
	expected := []string{"exitA", "transAction", "enterB"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, name := range expected {
		if order[i] != name {
			t.Fatalf("action[%d]: expected %q, got %q", i, name, order[i])
		}
	}
}

func TestSMEngine_SetStateAndContext(t *testing.T) {
	def := mustParseMachine(t, simpleMachineJSON)
	eng := nodes.NewSMEngine(def, nil, nil)

	eng.SetState("running")
	if eng.CurrentState() != "running" {
		t.Fatalf("expected 'running', got %q", eng.CurrentState())
	}

	eng.SetContext(map[string]any{"count": float64(42)})
	ctx := eng.Context()
	if ctx["count"] != float64(42) {
		t.Fatalf("expected count=42, got %v", ctx["count"])
	}
}

func TestSMEngine_GuardedArray(t *testing.T) {
	machineJSON := `{
		"id": "doorLock",
		"initial": "locked",
		"context": {},
		"states": {
			"locked": {
				"on": {
					"UNLOCK": [
						{ "target": "unlocked", "guard": "pinCorrect" },
						{ "target": "alarm" }
					]
				}
			},
			"unlocked": {},
			"alarm": {}
		}
	}`

	def := mustParseMachine(t, machineJSON)

	guard := func(name string, ctx map[string]any, event map[string]any) bool {
		if name == "pinCorrect" {
			return event["pin"] == "1234"
		}
		return false
	}

	// Wrong pin → fallback to alarm.
	eng := nodes.NewSMEngine(def, guard, nil)
	result := eng.Send("UNLOCK", map[string]any{"pin": "0000"})
	if result == nil || !result.Changed {
		t.Fatal("expected transition to alarm")
	}
	if eng.CurrentState() != "alarm" {
		t.Fatalf("expected 'alarm', got %q", eng.CurrentState())
	}

	// Correct pin → unlocked.
	eng2 := nodes.NewSMEngine(def, guard, nil)
	result2 := eng2.Send("UNLOCK", map[string]any{"pin": "1234"})
	if result2 == nil || !result2.Changed {
		t.Fatal("expected transition to unlocked")
	}
	if eng2.CurrentState() != "unlocked" {
		t.Fatalf("expected 'unlocked', got %q", eng2.CurrentState())
	}
}

func TestParseDelayMs(t *testing.T) {
	tests := []struct {
		input string
		want  int
		err   bool
	}{
		{"5000", 5000, false},
		{"0", 0, false},
		{"100", 100, false},
		{"-1", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		got, err := nodes.ParseDelayMs(tt.input)
		if tt.err && err == nil {
			t.Errorf("ParseDelayMs(%q): expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("ParseDelayMs(%q): unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseDelayMs(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
