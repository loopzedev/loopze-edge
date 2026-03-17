// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package nats provides an embedded NATS server with JetStream for the Flint
// runtime. It is used for debug message persistence (streams), shared state
// (KV stores), and future fleet communication — but NOT for internal
// node-to-node message routing (which uses Go channels).
package nats

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Broker wraps an embedded NATS server with JetStream enabled and provides
// a client connection for runtime use (creating KV stores, streams, etc.).
type Broker struct {
	server *server.Server
	conn   *nats.Conn
	js     jetstream.JetStream
	opts   *server.Options
}

// Config holds configuration for the embedded NATS broker.
type Config struct {
	// DataDir is the base directory for JetStream storage.
	DataDir string

	// Port is the NATS client port. Use -1 for auto-assigned port (default).
	Port int
}

// New creates and starts an embedded NATS server with JetStream enabled.
// The server stores JetStream data under dataDir/jetstream.
func New(cfg Config) (*Broker, error) {
	storeDir := filepath.Join(cfg.DataDir, "jetstream")

	opts := &server.Options{
		ServerName: "flint-embedded",
		Port:       cfg.Port,
		DontListen: cfg.Port == -1,
		NoSigs:     true,
		JetStream:  true,
		StoreDir:   storeDir,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("nats: failed to create server: %w", err)
	}

	// Route NATS logs through slog.
	ns.SetLoggerV2(newSlogAdapter(), false, false, false)

	// Start the server.
	ns.Start()

	// Wait for the server to be ready.
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		return nil, fmt.Errorf("nats: server not ready after 10s")
	}

	// For in-process connections when not listening on a TCP port.
	var nc *nats.Conn
	if cfg.Port == -1 {
		nc, err = nats.Connect("", nats.InProcessServer(ns))
	} else {
		nc, err = nats.Connect(ns.ClientURL())
	}
	if err != nil {
		ns.Shutdown()
		return nil, fmt.Errorf("nats: failed to connect internal client: %w", err)
	}

	slog.Info("embedded NATS started",
		"in_process", cfg.Port == -1,
		"store_dir", storeDir,
	)

	// Create JetStream context.
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		ns.Shutdown()
		return nil, fmt.Errorf("nats: failed to create jetstream context: %w", err)
	}

	slog.Info("NATS JetStream ready")

	return &Broker{
		server: ns,
		conn:   nc,
		js:     js,
		opts:   opts,
	}, nil
}

// Shutdown gracefully stops the internal client connection and the NATS server.
func (b *Broker) Shutdown() {
	if b.conn != nil {
		b.conn.Close()
	}
	if b.server != nil {
		b.server.Shutdown()
		b.server.WaitForShutdown()
		slog.Info("embedded NATS stopped")
	}
}

// Conn returns the internal NATS client connection.
func (b *Broker) Conn() *nats.Conn {
	return b.conn
}

// JetStream returns the JetStream context for creating streams and KV stores.
func (b *Broker) JetStream() jetstream.JetStream {
	return b.js
}

// ClientURL returns the connection URL for external NATS clients (e.g. nats-cli).
// Returns empty string when running in-process mode (port -1).
func (b *Broker) ClientURL() string {
	if b.opts.DontListen {
		return ""
	}
	return b.server.ClientURL()
}

// InProcessServer returns the underlying NATS server for in-process client connections.
func (b *Broker) InProcessServer() *server.Server {
	return b.server
}

// slogAdapter bridges NATS server logging to Go's slog.
type slogAdapter struct{}

func newSlogAdapter() *slogAdapter { return &slogAdapter{} }

func (s *slogAdapter) Noticef(format string, v ...any) {
	slog.Info(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Warnf(format string, v ...any) {
	slog.Warn(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Fatalf(format string, v ...any) {
	slog.Error(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Errorf(format string, v ...any) {
	slog.Error(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Debugf(format string, v ...any) {
	slog.Debug(fmt.Sprintf(format, v...), "component", "nats")
}

func (s *slogAdapter) Tracef(format string, v ...any) {
	slog.Debug(fmt.Sprintf(format, v...), "component", "nats")
}
