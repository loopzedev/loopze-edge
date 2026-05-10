// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"sort"
	"sync"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// FlowNodeRegistration describes one flow node that belongs to a group.
type FlowNodeRegistration struct {
	Type    string
	Factory flow.NodeFactory
	Info    flow.NodeTypeInfo
}

// ConfigNodeRegistration describes one config node that belongs to a group.
type ConfigNodeRegistration struct {
	Type    string
	Factory flow.ConfigNodeFactory
	Info    flow.ConfigTypeInfo
}

// Group bundles a set of related nodes (e.g., all S7 nodes) and is the unit
// of enable/disable in the runtime config. Subpackages call RegisterGroup
// from their init() to declare a group; the server applies the registrations
// at startup honoring the user's enable/disable selection.
type Group struct {
	Name        string                   // short identifier, e.g. "s7", "modbus"
	Description string                   // human-readable summary
	Nodes       []FlowNodeRegistration   // flow nodes contributed by this group
	ConfigNodes []ConfigNodeRegistration // config nodes contributed by this group
}

var (
	groupsMu sync.RWMutex
	groups   = map[string]*Group{}
)

// RegisterGroup declares a node group. Intended to be called from package
// init() in each subpackage. Calling RegisterGroup twice with the same name
// accumulates into a single group (useful when a large protocol package
// splits its registrations across several files).
func RegisterGroup(g Group) {
	groupsMu.Lock()
	defer groupsMu.Unlock()
	if existing, ok := groups[g.Name]; ok {
		existing.Nodes = append(existing.Nodes, g.Nodes...)
		existing.ConfigNodes = append(existing.ConfigNodes, g.ConfigNodes...)
		return
	}
	cp := g
	groups[g.Name] = &cp
}

// AllGroups returns shallow copies of every registered group, sorted by name.
func AllGroups() []Group {
	groupsMu.RLock()
	defer groupsMu.RUnlock()
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GroupSelection controls which groups get registered into a flow registry.
//
// Empty Enable means "all groups are candidates" — Disable then filters out
// specific ones. A non-empty Enable acts as an allow-list; Disable still
// applies on top so a group can be blacklisted even if it appears in Enable.
type GroupSelection struct {
	Enable  []string
	Disable []string
}

// Apply registers all groups matching the selection onto the given registry.
// Returns the names of groups that were applied, sorted, for logging.
func Apply(r *flow.NodeRegistry, sel GroupSelection) []string {
	groupsMu.RLock()
	defer groupsMu.RUnlock()

	var enableSet map[string]bool
	if len(sel.Enable) > 0 {
		enableSet = make(map[string]bool, len(sel.Enable))
		for _, n := range sel.Enable {
			enableSet[n] = true
		}
	}
	disableSet := make(map[string]bool, len(sel.Disable))
	for _, n := range sel.Disable {
		disableSet[n] = true
	}

	applied := make([]string, 0, len(groups))
	for name, g := range groups {
		if enableSet != nil && !enableSet[name] {
			continue
		}
		if disableSet[name] {
			continue
		}
		for _, n := range g.Nodes {
			r.Register(n.Type, n.Factory, n.Info)
		}
		for _, c := range g.ConfigNodes {
			r.RegisterConfig(c.Type, c.Factory, c.Info)
		}
		applied = append(applied, name)
	}
	sort.Strings(applied)
	return applied
}
