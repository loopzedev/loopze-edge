// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// memCertStorage backs the migration tests with the same shape that
// CertStore expects from production storage but without touching disk.
type memCertStorage struct {
	mu   sync.Mutex
	data []byte
}

func (m *memCertStorage) LoadCredentials() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return nil, nil
	}
	out := make([]byte, len(m.data))
	copy(out, m.data)
	return out, nil
}

func (m *memCertStorage) SaveCredentials(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make([]byte, len(b))
	copy(m.data, b)
	return nil
}

func newMigrationStore(t *testing.T) *credentials.CertStore {
	t.Helper()
	dir := t.TempDir()
	cm := credentials.NewCredentialManager(filepath.Join(dir, "loopze.key"))
	if err := cm.EnsureKeyFile(); err != nil {
		t.Fatal(err)
	}
	store := credentials.NewCertStore(cm, &memCertStorage{})
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	return store
}

// writeMigrationCertPair drops a fresh self-signed ed25519 client-pair
// onto disk in t.TempDir() and returns the two paths.
func writeMigrationCertPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "migration-client"},
		Issuer:                pkix.Name{CommonName: "migration-client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "client.crt")
	keyPath = filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestMigrateOpcuaCertConfigs_HappyPath(t *testing.T) {
	store := newMigrationStore(t)
	certPath, keyPath := writeMigrationCertPair(t)

	ws := &flow.Workspace{
		Configs: []flow.ConfigNode{{
			ID: "opcua-prod", Type: "opcua-server", Name: "Production",
			Config: map[string]any{
				"endpointUrl":    "opc.tcp://prod:4840",
				"clientCertFile": certPath,
				"clientKeyFile":  keyPath,
			},
		}},
	}

	n, err := migrateOpcuaCertConfigs(ws, store)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 1 {
		t.Errorf("migrated = %d, want 1", n)
	}
	if got := ws.Configs[0].Config["certRef"]; got != "opcua-opcua-prod" {
		t.Errorf("certRef = %v, want opcua-opcua-prod", got)
	}
	if _, present := ws.Configs[0].Config["clientCertFile"]; present {
		t.Error("clientCertFile should be removed after migration")
	}
	if _, present := ws.Configs[0].Config["clientKeyFile"]; present {
		t.Error("clientKeyFile should be removed after migration")
	}
	entry, ok := store.Get("opcua-opcua-prod")
	if !ok {
		t.Fatal("expected cert entry to be created in store")
	}
	if entry.Source != credentials.SourceFile {
		t.Errorf("Source = %q, want file", entry.Source)
	}
	if entry.CertPath != certPath || entry.KeyPath != keyPath {
		t.Errorf("paths not preserved: cert=%q key=%q", entry.CertPath, entry.KeyPath)
	}
}

func TestMigrateOpcuaCertConfigs_Idempotent(t *testing.T) {
	store := newMigrationStore(t)
	certPath, keyPath := writeMigrationCertPair(t)
	ws := &flow.Workspace{
		Configs: []flow.ConfigNode{{
			ID: "node-x", Type: "opcua-server",
			Config: map[string]any{"clientCertFile": certPath, "clientKeyFile": keyPath},
		}},
	}

	if _, err := migrateOpcuaCertConfigs(ws, store); err != nil {
		t.Fatal(err)
	}

	// Second invocation must be a no-op: certRef is already set so the
	// loop skips the config entirely.
	n, err := migrateOpcuaCertConfigs(ws, store)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second pass migrated = %d, want 0", n)
	}
}

func TestMigrateOpcuaCertConfigs_SkipsConfigsWithoutFiles(t *testing.T) {
	store := newMigrationStore(t)
	ws := &flow.Workspace{
		Configs: []flow.ConfigNode{
			{
				ID: "anonymous", Type: "opcua-server",
				Config: map[string]any{"endpointUrl": "opc.tcp://x:4840"},
			},
			{
				ID: "username-auth", Type: "opcua-server",
				Config: map[string]any{
					"endpointUrl": "opc.tcp://x:4840",
					"username":    "u", "password": "p",
				},
			},
		},
	}

	n, err := migrateOpcuaCertConfigs(ws, store)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("nothing should migrate, got %d", n)
	}
}

func TestMigrateOpcuaCertConfigs_PreservesExistingCertRef(t *testing.T) {
	store := newMigrationStore(t)
	certPath, keyPath := writeMigrationCertPair(t)
	ws := &flow.Workspace{
		Configs: []flow.ConfigNode{{
			ID: "preset", Type: "opcua-server",
			Config: map[string]any{
				"certRef":        "operator-managed",
				"clientCertFile": certPath, // ignored — not migrated when certRef is present
				"clientKeyFile":  keyPath,
			},
		}},
	}

	n, err := migrateOpcuaCertConfigs(ws, store)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("nothing should migrate when certRef is preset, got %d", n)
	}
	if got := ws.Configs[0].Config["certRef"]; got != "operator-managed" {
		t.Errorf("certRef changed: got %v", got)
	}
}

func TestMigrateOpcuaCertConfigs_NilInputsAreNoop(t *testing.T) {
	if n, err := migrateOpcuaCertConfigs(nil, nil); n != 0 || err != nil {
		t.Errorf("nil inputs: got n=%d err=%v", n, err)
	}
}
