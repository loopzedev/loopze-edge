// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"log/slog"
	"net/http"
)

// handleGetDashboardLayout returns the current dashboard layout snapshot
// computed by dashboard.BuildLayout on the last successful Deploy.
//
// GET /api/v1/dashboard/layout
func (d *Deps) handleGetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	if d.Dashboard == nil {
		slog.Warn("dashboard handler called without hub configured")
		jsonError(w, http.StatusServiceUnavailable, "dashboard not configured")
		return
	}
	jsonResponse(w, http.StatusOK, d.Dashboard.Snapshot())
}

// handleGetDashboardTheme returns the resolved theme + density + accent
// from the current ui-base config. Convenience endpoint so the
// dashboard SPA can render its shell before the full layout fetch.
//
// GET /api/v1/dashboard/theme
func (d *Deps) handleGetDashboardTheme(w http.ResponseWriter, r *http.Request) {
	if d.Dashboard == nil {
		jsonError(w, http.StatusServiceUnavailable, "dashboard not configured")
		return
	}
	snap := d.Dashboard.Snapshot()
	if snap.Base == nil {
		jsonResponse(w, http.StatusOK, map[string]any{
			"name":        "LOOPZE Dashboard",
			"theme":       "dark",
			"accentColor": "#58a6ff",
			"density":     "default",
			"showNav":     true,
		})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"name":        snap.Base.Name,
		"theme":       snap.Base.Theme,
		"accentColor": snap.Base.AccentColor,
		"density":     snap.Base.Density,
		"showNav":     snap.Base.ShowNav,
	})
}
