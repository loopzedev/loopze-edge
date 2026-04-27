// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

// Package scripting provides shared utilities for the three function-node
// engines (Goja JS, expr, Yaegi Go). Each engine lives in its own sub-package
// (goja, expr, yaegi) — this top-level file holds only the helpers that all
// three need: message ↔ env conversion, result application, and buffer-shape
// bridging between the wire format ([]int) and Go's []byte.
package scripting

import (
	"github.com/niceclouds/flint/internal/flow"
)

// MessageEnv builds the env map that single-expression engines (expr) operate
// on. The shape is intentionally flat and minimal — `payload` and `topic` are
// pulled out as top-level fields, and the full `msg` map is exposed for
// escape-hatch access to less-common fields (e.g. `_id`, custom keys).
//
// Returned values are the live message data — callers must not mutate them.
func MessageEnv(msg *flow.Message) map[string]any {
	view := msg.DataView()
	return map[string]any{
		"payload": view["payload"],
		"topic":   view["topic"],
		"msg":     view,
	}
}

// ApplyResult turns the raw engine output into the message that goes
// downstream. Two modes:
//
//   - passThrough = true:  the original message is reused, with `outputProperty`
//     overwritten by `result`. All other fields survive.
//   - passThrough = false: a new message is created carrying only `topic` (from
//     the input) and the result on `outputProperty`. Cleaner output for pipeline
//     transforms where the input fields are not interesting.
//
// outputProperty supports dot-paths via flow.Message.Set (e.g. "payload.value").
// The returned message has a fresh ID in the passThrough=false case; the
// passThrough=true case preserves the input's ID.
func ApplyResult(msg *flow.Message, result any, outputProperty string, passThrough bool) *flow.Message {
	if outputProperty == "" {
		outputProperty = "payload"
	}
	if passThrough {
		msg.Set(outputProperty, result)
		return msg
	}
	out := flow.NewMessage()
	if t := msg.Topic(); t != "" {
		out.SetTopic(t)
	}
	out.Set(outputProperty, result)
	return out
}

// IntsToBuffer converts a wire-format buffer payload into a []byte. mqtt-in's
// buffer mode emits []int (one int per byte); JSON-decoded payloads usually
// arrive as []any with each element a float64 or int64. This helper accepts
// all three shapes plus a passthrough for []byte itself.
//
// Returns nil if the value isn't a recognised buffer shape — callers can use
// IsBufferShape first if they need to distinguish "not a buffer" from "empty
// buffer".
func IntsToBuffer(v any) []byte {
	switch s := v.(type) {
	case []byte:
		return s
	case []int:
		out := make([]byte, len(s))
		for i, n := range s {
			out[i] = byte(n)
		}
		return out
	case []int64:
		out := make([]byte, len(s))
		for i, n := range s {
			out[i] = byte(n)
		}
		return out
	case []any:
		out := make([]byte, len(s))
		for i, item := range s {
			switch n := item.(type) {
			case int:
				out[i] = byte(n)
			case int64:
				out[i] = byte(n)
			case float64:
				out[i] = byte(n)
			}
		}
		return out
	}
	return nil
}

// BufferToInts converts a []byte to the wire-format []int representation that
// the rest of the flow expects. Used by engines that work with []byte
// internally but need to emit a payload compatible with mqtt-out's buffer mode
// and the debug viewer.
func BufferToInts(b []byte) []int {
	out := make([]int, len(b))
	for i, by := range b {
		out[i] = int(by)
	}
	return out
}

// IsBufferShape reports whether v looks like a buffer payload (a slice of
// byte-sized integers). It does NOT require all elements to be in 0..255 —
// the heuristic checks the container type only, since mqtt-in always produces
// well-formed []int and JSON-decoded numeric arrays may be of mixed origin.
func IsBufferShape(v any) bool {
	switch s := v.(type) {
	case []byte, []int, []int64:
		return true
	case []any:
		if len(s) == 0 {
			return false
		}
		// Sample the first element — cheaper than scanning the whole slice
		// and good enough for the typical mqtt-in output shape.
		switch s[0].(type) {
		case int, int64, float64:
			return true
		}
	}
	return false
}
