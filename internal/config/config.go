// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package config provides configuration management for the LOOPZE runtime.
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
	"time"
)

// Config holds the runtime configuration for the LOOPZE application.
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

	// UsersFile is the filename for the user records file (relative to DataDir).
	// Stores Argon2id password hashes for local users.
	UsersFile string

	// KeyFile is the filename for the AES-256-GCM encryption key (relative to DataDir).
	KeyFile string

	// SessionKeyFile is the filename for the HMAC session-cookie signing
	// key (relative to DataDir). Auto-generated on first run.
	SessionKeyFile string

	// AuthDisable, when true, bypasses authentication entirely. A
	// synthetic admin user is injected into every request. Intended for
	// localhost development and CI.
	AuthDisable bool

	// SessionTTL is the lifetime of a session, sliding-window. Refreshed
	// on every authenticated request.
	SessionTTL time.Duration

	// NATSPort is the TCP port for the embedded NATS server. Use -1 for auto-assigned.
	NATSPort int

	// LogLevel controls the minimum log level: debug, info, warn, error.
	LogLevel string

	// LogBufferSize is the capacity of the in-memory log ring buffer that
	// backs the Terminal Log panel in the editor.
	LogBufferSize int

	// ShowVersion, when true, instructs main to print version information
	// and exit before any runtime initialisation.
	ShowVersion bool

	// BasePath is the URL prefix the application is served under, e.g.
	// "/loopze". Empty means the app is mounted at the root. When set, all
	// routes (API, /ws, frontend) live under this prefix and the embedded
	// frontend is served with a matching <base href> so client-side
	// routing and asset URLs resolve correctly behind a reverse proxy.
	// Must start with "/" and must not end with "/".
	BasePath string

	// TrustedProxies is the list of CIDR blocks (or single IPs) whose
	// X-Forwarded-For / X-Real-IP / Forwarded headers are honoured. Any
	// request whose direct peer is not in this list keeps r.RemoteAddr
	// untouched, so spoofed headers from untrusted upstreams are ignored.
	// Empty means "trust no upstream" — appropriate when the binary is
	// directly internet-facing.
	TrustedProxies []string

	// TrustedOrigins is the list of HTTP Origin values allowed to open
	// WebSocket connections (CSWSH protection). A leading "*." matches
	// any subdomain. Empty falls back to same-origin checks against the
	// request Host header.
	TrustedOrigins []string

	// HTTPNodeRoot is the URL prefix under which flow-defined HTTP
	// endpoints (http-in nodes) are mounted. Routes here bypass auth and
	// CSRF — the user is responsible for any in-flow authentication.
	// Must start with "/" and must not be empty or a bare "/" (that
	// would collide with the management UI). Default: "/endpoint".
	HTTPNodeRoot string
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
	defaultUsersFile       = "users.json"
	defaultKeyFile         = "loopze.key"
	defaultSessionKeyFile  = "loopze.session.key"
	defaultNATSPort        = 4222
	defaultLogLevel        = "info"
	defaultLogBufferSize   = 1000
	defaultSessionTTL      = 12 * time.Hour
	defaultHTTPNodeRoot    = "/endpoint"

	envPrefix = "LOOPZE_"
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
	flag.StringVar(&cfg.UsersFile, "users-file", defaultUsersFile, "filename for user records")
	flag.StringVar(&cfg.KeyFile, "key-file", defaultKeyFile, "filename for encryption key")
	flag.StringVar(&cfg.SessionKeyFile, "session-key-file", defaultSessionKeyFile, "filename for session signing key")
	flag.IntVar(&cfg.NATSPort, "nats-port", defaultNATSPort, "port for the embedded NATS server (-1 for auto)")
	flag.StringVar(&cfg.LogLevel, "log-level", defaultLogLevel, "log level: debug, info, warn, error")
	flag.IntVar(&cfg.LogBufferSize, "log-buffer-size", defaultLogBufferSize, "in-memory log ring buffer capacity (entries)")
	flag.BoolVar(&cfg.AuthDisable, "auth-disable", false, "bypass authentication; inject a synthetic admin (development only)")
	flag.DurationVar(&cfg.SessionTTL, "session-ttl", defaultSessionTTL, "lifetime of an authenticated session (sliding window)")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "print version information and exit")
	flag.BoolVar(&cfg.ShowVersion, "v", false, "print version information and exit (shorthand)")
	flag.StringVar(&cfg.BasePath, "base-path", "", "URL prefix the app is served under (e.g. /loopze); empty for root")
	flag.StringVar(&cfg.HTTPNodeRoot, "http-node-root", defaultHTTPNodeRoot, "URL prefix for flow-defined HTTP endpoints (must not be empty or '/')")
	var trustedProxies, trustedOrigins string
	flag.StringVar(&trustedProxies, "trusted-proxies", "", "comma-separated CIDRs/IPs whose X-Forwarded-* headers are honoured")
	flag.StringVar(&trustedOrigins, "trusted-origins", "", "comma-separated WebSocket origins to accept (e.g. https://app.example.com); empty = same-origin only")

	flag.Parse()

	cfg.TrustedProxies = splitCSV(trustedProxies)
	cfg.TrustedOrigins = splitCSV(trustedOrigins)

	// Override with environment variables if flags were not explicitly set.
	applyEnvOverrides(cfg)

	if cfg.LogBufferSize < 1 {
		cfg.LogBufferSize = defaultLogBufferSize
	}

	cfg.BasePath = NormalizeBasePath(cfg.BasePath)
	cfg.HTTPNodeRoot = NormalizeHTTPNodeRoot(cfg.HTTPNodeRoot)

	return cfg
}

