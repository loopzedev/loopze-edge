// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package api defines the REST API routes and handlers for the LOOPZE runtime.
// All API endpoints are mounted under /api/v1/ and provide the interface
// between the Vue 3 frontend editor and the Go backend engine.
package api

import (
	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/auth"
)

// RegisterRoutes mounts all LOOPZE API v1 route handlers onto the given router.
// The Deps struct provides access to the flow engine, storage layer and
// auth subsystem.
//
// Layering: every request first runs through Authenticate so handlers and
// nested middlewares can read the user via auth.WithUser. The setup gate
// then blocks all routes except /setup and /auth/* until the first admin
// exists. Inside the gated subtree routes are split into Viewer / Editor
// / Admin groups.
func RegisterRoutes(r chi.Router, deps *Deps) {
	r.Use(deps.AuthMW.Authenticate)

	// Open routes — must work even before first-run setup is complete or
	// when the caller is not authenticated.
	r.Post("/setup", deps.handleSetup)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", deps.handleLogin)
		r.Post("/logout", deps.handleLogout)
		r.Get("/me", deps.handleMe)
		r.Get("/status", deps.handleAuthStatus)
	})

	// Everything below this point requires that the first admin exists.
	r.Group(func(r chi.Router) {
		r.Use(deps.AuthMW.RequireSetupComplete())

		// Read-only routes — viewer or higher.
		r.Group(func(r chi.Router) {
			r.Use(deps.AuthMW.RequireRole(auth.RoleViewer))

			r.Get("/flows", deps.handleGetFlows)
			r.Get("/flows/{id}", deps.handleGetFlow)
			r.Get("/nodes", deps.handleGetNodes)
			r.Get("/configs/types", deps.handleGetConfigTypes)
			r.Get("/settings", deps.handleGetSettings)
			r.Get("/status/nodes", deps.handleGetNodeStatuses)
			r.Get("/debug/messages", deps.handleGetDebugMessages)
			r.Get("/logs", deps.handleGetLogs)

			r.Get("/context/global/{storage}", deps.handleGetContextStore)
			r.Get("/context/global/{storage}/{key}", deps.handleGetContextKey)
			r.Get("/context/flow/{flowID}/{storage}", deps.handleGetContextStore)
			r.Get("/context/flow/{flowID}/{storage}/{key}", deps.handleGetContextKey)

			r.Get("/state-machines", deps.handleListAllStateMachines)
			r.Get("/state-machines/flow/{flowID}", deps.handleListStateMachines)
			r.Get("/state-machines/flow/{flowID}/{nodeID}", deps.handleGetStateMachineSnapshot)

			r.Get("/certs", deps.handleListCerts)
			r.Get("/certs/{id}", deps.handleGetCert)
		})

		// Mutating routes — editor or higher.
		r.Group(func(r chi.Router) {
			r.Use(deps.AuthMW.RequireRole(auth.RoleEditor))

			r.Post("/flows", deps.handleDeployFlows)
			r.Post("/inject/{id}", deps.handleInjectNode)
			r.Post("/opcua/test-connection", deps.handleOpcuaTestConnection)
			r.Post("/opcua/browse", deps.handleOpcuaBrowse)
			r.Post("/opcua/read", deps.handleOpcuaRead)

			r.Delete("/context/global/{storage}", deps.handleClearContextStore)
			r.Delete("/context/global/{storage}/{key}", deps.handleDeleteContextKey)
			r.Delete("/context/flow/{flowID}/{storage}", deps.handleClearContextStore)
			r.Delete("/context/flow/{flowID}/{storage}/{key}", deps.handleDeleteContextKey)

			r.Post("/certs", deps.handleCreateCert)
			r.Put("/certs/{id}", deps.handleUpdateCert)
			r.Delete("/certs/{id}", deps.handleDeleteCert)
			r.Post("/certs/validate", deps.handleValidateCert)
		})

		// User management — admin only.
		r.Group(func(r chi.Router) {
			r.Use(deps.AuthMW.RequireRole(auth.RoleAdmin))

			r.Get("/users", deps.handleListUsers)
			r.Post("/users", deps.handleCreateUser)
			r.Patch("/users/{id}", deps.handleUpdateUser)
			r.Post("/users/{id}/password", deps.handleSetPassword)
		})
	})
}
