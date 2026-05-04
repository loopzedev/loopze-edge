// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

// RouteSpec describes one HTTP route a flow node wants to expose under
// the configured flow-endpoint prefix. The mux owns the chi router; nodes
// only describe their intent via this value type.
//
// Method may be a concrete verb ("GET", "POST", …) or "*" / "" to match
// any verb. Path is chi-style and must start with "/".
type RouteSpec struct {
	NodeID  string
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// RouteConflict reports a spec that could not be registered because it
// collided with another. Both sides of a duplicate get a conflict entry
// so each affected http-in node can flag itself red.
type RouteConflict struct {
	Spec   RouteSpec
	Reason string
}

// routerHolder wraps a chi.Router so atomic.Pointer can hold a typed
// pointer (the bare interface can't be stored directly).
type routerHolder struct {
	r chi.Router
}

// FlowEndpointMux is the dynamic route table for flow-defined HTTP
// endpoints. The current router is held in an atomic.Pointer that is
// replaced atomically on every deploy: in-flight handlers continue to
// run on the captured pointer while new requests resolve through the
// new router. Nodes never mutate the live router.
type FlowEndpointMux struct {
	current atomic.Pointer[routerHolder]
}

// NewFlowEndpointMux returns a mux whose current router is empty
// (every request 404s). Call Swap to install a real route table.
func NewFlowEndpointMux() *FlowEndpointMux {
	m := &FlowEndpointMux{}
	m.current.Store(&routerHolder{r: chi.NewRouter()})
	return m
}

// ServeHTTP delegates to the currently active chi router.
func (m *FlowEndpointMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.current.Load().r.ServeHTTP(w, r)
}

// Swap builds a fresh chi router from specs and atomically replaces the
// previous one. Returns the conflicts detected during build (duplicate
// method+path entries are dropped from the new router and reported here
// so the engine can mark the affected nodes red).
func (m *FlowEndpointMux) Swap(specs []RouteSpec) []RouteConflict {
	router, conflicts := buildRouter(specs)
	m.current.Store(&routerHolder{r: router})
	return conflicts
}

// buildRouter produces a chi.Router from the given specs, detecting
// duplicate (method, path) pairs. Both sides of a duplicate are skipped
// and emitted as conflicts — neither path serves until one of the
// nodes is corrected on the next deploy.
func buildRouter(specs []RouteSpec) (chi.Router, []RouteConflict) {
	seen := make(map[string][]int, len(specs))
	for i, sp := range specs {
		key := normalizeMethod(sp.Method) + " " + sp.Path
		seen[key] = append(seen[key], i)
	}

	skip := make(map[int]bool, len(specs))
	var conflicts []RouteConflict
	for _, indices := range seen {
		if len(indices) <= 1 {
			continue
		}
		for _, i := range indices {
			skip[i] = true
			conflicts = append(conflicts, RouteConflict{
				Spec:   specs[i],
				Reason: fmt.Sprintf("duplicate route %s %s", normalizeMethod(specs[i].Method), specs[i].Path),
			})
		}
	}

	r := chi.NewRouter()
	for i, sp := range specs {
		if skip[i] {
			continue
		}
		method := normalizeMethod(sp.Method)
		if method == "*" {
			r.Handle(sp.Path, sp.Handler)
			continue
		}
		r.Method(method, sp.Path, sp.Handler)
	}
	return r, conflicts
}

// normalizeMethod uppercases a method and treats empty / "*" as the
// any-verb wildcard.
func normalizeMethod(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		return "*"
	}
	return m
}
