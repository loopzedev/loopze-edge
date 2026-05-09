// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// CertStorage is the storage facet that CertStore needs. It is a subset of
// storage.Storage, scoped to the credentials file. Both methods receive
// and return the encrypted bytes; CertStore owns the AES-256-GCM
// encryption via its CredentialManager.
type CertStorage interface {
	LoadCredentials() ([]byte, error)
	SaveCredentials(data []byte) error
}

// CertStore manages certificate entries that connection nodes (TCP, HTTP,
// MQTT, OPC UA) reference by ID. It persists entries to credentials.json
// alongside node credentials, sharing the v2 envelope and master key.
//
// The store is safe for concurrent use. All public methods take an
// internal mutex; mutating methods acquire the write lock for the full
// validate / parse / persist cycle so a Save failure can be rolled back
// without observers ever seeing the half-written state.
type CertStore struct {
	cm      *CredentialManager
	storage CertStorage

	mu    sync.RWMutex
	cache map[string]CertEntry
	creds map[string]json.RawMessage // pass-through half of the envelope
}

// Errors returned by CertStore mutators.
var (
	ErrCertExists       = errors.New("credentials: cert ID already exists")
	ErrCertNotFound     = errors.New("credentials: cert ID not found")
	ErrCertTypeMismatch = errors.New("credentials: cert ref points to an entry of the wrong type")
)

// NewCertStore returns a CertStore that uses cm for encrypt/decrypt and
// storage for the persisted bytes. Load must be called before any other
// method to populate the in-memory cache.
func NewCertStore(cm *CredentialManager, storage CertStorage) *CertStore {
	return &CertStore{
		cm:      cm,
		storage: storage,
		cache:   map[string]CertEntry{},
		creds:   map[string]json.RawMessage{},
	}
}

// Load reads the encrypted credentials file from storage, decrypts it,
// and populates the in-memory cache. Missing or empty files are treated
// as first-run state (empty cache, no error).
func (s *CertStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.storage.LoadCredentials()
	if err != nil {
		return fmt.Errorf("credentials: failed to load credentials file: %w", err)
	}

	var plain []byte
	if len(raw) > 0 {
		plain, err = s.cm.Decrypt(raw)
		if err != nil {
			return fmt.Errorf("credentials: failed to decrypt credentials file: %w", err)
		}
	}

	env, err := decodeEnvelope(plain)
	if err != nil {
		return err
	}

	s.cache = env.Certs
	s.creds = env.Credentials
	slog.Debug("cert store loaded", "certs", len(s.cache), "credentials", len(s.creds))
	return nil
}

// saveLocked encodes the envelope and writes it via storage. Caller must
// hold s.mu (write lock) so the encoded snapshot reflects a single
// consistent state of the cache.
func (s *CertStore) saveLocked() error {
	env := &envelope{
		Version:     envelopeVersion,
		Credentials: s.creds,
		Certs:       s.cache,
	}
	plain, err := encodeEnvelope(env)
	if err != nil {
		return err
	}
	enc, err := s.cm.Encrypt(plain)
	if err != nil {
		return fmt.Errorf("credentials: failed to encrypt credentials file: %w", err)
	}
	if err := s.storage.SaveCredentials(enc); err != nil {
		return fmt.Errorf("credentials: failed to save credentials file: %w", err)
	}
	return nil
}

// Store inserts a new cert entry. The ID must not already be in use; use
// Update to replace an existing entry. Validation and PEM parsing run
// before persistence; on persist failure the in-memory cache is rolled
// back so the store stays consistent.
func (s *CertStore) Store(e CertEntry) (CertEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := e.Validate(); err != nil {
		return CertEntry{}, err
	}
	if _, exists := s.cache[e.ID]; exists {
		return CertEntry{}, ErrCertExists
	}
	if err := e.parseAndPopulate(); err != nil {
		return CertEntry{}, err
	}

	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now

	s.cache[e.ID] = e
	if err := s.saveLocked(); err != nil {
		delete(s.cache, e.ID)
		return CertEntry{}, err
	}
	slog.Info("cert stored", "id", e.ID, "type", e.Type, "source", e.Source, "fingerprint", e.Fingerprint)
	return e, nil
}

// Update replaces the entry at id with the given values. CreatedAt is
// preserved from the existing entry; UpdatedAt is refreshed. The entry's
// ID field is forced to id so callers cannot accidentally rename via
// Update (use a Delete + Store pair if a rename is really needed).
func (s *CertStore) Update(id string, e CertEntry) (CertEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.cache[id]
	if !ok {
		return CertEntry{}, ErrCertNotFound
	}

	e.ID = id
	if err := e.Validate(); err != nil {
		return CertEntry{}, err
	}
	if err := e.parseAndPopulate(); err != nil {
		return CertEntry{}, err
	}

	e.CreatedAt = existing.CreatedAt
	e.UpdatedAt = time.Now().UTC()

	s.cache[id] = e
	if err := s.saveLocked(); err != nil {
		s.cache[id] = existing
		return CertEntry{}, err
	}
	slog.Info("cert updated", "id", id, "fingerprint", e.Fingerprint)
	return e, nil
}

// Get returns a copy of the entry. Callers cannot mutate cached state
// through the returned value.
func (s *CertStore) Get(id string) (CertEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.cache[id]
	return e, ok
}

