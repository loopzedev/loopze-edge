// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/niceclouds/loopze/internal/flow"
	"github.com/robfig/cron/v3"
)

// cronParser parses 6-field cron expressions (with seconds) plus standard
// descriptors like @hourly. The frontend mirrors these capabilities.
var cronParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// InjectProp defines a single property to set on the injected message.
type InjectProp struct {
	Property string // msg property name (e.g. "payload", "topic", "qos")
	VType    string // value type: str, num, bool, json, date, env, flow, global
	Value    string // raw value string
	Storage  string // "memory" or "persistent" (for flow/global)
}

// InjectNode is a source node that generates messages on a timer or once at startup.
// It has 0 inputs and 1 output. The outgoing message properties are defined by
// a configurable list of rules (props), similar to the Change node's "set" rules.
type InjectNode struct {
	config flow.NodeConfig
	send   flow.SendFunc
	status flow.StatusFunc
	debug  flow.DebugFunc

	// Context stores received via ContextProvider.
	ctxMem   flow.ContextStore
	ctxPers  flow.ContextStore
	flowMem  flow.ContextStore
	flowPers flow.ContextStore

	// Parsed from Properties.
	once     bool           // send one message immediately on Start
	mode     string         // "interval" or "cron"
	interval time.Duration  // recurring interval for interval mode (0 = disabled)
	cronExpr string         // raw cron expression (kept for status/debug)
	schedule cron.Schedule  // parsed cron schedule (nil when mode != cron or parse failed)
	props    []InjectProp   // properties to set on each emitted message

	done chan struct{}
	wg   sync.WaitGroup
}

// NewInjectNode is the NodeFactory for the inject node type.
func NewInjectNode(config flow.NodeConfig) (flow.NodeInstance, error) {
	n := &InjectNode{
		config: config,
		done:   make(chan struct{}),
	}
	return n, nil
}

// Init parses and validates the node configuration from Properties.
func (n *InjectNode) Init() error {
	cfgProps := n.config.Properties

	if v, ok := cfgProps["once"].(bool); ok {
		n.once = v
	}

	n.mode = stringVal(cfgProps, "mode", "interval")

	if v, ok := cfgProps["interval"].(float64); ok && v > 0 {
		n.interval = time.Duration(v) * time.Millisecond
	}

	if expr, ok := cfgProps["cron"].(string); ok {
		n.cronExpr = expr
	}

	// Parse props list (array of rule objects).
	rawProps, ok := cfgProps["props"].([]any)
	if ok && len(rawProps) > 0 {
		for _, raw := range rawProps {
			propMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			n.props = append(n.props, InjectProp{
				Property: stringVal(propMap, "p", "payload"),
				VType:    stringVal(propMap, "vt", "str"),
				Value:    stringVal(propMap, "v", ""),
				Storage:  stringVal(propMap, "vs", "memory"),
			})
		}
	}

	// Default: timestamp payload if no props configured.
	if len(n.props) == 0 {
		n.props = []InjectProp{
			{Property: "payload", VType: "date", Value: "rfc3339"},
		}
	}

	if n.mode == "cron" && n.cronExpr != "" {
		schedule, err := cronParser.Parse(n.cronExpr)
		if err != nil {
			slog.Warn("inject node: invalid cron expression",
				"node_id", n.config.ID, "expression", n.cronExpr, "error", err)
			if n.status != nil {
				n.status("red", "invalid cron expression")
			}
		} else {
			n.schedule = schedule
		}
	}

	if !n.once && !n.hasRecurringTrigger() {
		slog.Warn("inject node has no trigger configured (neither once, interval, nor cron)",
			"node_id", n.config.ID,
		)
	}

	return nil
}

// hasRecurringTrigger reports whether the active trigger mode has a valid
// recurring schedule configured.
func (n *InjectNode) hasRecurringTrigger() bool {
	switch n.mode {
	case "cron":
		return n.schedule != nil
	default:
		return n.interval > 0
	}
}

// SetSend stores the engine-provided callback for sending messages downstream.
func (n *InjectNode) SetSend(fn flow.SendFunc) {
	n.send = fn
}

// SetStatus stores the engine-provided callback for reporting node status.
func (n *InjectNode) SetStatus(fn flow.StatusFunc) {
	n.status = fn
}

