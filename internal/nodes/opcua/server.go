// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package opcua

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

	// Security + auth parameters captured at factory time. The actual
	// gopcua options are assembled in applyCertOptionsLocked at deploy
	// time so endpoint discovery can run first (required for any
	// non-None SecurityPolicy — gopcua needs the server's certificate
	// before it can send the asymmetric OpenSecureChannel request).
	securityPolicy string
	securityMode   string
	authMode       string
	username       string
	password       string

	// Certificate state. certRef references an entry in the central
	// cert store; the legacy fields are honoured for one release of
	// grace with a deprecation WARN. certRef and the legacy paths are
	// mutually exclusive.
	certRef              string
	legacyClientCertFile string
	legacyClientKeyFile  string
	certStore            *credentials.CertStore


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

	// Application-identity options live in clientOptions because they
	// are used at every connect (initial + reconnect). Security policy /
	// mode and auth tokens are NOT here — they are assembled per
	// connect in applyCertOptionsLocked so endpoint discovery can run
	// when needed.
	opts := []opcua.Option{
		opcua.ApplicationName(applicationName),
		opcua.ApplicationURI(applicationURI),
		opcua.SessionTimeout(sessionTimeout),
		opcua.AutoReconnect(true),
		opcua.ReconnectInterval(2 * time.Second),
	}

	username, _ := props["username"].(string)
	password, _ := props["password"].(string)

	certRef, _ := props["certRef"].(string)
	legacyClientCertFile, _ := props["clientCertFile"].(string)
	legacyClientKeyFile, _ := props["clientKeyFile"].(string)
	if certRef != "" && (legacyClientCertFile != "" || legacyClientKeyFile != "") {
		return nil, fmt.Errorf("opcua-server %s: certRef and clientCertFile/clientKeyFile are mutually exclusive", cfg.ID)
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
		securityPolicy:       securityPolicy,
		securityMode:         securityMode,
		authMode:             authMode,
		username:             username,
		password:             password,
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

// applyCertOptionsLocked assembles the security + auth + cert options
// gopcua needs for the requested SecurityPolicy / SecurityMode /
// authMode. For any non-None policy it performs an OPC UA endpoint
// discovery first (a plaintext call) to retrieve the server's
// certificate; without that, gopcua's asymmetric OpenSecureChannel
// fails with a confusing "x509 malformed format" error.
//
// PEM material (cert + key) is parsed in-process and handed to gopcua
// in already-parsed form (Certificate(der) / PrivateKey(*rsa.PrivateKey))
// so format quirks of gopcua's file-based loaders (PKCS#1 only) don't
// surface — both PKCS#1 and PKCS#8 keys work.
//
// Caller must hold s.mu.
func (s *OpcuaServer) applyCertOptionsLocked(ctx context.Context, base []opcua.Option) ([]opcua.Option, error) {
	certDER, rsaKey, err := s.loadCertMaterialLocked()
	if err != nil {
		return nil, err
	}

	wantSecurePolicy := s.securityPolicy != "" && s.securityPolicy != "None"
	wantAuthCert := s.authMode == "certificate"

	if wantAuthCert && (certDER == nil || rsaKey == nil) {
		return nil, fmt.Errorf("authMode=certificate requires a cert + key (set certRef or the legacy clientCertFile/clientKeyFile)")
	}

	// Endpoint discovery for non-None policies — the server's cert is
	// only available via GetEndpoints, and gopcua refuses to open the
	// secure channel without it.
	if wantSecurePolicy {
		discoveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		endpoints, err := opcua.GetEndpoints(discoveryCtx, s.endpointURL)
		if err != nil {
			return nil, fmt.Errorf("get endpoints: %w", err)
		}
		ep, err := opcua.SelectEndpoint(endpoints, securityPolicyURI(s.securityPolicy), securityModeFromString(s.securityMode))
		if err != nil || ep == nil {
			return nil, fmt.Errorf("no endpoint matches SecurityPolicy=%q SecurityMode=%q on %s: %w",
				s.securityPolicy, s.securityMode, s.endpointURL, err)
		}
		base = append(base, opcua.SecurityFromEndpoint(ep, userTokenTypeFor(s.authMode)))
	} else {
		base = append(base, opcua.SecurityPolicy("None"), opcua.SecurityModeString("None"))
	}

	// Channel-level cert (only useful for non-None policies, but
	// harmless otherwise — gopcua only consults it when the policy
	// requires asymmetric encryption).
	if certDER != nil {
		base = append(base, opcua.Certificate(certDER))
	}
	if rsaKey != nil {
		base = append(base, opcua.PrivateKey(rsaKey))
	}

	// User identity token. SecurityFromEndpoint sets the *type* of the
	// token (Anonymous / UserName / X509); the actual credentials are
	// supplied here.
	switch s.authMode {
	case "username":
		base = append(base, opcua.AuthUsername(s.username, s.password))
	case "certificate":
		base = append(base, opcua.AuthCertificate(certDER), opcua.AuthPrivateKey(rsaKey))
	default:
		base = append(base, opcua.AuthAnonymous())
	}

	return base, nil
}

// loadCertMaterialLocked resolves the configured cert reference (or the
// legacy file paths) and returns parsed cert DER + RSA key. Both
// returns are nil when no cert is configured (e.g. anonymous auth on
// SecurityPolicy=None).
//
// Caller must hold s.mu.
func (s *OpcuaServer) loadCertMaterialLocked() (certDER []byte, key *rsa.PrivateKey, err error) {
	var certPEM, keyPEM []byte

	switch {
	case s.certRef != "":
		if s.certStore == nil {
			return nil, nil, fmt.Errorf("certRef %q is set but no cert store is wired in", s.certRef)
		}
		certPEM, keyPEM, err = s.certStore.LoadMaterial(s.certRef, credentials.TypeClientPair)
		if err != nil {
			return nil, nil, err
		}
	case s.legacyClientCertFile != "" || s.legacyClientKeyFile != "":
		slog.Warn("opcua-server: clientCertFile/clientKeyFile are deprecated, migrate to certRef",
			"id", s.id, "name", s.name,
		)
		if s.legacyClientCertFile != "" {
			certPEM, err = os.ReadFile(s.legacyClientCertFile)
			if err != nil {
				return nil, nil, fmt.Errorf("read clientCertFile %q: %w", s.legacyClientCertFile, err)
			}
		}
		if s.legacyClientKeyFile != "" {
			keyPEM, err = os.ReadFile(s.legacyClientKeyFile)
			if err != nil {
				return nil, nil, fmt.Errorf("read clientKeyFile %q: %w", s.legacyClientKeyFile, err)
			}
		}
	default:
		return nil, nil, nil
	}

	if len(certPEM) > 0 {
		certDER, err = decodePEMCertificate(certPEM)
		if err != nil {
			return nil, nil, fmt.Errorf("opcua-server %s: %w", s.id, err)
		}
	}
	if len(keyPEM) > 0 {
		key, err = decodePEMRSAKey(keyPEM)
		if err != nil {
			return nil, nil, fmt.Errorf("opcua-server %s: %w", s.id, err)
		}
	}
	return certDER, key, nil
}

// securityModeFromString turns the LOOPZE config string ("None",
// "Sign", "SignAndEncrypt") into the gopcua enum used for endpoint
// matching.
func securityModeFromString(mode string) ua.MessageSecurityMode {
	switch mode {
	case "Sign":
		return ua.MessageSecurityModeSign
	case "SignAndEncrypt":
		return ua.MessageSecurityModeSignAndEncrypt
	default:
		return ua.MessageSecurityModeNone
	}
}

// userTokenTypeFor maps our authMode strings to the OPC UA enum used
// by SecurityFromEndpoint to find a matching identity policy.
func userTokenTypeFor(authMode string) ua.UserTokenType {
	switch authMode {
	case "username":
		return ua.UserTokenTypeUserName
	case "certificate":
		return ua.UserTokenTypeCertificate
	default:
		return ua.UserTokenTypeAnonymous
	}
}

// decodePEMCertificate finds the first CERTIFICATE block in pemData and
// returns its DER bytes. Additional blocks (intermediates) are ignored —
// OPC UA's secure-channel layer wants the leaf cert only.
func decodePEMCertificate(pemData []byte) ([]byte, error) {
	rest := pemData
	for {
		block, remainder := pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("decode certificate PEM: no CERTIFICATE block found")
		}
		if block.Type == "CERTIFICATE" {
			// Validate parseability so a malformed cert fails here with
			// a clear error rather than during the secure-channel open.
			if _, err := x509.ParseCertificate(block.Bytes); err != nil {
				return nil, fmt.Errorf("decode certificate PEM: %w", err)
			}
			return block.Bytes, nil
		}
		rest = remainder
	}
}

