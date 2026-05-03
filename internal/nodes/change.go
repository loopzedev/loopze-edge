// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/scripting"
	scriptingexpr "github.com/loopzedev/loopze-edge/internal/scripting/expr"
)

// Rule represents a single change operation within the Change Node.
type Rule struct {
	Type        string // "set", "change", "delete", "move"
	Property    string // Target property name (without scope prefix)
	PropType    string // Target scope: "msg", "flow", "global"
	PropStorage string // "memory" or "persistent" (only when PropType is flow/global)
	To          string // Value or target property
	ToType      string // Value type: "msg", "flow", "global", "str", "num", "bool", "json", "date", "env", "expr"
	ToStorage   string // "memory" or "persistent" (only when ToType is flow/global)
	From        string // Search string (only for "change")
	FromType    string // Search type: "str", "re", "num", "bool", "env"
	FromStorage string // "memory" or "persistent" (only when FromType is flow/global)
	FromRE      *regexp.Regexp        // Compiled regex (only when FromType == "re")
	ToExpr      *scriptingexpr.Program // Compiled expr program (only when ToType == "expr")
}

// ChangeNode manipulates message properties, flow context, and global context
// based on a configurable list of rules. It supports four operations:
// set, change (search/replace), delete, and move.
type ChangeNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Context stores received via ContextProvider.
	ctxMem   flow.ContextStore
	ctxPers  flow.ContextStore
	flowMem  flow.ContextStore
	flowPers flow.ContextStore

	rules []Rule
}

// NewChangeNode is the NodeFactory for the change node type.
func NewChangeNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &ChangeNode{
		config: config,
	}, nil
}

// Init parses the rules array from node configuration.
func (n *ChangeNode) Init() error {
	props := n.config.Properties

	rawRules, ok := props["rules"].([]any)
	if !ok || len(rawRules) == 0 {
		slog.Warn("change node has no rules configured", "node_id", n.config.ID)
		return nil
	}

	for i, raw := range rawRules {
		ruleMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		rule := Rule{
			Type:     stringVal(ruleMap, "t", "set"),
			Property: stringVal(ruleMap, "p", "payload"),
			PropType:    stringVal(ruleMap, "pt", "msg"),
			PropStorage: stringVal(ruleMap, "ps", "memory"),
			To:          stringVal(ruleMap, "to", ""),
			ToType:      stringVal(ruleMap, "tot", "str"),
			ToStorage:   stringVal(ruleMap, "tos", "memory"),
			From:        stringVal(ruleMap, "from", ""),
			FromType:    stringVal(ruleMap, "fromt", "str"),
			FromStorage: stringVal(ruleMap, "froms", "memory"),
		}

		// Compile regex if search type is "re".
		if rule.Type == "change" && rule.FromType == "re" && rule.From != "" {
			re, err := regexp.Compile(rule.From)
			if err != nil {
				return fmt.Errorf("change node %s: rule %d: invalid regex %q: %w",
					n.config.ID, i, rule.From, err)
			}
			rule.FromRE = re
		}

		// Compile expr program if value type is "expr". The empty-string case
		// is left uncompiled so that {valueType: expr, value: ""} doesn't fail
		// deploys for partially configured rules.
		if rule.ToType == "expr" && rule.To != "" {
			prog, err := scriptingexpr.Compile(rule.To, exprEnvShape())
			if err != nil {
				return fmt.Errorf("change node %s: rule %d: invalid expr %q: %w",
					n.config.ID, i, rule.To, err)
			}
			rule.ToExpr = prog
		}

		n.rules = append(n.rules, rule)
	}

	return nil
}

// exprEnvShape declares the variables an expr-typed change rule sees at
// runtime. payload is dynamic (left undefined → AllowUndefinedVariables in
// the engine) so type-checks defer to runtime; topic is always a string;
// msg gives full-message access for less-common fields.
func exprEnvShape() map[string]any {
	return map[string]any{
		"topic": "",
		"msg":   map[string]any{},
	}
}