// SetDebug stores the engine-provided callback for emitting debug messages.
func (n *InjectNode) SetDebug(fn flow.DebugFunc) {
	n.debug = fn
}

// SetContext implements flow.ContextProvider.
func (n *InjectNode) SetContext(globalMem, globalPers, flowMem, flowPers flow.ContextStore) {
	n.ctxMem = globalMem
	n.ctxPers = globalPers
	n.flowMem = flowMem
	n.flowPers = flowPers
}

// Start begins message generation. If once is true, a message is sent immediately.
// If interval is set, a background goroutine sends messages at the configured rate.
func (n *InjectNode) Start() error {
	if n.send == nil {
		return fmt.Errorf("inject node %s: send function not set", n.config.ID)
	}

	if n.once {
		n.emit()
	}

	switch {
	case n.mode == "cron" && n.schedule != nil:
		n.wg.Add(1)
		go n.cronLoop()
	case n.mode != "cron" && n.interval > 0:
		n.wg.Add(1)
		go n.tickerLoop()
	}

	slog.Info("inject node started",
		"node_id", n.config.ID,
		"once", n.once,
		"mode", n.mode,
		"interval", n.interval,
		"cron", n.cronExpr,
		"props", len(n.props),
	)
	return nil
}

// HandleMessage allows manual triggering of the inject node (e.g. via API).
// The incoming message is ignored; a new message is generated and sent.
func (n *InjectNode) HandleMessage(_ *flow.Message) ([][]*flow.Message, error) {
	n.emit()
	return nil, nil
}

// Stop signals the background goroutine to exit and waits for it to finish.
func (n *InjectNode) Stop() error {
	close(n.done)
	n.wg.Wait()

	slog.Info("inject node stopped", "node_id", n.config.ID)
	return nil
}

// valueContext builds a ValueContext from the node's context stores.
func (n *InjectNode) valueContext() ValueContext {
	return ValueContext{
		FlowMem:    n.flowMem,
		FlowPers:   n.flowPers,
		GlobalMem:  n.ctxMem,
		GlobalPers: n.ctxPers,
	}
}

// emit creates a new message, applies all configured props, and sends it to output port 0.
func (n *InjectNode) emit() {
	msg := flow.NewMessage()
	ctx := n.valueContext()

	for _, prop := range n.props {
		val, err := ResolveValue(prop.VType, prop.Value, prop.Storage, nil, ctx)
		if err != nil {
			slog.Warn("inject node: failed to resolve property",
				"node_id", n.config.ID, "property", prop.Property, "error", err)
			continue
		}
		if val != nil {
			msg.Set(prop.Property, val)
		}
	}

	n.send(0, msg)
}

// tickerLoop runs in a goroutine and emits messages at the configured interval.
func (n *InjectNode) tickerLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()

	for {
		select {
		case <-n.done:
			return
		case <-ticker.C:
			n.emit()
		}
	}
}

// cronLoop runs in a goroutine and emits messages at each next-fire time
// computed by the parsed cron schedule. It uses a one-shot timer per fire
// rather than a fixed Ticker so that uneven intervals (e.g. weekday-only
// schedules) are honoured correctly.
func (n *InjectNode) cronLoop() {
	defer n.wg.Done()

	for {
		next := n.schedule.Next(time.Now())
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-n.done:
			timer.Stop()
			return
		case <-timer.C:
			n.emit()
		}
	}
}

// InjectTypeInfo returns the NodeTypeInfo for registering the inject node.
func InjectTypeInfo() flow.NodeTypeInfo {
	return flow.NodeTypeInfo{
		Type:        "inject",
		Category:    "common",
		Label:       "Inject",
		Description: "Injects a message into the flow at startup or at a recurring interval",
		Icon:        "mdi-play",
		Defaults: map[string]any{
			"once":     false,
			"mode":     "interval",
			"interval": 0,
			"cron":     "",
			"props": []any{
				map[string]any{"p": "payload", "vt": "date", "v": "rfc3339"},
				map[string]any{"p": "topic", "vt": "str", "v": ""},
			},
		},
		Inputs:  0,
		Outputs: 1,
	}
}
