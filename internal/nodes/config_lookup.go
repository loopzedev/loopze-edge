// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package nodes

import (
	"fmt"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// resolveConfigParams describes the human-readable strings that
// resolveConfigInstance weaves into status updates and error messages.
// The specific phrasing matches what the modbus/opcua nodes used before
// they switched to the helper, so log filters keep working.
type resolveConfigParams struct {
	NodeKind   string // e.g. "modbus-read"
	NodeID     string // the running node's ID
	ConfigKind string // singular noun for the config type — "server", "broker"
	TypeLabel  string // article + concrete type — "a Modbus server", "an OPC UA server"
}

// resolveConfigInstance looks up a config-node instance by ID and type-asserts
// it to *T. On a missing or wrong-typed config it returns a formatted error
// (and pushes a "red" status if status is non-nil) so callers get a single
// line at the start of their Start() method instead of a 12-line ladder.
func resolveConfigInstance[T any](
	lookup flow.ConfigLookupFunc,
	configID string,
	status flow.StatusFunc,
	p resolveConfigParams,
) (*T, error) {
	if lookup == nil {
		return nil, fmt.Errorf("%s %s: config lookup not available", p.NodeKind, p.NodeID)
	}
	inst, ok := lookup(configID)
	if !ok {
		if status != nil {
			status("red", p.ConfigKind+" not found")
		}
		return nil, fmt.Errorf("%s %s: %s %q not found", p.NodeKind, p.NodeID, p.ConfigKind, configID)
	}
	target, ok := any(inst).(*T)
	if !ok {
		return nil, fmt.Errorf("%s %s: config %q is not %s", p.NodeKind, p.NodeID, configID, p.TypeLabel)
	}
	return target, nil
}
