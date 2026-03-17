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
	"github.com/niceclouds/flint/internal/server"
)

func main() {
	// Load configuration from flags, environment variables, and defaults.
	cfg := config.Load()

	// Initialize structured logger with configured log level.
	logLevel := cfg.ParseLogLevel()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

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
	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

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
