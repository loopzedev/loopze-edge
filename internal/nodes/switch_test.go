// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"testing"

	"github.com/niceclouds/flint/internal/flow"
)

// newSwitchNode builds a SwitchNode with the given top-level config.
// Pass rules as []map[string]any; they are converted to []any internally.
func newSwitchNode(t *testing.T, props map[string]any) *SwitchNode {
	t.Helper()

	if rules, ok := props["rules"].([]map[string]any); ok {
		props["rules"] = toAnySlice(rules)
	}

	config := flow.NodeConfig{
		ID:         "switch-test",
		Type:       "switch",
		Properties: props,
	}

	inst, err := NewSwitchNode(config)
	if err != nil {
		t.Fatalf("NewSwitchNode: %v", err)
	}

	n := inst.(*SwitchNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	n.SetSend(func(port int, msg *flow.Message) {})
	n.SetStatus(func(fill, text string) {})
	n.SetDebug(func(msg flow.DebugMessage) {})

	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })

	return n
}

func switchMsg(payload any) *flow.Message {
	msg := flow.NewMessage()
	msg.SetPayload(payload)
	return msg
}

// matchedPorts returns the indices of output slots that received a message.
func matchedPorts(outputs [][]*flow.Message) []int {
	var ports []int
	for i, slot := range outputs {
		if len(slot) > 0 {
			ports = append(ports, i)
		}
	}
	return ports
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── Basic comparisons ────────────────────────────────────────────

func TestSwitch_Eq_SameType(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "active", "vt": "str"},
		},
	})

	out, err := n.HandleMessage(switchMsg("active"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("expected match on port 0, got %v", matchedPorts(out))
	}
}

func TestSwitch_Eq_LooseStringNumber(t *testing.T) {
	// Spec: "10" == 10 must be true (loose equality).
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "10", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg(float64(10)))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("loose '10'==10: expected match, got %v", matchedPorts(out))
	}
}

func TestSwitch_Eq_LooseBoolString(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "true", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg(true))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("loose true=='true': expected match, got %v", matchedPorts(out))
	}
}

func TestSwitch_Neq(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "neq", "v": "active", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("inactive"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("neq: expected match, got %v", matchedPorts(out))
	}
}

func TestSwitch_OrderingOperators(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"checkall":     true,
		"rules": []map[string]any{
			{"t": "lt", "v": "20", "vt": "num"},
			{"t": "lte", "v": "10", "vt": "num"},
			{"t": "gt", "v": "5", "vt": "num"},
			{"t": "gte", "v": "10", "vt": "num"},
		},
	})
	out, _ := n.HandleMessage(switchMsg(float64(10)))
	// 10: <20 ✓, <=10 ✓, >5 ✓, >=10 ✓
	if !equalInts(matchedPorts(out), []int{0, 1, 2, 3}) {
		t.Errorf("expected all four to match, got %v", matchedPorts(out))
	}
}

func TestSwitch_OrderingNilProperty(t *testing.T) {
	// nil propVal must not panic; comparisons return false.
	n := newSwitchNode(t, map[string]any{
		"property":     "missing",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "lt", "v": "10", "vt": "num"},
		},
	})
	out, err := n.HandleMessage(switchMsg("anything"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(matchedPorts(out)) != 0 {
		t.Errorf("expected no match for nil < 10, got %v", matchedPorts(out))
	}
}

// ── Between ──────────────────────────────────────────────────────

func TestSwitch_Between(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "btwn", "v": "0", "vt": "num", "v2": "10", "v2t": "num"},
		},
	})

	cases := []struct {
		val   float64
		match bool
	}{
		{-1, false},
		{0, true},  // inclusive low
		{5, true},
		{10, true}, // inclusive high
		{11, false},
	}
	for _, tc := range cases {
		out, _ := n.HandleMessage(switchMsg(tc.val))
		got := len(matchedPorts(out)) > 0
		if got != tc.match {
			t.Errorf("btwn %v: want match=%v, got %v", tc.val, tc.match, got)
		}
	}
}

// ── Regex ────────────────────────────────────────────────────────

func TestSwitch_Regex_CaseInsensitiveDefault(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "regex", "v": "^err_", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("ERR_500"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("default regex should be case-insensitive, got %v", matchedPorts(out))
	}
}

func TestSwitch_Regex_CaseSensitive(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "regex", "v": "^err_", "vt": "str", "case": true},
		},
	})
	out, _ := n.HandleMessage(switchMsg("ERR_500"))
	if len(matchedPorts(out)) != 0 {
		t.Errorf("case-sensitive regex must not match, got %v", matchedPorts(out))
	}
}

