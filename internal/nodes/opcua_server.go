// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/ua"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// init turns on gopcua's verbose debug logging when LOOPZE_OPCUA_DEBUG=1 is
// set in the environment. The library prints to stderr; combined with our
// own slog Debug level it gives a full request/response trace per session.
func init() {
	if os.Getenv("LOOPZE_OPCUA_DEBUG") != "" {
		debug.Enable = true
	}
}

// OpcuaServer is a config node that manages a shared OPC UA client session.
// Multiple opcua-read, opcua-subscribe and opcua-write nodes can reference
// the same OpcuaServer instance and share its session and reconnect cycle.
//
// The underlying gopcua client owns the reconnect loop (AutoReconnect=true).
// We subscribe to its state-change channel to translate ConnState into the
// LOOPZE status colour scheme and to broadcast it to every registered node.
//
// Implements flow.ConfigInstance.
type OpcuaServer struct {
	id   string
	name string

	// Connection parameters captured at construction time so a Re-Deploy
	// (Stop+Start without re-creating the instance) can reconnect cleanly.
	endpointURL    string
	clientOptions  []opcua.Option
	requestTimeout time.Duration

	// Certificate authentication state. Resolved during Start (after the
	// engine has injected certStore) so a stored client-pair entry is
	// applied to gopcua via CertificateFile / PrivateKeyFile. The legacy
	// fields are honoured for one release of grace with a deprecation
	// WARN; certRef and the legacy paths are mutually exclusive.
	certRef              string
	legacyClientCertFile string
	legacyClientKeyFile  string
	certStore            *credentials.CertStore

	// tempCertPath / tempKeyPath are non-empty when an inline-source
	// cert entry was materialised to disk for gopcua to consume; they
	// are removed in Stop().
	tempCertPath string
	tempKeyPath  string

	mu     sync.RWMutex
	client *opcua.Client
	cancel context.CancelFunc
	stateC chan opcua.ConnState

	statusFuncs  []flow.StatusFunc
	currentFill  string
	currentText  string

	// Reconnect callbacks fired after every successful (re-)connect — used
	// by Subscribe nodes to recreate their server-side subscriptions.
	reconnectCallbacks []func()

	// typeCache holds NodeID → TypeID lookups shared between Read/Subscribe
	// /Write so each NodeID's DataType is resolved at most once per session.
	// Invalidated on reconnect.
	typeCache *typeCache

	// structResolver loads server-side StructureDefinition / EnumDefinition
	// once per data type and feeds the binary codec for ExtensionObjects.
	// Lives next to typeCache because both share the same reset-on-reconnect
	// semantics.
	structResolver *structResolver

	// prewarmed tracks Variable NodeIDs whose schema has already been
	// looked up so PrewarmStructForVariable becomes a near-zero-cost call
	// after the first hit. Reset on reconnect alongside the type cache.
	prewarmedMu sync.RWMutex
	prewarmed   map[string]bool

	// browseNameCache stores resolved BrowseName values per Variable NodeID
	// so the "by-name" output shape doesn't trigger one extra Read attribute
	// call per node on every cycle. Reset on reconnect — BrowseNames don't
	// change at runtime in well-behaved servers, but a server restart could.
	browseNameMu    sync.RWMutex
	browseNameCache map[string]string
}

