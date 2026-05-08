// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package server provides the core HTTP server for the LOOPZE application.
// It wires together the Chi router, REST API routes, WebSocket endpoint,
// and the embedded Vue 3 frontend into a single, cohesive server.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go"

	"github.com/loopzedev/loopze-edge/internal/api"
	"github.com/loopzedev/loopze-edge/internal/auth"
	"github.com/loopzedev/loopze-edge/internal/config"
	"github.com/loopzedev/loopze-edge/internal/flow"
	"github.com/loopzedev/loopze-edge/internal/logbuffer"
	loopzenats "github.com/loopzedev/loopze-edge/internal/nats"
	"github.com/loopzedev/loopze-edge/internal/nodes"
	"github.com/loopzedev/loopze-edge/internal/storage"
	"github.com/loopzedev/loopze-edge/internal/ws"
	"github.com/loopzedev/loopze-edge/web"
)

const (
	// ShutdownTimeout is the maximum duration the server will wait for
	// in-flight requests to complete during graceful shutdown.
	ShutdownTimeout = 15 * time.Second
)

// Server is the main HTTP server for LOOPZE. It holds the Chi router,
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
	broker *loopzenats.Broker

	// store is the persistent storage for flows and credentials.
	store storage.Storage

	// logBuffer holds recent application log entries for the Terminal Log panel.
	logBuffer *logbuffer.Buffer

	// users persists local user records.
	users auth.UserStore

	// sessions issues and validates session cookies.
	sessions *auth.SessionManager

	// authMW provides the request-time auth gates (Authenticate,
	// RequireRole, RequireSetupComplete) and is also reused by the
	// WebSocket handler for WS-upgrade authentication.
	authMW *auth.Middleware

	// proxyChecker decides which TCP peers are allowed to set
	// X-Forwarded-* headers. Empty allowlist disables header rewriting.
	proxyChecker *trustedProxyChecker

	// flowEndpointMux holds the live route table for HTTP endpoints
	// defined by http-in flow nodes. The router is rebuilt and
	// atomically swapped on every engine deploy.
	flowEndpointMux *FlowEndpointMux
}