func TestSwitch_Regex_InvalidErrors(t *testing.T) {
	config := flow.NodeConfig{
		ID:   "switch-bad",
		Type: "switch",
		Properties: map[string]any{
			"property":     "payload",
			"propertyType": "msg",
			"rules": []any{
				map[string]any{"t": "regex", "v": "[invalid", "vt": "str"},
			},
		},
	}
	inst, _ := NewSwitchNode(config)
	if err := inst.(*SwitchNode).Init(); err == nil {
		t.Errorf("expected Init to fail on invalid regex")
	}
}

// ── Contains ─────────────────────────────────────────────────────

func TestSwitch_Contains_String(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "cont", "v": "World", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("Hello World"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("string contains: expected match, got %v", matchedPorts(out))
	}
}

func TestSwitch_Contains_Array(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "cont", "v": "b", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg([]any{"a", "b", "c"}))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("array contains: expected match, got %v", matchedPorts(out))
	}
}

// ── Type / existence ─────────────────────────────────────────────

func TestSwitch_TrueFalse(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"checkall":     true,
		"rules": []map[string]any{
			{"t": "true"},
			{"t": "false"},
		},
	})

	out, _ := n.HandleMessage(switchMsg(true))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("true: expected port 0, got %v", matchedPorts(out))
	}

	out, _ = n.HandleMessage(switchMsg(false))
	if !equalInts(matchedPorts(out), []int{1}) {
		t.Errorf("false: expected port 1, got %v", matchedPorts(out))
	}

	out, _ = n.HandleMessage(switchMsg("not a bool"))
	if len(matchedPorts(out)) != 0 {
		t.Errorf("non-bool: expected no match, got %v", matchedPorts(out))
	}
}

func TestSwitch_NullNotNull(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "missing",
		"propertyType": "msg",
		"checkall":     true,
		"rules": []map[string]any{
			{"t": "null"},
			{"t": "nnull"},
		},
	})

	// missing property → nil → null matches, nnull does not
	out, _ := n.HandleMessage(switchMsg("payload-value"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("null check on missing: expected port 0, got %v", matchedPorts(out))
	}
}

func TestSwitch_EmptyNotEmpty(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		empty   bool
	}{
		{"empty string", "", true},
		{"non-empty string", "x", false},
		{"empty array", []any{}, true},
		{"non-empty array", []any{1}, false},
		{"empty object", map[string]any{}, true},
		{"non-empty object", map[string]any{"k": 1}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newSwitchNode(t, map[string]any{
				"property":     "payload",
				"propertyType": "msg",
				"checkall":     true,
				"rules": []map[string]any{
					{"t": "empty"},
					{"t": "nempty"},
				},
			})
			out, _ := n.HandleMessage(switchMsg(tc.payload))
			matched := matchedPorts(out)
			wantPort := 1
			if tc.empty {
				wantPort = 0
			}
			if !equalInts(matched, []int{wantPort}) {
				t.Errorf("payload=%v: want port %d, got %v", tc.payload, wantPort, matched)
			}
		})
	}
}

func TestSwitch_IsType(t *testing.T) {
	cases := []struct {
		name     string
		payload  any
		typeName string
		want     bool
	}{
		{"string yes", "abc", "string", true},
		{"string no on number", float64(1), "string", false},
		{"number yes", float64(42), "number", true},
		{"number no on bool", true, "number", false},
		{"number no on numeric string", "10", "number", false},
		{"boolean yes", false, "boolean", true},
		{"array yes", []any{1, 2}, "array", true},
		{"object yes", map[string]any{"x": 1}, "object", true},
		{"null yes on nil", nil, "null", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newSwitchNode(t, map[string]any{
				"property":     "payload",
				"propertyType": "msg",
				"rules": []map[string]any{
					{"t": "istype", "v": tc.typeName, "vt": "str"},
				},
			})
			msg := flow.NewMessage()
			if tc.payload != nil {
				msg.SetPayload(tc.payload)
			}
			out, _ := n.HandleMessage(msg)
			got := len(matchedPorts(out)) > 0
			if got != tc.want {
				t.Errorf("istype=%s payload=%v: want %v, got %v", tc.typeName, tc.payload, tc.want, got)
			}
		})
	}
}

func TestSwitch_IsType_DefaultsToStringWhenEmpty(t *testing.T) {
	// Older configs / never-touched dropdowns store `v=""` for istype.
	// Backend must treat that as `string` (matches the UI's displayed default).
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "istype", "v": "", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("hello"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("istype empty -> string: expected match on string payload, got %v", matchedPorts(out))
	}
}

// ── Else / catch-all ─────────────────────────────────────────────