// NewOpcuaServer constructs an OpcuaServer instance from a workspace
// ConfigNode. Only the endpoint URL is strictly required; everything else
// falls back to safe defaults.
func NewOpcuaServer(cfg flow.ConfigNode) (flow.ConfigInstance, error) {
	props := cfg.Config

	endpoint, _ := props["endpointUrl"].(string)
	if endpoint == "" {
		return nil, fmt.Errorf("opcua-server %s: endpointUrl is required", cfg.ID)
	}

	securityPolicy, _ := props["securityPolicy"].(string)
	if securityPolicy == "" {
		securityPolicy = "None"
	}
	securityMode, _ := props["securityMode"].(string)
	if securityMode == "" {
		securityMode = "None"
	}
	// Enforce spec rule: SecurityPolicy=None implies SecurityMode=None.
	if securityPolicy == "None" {
		securityMode = "None"
	}

	authMode, _ := props["authMode"].(string)
	if authMode == "" {
		authMode = "anonymous"
	}

	applicationURI, _ := props["applicationUri"].(string)
	if applicationURI == "" {
		applicationURI = "urn:loopze:client"
	}
	applicationName, _ := props["applicationName"].(string)
	if applicationName == "" {
		applicationName = "LOOPZE OPC UA Client"
	}

	sessionTimeout := 60 * time.Second
	if v, ok := props["sessionTimeout"].(float64); ok && v > 0 {
		sessionTimeout = time.Duration(v) * time.Millisecond
	}
	requestTimeout := 5 * time.Second
	if v, ok := props["requestTimeout"].(float64); ok && v > 0 {
		requestTimeout = time.Duration(v) * time.Millisecond
	}

	opts := []opcua.Option{
		opcua.SecurityPolicy(securityPolicyURI(securityPolicy)),
		opcua.SecurityModeString(securityMode),
		opcua.ApplicationName(applicationName),
		opcua.ApplicationURI(applicationURI),
		opcua.SessionTimeout(sessionTimeout),
		opcua.AutoReconnect(true),
		opcua.ReconnectInterval(2 * time.Second),
	}

	certRef, _ := props["certRef"].(string)
	legacyClientCertFile, _ := props["clientCertFile"].(string)
	legacyClientKeyFile, _ := props["clientKeyFile"].(string)
	if certRef != "" && (legacyClientCertFile != "" || legacyClientKeyFile != "") {
		return nil, fmt.Errorf("opcua-server %s: certRef and clientCertFile/clientKeyFile are mutually exclusive", cfg.ID)
	}

	switch authMode {
	case "username":
		username, _ := props["username"].(string)
		password, _ := props["password"].(string)
		opts = append(opts, opcua.AuthUsername(username, password))
	case "certificate":
		// Certificate auth still pairs with anonymous user identity in
		// gopcua's option model — the actual cert/key options are added
		// in Start() after SetCertStore has run, so a stored entry can
		// be resolved (file source) or materialised (inline source).
		opts = append(opts, opcua.AuthAnonymous())
	default:
		opts = append(opts, opcua.AuthAnonymous())
	}

	srv := &OpcuaServer{
		id:                   cfg.ID,
		name:                 cfg.Name,
		endpointURL:          endpoint,
		clientOptions:        opts,
		requestTimeout:       requestTimeout,
		currentFill:          "grey",
		currentText:          "disconnected",
		typeCache:            newTypeCache(),
		certRef:              certRef,
		legacyClientCertFile: legacyClientCertFile,
		legacyClientKeyFile:  legacyClientKeyFile,
	}
	srv.structResolver = newStructResolver(srv)
	return srv, nil
}

// OpcuaServerConfigTypeInfo returns the registry metadata so the frontend
// knows the type exists and can populate sensible defaults.
func OpcuaServerConfigTypeInfo() flow.ConfigTypeInfo {
	return flow.ConfigTypeInfo{
		Type:        "opcua-server",
		Label:       "OPC UA Server",
		Description: "Connection to an OPC UA server (opc.tcp endpoint)",
		Defaults: map[string]any{
			"endpointUrl":       "opc.tcp://localhost:4840",
			"securityPolicy":    "None",
			"securityMode":      "None",
			"authMode":          "anonymous",
			"username":          "",
			"password":          "",
			"clientCertFile":    "",
			"clientKeyFile":     "",
			"applicationUri":    "urn:loopze:client",
			"applicationName":   "LOOPZE OPC UA Client",
			"sessionTimeout":    60000,
			"requestTimeout":    5000,
			"keepaliveInterval": 10000,
			"serverCertTrust":   "pinned",
		},
	}
}

// SetCertStore implements flow.CertStoreProvider. Called by the engine
// between the factory and Start so cert refs on the config can be
// resolved against the live cert catalogue.
func (s *OpcuaServer) SetCertStore(c *credentials.CertStore) {
	s.certStore = c
}

