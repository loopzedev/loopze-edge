// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// SwitchRule represents one routing rule. Each rule corresponds to one output
// port; the rule's index in the rules slice equals the port index.
type SwitchRule struct {
	ID   string // stable rule ID used by the frontend for wire-mapping
	Type string // operator: eq, neq, lt, lte, gt, gte, btwn, cont, regex,
	//          true, false, null, nnull, empty, nempty, istype, else
	V   string // primary comparison value
	VT  string // value type: msg, flow, global, str, num, bool, json, env
	VS  string // storage for V (memory/persistent) when VT is flow/global
	V2  string // secondary value (only for btwn)
	V2T string
	V2S string
	Case bool // case-sensitivity (regex/cont)
	re   *regexp.Regexp
}

// SwitchNode routes incoming messages to one or more output ports based on
// configurable rules. The node has 1 input and len(rules) outputs.
type SwitchNode struct {
	config flow.NodeConfig
	nodes.BaseNode
	ctxMem   flow.ContextStore
	ctxPers  flow.ContextStore
	flowMem  flow.ContextStore
	flowPers flow.ContextStore

	property     string // property path (without scope prefix)
	propertyType string // msg | flow | global
	propStorage  string // memory | persistent (when propertyType is flow/global)
	checkAll     bool   // false = stop after first match, true = evaluate all rules
	rules        []SwitchRule
}

// NewSwitchNode is the NodeFactory for the switch node type.
func NewSwitchNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &SwitchNode{config: config}, nil
}

// Init parses the configuration: top-level property + rules. Regexes for
// `regex` operators are compiled here so HandleMessage stays cheap.
func (n *SwitchNode) Init() error {
	props := n.config.Properties

	n.property = nodes.StringVal(props, "property", "payload")
	n.propertyType = nodes.StringVal(props, "propertyType", "msg")
	n.propStorage = nodes.StringVal(props, "propertyStorage", "memory")
	if v, ok := props["checkall"].(bool); ok {
		n.checkAll = v
	}

	rawRules, ok := props["rules"].([]any)
	if !ok || len(rawRules) == 0 {
		slog.Warn("switch node has no rules configured", "node_id", n.config.ID)
		return nil
	}

	for i, raw := range rawRules {
		ruleMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		rule := SwitchRule{
			ID:   nodes.StringVal(ruleMap, "id", ""),
			Type: nodes.StringVal(ruleMap, "t", "eq"),
			V:    nodes.StringVal(ruleMap, "v", ""),
			VT:   nodes.StringVal(ruleMap, "vt", "str"),
			VS:   nodes.StringVal(ruleMap, "vs", "memory"),
			V2:   nodes.StringVal(ruleMap, "v2", ""),
			V2T:  nodes.StringVal(ruleMap, "v2t", "str"),
			V2S:  nodes.StringVal(ruleMap, "v2s", "memory"),
		}
		if v, ok := ruleMap["case"].(bool); ok {
			rule.Case = v
		}

		// `istype` with an empty type-name defaults to "string" — matches the
		// frontend's displayed default when the user never opens the dropdown.
		if rule.Type == "istype" && rule.V == "" {
			rule.V = "string"
		}

		if rule.Type == "regex" && rule.V != "" {
			pattern := rule.V
			if !rule.Case {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("switch node %s: rule %d: invalid regex %q: %w",
					n.config.ID, i, rule.V, err)
			}
			rule.re = re
		}

		n.rules = append(n.rules, rule)
	}

	return nil
}