// decodePEMRSAKey parses an RSA private key from PEM, accepting both
// PKCS#1 (`-----BEGIN RSA PRIVATE KEY-----`, the format gopcua's own
// loader expects) and PKCS#8 (`-----BEGIN PRIVATE KEY-----`, openssl's
// default since OpenSSL 3). Anything else is rejected with a clear
// error.
func decodePEMRSAKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("decode private key PEM: no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("decode PKCS#8 private key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("decode private key PEM: OPC UA requires RSA, got %T", key)
		}
		return rsaKey, nil
	case "EC PRIVATE KEY":
		return nil, fmt.Errorf("decode private key PEM: OPC UA requires RSA, got EC key")
	default:
		return nil, fmt.Errorf("decode private key PEM: unsupported block type %q", block.Type)
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
	// Discovery (when SecurityPolicy != None) needs its own bounded
	// context — Start itself isn't ctx-driven, so we cap it locally.
	prepCtx, prepCancel := context.WithTimeout(context.Background(), 10*time.Second)
	opts, err := s.applyCertOptionsLocked(prepCtx, opts)
	prepCancel()
	if err != nil {
		s.setStatusLocked("red", err.Error())
		return fmt.Errorf("opcua-server %s: %w", s.id, err)
	}
	opts = append(opts, opcua.StateChangedCh(stateC))

	client, err := opcua.NewClient(s.endpointURL, opts...)
	if err != nil {
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
	opts, err := srv.applyCertOptionsLocked(ctx, append([]opcua.Option{}, srv.clientOptions...))
	srv.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("cert options: %w", err)
	}

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