// applyCertOptionsLocked resolves the configured cert reference (or the
// legacy file paths) into gopcua's CertificateFile / PrivateKeyFile
// options and returns the augmented option slice. For inline-source
// cert entries the PEM material is materialised to a private temp file
// in s.tempCertPath / s.tempKeyPath; Stop is responsible for removing
// these.
//
// Caller must hold s.mu.
func (s *OpcuaServer) applyCertOptionsLocked(base []opcua.Option) ([]opcua.Option, error) {
	switch {
	case s.certRef != "":
		if s.certStore == nil {
			return nil, fmt.Errorf("certRef %q is set but no cert store is wired in", s.certRef)
		}
		entry, ok := s.certStore.Get(s.certRef)
		if !ok {
			return nil, fmt.Errorf("certRef %q not found in cert store", s.certRef)
		}
		if entry.Type != credentials.TypeClientPair {
			return nil, fmt.Errorf("certRef %q is type %q, want %q", s.certRef, entry.Type, credentials.TypeClientPair)
		}
		certPath, keyPath, err := s.materialiseCertEntryLocked(entry)
		if err != nil {
			return nil, err
		}
		base = append(base, opcua.CertificateFile(certPath), opcua.PrivateKeyFile(keyPath))

	case s.legacyClientCertFile != "" || s.legacyClientKeyFile != "":
		slog.Warn("opcua-server: clientCertFile/clientKeyFile are deprecated, migrate to certRef",
			"id", s.id, "name", s.name,
		)
		if s.legacyClientCertFile != "" {
			base = append(base, opcua.CertificateFile(s.legacyClientCertFile))
		}
		if s.legacyClientKeyFile != "" {
			base = append(base, opcua.PrivateKeyFile(s.legacyClientKeyFile))
		}
	}
	return base, nil
}

// materialiseCertEntryLocked returns disk paths suitable for gopcua's
// CertificateFile / PrivateKeyFile options. File-source entries pass
// through verbatim. Inline-source entries are written to private temp
// files (0600) so gopcua can consume them; Stop deletes the temps.
//
// Caller must hold s.mu.
func (s *OpcuaServer) materialiseCertEntryLocked(entry credentials.CertEntry) (certPath, keyPath string, err error) {
	if entry.Source == credentials.SourceFile {
		return entry.CertPath, entry.KeyPath, nil
	}
	// Inline source: materialise to disk for gopcua.
	cleanup := func() {
		if s.tempCertPath != "" {
			_ = os.Remove(s.tempCertPath)
			s.tempCertPath = ""
		}
		if s.tempKeyPath != "" {
			_ = os.Remove(s.tempKeyPath)
			s.tempKeyPath = ""
		}
	}
	certFile, err := os.CreateTemp("", "loopze-opcua-cert-*.pem")
	if err != nil {
		return "", "", fmt.Errorf("create temp cert file: %w", err)
	}
	if _, err := certFile.WriteString(entry.CertPEM); err != nil {
		certFile.Close()
		_ = os.Remove(certFile.Name())
		return "", "", fmt.Errorf("write temp cert: %w", err)
	}
	certFile.Close()
	if err := os.Chmod(certFile.Name(), 0o600); err != nil {
		_ = os.Remove(certFile.Name())
		return "", "", fmt.Errorf("chmod temp cert: %w", err)
	}
	s.tempCertPath = certFile.Name()

	keyFile, err := os.CreateTemp("", "loopze-opcua-key-*.pem")
	if err != nil {
		cleanup()
		return "", "", fmt.Errorf("create temp key file: %w", err)
	}
	if _, err := keyFile.WriteString(entry.KeyPEM); err != nil {
		keyFile.Close()
		cleanup()
		_ = os.Remove(keyFile.Name())
		return "", "", fmt.Errorf("write temp key: %w", err)
	}
	keyFile.Close()
	if err := os.Chmod(keyFile.Name(), 0o600); err != nil {
		cleanup()
		_ = os.Remove(keyFile.Name())
		return "", "", fmt.Errorf("chmod temp key: %w", err)
	}
	s.tempKeyPath = keyFile.Name()

	return s.tempCertPath, s.tempKeyPath, nil
}

// removeTempCertFilesLocked cleans up any per-deploy temp files left by
// materialiseCertEntryLocked. Idempotent. Caller must hold s.mu.
func (s *OpcuaServer) removeTempCertFilesLocked() {
	if s.tempCertPath != "" {
		_ = os.Remove(s.tempCertPath)
		s.tempCertPath = ""
	}
	if s.tempKeyPath != "" {
		_ = os.Remove(s.tempKeyPath)
		s.tempKeyPath = ""
	}
}

