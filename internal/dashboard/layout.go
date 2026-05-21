// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package dashboard implements the LOOPZE dashboard runtime. It owns
// the dashboard WebSocket hub, the per-widget last-value cache, and the
// layout snapshot computed from each successful Deploy. Widget nodes
// (internal/nodes/dashboard) push values into the hub and register
// input callbacks; the dashboard SPA reads the layout from
// /api/dashboard/layout and connects to /api/dashboard/ws for live
// updates.
package dashboard

import (
	"fmt"
	"sort"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Snapshot is the dashboard's view of the deployed workspace. The hub
// stores one Snapshot at a time, computed by BuildLayout on every
// successful engine Deploy. /api/dashboard/layout and the WS snapshot
// handshake both read from this single source.
type Snapshot struct {
	Base    *LayoutBase    `json:"base,omitempty"`
	Pages   []LayoutPage   `json:"pages"`
	Groups  []LayoutGroup  `json:"groups"`
	Widgets []LayoutWidget `json:"widgets"`
	Errors  []LayoutError  `json:"errors,omitempty"`
}

// LayoutBase mirrors the resolved ui-base config.
type LayoutBase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Theme       string `json:"theme"`
	AccentColor string `json:"accentColor"`
	Auth        string `json:"auth"`
	ShowNav     bool   `json:"showNav"`
	Density     string `json:"density"`
}

// LayoutPage mirrors a ui-page config.
type LayoutPage struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Icon   string `json:"icon"`
	Layout string `json:"layout"`
	Order  int    `json:"order"`
}

