// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package main is the entry point for the Flint industrial flow automation platform.
// It parses command-line flags, initializes the runtime, and starts the HTTP server
// with graceful shutdown support.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/niceclouds/flint/internal/config"
	"github.com/niceclouds/flint/internal/logbuffer"
	"github.com/niceclouds/flint/internal/server"
	"github.com/niceclouds/flint/internal/ws"
)

func main() {
	// Load configuration from flags, environment variables, and defaults.
	cfg := config.Load()

	// Initialize structured logger with configured log level. The logbuffer
	// handler wraps the stdout TextHandler so every record is also captured
	// into an in-memory ring buffer (for the Terminal Log panel) without
	// changing the stdout output. The notify callback is wired below once
	// the WebSocket hub exists.
	logLevel := cfg.ParseLogLevel()
	logBuf := logbuffer.New(cfg.LogBufferSize)
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	logHandler := logbuffer.NewHandler(textHandler, logBuf)
	slog.SetDefault(slog.New(logHandler))

	slog.Info("starting Flint",
		"version", config.Version,
		"commit", config.Commit,
		"log_level", cfg.LogLevel,
	)

	slog.Info("configuration loaded",
		"host", cfg.Host,
		"port", cfg.Port,
		"data_dir", cfg.DataDir,
	)

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		slog.Error("failed to create data directory", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	// Create and start the server.
	srv, err := server.New(cfg, logBuf)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	// Now that the WebSocket hub exists, route every captured log entry to
	// all connected clients. Hub.Broadcast is non-blocking (drops on full
	// channel), which keeps any reentrant log line — e.g. the hub itself
	// warning about a full channel — from spinning into a hot loop.
	logHandler.SetNotify(func(e logbuffer.LogEntry) {
		srv.Hub().Broadcast(ws.EventLog, e)
	})

	// Start the server in a goroutine so we can listen for shutdown signals.
	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("Flint is running", "address", cfg.ListenAddr())

	// Wait for interrupt signal for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("shutdown signal received", "signal", sig.String())

	// Create a context with timeout for the graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("Flint stopped gracefully")
}
