// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cbroglie/mustache"
	"github.com/niceclouds/flint/internal/flow"
)

// TemplateNode renders a Mustache template using values from msg, flow and
// global context, then writes the result into a configurable target property.
type TemplateNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	template     string
	field        string
	fieldType    string
	fieldStorage string
	format       string
	syntax       string

	parsed *mustache.Template

	flowMem    flow.ContextStore
	flowPers   flow.ContextStore
	globalMem  flow.ContextStore
	globalPers flow.ContextStore
}

// NewTemplateNode is the NodeFactory for the template node type.
func NewTemplateNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	return &TemplateNode{config: config}, nil
}

// Init reads the configuration and pre-parses the Mustache template so syntax
// errors surface at deploy time rather than at every message.
func (n *TemplateNode) Init() error {
	props := n.config.Properties

	n.template = stringVal(props, "template", "")
	n.field = stringVal(props, "field", "payload")
	n.fieldType = stringVal(props, "fieldType", "msg")
	n.fieldStorage = stringVal(props, "fieldStorage", "memory")
	n.format = stringVal(props, "format", "plain")
	n.syntax = stringVal(props, "syntax", "mustache")

	if n.syntax == "mustache" && n.template != "" {
		parsed, err := mustache.ParseString(n.template)
		if err != nil {
			return fmt.Errorf("template node %s: parse: %w", n.config.ID, err)
		}
		n.parsed = parsed
	}

	return nil
}

func (n *TemplateNode) SetSend(fn flow.SendFunc)     { n.send = fn }
func (n *TemplateNode) SetStatus(fn flow.StatusFunc) { n.status = fn }
func (n *TemplateNode) SetDebug(fn flow.DebugFunc)   { n.debug = fn }

// SetContext implements flow.ContextProvider.
func (n *TemplateNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.globalMem = globalMem
	n.globalPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start logs the node startup.
func (n *TemplateNode) Start() error {
	slog.Info("template node started",
		"node_id", n.config.ID,
		"format", n.format,
		"syntax", n.syntax,
	)
	return nil
}

// Stop is a no-op for the template node.
func (n *TemplateNode) Stop() error {
	slog.Info("template node stopped", "node_id", n.config.ID)
	return nil
}

// HandleMessage renders the template and writes the result into the target
// property. Render or post-processing errors are returned so the engine can
// route them to Catch nodes.
func (n *TemplateNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
	if msg == nil {
		return nil, nil
	}

	rendered, err := n.render(msg)
	if err != nil {
		if n.status != nil {
			n.status("red", "template error")
		}
		return nil, fmt.Errorf("template render: %w", err)
	}

	value, err := n.applyFormat(rendered)
	if err != nil {
		if n.status != nil {
			n.status("red", "json parse error")
		}
		return nil, fmt.Errorf("template format: %w", err)
	}

	if err := n.writeOutput(msg, value); err != nil {
		return nil, fmt.Errorf("template write: %w", err)
	}

	return [][]*flow.Message{{msg}}, nil
}

// render produces the raw output string from the template.
func (n *TemplateNode) render(msg *flow.Message) (string, error) {
	if n.syntax == "plain" {
		return n.template, nil
	}
	if n.parsed == nil {
		return "", nil
	}
	return n.parsed.Render(n.buildView(msg))
}

// buildView assembles the Mustache context: msg fields at the top level, with
// `flow` and `global` exposed as nested maps populated from the memory stores.
// Reading from persistent stores is intentionally not supported in this phase.
func (n *TemplateNode) buildView(msg *flow.Message) map[string]any {
	src := msg.DataView()
	view := make(map[string]any, len(src)+2)
	for k, v := range src {
		view[k] = v
	}
	view["flow"] = collectStoreValues(n.flowMem)
	view["global"] = collectStoreValues(n.globalMem)
	return view
}

// collectStoreValues snapshots all keys from a context store into a map.
// Returns an empty map when the store is nil or fails to enumerate keys.
func collectStoreValues(store flow.ContextStore) map[string]any {
	out := map[string]any{}
	if store == nil {
		return out
	}
	keys, err := store.Keys()
	if err != nil {
		return out
	}
	for _, k := range keys {
		v, err := store.Get(k)
		if err != nil {
			continue
		}
		out[k] = v
	}
	return out
}

// applyFormat post-processes the rendered string according to the configured
// output format.
func (n *TemplateNode) applyFormat(rendered string) (any, error) {
	switch n.format {
	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	default:
		return rendered, nil
	}
}

// writeOutput stores the rendered value into the configured target.
func (n *TemplateNode) writeOutput(msg *flow.Message, value any) error {
	switch n.fieldType {
	case "msg":
		msg.Set(n.field, value)
		return nil
	case "flow", "global":
		store := PickContextStore(n.valueContext(), n.fieldType, n.fieldStorage)
		if store == nil {
			return nil
		}
		return store.Set(n.field, value)
	default:
		return fmt.Errorf("unknown field scope: %s", n.fieldType)
	}
}

func (n *TemplateNode) valueContext() ValueContext {
	return ValueContext{
		FlowMem:    n.flowMem,
		FlowPers:   n.flowPers,
		GlobalMem:  n.globalMem,
		GlobalPers: n.globalPers,
	}
}

// TemplateTypeInfo returns the NodeTypeInfo for the template node.
func TemplateTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "template",
		Category:    "function",
		Label:       "Template",
		Description: "Build a string from a Mustache template using msg/flow/global values",
		Icon:        "template",
		Defaults: map[string]any{
			"template":     "This is the payload: {{payload}}!",
			"field":        "payload",
			"fieldType":    "msg",
			"fieldStorage": "memory",
			"format":       "plain",
			"syntax":       "mustache",
		},
		Inputs:  1,
		Outputs: 1,
	}
}