// SetContext implements flow.ContextProvider.
func (n *SwitchNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

func (n *SwitchNode) Start() error {
	slog.Info("switch node started",
		"node_id", n.config.ID,
		"rules", len(n.rules),
		"checkall", n.checkAll,
	)
	return nil
}

func (n *SwitchNode) Stop() error {
	slog.Info("switch node stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage evaluates all rules against the configured property value and
// returns one slot per rule. The engine clones the message per wire on send,
// so we can safely reuse the same *Message pointer in multiple slots.
func (n *SwitchNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	outputs := make([][]*flow.Message, len(n.rules))
	if len(n.rules) == 0 {
		return outputs, nil
	}

	propVal, _ := n.getProperty(msg, n.propertyType, n.property, n.propStorage)

	anyMatched := false
	for i, rule := range n.rules {
		if rule.Type == "else" {
			continue // handled in second pass
		}
		ok, err := n.match(rule, propVal, msg)
		if err != nil {
			slog.Warn("switch node rule error",
				"node_id", n.config.ID, "rule", i, "type", rule.Type, "error", err)
			continue
		}
		if ok {
			outputs[i] = []*flow.Message{msg}
			anyMatched = true
			if !n.checkAll {
				return outputs, nil
			}
		}
	}

	if !anyMatched {
		for i, rule := range n.rules {
			if rule.Type == "else" {
				outputs[i] = []*flow.Message{msg}
			}
		}
	}

	return outputs, nil
}

// match dispatches to the appropriate matcher based on the operator family.
func (n *SwitchNode) match(rule SwitchRule, propVal any, msg *flow.Message) (bool, error) {
	switch rule.Type {
	case "eq", "neq", "lt", "lte", "gt", "gte", "btwn":
		return n.matchCompare(rule, propVal, msg)
	case "cont", "regex":
		return n.matchString(rule, propVal, msg)
	case "true", "false", "null", "nnull", "empty", "nempty", "istype":
		return n.matchTypeCheck(rule, propVal)
	default:
		return false, fmt.Errorf("unknown operator: %s", rule.Type)
	}
}

// matchCompare handles all numeric/equality comparisons. eq/neq use loose
// equality; the ordering operators coerce both sides to float64.
func (n *SwitchNode) matchCompare(rule SwitchRule, propVal any, msg *flow.Message) (bool, error) {
	ruleVal, err := n.resolveValue(msg, rule.V, rule.VT, rule.VS)
	if err != nil {
		return false, err
	}

	switch rule.Type {
	case "eq":
		return looseEqual(propVal, ruleVal), nil
	case "neq":
		return !looseEqual(propVal, ruleVal), nil
	case "lt", "lte", "gt", "gte":
		a, aok := coerceToFloat(propVal)
		b, bok := coerceToFloat(ruleVal)
		if !aok || !bok {
			return false, nil
		}
		switch rule.Type {
		case "lt":
			return a < b, nil
		case "lte":
			return a <= b, nil
		case "gt":
			return a > b, nil
		case "gte":
			return a >= b, nil
		}
	case "btwn":
		ruleVal2, err := n.resolveValue(msg, rule.V2, rule.V2T, rule.V2S)
		if err != nil {
			return false, err
		}
		a, aok := coerceToFloat(propVal)
		b, bok := coerceToFloat(ruleVal)
		c, cok := coerceToFloat(ruleVal2)
		if !aok || !bok || !cok {
			return false, nil
		}
		lo, hi := b, c
		if lo > hi {
			lo, hi = hi, lo
		}
		return a >= lo && a <= hi, nil
	}
	return false, nil
}

// matchString handles `contains` and `regex`. Regex was compiled in Init.
func (n *SwitchNode) matchString(rule SwitchRule, propVal any, msg *flow.Message) (bool, error) {
	if rule.Type == "regex" {
		if rule.re == nil {
			return false, nil
		}
		return rule.re.MatchString(fmt.Sprintf("%v", propVal)), nil
	}

	// "cont" — string substring or array-element containment
	ruleVal, err := n.resolveValue(msg, rule.V, rule.VT, rule.VS)
	if err != nil {
		return false, err
	}

	if arr, ok := propVal.([]any); ok {
		for _, el := range arr {
			if looseEqual(el, ruleVal) {
				return true, nil
			}
		}
		return false, nil
	}

	hay := fmt.Sprintf("%v", propVal)
	needle := fmt.Sprintf("%v", ruleVal)
	if !rule.Case {
		hay = strings.ToLower(hay)
		needle = strings.ToLower(needle)
	}
	return strings.Contains(hay, needle), nil
}

// matchTypeCheck handles the value-less type/existence operators.
func (n *SwitchNode) matchTypeCheck(rule SwitchRule, propVal any) (bool, error) {
	switch rule.Type {
	case "true":
		b, ok := propVal.(bool)
		return ok && b, nil
	case "false":
		b, ok := propVal.(bool)
		return ok && !b, nil
	case "null":
		return propVal == nil, nil
	case "nnull":
		return propVal != nil, nil
	case "empty":
		return isEmpty(propVal), nil
	case "nempty":
		return propVal != nil && !isEmpty(propVal), nil
	case "istype":
		return matchesType(propVal, rule.V), nil
	}
	return false, nil
}

// looseEqual implements the spec's loose equality: numeric strings coerce to
// numbers, "true"/"false" strings coerce to booleans. Everything else is
// strict (==). This is intentionally narrower than JS's `==`.
func looseEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}

	// Numeric coercion: both sides convertible to float64.
	if af, aok := coerceToFloat(a); aok {
		if bf, bok := coerceToFloat(b); bok {
			return af == bf
		}
	}

	// Boolean coercion: true == "true", false == "false"
	if ab, ok := a.(bool); ok {
		if bs, ok2 := b.(string); ok2 {
			return (ab && bs == "true") || (!ab && bs == "false")
		}
	}
	if bb, ok := b.(bool); ok {
		if as, ok2 := a.(string); ok2 {
			return (bb && as == "true") || (!bb && as == "false")
		}
	}

	return false
}

// coerceToFloat tries to interpret a value as float64. Accepts numeric types
// and numeric strings. Returns false for anything else (incl. nil and bool).
func coerceToFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// isEmpty reports whether a value is considered empty for the `empty`
// operator: nil, "", empty slice/array, empty map.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// matchesType reports whether v matches the named JSON type. Supported:
// string, number, boolean, array, object, null. Buffer/date intentionally
// excluded for the MVP (see SWITCH_NODE.md open questions).
func matchesType(v any, typeName string) bool {
	switch typeName {
	case "null":
		return v == nil
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := coerceToFloat(v)
		// Exclude bool (which is not numeric in user-facing terms).
		if _, isBool := v.(bool); isBool {
			return false
		}
		// Exclude string (a numeric string is not "of type number").
		if _, isStr := v.(string); isStr {
			return false
		}
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return false
}

// resolveValue delegates to the shared nodes.ResolveValue utility.
func (n *SwitchNode) resolveValue(msg *flow.Message, value, valueType, storage string) (any, error) {
	return nodes.ResolveValue(valueType, value, storage, msg, n.valueContext())
}

// getProperty reads a value from the configured scope + storage.
func (n *SwitchNode) getProperty(msg *flow.Message, scope, key, storage string) (any, error) {
	switch scope {
	case "msg":
		return msg.Get(key), nil
	case "flow", "global":
		if store := nodes.PickContextStore(n.valueContext(), scope, storage); store != nil {
			return store.Get(key)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown scope: %s", scope)
	}
}

func (n *SwitchNode) valueContext() nodes.ValueContext {
	return nodes.ValueContext{
		FlowMem:    n.flowMem,
		FlowPers:   n.flowPers,
		GlobalMem:  n.ctxMem,
		GlobalPers: n.ctxPers,
	}
}

// SwitchTypeInfo returns the NodeTypeInfo for the switch node. Outputs is set
// to match the default rule count; the frontend mirrors len(rules) onto
// node.outputs at runtime, so the engine sees the correct port count.
func SwitchTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "switch",
		Category:    "function",
		Label:       "Switch",
		Description: "Route messages based on property values",
		Icon:        "mdi-call-split",
		Defaults: map[string]any{
			"property":     "payload",
			"propertyType": "msg",
			"checkall":     false,
			"rules": []any{
				map[string]any{"id": "r1", "t": "eq", "v": "", "vt": "str"},
				map[string]any{"id": "r2", "t": "else"},
			},
		},
		Inputs:  1,
		Outputs: 2,
	}
}