func (n *ChangeNode) SetSend(fn flow.SendFunc)    { n.send = fn }
func (n *ChangeNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *ChangeNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// SetContext implements flow.ContextProvider.
func (n *ChangeNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start is a no-op for the change node.
func (n *ChangeNode) Start() error {
	slog.Info("change node started",
		"node_id", n.config.ID,
		"rules", len(n.rules),
	)
	return nil
}

// HandleMessage applies all rules sequentially to the incoming message.
func (n *ChangeNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	for i, rule := range n.rules {
		if err := n.applyRule(msg, rule); err != nil {
			slog.Warn("change node rule error",
				"node_id", n.config.ID,
				"rule", i,
				"type", rule.Type,
				"error", err,
			)
		}
	}

	return [][]*flow.Message{{msg}}, nil
}

// Stop is a no-op for the change node.
func (n *ChangeNode) Stop() error {
	slog.Info("change node stopped", "node_id", n.config.ID)
	return nil
}

// applyRule executes a single rule on the message.
func (n *ChangeNode) applyRule(msg *flow.Message, rule Rule) error {
	switch rule.Type {
	case "set":
		return n.applySet(msg, rule)
	case "change":
		return n.applyChange(msg, rule)
	case "delete":
		return n.applyDelete(msg, rule)
	case "move":
		return n.applyMove(msg, rule)
	default:
		return fmt.Errorf("unknown rule type: %s", rule.Type)
	}
}

// applySet sets a property to a resolved value.
func (n *ChangeNode) applySet(msg *flow.Message, rule Rule) error {
	val, err := n.resolveRuleValue(msg, rule)
	if err != nil {
		return fmt.Errorf("set: resolve value: %w", err)
	}

	return n.setProperty(msg, rule.PropType, rule.Property, rule.PropStorage, val)
}

// applyChange performs search-and-replace on a string property.
func (n *ChangeNode) applyChange(msg *flow.Message, rule Rule) error {
	// Get current value as string.
	current, err := n.getProperty(msg, rule.PropType, rule.Property, rule.PropStorage)
	if err != nil {
		return fmt.Errorf("change: get property: %w", err)
	}
	currentStr := fmt.Sprintf("%v", current)

	// Resolve the replacement value.
	replaceVal, err := n.resolveRuleValue(msg, rule)
	if err != nil {
		return fmt.Errorf("change: resolve replacement: %w", err)
	}
	replaceStr := fmt.Sprintf("%v", replaceVal)

	// Perform the replacement.
	var result string
	if rule.FromRE != nil {
		result = rule.FromRE.ReplaceAllString(currentStr, replaceStr)
	} else {
		// Resolve the search value.
		searchVal, err := n.resolveValue(msg, rule.From, rule.FromType, rule.FromStorage)
		if err != nil {
			return fmt.Errorf("change: resolve search: %w", err)
		}
		searchStr := fmt.Sprintf("%v", searchVal)
		result = strings.ReplaceAll(currentStr, searchStr, replaceStr)
	}

	return n.setProperty(msg, rule.PropType, rule.Property, rule.PropStorage, result)
}

// applyDelete removes a property.
func (n *ChangeNode) applyDelete(msg *flow.Message, rule Rule) error {
	return n.deleteProperty(msg, rule.PropType, rule.Property, rule.PropStorage)
}

// applyMove gets a value from source, sets it at target, then deletes the source.
func (n *ChangeNode) applyMove(msg *flow.Message, rule Rule) error {
	// Get the value from source.
	val, err := n.getProperty(msg, rule.PropType, rule.Property, rule.PropStorage)
	if err != nil {
		return fmt.Errorf("move: get source: %w", err)
	}

	// Set at target.
	if err := n.setProperty(msg, rule.ToType, rule.To, rule.ToStorage, val); err != nil {
		return fmt.Errorf("move: set target: %w", err)
	}

	// Delete source.
	_ = n.deleteProperty(msg, rule.PropType, rule.Property, rule.PropStorage)
	return nil
}

// resolveValue delegates to the shared ResolveValue utility.
func (n *ChangeNode) resolveValue(msg *flow.Message, value, valueType, storage string) (any, error) {
	return ResolveValue(valueType, value, storage, msg, n.valueContext())
}

// resolveRuleValue resolves a rule's `to` value, handling the expr value-type
// out of band (it needs the per-rule compiled program) and delegating
// everything else to the generic resolveValue path.
func (n *ChangeNode) resolveRuleValue(msg *flow.Message, rule Rule) (any, error) {
	if rule.ToType == "expr" {
		if rule.ToExpr == nil {
			// Empty expression — null value, mirroring Init's tolerance.
			return nil, nil
		}
		return rule.ToExpr.Run(scripting.MessageEnv(msg))
	}
	return n.resolveValue(msg, rule.To, rule.ToType, rule.ToStorage)
}

// getProperty reads a value from the specified scope + storage.
func (n *ChangeNode) getProperty(msg *flow.Message, scope, key, storage string) (any, error) {
	switch scope {
	case "msg":
		return msg.Get(key), nil
	case "flow", "global":
		if store := n.pickContextStore(scope, storage); store != nil {
			return store.Get(key)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown scope: %s", scope)
	}
}

// setProperty writes a value to the specified scope + storage.
func (n *ChangeNode) setProperty(msg *flow.Message, scope, key, storage string, value any) error {
	switch scope {
	case "msg":
		msg.Set(key, value)
	case "flow", "global":
		if store := n.pickContextStore(scope, storage); store != nil {
			return store.Set(key, value)
		}
	}
	return nil
}

// deleteProperty removes a key from the specified scope + storage.
func (n *ChangeNode) deleteProperty(msg *flow.Message, scope, key, storage string) error {
	switch scope {
	case "msg":
		msg.Delete(key)
	case "flow", "global":
		if store := n.pickContextStore(scope, storage); store != nil {
			return store.Delete(key)
		}
	}
	return nil
}

// valueContext builds a ValueContext from the node's context stores.
func (n *ChangeNode) valueContext() ValueContext {
	return ValueContext{
		FlowMem:    n.flowMem,
		FlowPers:   n.flowPers,
		GlobalMem:  n.ctxMem,
		GlobalPers: n.ctxPers,
	}
}

// pickContextStore selects the correct store based on scope and storage type.
func (n *ChangeNode) pickContextStore(scope, storage string) flow.ContextStore {
	return PickContextStore(n.valueContext(), scope, storage)
}

// stringVal extracts a string from a map with a default fallback.
func stringVal(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

// ChangeTypeInfo returns the NodeTypeInfo for the change node.
func ChangeTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "change",
		Category:    "function",
		Label:       "Change",
		Description: "Set, change, delete or move message properties",
		Icon:        "mdi-pencil",
		Defaults: map[string]any{
			"rules": []any{
				map[string]any{
					"t": "set", "p": "payload", "pt": "msg",
					"to": "", "tot": "str",
				},
			},
		},
		Inputs:  1,
		Outputs: 1,
	}
}