// LayoutGroup mirrors a ui-group config and references its parent page.
// X/Y are explicit grid coordinates (0-based) inside the page's
// 12-col grid. Height is in 50 px row units.
type LayoutGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PageID      string `json:"pageId"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Collapsible bool   `json:"collapsible"`
	// Order is legacy; the renderer uses x/y. Kept on the JSON for
	// debugging and during the deprecation window.
	Order int `json:"order"`
}

// LayoutWidget describes one widget instance on the dashboard. Config
// carries every field the widget needs to render — the dashboard SPA
// is config-driven and does not need to know per-type schemas at the
// transport layer.
type LayoutWidget struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
	GroupID string `json:"groupId"`
	// X is the 0-based column index within the group's 12-col grid.
	X int `json:"x"`
	// Y is the 0-based row index within the group's grid (50 px per
	// row unit).
	Y int `json:"y"`
	// Width is the number of group-grid columns this widget spans
	// (1–12). 0 means "full group width".
	Width int `json:"width"`
	// Height is the number of grid rows this widget spans (1–12).
	Height int `json:"height"`
	// Order is the legacy ordering field. Kept on the JSON for
	// debugging during the migration; the renderer uses x/y.
	Order  int            `json:"order"`
	Config map[string]any `json:"config"`
}

// LayoutError is a validation failure surfaced through the snapshot.
// The deploy handler reads these and includes them in the deploy
// response so the editor can render them on the offending nodes.
type LayoutError struct {
	NodeID  string `json:"nodeId,omitempty"`
	Message string `json:"message"`
}

// dashboardWidgetTypes is the set of flow-node types that contribute a
// widget to the dashboard. Extended in PR 4 with ui-chart.
var dashboardWidgetTypes = map[string]bool{
	"ui-button": true,
	"ui-text":   true,
	"ui-led":    true,
	"ui-gauge":  true,
}

// widgetSizeDefault is the per-type fallback used by the layout
// builder when a widget config omits `width`/`height` or carries
// legacy zero values. Same shape exists on the frontend in
// frontend/src/nodes/dashboard/sizing.ts — keep both in sync.
//
// Width meaning: 0 means "full row" (12 cols) at render time; the
// renderer translates it. Height is always >= 1.
type sizeDefault struct{ Width, Height int }

var widgetSizeDefault = map[string]sizeDefault{
	"ui-button": {Width: 0, Height: 1},
	"ui-text":   {Width: 0, Height: 1},
	"ui-led":    {Width: 0, Height: 1},
	"ui-gauge":  {Width: 6, Height: 4},
}

// groupSizeDefault provides defaults for a ui-group's grid box on
// the page. Groups occupy the full row by default (width=12, x=0)
// and stack vertically via migration if no y is set.
var groupSizeDefault = sizeDefault{Width: 12, Height: 6}

func effectiveWidth(widgetType string, cfg map[string]any) int {
	if v, ok := readInt(cfg, "width"); ok && v > 0 {
		return clampInt(v, 1, 12)
	}
	if d, ok := widgetSizeDefault[widgetType]; ok {
		return d.Width
	}
	return 0
}

func effectiveHeight(widgetType string, cfg map[string]any) int {
	if v, ok := readInt(cfg, "height"); ok && v > 0 {
		return clampInt(v, 1, 12)
	}
	if d, ok := widgetSizeDefault[widgetType]; ok {
		return d.Height
	}
	return 1
}

func readInt(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// isDashboardConfigType reports whether the given config-node type is
// part of the dashboard layout (used by the layout builder to filter
// the workspace configs).
func isDashboardConfigType(typ string) bool {
	switch typ {
	case "ui-base", "ui-page", "ui-group", "ui-spacer":
		return true
	}
	return false
}

// BuildLayout computes a Snapshot from the just-deployed workspace.
// Always returns a non-nil snapshot. Validation problems (no ui-base,
// dangling references) appear in snap.Errors; a fatal singleton
// violation returns an empty snapshot with the error attached.
func BuildLayout(ws flow.Workspace) *Snapshot {
	snap := &Snapshot{
		Pages:   []LayoutPage{},
		Groups:  []LayoutGroup{},
		Widgets: []LayoutWidget{},
	}

	var baseConfigs []flow.ConfigNode
	pageConfigs := []flow.ConfigNode{}
	groupConfigs := []flow.ConfigNode{}

	for _, cfg := range ws.Configs {
		if !isDashboardConfigType(cfg.Type) {
			continue
		}
		switch cfg.Type {
		case "ui-base":
			baseConfigs = append(baseConfigs, cfg)
		case "ui-page":
			pageConfigs = append(pageConfigs, cfg)
		case "ui-group":
			groupConfigs = append(groupConfigs, cfg)
		}
		// ui-spacer is layout-only and joins via its parent ui-group config
		// in PR 3 (when the group editor exposes a spacer list).
	}

	if len(baseConfigs) == 0 {
		return snap
	}
	if len(baseConfigs) > 1 {
		snap.Errors = append(snap.Errors, LayoutError{
			Message: fmt.Sprintf("ui-base must be a singleton, found %d", len(baseConfigs)),
		})
		return snap
	}

	base := baseConfigs[0]
	snap.Base = &LayoutBase{
		ID:          base.ID,
		Name:        stringProp(base.Config, "name", "LOOPZE Dashboard"),
		Path:        stringProp(base.Config, "path", "/dashboard"),
		Theme:       stringProp(base.Config, "theme", "dark"),
		AccentColor: stringProp(base.Config, "accentColor", "#58a6ff"),
		Auth:        stringProp(base.Config, "auth", "session"),
		ShowNav:     boolProp(base.Config, "showNav", true),
		Density:     stringProp(base.Config, "density", "default"),
	}

	pageIDs := make(map[string]struct{}, len(pageConfigs))
	for _, p := range pageConfigs {
		pageIDs[p.ID] = struct{}{}
		snap.Pages = append(snap.Pages, LayoutPage{
			ID:     p.ID,
			Name:   stringProp(p.Config, "name", "Page"),
			Path:   stringProp(p.Config, "path", ""),
			Icon:   stringProp(p.Config, "icon", ""),
			Layout: stringProp(p.Config, "layout", "grid"),
			Order:  intProp(p.Config, "order", 0),
		})
	}

	groupIDs := make(map[string]struct{}, len(groupConfigs))
	for _, g := range groupConfigs {
		pageRef := stringProp(g.Config, "page", "")
		if _, ok := pageIDs[pageRef]; !ok {
			snap.Errors = append(snap.Errors, LayoutError{
				NodeID:  g.ID,
				Message: fmt.Sprintf("ui-group %q references unknown page %q", g.ID, pageRef),
			})
			continue
		}
		groupIDs[g.ID] = struct{}{}
		snap.Groups = append(snap.Groups, LayoutGroup{
			ID:     g.ID,
			Name:   stringProp(g.Config, "name", "Group"),
			PageID: pageRef,
			// x/y/width/height filled by the migration pass below
			// so we can derive missing y from cumulative heights of
			// lower-order siblings.
			Width:       clampInt(intProp(g.Config, "width", groupSizeDefault.Width), 1, 12),
			Height:      clampInt(intProp(g.Config, "height", groupSizeDefault.Height), 1, 100),
			Collapsible: boolProp(g.Config, "collapsible", false),
			Order:       intProp(g.Config, "order", 0),
		})
	}

	for _, f := range ws.Flows {
		if f.Disabled {
			continue
		}
		for _, n := range f.Nodes {
			if !dashboardWidgetTypes[n.Type] {
				continue
			}
			groupRef := stringProp(n.Config, "group", "")
			if _, ok := groupIDs[groupRef]; !ok {
				snap.Errors = append(snap.Errors, LayoutError{
					NodeID:  n.ID,
					Message: fmt.Sprintf("widget %q (%s) references unknown group %q", n.ID, n.Type, groupRef),
				})
				continue
			}
			snap.Widgets = append(snap.Widgets, LayoutWidget{
				ID:      n.ID,
				Type:    n.Type,
				Name:    n.Name,
				Label:   stringProp(n.Config, "label", ""),
				Tooltip: stringProp(n.Config, "tooltip", ""),
				GroupID: groupRef,
				Width:   effectiveWidth(n.Type, n.Config),
				Height:  effectiveHeight(n.Type, n.Config),
				Order:   intProp(n.Config, "order", 0),
				Config:  n.Config,
			})
		}
	}

	// Position migration: widgets and groups missing explicit x/y
	// fall back to (x=0, y=cumulative_height_of_lower_order_siblings).
	// Old workspaces stack vertically full-width — same visual as the
	// previous auto-flow renderer.
	migrateWidgetPositions(snap.Widgets)
	migrateGroupPositions(snap.Groups)

	return snap
}

// posSlot tracks the migration state for one widget/group during the
// position-migration post-pass. Shared between widgets and groups so
// the sort + cumulative-height logic doesn't duplicate.
type posSlot struct {
	idx     int    // index into the source slice for write-back
	hasX    bool   // explicit x in config
	hasY    bool   // explicit y in config
	x, y, h int    // values to apply
	order   int    // legacy ordering field — sort key for unset slots
	id      string // tie-breaker
}

// migrateWidgetPositions fills in x/y on widgets that don't carry
// explicit values in their node config. Sorted per group by `order`
// (then id for stability), each widget without `y` gets
// y=cumulative height of earlier widgets. Explicit-y widgets stay
// where the user put them and advance the cumulative counter past
// their footprint so unset siblings stack below.
func migrateWidgetPositions(widgets []LayoutWidget) {
	byGroup := map[string][]*posSlot{}
	for i := range widgets {
		w := &widgets[i]
		s := &posSlot{
			idx:   i,
			order: intProp(w.Config, "order", 0),
			id:    w.ID,
			h:     w.Height,
		}
		if v, ok := readInt(w.Config, "x"); ok {
			s.x = clampInt(v, 0, 11)
			s.hasX = true
		}
		if v, ok := readInt(w.Config, "y"); ok {
			s.y = clampInt(v, 0, 10000)
			s.hasY = true
		}
		byGroup[w.GroupID] = append(byGroup[w.GroupID], s)
	}
	for _, slots := range byGroup {
		sortSlots(slots)
		cumY := 0
		for _, s := range slots {
			if s.hasY {
				if s.y+s.h > cumY {
					cumY = s.y + s.h
				}
			} else {
				s.y = cumY
				cumY += s.h
			}
			widgets[s.idx].X = s.x
			widgets[s.idx].Y = s.y
		}
	}
}

// migrateGroupPositions does the same per page. Group x/y aren't
// read from the group config in Phase 1 (the editor doesn't expose
// them yet); migration just stacks groups vertically by their order.
// When the editor adds x/y for groups, this function will read from
// config the same way migrateWidgetPositions does — for now group
// x/y default to (0, cumulative).
func migrateGroupPositions(groups []LayoutGroup) {
	byPage := map[string][]*posSlot{}
	for i := range groups {
		g := &groups[i]
		byPage[g.PageID] = append(byPage[g.PageID], &posSlot{
			idx:   i,
			order: g.Order,
			id:    g.ID,
			h:     g.Height,
		})
	}
	for _, slots := range byPage {
		sortSlots(slots)
		cumY := 0
		for _, s := range slots {
			s.y = cumY
			cumY += s.h
			groups[s.idx].X = s.x
			groups[s.idx].Y = s.y
		}
	}
}

// sortSlots: explicit-y anchors come first sorted by their y (so
// they stay in place); legacy slots follow in `order` then id.
func sortSlots(slots []*posSlot) {
	sort.SliceStable(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.hasY != b.hasY {
			return a.hasY // anchors first
		}
		if a.hasY {
			return a.y < b.y
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.id < b.id
	})
}

// WidgetIDs returns the set of widget node IDs present in the snapshot.
// Used by the hub to purge cache entries for widgets that no longer
// exist after a deploy.
func (s *Snapshot) WidgetIDs() map[string]struct{} {
	out := make(map[string]struct{}, len(s.Widgets))
	for _, w := range s.Widgets {
		out[w.ID] = struct{}{}
	}
	return out
}

func stringProp(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func intProp(m map[string]any, key string, def int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return def
}

func boolProp(m map[string]any, key string, def bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}