// New creates a new Server with the given configuration. It sets up the Chi
// router, registers middleware, mounts API routes, the WebSocket endpoint,
// and the embedded frontend file server.
//
// logBuffer may be nil; when nil the /api/v1/logs endpoint returns an empty
// array and no log events are broadcast.
func New(cfg *config.Config, logBuffer *logbuffer.Buffer) (*Server, error) {
	// Start the embedded NATS broker with JetStream before anything else.
	broker, err := loopzenats.New(loopzenats.Config{
		DataDir: cfg.DataDir,
		Port:    cfg.NATSPort,
	})
	if err != nil {
		return nil, fmt.Errorf("server: failed to start embedded NATS: %w", err)
	}

	engine := flow.NewEngine(cfg)
	registerNodes(engine.Registry())

	store, err := storage.NewFileStorage(cfg.FlowFilePath(), cfg.CredentialsFilePath(), cfg.UsersFilePath())
	if err != nil {
		return nil, fmt.Errorf("server: failed to create storage: %w", err)
	}

	users, err := auth.NewFileStore(store)
	if err != nil {
		return nil, fmt.Errorf("server: failed to load user store: %w", err)
	}

	signKey, err := auth.EnsureSessionKey(cfg.SessionKeyFilePath())
	if err != nil {
		return nil, fmt.Errorf("server: failed to ensure session key: %w", err)
	}

	sessionKV, err := broker.SetupSessionKV(context.Background(), cfg.SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("server: failed to setup session KV: %w", err)
	}

	sessions, err := auth.NewSessionManager(auth.NewNATSSessionStore(sessionKV), signKey, cfg.SessionTTL)
	if err != nil {
		return nil, fmt.Errorf("server: failed to create session manager: %w", err)
	}

	authMW := &auth.Middleware{
		Users:    users,
		Sessions: sessions,
	}
	if cfg.AuthDisable {
		slog.Warn("⚠ LOOPZE_AUTH_DISABLE is set — authentication is bypassed; do NOT use in production")
		authMW.DevUser = &auth.User{
			ID:           "dev-bypass",
			Username:     "dev",
			Role:         auth.RoleAdmin,
			AuthProvider: auth.ProviderLocal,
		}
	}

	proxyChecker, err := newTrustedProxyChecker(cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("server: %w", err)
	}

	s := &Server{
		router:          chi.NewRouter(),
		cfg:             cfg,
		engine:          engine,
		hub:             ws.NewHub(cfg.TrustedOrigins),
		broker:          broker,
		store:           store,
		logBuffer:       logBuffer,
		users:           users,
		sessions:        sessions,
		authMW:          authMW,
		proxyChecker:    proxyChecker,
		flowEndpointMux: NewFlowEndpointMux(),
	}

	// Wire the engine to the flow-endpoint mux so http-in nodes can
	// register routes on every deploy. The closure adapts the chi-free
	// flow.HTTPRouteSpec to the server-internal RouteSpec, swaps the
	// router atomically, and translates any conflicts back into the
	// engine-facing form.
	engine.SetHTTPMuxBuilder(func(specs []flow.HTTPRouteSpec) []flow.HTTPRouteConflict {
		return s.swapFlowEndpointRoutes(specs)
	}, cfg.HTTPNodeRoot)

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
	// Heartbeat runs before any base-path stripping so /health answers
	// at the root regardless of where the rest of the app is mounted.
	// This is what L4/L7 health checks expect; reverse proxies typically
	// probe an absolute path that is not subject to subpath routing.
	s.router.Use(middleware.Heartbeat("/health"))

	// Strip the configured base path so route matching, logging and
	// metrics use the canonical path. /health (above) is intentionally
	// outside this strip so it stays root-addressable.
	if s.cfg.BasePath != "" {
		s.router.Use(stripBasePath(s.cfg.BasePath))
	}

	// RequestID injects a unique request ID into the context of each request.
	s.router.Use(middleware.RequestID)

	// Honour X-Forwarded-* / Forwarded headers only when the direct peer
	// is in the configured trusted-proxy allowlist. With no proxies
	// configured the middleware is a no-op and r.RemoteAddr stays the
	// raw TCP peer, so spoofed headers from the open internet are
	// ignored by default.
	s.router.Use(realIPMiddleware(s.proxyChecker))

	// Structured request logging via slog.
	s.router.Use(slogRequestLogger)

	// Recoverer catches panics in handlers and returns a 500 instead of crashing.
	s.router.Use(middleware.Recoverer)
}

// swapFlowEndpointRoutes is the engine-facing builder closure. It
// translates flow.HTTPRouteSpec into the server's RouteSpec, asks the
// FlowEndpointMux to atomically replace its current router, and
// translates any returned conflicts back so the engine can route them
// to the affected nodes' errorFn.
func (s *Server) swapFlowEndpointRoutes(specs []flow.HTTPRouteSpec) []flow.HTTPRouteConflict {
	if s.flowEndpointMux == nil {
		return nil
	}
	internal := make([]RouteSpec, len(specs))
	for i, sp := range specs {
		internal[i] = RouteSpec{
			NodeID:  sp.NodeID,
			Method:  sp.Method,
			Path:    sp.Path,
			Handler: sp.Handler,
		}
	}
	conflicts := s.flowEndpointMux.Swap(internal)
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]flow.HTTPRouteConflict, len(conflicts))
	for i, c := range conflicts {
		out[i] = flow.HTTPRouteConflict{
			NodeID: c.Spec.NodeID,
			Method: c.Spec.Method,
			Path:   c.Spec.Path,
			Reason: c.Reason,
		}
	}
	return out
}

