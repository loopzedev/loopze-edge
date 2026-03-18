// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package server provides the core HTTP server for the Flint application.
// It wires together the Chi router, REST API routes, WebSocket endpoint,
// and the embedded Vue 3 frontend into a single, cohesive server.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go"

	"github.com/niceclouds/flint/internal/api"
	"github.com/niceclouds/flint/internal/config"
	"github.com/niceclouds/flint/internal/flow"
	flintnats "github.com/niceclouds/flint/internal/nats"
	"github.com/niceclouds/flint/internal/nodes"
	"github.com/niceclouds/flint/internal/storage"
	"github.com/niceclouds/flint/internal/ws"
	"github.com/niceclouds/flint/web"
)

const (
	// ShutdownTimeout is the maximum duration the server will wait for
	// in-flight requests to complete during graceful shutdown.
	ShutdownTimeout = 15 * time.Second
)

// Server is the main HTTP server for Flint. It holds the Chi router,
// application configuration, the flow runtime engine, and the WebSocket hub.
type Server struct {
	// router is the Chi multiplexer that handles all HTTP routing.
	router chi.Router

	// cfg holds the application configuration (host, port, data dir, etc.).
	cfg *config.Config

	// httpServer is the underlying net/http server instance.
	httpServer *http.Server

	// engine is the flow runtime engine that executes deployed flows.
	engine *flow.Engine

	// hub is the WebSocket hub for real-time communication with connected clients.
	hub *ws.Hub

	// broker is the embedded NATS server with JetStream for persistence and messaging.
	broker *flintnats.Broker

	// store is the persistent storage for flows and credentials.
	store storage.Storage
}

// New creates a new Server with the given configuration. It sets up the Chi
// router, registers middleware, mounts API routes, the WebSocket endpoint,
// and the embedded frontend file server.
func New(cfg *config.Config) (*Server, error) {
	// Start the embedded NATS broker with JetStream before anything else.
	broker, err := flintnats.New(flintnats.Config{
		DataDir: cfg.DataDir,
		Port:    cfg.NATSPort,
	})
	if err != nil {
		return nil, fmt.Errorf("server: failed to start embedded NATS: %w", err)
	}

	engine := flow.NewEngine(cfg)
	registerNodes(engine.Registry())

	store, err := storage.NewFileStorage(cfg.FlowFilePath(), cfg.CredentialsFilePath())
	if err != nil {
		return nil, fmt.Errorf("server: failed to create storage: %w", err)
	}

	s := &Server{
		router: chi.NewRouter(),
		cfg:    cfg,
		engine: engine,
		hub:    ws.NewHub(),
		broker: broker,
		store:  store,
	}

	s.setupMiddleware()
	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return s, nil
}

// setupMiddleware registers Chi middleware for all routes. Middleware is
// executed in the order it is added.
func (s *Server) setupMiddleware() {
	// RequestID injects a unique request ID into the context of each request.
	s.router.Use(middleware.RequestID)

	// RealIP extracts the real client IP from X-Forwarded-For / X-Real-IP headers.
	s.router.Use(middleware.RealIP)

	// Structured request logging via slog.
	s.router.Use(slogRequestLogger)

	// Recoverer catches panics in handlers and returns a 500 instead of crashing.
	s.router.Use(middleware.Recoverer)

	// Set response content type for API routes.
	s.router.Use(middleware.Heartbeat("/health"))
}

// setupRoutes configures all HTTP routes: API endpoints, WebSocket, and frontend.
func (s *Server) setupRoutes() {
	// Mount REST API routes under /api/v1/.
	deps := &api.Deps{
		Engine:  s.engine,
		Storage: s.store,
		Broker:  s.broker,
		Hub:     s.hub,
	}
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		api.RegisterRoutes(r, deps)
	})

	// WebSocket endpoint for real-time editor communication.
	s.router.Get("/ws", s.hub.ServeWS)

	// Serve the embedded Vue 3 frontend as a single-page application.
	s.serveFrontend()
}

// serveFrontend configures the router to serve the embedded frontend files.
// It serves static assets from the embedded filesystem and falls back to
// index.html for SPA client-side routing.
func (s *Server) serveFrontend() {
	frontendFS, err := web.GetFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend filesystem, frontend will not be available", "error", err)
		return
	}

	// Serve static files from the embedded filesystem.
	fileServer := http.FileServer(http.FS(frontendFS))

	s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file first.
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if the file exists in the embedded filesystem.
		f, err := frontendFS.Open(path[1:]) // strip leading /
		if err != nil {
			// File not found — serve index.html for SPA client-side routing.
			if errors.Is(err, fs.ErrNotExist) {
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		// File exists — serve it directly.
		fileServer.ServeHTTP(w, r)
	})
}

