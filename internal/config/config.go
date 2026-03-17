// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package config provides configuration management for the Flint runtime.
// It reads configuration from command-line flags and environment variables,
// applying sensible defaults for standalone edge deployment.
package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the runtime configuration for the Flint application.
type Config struct {
	// Host is the address the HTTP server binds to.
	Host string

	// Port is the TCP port the HTTP server listens on.
	Port int

	// DataDir is the directory where flows, credentials, and key files are stored.
	DataDir string

	// FlowFile is the filename for the flow definitions JSON file (relative to DataDir).
	FlowFile string

	// CredentialsFile is the filename for the encrypted credentials file (relative to DataDir).
	CredentialsFile string

	// KeyFile is the filename for the AES-256-GCM encryption key (relative to DataDir).
	KeyFile string

	// NATSPort is the TCP port for the embedded NATS server. Use -1 for auto-assigned.
	NATSPort int

	// LogLevel controls the minimum log level: debug, info, warn, error.
	LogLevel string
}

// Build-time variables injected via ldflags.
// See Makefile for the -X flags that set these values.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

const (
	defaultHost            = "0.0.0.0"
	defaultPort            = 1880
	defaultDataDir         = "./data"
	defaultFlowFile        = "workspace.json"
	defaultCredentialsFile = "credentials.json"
	defaultKeyFile         = "flint.key"
	defaultNATSPort        = 4222
	defaultLogLevel        = "info"

	envPrefix = "FLINT_"
)

// Load reads configuration from command-line flags and environment variables.
// Precedence order (highest to lowest): flags → environment variables → defaults.
func Load() *Config {
	cfg := &Config{}

	// Define command-line flags.
	flag.StringVar(&cfg.Host, "host", defaultHost, "address to bind the HTTP server to")
	flag.IntVar(&cfg.Port, "port", defaultPort, "port to listen on")
	flag.StringVar(&cfg.DataDir, "data-dir", defaultDataDir, "directory for data storage (flows, credentials, keys)")
	flag.StringVar(&cfg.FlowFile, "flow-file", defaultFlowFile, "filename for flow definitions")
	flag.StringVar(&cfg.CredentialsFile, "credentials-file", defaultCredentialsFile, "filename for encrypted credentials")
	flag.StringVar(&cfg.KeyFile, "key-file", defaultKeyFile, "filename for encryption key")
	flag.IntVar(&cfg.NATSPort, "nats-port", defaultNATSPort, "port for the embedded NATS server (-1 for auto)")
	flag.StringVar(&cfg.LogLevel, "log-level", defaultLogLevel, "log level: debug, info, warn, error")

	flag.Parse()

	// Override with environment variables if flags were not explicitly set.
	applyEnvOverrides(cfg)

	return cfg
}

// applyEnvOverrides checks for FLINT_* environment variables and applies them
// when the corresponding flag was not explicitly provided on the command line.
func applyEnvOverrides(cfg *Config) {
	if v, ok := getenv("HOST"); ok && !flagProvided("host") {
		cfg.Host = v
	}
	if v, ok := getenv("PORT"); ok && !flagProvided("port") {
		if port, err := strconv.Atoi(v); err == nil && port > 0 && port <= 65535 {
			cfg.Port = port
		}
	}
	if v, ok := getenv("DATA_DIR"); ok && !flagProvided("data-dir") {
		cfg.DataDir = v
	}
	if v, ok := getenv("FLOW_FILE"); ok && !flagProvided("flow-file") {
		cfg.FlowFile = v
	}
	if v, ok := getenv("CREDENTIALS_FILE"); ok && !flagProvided("credentials-file") {
		cfg.CredentialsFile = v
	}
	if v, ok := getenv("KEY_FILE"); ok && !flagProvided("key-file") {
		cfg.KeyFile = v
	}
	if v, ok := getenv("NATS_PORT"); ok && !flagProvided("nats-port") {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.NATSPort = port
		}
	}
	if v, ok := getenv("LOG_LEVEL"); ok && !flagProvided("log-level") {
		cfg.LogLevel = v
	}
}

// FlowFilePath returns the full path to the flow definitions file.
func (c *Config) FlowFilePath() string {
	return filepath.Join(c.DataDir, c.FlowFile)
}

// CredentialsFilePath returns the full path to the encrypted credentials file.
func (c *Config) CredentialsFilePath() string {
	return filepath.Join(c.DataDir, c.CredentialsFile)
}

// KeyFilePath returns the full path to the encryption key file.
func (c *Config) KeyFilePath() string {
	return filepath.Join(c.DataDir, c.KeyFile)
}

// ListenAddr returns the formatted host:port address string for the HTTP server.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ParseLogLevel converts the configured log level string to a slog.Level.
// Defaults to slog.LevelInfo for unrecognised values.
func (c *Config) ParseLogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// getenv looks up a FLINT_-prefixed environment variable.
func getenv(key string) (string, bool) {
	return os.LookupEnv(envPrefix + key)
}

// flagProvided returns true if the named flag was explicitly set on the command line.
func flagProvided(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