func TestSwitch_Else_TriggersWhenNoMatch(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "nope", "vt": "str"},
			{"t": "else"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("anything"))
	if !equalInts(matchedPorts(out), []int{1}) {
		t.Errorf("else: expected port 1, got %v", matchedPorts(out))
	}
}

func TestSwitch_Else_SuppressedByMatch(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "yes", "vt": "str"},
			{"t": "else"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("yes"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("else with prior match: expected only port 0, got %v", matchedPorts(out))
	}
}

func TestSwitch_Else_CheckAllMode(t *testing.T) {
	// checkall=true: else should still only match if nothing else did.
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"checkall":     true,
		"rules": []map[string]any{
			{"t": "eq", "v": "yes", "vt": "str"},
			{"t": "lt", "v": "100", "vt": "num"},
			{"t": "else"},
		},
	})

	// "yes" matches rule 0 but not rule 1 (lt of non-numeric → false).
	// else must NOT trigger because rule 0 matched.
	out, _ := n.HandleMessage(switchMsg("yes"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("checkall else: expected port 0 only, got %v", matchedPorts(out))
	}

	// nothing matches → only else triggers.
	out, _ = n.HandleMessage(switchMsg("nope"))
	if !equalInts(matchedPorts(out), []int{2}) {
		t.Errorf("checkall else fallback: expected port 2 only, got %v", matchedPorts(out))
	}
}

func TestSwitch_Else_AsFirstRule_TolerantPosition(t *testing.T) {
	// `else` need not be the last rule; it always triggers iff !anyMatched.
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "else"},
			{"t": "eq", "v": "yes", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("yes"))
	if !equalInts(matchedPorts(out), []int{1}) {
		t.Errorf("else first, real match second: expected port 1, got %v", matchedPorts(out))
	}

	out, _ = n.HandleMessage(switchMsg("no"))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("else first, no match: expected port 0, got %v", matchedPorts(out))
	}
}

// ── Modes: stop after first match vs check all ───────────────────

func TestSwitch_StopAfterFirstMatch_Default(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "lt", "v": "100", "vt": "num"},
			{"t": "lt", "v": "200", "vt": "num"},
		},
	})
	out, _ := n.HandleMessage(switchMsg(float64(50)))
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("stop-after-first: expected only port 0, got %v", matchedPorts(out))
	}
}

func TestSwitch_CheckAll(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"checkall":     true,
		"rules": []map[string]any{
			{"t": "lt", "v": "100", "vt": "num"},
			{"t": "lt", "v": "200", "vt": "num"},
		},
	})
	out, _ := n.HandleMessage(switchMsg(float64(50)))
	if !equalInts(matchedPorts(out), []int{0, 1}) {
		t.Errorf("checkall: expected ports 0 and 1, got %v", matchedPorts(out))
	}
}

// ── Property paths and scopes ────────────────────────────────────

func TestSwitch_NestedPropertyPath(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload.status.code",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "200", "vt": "num"},
		},
	})

	msg := flow.NewMessage()
	msg.SetPayload(map[string]any{
		"status": map[string]any{
			"code": float64(200),
		},
	})
	out, _ := n.HandleMessage(msg)
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("nested path: expected match, got %v", matchedPorts(out))
	}
}

func TestSwitch_FlowScope_NoContextProvider(t *testing.T) {
	// flow scope without SetContext must not panic; propVal becomes nil.
	n := newSwitchNode(t, map[string]any{
		"property":     "key",
		"propertyType": "flow",
		"rules": []map[string]any{
			{"t": "null"},
		},
	})
	out, err := n.HandleMessage(switchMsg("anything"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if !equalInts(matchedPorts(out), []int{0}) {
		t.Errorf("flow scope without provider: expected null match, got %v", matchedPorts(out))
	}
}

// ── Output shape invariants ──────────────────────────────────────

func TestSwitch_OutputSlotCountEqualsRuleCount(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
		"rules": []map[string]any{
			{"t": "eq", "v": "a", "vt": "str"},
			{"t": "eq", "v": "b", "vt": "str"},
			{"t": "eq", "v": "c", "vt": "str"},
		},
	})
	out, _ := n.HandleMessage(switchMsg("b"))
	if len(out) != 3 {
		t.Errorf("output slot count: want 3, got %d", len(out))
	}
	if !equalInts(matchedPorts(out), []int{1}) {
		t.Errorf("expected port 1 only, got %v", matchedPorts(out))
	}
}

func TestSwitch_NoRules_NoCrash(t *testing.T) {
	n := newSwitchNode(t, map[string]any{
		"property":     "payload",
		"propertyType": "msg",
	})
	out, err := n.HandleMessage(switchMsg("x"))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("no rules: expected empty output, got len %d", len(out))
	}
}