// NormalizeBasePath cleans a configured base path so callers can rely on
// it being either "" (root) or "/segment[/segment...]" with no trailing
// slash. Returns "" when the input is empty or invalid (a single "/" also
// normalises to "" so root deployments stay simple).
func NormalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	return p
}

// NormalizeHTTPNodeRoot cleans the flow-endpoint mount prefix to the form
// "/segment[/segment...]" with no trailing slash. Empty / bare "/" inputs
// fall back to the default ("/endpoint") because mounting flow endpoints
// at the root would collide with the management UI and frontend SPA.
func NormalizeHTTPNodeRoot(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return defaultHTTPNodeRoot
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return defaultHTTPNodeRoot
	}
	return p
}

// splitCSV splits a comma-separated value into a trimmed, non-empty list.
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applyEnvOverrides checks for LOOPZE_* environment variables and applies them
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
	if v, ok := getenv("USERS_FILE"); ok && !flagProvided("users-file") {
		cfg.UsersFile = v
	}
	if v, ok := getenv("KEY_FILE"); ok && !flagProvided("key-file") {
		cfg.KeyFile = v
	}
	if v, ok := getenv("SESSION_KEY_FILE"); ok && !flagProvided("session-key-file") {
		cfg.SessionKeyFile = v
	}
	if v, ok := getenv("AUTH_DISABLE"); ok && !flagProvided("auth-disable") {
		cfg.AuthDisable = parseBool(v)
	}
	if v, ok := getenv("SESSION_TTL"); ok && !flagProvided("session-ttl") {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SessionTTL = d
		}
	}
	if v, ok := getenv("NATS_PORT"); ok && !flagProvided("nats-port") {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.NATSPort = port
		}
	}
	if v, ok := getenv("LOG_LEVEL"); ok && !flagProvided("log-level") {
		cfg.LogLevel = v
	}
	if v, ok := getenv("LOG_BUFFER_SIZE"); ok && !flagProvided("log-buffer-size") {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LogBufferSize = n
		}
	}
	if v, ok := getenv("BASE_PATH"); ok && !flagProvided("base-path") {
		cfg.BasePath = v
	}
	if v, ok := getenv("HTTP_NODE_ROOT"); ok && !flagProvided("http-node-root") {
		cfg.HTTPNodeRoot = v
	}
	if v, ok := getenv("TRUSTED_PROXIES"); ok && !flagProvided("trusted-proxies") {
		cfg.TrustedProxies = splitCSV(v)
	}
	if v, ok := getenv("TRUSTED_ORIGINS"); ok && !flagProvided("trusted-origins") {
		cfg.TrustedOrigins = splitCSV(v)
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

// UsersFilePath returns the full path to the user records file.
func (c *Config) UsersFilePath() string {
	return filepath.Join(c.DataDir, c.UsersFile)
}

// KeyFilePath returns the full path to the encryption key file.
func (c *Config) KeyFilePath() string {
	return filepath.Join(c.DataDir, c.KeyFile)
}

// SessionKeyFilePath returns the full path to the session signing key file.
func (c *Config) SessionKeyFilePath() string {
	return filepath.Join(c.DataDir, c.SessionKeyFile)
}

// parseBool accepts "1"/"true"/"yes"/"on" (case-insensitive) as true.
// Anything else (including empty) is false.
func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// getenv looks up a LOOPZE_-prefixed environment variable.
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