// Start opens the OPC UA connection asynchronously: the gopcua client owns
// the reconnect loop; we run a goroutine that translates ConnState into the
// LOOPZE status colour scheme and fires reconnect callbacks for nodes that
// need to recreate server-side state.
func (s *OpcuaServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stateC := make(chan opcua.ConnState, 8)
	opts := append([]opcua.Option{}, s.clientOptions...)
	opts, err := s.applyCertOptionsLocked(opts)
	if err != nil {
		s.setStatusLocked("red", err.Error())
		return fmt.Errorf("opcua-server %s: %w", s.id, err)
	}
	opts = append(opts, opcua.StateChangedCh(stateC))

	client, err := opcua.NewClient(s.endpointURL, opts...)
	if err != nil {
		s.removeTempCertFilesLocked()
		s.setStatusLocked("red", err.Error())
		return fmt.Errorf("opcua-server %s: NewClient: %w", s.id, err)
	}
	s.client = client
	s.stateC = stateC
	s.setStatusLocked("yellow", "connecting...")

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.watchState(ctx, stateC)
	go s.connect(ctx, client)

	slog.Info("opcua server starting", "id", s.id, "endpoint", s.endpointURL)
	return nil
}

// Stop tears down the client and the watcher goroutine.
func (s *OpcuaServer) Stop() error {
	s.mu.Lock()
	client := s.client
	cancel := s.cancel
	s.removeTempCertFilesLocked()
	s.client = nil
	s.cancel = nil
	s.stateC = nil
	s.setStatusLocked("grey", "stopped")
	s.mu.Unlock()

	if client != nil {
		ctx, cc := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Close(ctx)
		cc()
	}
	if cancel != nil {
		cancel()
	}
	slog.Info("opcua server stopped", "id", s.id)
	return nil
}

// Status returns the cached colour/text combination.
func (s *OpcuaServer) Status() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentFill, s.currentText
}

// Client exposes the underlying gopcua client to operation nodes (Read,
// Subscribe, Write). Returns nil while the connection is not yet up.
func (s *OpcuaServer) Client() *opcua.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// EndpointURL is read-only metadata exposed for diagnostics and the
// /api/v1/opcua/test-connection round-trip.
func (s *OpcuaServer) EndpointURL() string {
	return s.endpointURL
}

// RequestTimeout is the per-service-call timeout — operation nodes wrap their
// Read/Write/Browse calls in a context with this deadline.
func (s *OpcuaServer) RequestTimeout() time.Duration {
	return s.requestTimeout
}

// RegisterStatusFunc lets an operation node receive status updates and the
// current snapshot immediately so its UI is in sync without waiting for the
// next state change.
func (s *OpcuaServer) RegisterStatusFunc(fn flow.StatusFunc) {
	s.mu.Lock()
	s.statusFuncs = append(s.statusFuncs, fn)
	fill, text := s.currentFill, s.currentText
	s.mu.Unlock()

	fn(fill, text)
}

// RegisterReconnectCallback is used by Subscribe nodes to recreate their
// server-side subscriptions after a reconnect. The callback is invoked once
// per (re-)connect, off the watcher goroutine.
func (s *OpcuaServer) RegisterReconnectCallback(fn func()) {
	s.mu.Lock()
	s.reconnectCallbacks = append(s.reconnectCallbacks, fn)
	s.mu.Unlock()
}

// connect kicks off the initial connection. AutoReconnect handles subsequent
// drops so we do NOT loop here on failure — a permanent error surfaces as a
// red status update via watchState.
func (s *OpcuaServer) connect(ctx context.Context, client *opcua.Client) {
	if err := client.Connect(ctx); err != nil {
		s.mu.Lock()
		s.setStatusLocked("red", err.Error())
		s.mu.Unlock()
		slog.Warn("opcua server initial connect failed", "id", s.id, "error", err)
	}
}

// watchState consumes the gopcua state channel and maps each transition to
// a status update + (on every transition INTO Connected) the registered
// reconnect callbacks.
func (s *OpcuaServer) watchState(ctx context.Context, stateC <-chan opcua.ConnState) {
	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-stateC:
			if !ok {
				return
			}
			fill, text := mapConnState(state)
			s.mu.Lock()
			s.setStatusLocked(fill, text)
			callbacks := append([]func(){}, s.reconnectCallbacks...)
			s.mu.Unlock()

			// Drop the type cache when the link breaks so the next reconnect
			// re-discovers DataTypes — server schema may have changed while
			// we were away.
			if state == opcua.Disconnected || state == opcua.Reconnecting {
				s.ResetTypeCache()
				if s.structResolver != nil {
					s.structResolver.reset()
				}
				s.prewarmedMu.Lock()
				s.prewarmed = nil
				s.prewarmedMu.Unlock()
				s.browseNameMu.Lock()
				s.browseNameCache = nil
				s.browseNameMu.Unlock()
			}

			if state == opcua.Connected {
				for _, cb := range callbacks {
					go func(cb func()) {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("opcua server reconnect callback panicked",
									"id", s.id, "panic", r)
							}
						}()
						cb()
					}(cb)
				}
			}
		}
	}
}

