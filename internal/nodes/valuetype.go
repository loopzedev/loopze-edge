// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package nodes

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ValueContext provides the context stores needed for value resolution.
// Nodes populate this from their ContextProvider.SetContext() fields.
type ValueContext struct {
	FlowMem   flow.ContextStore
	FlowPers  flow.ContextStore
	GlobalMem flow.ContextStore
	GlobalPers flow.ContextStore
}

// PickContextStore selects the correct context store based on scope and storage type.
func PickContextStore(ctx ValueContext, scope, storage string) flow.ContextStore {
	switch scope {
	case "flow":
		if storage == "persistent" {
			return ctx.FlowPers
		}
		return ctx.FlowMem
	case "global":
		if storage == "persistent" {
			return ctx.GlobalPers
		}
		return ctx.GlobalMem
	}
	return nil
}

// ResolveValue converts a typed value specification into its actual Go value.
// msg may be nil (e.g. for inject nodes that have no incoming message).
//
// Supported value types:
//   - str: raw string
//   - num: float64
//   - bool: true/false
//   - json: parsed JSON
//   - date: current timestamp ("rfc3339" → RFC3339Nano string, else → epoch ms as float64)
//   - env: OS environment variable
//   - msg: message property (requires non-nil msg)
//   - flow, global: context store value (requires stores in ValueContext)
func ResolveValue(valueType, value, storage string, msg *flow.Message, ctx ValueContext) (any, error) {
	switch valueType {
	case "str":
		return value, nil
	case "num":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", value, err)
		}
		return f, nil
	case "bool":
		return value == "true", nil
	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON %q: %w", value, err)
		}
		return parsed, nil
	case "date":
		if value == "rfc3339" {
			return time.Now().UTC().Format(time.RFC3339Nano), nil
		}
		return float64(time.Now().UnixMilli()), nil
	case "env":
		return os.Getenv(value), nil
	case "msg":
		if msg == nil {
			return nil, nil
		}
		return msg.Get(value), nil
	case "flow", "global":
		if store := PickContextStore(ctx, valueType, storage); store != nil {
			val, err := store.Get(value)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
		return nil, nil
	default:
		return value, nil
	}
}