// setupRoutes configures all HTTP routes: API endpoints, WebSocket, and frontend.
func (s *Server) setupRoutes() {
	// Mount the flow-endpoint mux BEFORE the management routes so its
	// reduced middleware (no auth, no CSRF) cannot accidentally inherit
	// any session/CSRF guard. Routes here are deliberately public — the
	// flow author is responsible for any in-flow authentication.
	s.router.Mount(s.cfg.HTTPNodeRoot, s.flowEndpointMux)

	// Mount REST API routes under /api/v1/.
	deps := &api.Deps{
		Engine:    s.engine,
		Storage:   s.store,
		Broker:    s.broker,
		Hub:       s.hub,
		LogBuffer: s.logBuffer,
	}
	deps.Users = s.users
	deps.Sessions = s.sessions
	deps.AuthMW = s.authMW
	deps.Throttle = auth.NewLoginThrottle(nil)
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		// Double-submit-cookie CSRF: issues loopze_csrf cookie on every
		// request and rejects state-changing requests without a matching
		// X-CSRF-Token header.
		r.Use(auth.CSRF())
		api.RegisterRoutes(r, deps)
	})

	// WebSocket endpoint for real-time editor communication. Reuses the
	// auth middleware to validate the session cookie before upgrading.
	s.router.Get("/ws", s.hub.ServeWSAuthed(s.wsAuthFunc()))

	// Tear down open WS connections immediately when their session is
	// invalidated (logout, disable, password reset).
	s.sessions.SetOnDeleted(s.hub.DisconnectUser)

	// Serve the embedded Vue 3 frontend as a single-page application.
	s.serveFrontend()
}

// wsAuthFunc returns the AuthFunc the WebSocket hub uses to validate
// upgrade requests. It honours the same dev-bypass that the HTTP
// middleware uses, so LOOPZE_AUTH_DISABLE turns off WS auth too.
func (s *Server) wsAuthFunc() ws.AuthFunc {
	return func(r *http.Request) (string, error) {
		if s.authMW.DevUser != nil {
			return s.authMW.DevUser.ID, nil
		}
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			return "", nil
		}
		sessionID, err := s.sessions.VerifyCookieValue(cookie.Value)
		if err != nil {
			return "", nil
		}
		sess, err := s.sessions.Get(r.Context(), sessionID)
		if err != nil {
			return "", nil
		}
		user, err := s.users.Get(sess.UserID)
		if err != nil {
			return "", nil
		}
		if user.Disabled {
			return "", nil
		}
		return user.ID, nil
	}
}