// setStatusLocked updates cached status and broadcasts it to every registered
// node-facing StatusFunc. Caller must hold s.mu.
func (s *OpcuaServer) setStatusLocked(fill, text string) {
	if s.currentFill == fill && s.currentText == text {
		return
	}
	s.currentFill = fill
	s.currentText = text
	for _, fn := range s.statusFuncs {
		go fn(fill, text)
	}
}

// mapConnState turns a gopcua ConnState into a LOOPZE (fill, text) pair.
func mapConnState(state opcua.ConnState) (string, string) {
	switch state {
	case opcua.Connected:
		return "green", "connected"
	case opcua.Connecting:
		return "yellow", "connecting..."
	case opcua.Reconnecting:
		return "yellow", "reconnecting..."
	case opcua.Disconnected:
		return "red", "disconnected"
	case opcua.Closed:
		return "grey", "closed"
	default:
		return "grey", state.String()
	}
}

// securityPolicyURI maps short names ("None", "Basic256Sha256", …) to the
// fully-qualified OPC UA security policy URI expected by the server. The
// gopcua library accepts both forms in SecurityPolicy(), so this is a no-op
// for the short names — kept here as a single point of change in case the
// server-side spelling matters in the future.
func securityPolicyURI(name string) string {
	return name
}

// OpcuaServerInfo is the result of a successful test connection — used by
// the /api/v1/opcua/test-connection endpoint to give the editor immediate
// feedback about reachability and basic server identity without persisting
// the configuration to the workspace.
type OpcuaServerInfo struct {
	EndpointURL string `json:"endpointUrl"`
	ServerTime  string `json:"serverTime,omitempty"`
}

// OpcuaTestConnect opens a short-lived OPC UA session against the given
// ConfigNode definition and reads ServerStatus.CurrentTime as a connectivity
// probe. The session is closed immediately. Returns server info on success,
// a wrapped error on failure (network, auth, security policy mismatch).
//
// Reuses NewOpcuaServer's parsing logic so the test path can never drift
// from the deployed-runtime path. certs may be nil when the caller knows
// no cert ref is set on the config; otherwise the live cert store is
// required to resolve the reference.
func OpcuaTestConnect(ctx context.Context, cfg flow.ConfigNode, certs *credentials.CertStore) (*OpcuaServerInfo, error) {
	inst, err := NewOpcuaServer(cfg)
	if err != nil {
		return nil, err
	}
	srv := inst.(*OpcuaServer)
	srv.SetCertStore(certs)

	srv.mu.Lock()
	opts, err := srv.applyCertOptionsLocked(append([]opcua.Option{}, srv.clientOptions...))
	srv.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("cert options: %w", err)
	}
	defer func() {
		srv.mu.Lock()
		srv.removeTempCertFilesLocked()
		srv.mu.Unlock()
	}()

	client, err := opcua.NewClient(srv.endpointURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Close(closeCtx)
		cancel()
	}()

	info := &OpcuaServerInfo{EndpointURL: srv.endpointURL}

	currentTime, err := ParseOpcuaNodeID("i=2258")
	if err != nil {
		return info, nil
	}
	resp, err := client.Read(ctx, currentTimeReadRequest(currentTime))
	if err != nil || resp == nil || len(resp.Results) == 0 {
		return info, nil
	}
	if v := resp.Results[0].Value; v != nil {
		if t, ok := v.Value().(time.Time); ok {
			info.ServerTime = t.UTC().Format(time.RFC3339Nano)
		}
	}
	return info, nil
}

// currentTimeReadRequest builds the canonical "read CurrentTime" request used
// by the test-connection probe.
func currentTimeReadRequest(nodeID *ua.NodeID) *ua.ReadRequest {
	return &ua.ReadRequest{
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: nodeID, AttributeID: ua.AttributeIDValue},
		},
	}
}