// Start begins listening for HTTP connections and starts background services.
// It starts the WebSocket hub and flow engine, then begins serving HTTP.
// This method blocks until the server is shut down or encounters a fatal error.
func (s *Server) Start() error {
	// Start the WebSocket hub in a background goroutine.
	go s.hub.Run()

	// Setup NATS streams and KV buckets.
	ctx := context.Background()
	if _, err := s.broker.SetupDebugStream(ctx); err != nil {
		slog.Error("failed to setup debug stream", "error", err)
	}
	if memKV, persKV, err := s.broker.SetupContextKV(ctx); err != nil {
		slog.Error("failed to setup global context KV", "error", err)
	} else {
		s.engine.SetContextStores(
			flintnats.NewKVContextStore(memKV),
			flintnats.NewKVContextStore(persKV),
		)
	}

	// Each flow gets its own dedicated KV buckets so there is zero cross-flow
	// key collision. The factory is called once per flow ID on every Deploy.
	s.engine.SetFlowContextFactory(func(flowID string) (flow.ContextStore, flow.ContextStore) {
		memKV, persKV, err := s.broker.SetupFlowContextKV(context.Background(), flowID)
		if err != nil {
			slog.Error("failed to create flow context KV", "flow_id", flowID, "error", err)
			return nil, nil
		}
		return flintnats.NewKVContextStore(memKV), flintnats.NewKVContextStore(persKV)
	})

	// Wire engine's debug publish to NATS.
	conn := s.broker.Conn()
	s.engine.SetPublishDebug(func(subject string, msg flow.DebugMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("failed to marshal debug message", "error", err)
			return
		}
		if err := conn.Publish(subject, data); err != nil {
			slog.Error("failed to publish debug message", "subject", subject, "error", err)
		}
	})

	// Wire engine's status publish to NATS.
	s.engine.SetPublishStatus(func(subject string, msg flow.StatusMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("failed to marshal status message", "error", err)
			return
		}
		if err := conn.Publish(subject, data); err != nil {
			slog.Error("failed to publish status message", "subject", subject, "error", err)
		}
	})

	// Subscribe to all status messages and broadcast to WebSocket clients.
	if _, err := conn.Subscribe("status.>", func(m *nats.Msg) {
		var status flow.StatusMessage
		if err := json.Unmarshal(m.Data, &status); err != nil {
			slog.Error("failed to unmarshal status message from NATS", "error", err)
			return
		}
		s.hub.Broadcast(ws.EventStatus, status)
	}); err != nil {
		slog.Error("failed to subscribe to status messages", "error", err)
	}

	// Subscribe to all debug messages and broadcast to WebSocket clients.
	if _, err := conn.Subscribe("debug.>", func(m *nats.Msg) {
		var dbg flow.DebugMessage
		if err := json.Unmarshal(m.Data, &dbg); err != nil {
			slog.Error("failed to unmarshal debug message from NATS", "error", err)
			return
		}
		s.hub.Broadcast(ws.EventDebug, dbg)
	}); err != nil {
		slog.Error("failed to subscribe to debug messages", "error", err)
	}

	// Start the flow runtime engine.
	if err := s.engine.Start(); err != nil {
		return fmt.Errorf("server: failed to start flow engine: %w", err)
	}

	// Load saved flows from storage and deploy them.
	if flows, err := s.store.LoadFlows(); err != nil {
		slog.Error("failed to load flows from storage", "error", err)
	} else if len(flows) > 0 {
		if err := s.engine.Deploy(flows); err != nil {
			slog.Error("failed to deploy saved flows", "error", err)
		} else {
			slog.Info("saved flows deployed on startup", "count", len(flows))
		}
	}

	slog.Info("flint server starting",
		"host", s.cfg.Host,
		"port", s.cfg.Port,
		"data_dir", s.cfg.DataDir,
		"address", s.cfg.ListenAddr(),
	)
	slog.Info(fmt.Sprintf("editor available at http://%s", s.cfg.ListenAddr()))

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen failed: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server, waiting for in-flight requests
// to complete within the given context deadline. It also stops the flow engine
// and the WebSocket hub.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("flint server shutting down…")

	// Stop the flow engine first to prevent new messages.
	if err := s.engine.Stop(); err != nil {
		slog.Error("error stopping flow engine", "error", err)
	}

	// Gracefully shut down the HTTP server.
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown failed: %w", err)
	}

	// Stop the embedded NATS broker last (other components may still need it).
	if s.broker != nil {
		s.broker.Shutdown()
	}

	slog.Info("flint server stopped")
	return nil
}

// Router returns the Chi router, primarily useful for testing.
func (s *Server) Router() chi.Router {
	return s.router
}

// Hub returns the WebSocket hub, useful for broadcasting events from other packages.
func (s *Server) Hub() *ws.Hub {
	return s.hub
}

// Engine returns the flow runtime engine.
func (s *Server) Engine() *flow.Engine {
	return s.engine
}

// Broker returns the embedded NATS broker.
func (s *Server) Broker() *flintnats.Broker {
	return s.broker
}

// registerNodes registers all built-in node types on the engine registry.
func registerNodes(registry *flow.NodeRegistry) {
	registry.Register("inject", nodes.NewInjectNode, nodes.InjectTypeInfo())
	registry.Register("debug", nodes.NewDebugNode, nodes.DebugTypeInfo())
	registry.Register("function", nodes.NewFunctionNode, nodes.FunctionTypeInfo())
	registry.Register("context-watch", nodes.NewContextWatchNode, nodes.ContextWatchTypeInfo())
	registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
}

// slogRequestLogger is a Chi-compatible middleware that logs each HTTP request
// using the structured slog logger.
func slogRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			slog.Debug("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