// serveFrontend configures the router to serve the embedded frontend files.
// Static assets stream straight from the embedded filesystem; index.html
// is rendered through indexInjector so the runtime base path can be
// stamped into a <base href> tag without rebuilding the frontend.
func (s *Server) serveFrontend() {
	frontendFS, err := web.GetFS()
	if err != nil {
		slog.Warn("failed to load embedded frontend filesystem, frontend will not be available", "error", err)
		return
	}

	indexer, err := newIndexInjector(frontendFS, s.cfg.BasePath)
	if err != nil {
		slog.Warn("failed to load index.html for runtime base-path injection", "error", err)
	}

	fileServer := http.FileServer(http.FS(frontendFS))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		if indexer == nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		body := indexer.render()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(body)
	}

	s.router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" {
			serveIndex(w, r)
			return
		}

		f, err := frontendFS.Open(path[1:])
		if err != nil {
			if errFSNotExist(err) {
				// SPA fallback: client-side route → index.html.
				serveIndex(w, r)
				return
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

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
			loopzenats.NewKVContextStore(memKV),
			loopzenats.NewKVContextStore(persKV),
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
		return loopzenats.NewKVContextStore(memKV), loopzenats.NewKVContextStore(persKV)
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

	// Load saved workspace from storage and deploy.
	if ws, err := s.store.LoadWorkspace(); err != nil {
		slog.Error("failed to load workspace from storage", "error", err)
	} else if len(ws.Flows) > 0 {
		if err := s.engine.Deploy(ws.Flows, ws.Configs, flow.DeployFull); err != nil {
			slog.Error("failed to deploy saved workspace", "error", err)
		} else {
			slog.Info("saved workspace deployed on startup", "flows", len(ws.Flows), "configs", len(ws.Configs))
		}
	}

	slog.Info("loopze server starting",
		"host", s.cfg.Host,
		"port", s.cfg.Port,
		"data_dir", s.cfg.DataDir,
		"address", s.cfg.ListenAddr(),
	)
	slog.Info(fmt.Sprintf("editor available at http://%s%s/", s.cfg.ListenAddr(), s.cfg.BasePath))

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen failed: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server, waiting for in-flight requests
// to complete within the given context deadline. It also stops the flow engine
// and the WebSocket hub.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("loopze server shutting down…")

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

	slog.Info("loopze server stopped")
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
func (s *Server) Broker() *loopzenats.Broker {
	return s.broker
}

// registerNodes registers all built-in node types on the engine registry.
func registerNodes(registry *flow.NodeRegistry) {
	registry.Register("inject", nodes.NewInjectNode, nodes.InjectTypeInfo())
	registry.Register("debug", nodes.NewDebugNode, nodes.DebugTypeInfo())
	registry.Register("function", nodes.NewFunctionNode, nodes.FunctionTypeInfo())
	registry.Register("function-expr", nodes.NewFunctionExprNode, nodes.FunctionExprTypeInfo())
	registry.Register("function-go", nodes.NewFunctionGoNode, nodes.FunctionGoTypeInfo())
	registry.Register("json", nodes.NewJSONParserNode, nodes.JSONParserTypeInfo())
	registry.Register("context-watch", nodes.NewContextWatchNode, nodes.ContextWatchTypeInfo())
	registry.Register("catch", nodes.NewCatchNode, nodes.CatchTypeInfo())
	registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
	registry.Register("delay", nodes.NewDelayNode, nodes.DelayTypeInfo())
	registry.Register("link-in", nodes.NewLinkInNode, nodes.LinkInTypeInfo())
	registry.Register("link-out", nodes.NewLinkOutNode, nodes.LinkOutTypeInfo())
	registry.Register("link-call", nodes.NewLinkCallNode, nodes.LinkCallTypeInfo())
	registry.Register("mqtt-in", nodes.NewMqttInNode, nodes.MqttInTypeInfo())
	registry.Register("mqtt-out", nodes.NewMqttOutNode, nodes.MqttOutTypeInfo())
	registry.Register("modbus-read", nodes.NewModbusReadNode, nodes.ModbusReadTypeInfo())
	registry.Register("modbus-write", nodes.NewModbusWriteNode, nodes.ModbusWriteTypeInfo())
	registry.Register("modbus-parser", nodes.NewModbusParserNode, nodes.ModbusParserTypeInfo())
	registry.Register("opcua-read", nodes.NewOpcuaReadNode, nodes.OpcuaReadTypeInfo())
	registry.Register("opcua-write", nodes.NewOpcuaWriteNode, nodes.OpcuaWriteTypeInfo())
	registry.Register("opcua-subscribe", nodes.NewOpcuaSubscribeNode, nodes.OpcuaSubscribeTypeInfo())
	registry.Register("statemachine", nodes.NewStateMachineNode, nodes.StateMachineTypeInfo())
	registry.Register("status", nodes.NewStatusNode, nodes.StatusTypeInfo())
	registry.Register("switch", nodes.NewSwitchNode, nodes.SwitchTypeInfo())
	registry.Register("template", nodes.NewTemplateNode, nodes.TemplateTypeInfo())
	registry.Register("http-in", nodes.NewHTTPInNode, nodes.HTTPInTypeInfo())
	registry.Register("http-response", nodes.NewHTTPResponseNode, nodes.HTTPResponseTypeInfo())
	registry.Register("http-request", nodes.NewHTTPRequestNode, nodes.HTTPRequestTypeInfo())
	registry.Register("tcp-in", nodes.NewTCPInNode, nodes.TCPInTypeInfo())
	registry.Register("tcp-out", nodes.NewTCPOutNode, nodes.TCPOutTypeInfo())
	registry.Register("tcp-request", nodes.NewTCPRequestNode, nodes.TCPRequestTypeInfo())
	registry.Register("udp-in", nodes.NewUDPInNode, nodes.UDPInTypeInfo())
	registry.Register("udp-out", nodes.NewUDPOutNode, nodes.UDPOutTypeInfo())

	// Config node types.
	registry.RegisterConfig("mqtt-broker", nodes.NewMqttBroker, nodes.MqttBrokerConfigTypeInfo())
	registry.RegisterConfig("modbus-server", nodes.NewModbusServer, nodes.ModbusServerConfigTypeInfo())
	registry.RegisterConfig("opcua-server", nodes.NewOpcuaServer, nodes.OpcuaServerConfigTypeInfo())
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