// List returns all entries as wire-safe summaries, sorted by ID. The
// summary intentionally omits CertPEM / KeyPEM so HTTP responses cannot
// leak key material.
func (s *CertStore) List() []CertEntrySummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]CertEntrySummary, 0, len(s.cache))
	for _, e := range s.cache {
		out = append(out, e.summary())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LoadMaterial resolves a cert ID to its PEM bytes. For file sources the
// content is read fresh from disk on every call so external rotation
// takes effect on the next deploy without touching the store.
//
// expectedType, if non-empty, is checked against the entry's Type and
// produces ErrCertTypeMismatch on disagreement. Pass an empty string to
// skip the type check (e.g. for diagnostic readers).
//
// keyPEM is nil for ca-bundles and any other source that does not carry
// a private key.
func (s *CertStore) LoadMaterial(id, expectedType string) (certPEM, keyPEM []byte, err error) {
	entry, ok := s.Get(id)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %q", ErrCertNotFound, id)
	}
	if expectedType != "" && entry.Type != expectedType {
		return nil, nil, fmt.Errorf("%w: %q is type %q, want %q",
			ErrCertTypeMismatch, id, entry.Type, expectedType)
	}
	return entry.loadMaterial()
}

// TLSRefOptions selects the certificates and verification behaviour for a
// BuildTLSConfig call. Either or both refs may be empty; when neither is
// set the result is a server-roots-only config (with the standard system
// CA pool).
type TLSRefOptions struct {
	// CABundleRef is the cert ID of an entry whose Type is "ca-bundle".
	// It populates the RootCAs of the returned config.
	CABundleRef string

	// ClientPairRef is the cert ID of an entry whose Type is "client-pair".
	// It populates Certificates of the returned config (mTLS).
	ClientPairRef string

	// ServerName is forwarded to tls.Config.ServerName for SNI / hostname
	// verification.
	ServerName string

	// InsecureSkipVerify disables hostname verification. When set, a WARN
	// log line is emitted with the NodeID for traceability.
	InsecureSkipVerify bool

	// NodeID is included in error messages and log lines so a deploy
	// failure points at the responsible node.
	NodeID string
}

// BuildTLSConfig resolves the requested cert refs and assembles a
// *tls.Config. It is the single entry point used by every connection
// node (after Step 5's ParseTLSBlock refactor).
//
// Behaviour:
//   - File-source entries are re-read from disk on every call so external
//     rotation (cert-manager, Let's Encrypt, OPC UA file paths) takes
//     effect on the next deploy without touching the store.
//   - Type mismatches (e.g. a CABundleRef pointing at a client-pair) are
//     a hard error; the store refuses to silently coerce.
//   - A leaf certificate whose NotAfter is already in the past produces a
//     WARN log but is NOT rejected — operators sometimes legitimately
//     run expired test certs. Test setups would otherwise fail to deploy.
func (s *CertStore) BuildTLSConfig(opts TLSRefOptions) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.InsecureSkipVerify, // nosec G402 — gated by config
	}

	if opts.InsecureSkipVerify {
		slog.Warn("tls: certificate verification disabled",
			"node_id", opts.NodeID, "server_name", opts.ServerName)
	}

	if opts.CABundleRef != "" {
		certPEM, _, err := s.LoadMaterial(opts.CABundleRef, TypeCABundle)
		if err != nil {
			return nil, fmt.Errorf("%w (node %q)", err, opts.NodeID)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(certPEM) {
			return nil, fmt.Errorf("credentials: caBundleRef %q contained no valid PEM certificates (node %q)",
				opts.CABundleRef, opts.NodeID)
		}
		cfg.RootCAs = pool
		warnIfExpired(certPEM, opts.CABundleRef, opts.NodeID)
	}

	if opts.ClientPairRef != "" {
		certPEM, keyPEM, err := s.LoadMaterial(opts.ClientPairRef, TypeClientPair)
		if err != nil {
			return nil, fmt.Errorf("%w (node %q)", err, opts.NodeID)
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("credentials: clientPairRef %q: %w (node %q)", opts.ClientPairRef, err, opts.NodeID)
		}
		cfg.Certificates = []tls.Certificate{pair}
		warnIfExpired(certPEM, opts.ClientPairRef, opts.NodeID)
	}

	return cfg, nil
}

// warnIfExpired logs a WARN if the leaf certificate in pemData has already
// expired. Errors during parsing are silent here — BuildTLSConfig will
// surface them through the AppendCertsFromPEM / X509KeyPair paths.
func warnIfExpired(pemData []byte, certID, nodeID string) {
	leaf, err := parseLeafCertificate(pemData)
	if err != nil {
		return
	}
	if time.Now().After(leaf.NotAfter) {
		slog.Warn("tls: cert is past notAfter, deploy proceeding anyway",
			"cert_id", certID, "node_id", nodeID, "not_after", leaf.NotAfter)
	}
}

// Delete removes the entry at id. Callers (e.g. the HTTP layer) are
// responsible for checking workspace references before calling Delete;
// the store itself does not know about flows.
func (s *CertStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.cache[id]
	if !ok {
		return ErrCertNotFound
	}
	delete(s.cache, id)
	if err := s.saveLocked(); err != nil {
		s.cache[id] = existing
		return err
	}
	slog.Info("cert deleted", "id", id)
	return nil
}
