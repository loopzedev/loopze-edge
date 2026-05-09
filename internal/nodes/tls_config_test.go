// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
)

// generateTestPair returns a self-signed ed25519 cert/key pair as PEM.
// Reproduced here (instead of importing the credentials test helper) so
// the nodes package has no test-only cross-package dependency.
func generateTestPair(t *testing.T, cn string, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		Issuer:                pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return string(certBytes), string(keyBytes)
}

// newTestCertStore returns a CertStore backed by an in-memory storage
// stub. Only the credentials.CertStorage methods are exercised here.
func newTestCertStore(t *testing.T) *credentials.CertStore {
	t.Helper()
	dir := t.TempDir()
	cm := credentials.NewCredentialManager(filepath.Join(dir, "loopze.key"))
	if err := cm.EnsureKeyFile(); err != nil {
		t.Fatalf("EnsureKeyFile: %v", err)
	}
	store := credentials.NewCertStore(cm, &tlsTestStorage{})
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

type tlsTestStorage struct{ data []byte }

func (s *tlsTestStorage) LoadCredentials() ([]byte, error) { return s.data, nil }
func (s *tlsTestStorage) SaveCredentials(b []byte) error {
	s.data = make([]byte, len(b))
	copy(s.data, b)
	return nil
}

// --- Inline-mode regression tests --------------------------------------

func TestParseTLSBlock_AbsentBlock(t *testing.T) {
	cfg, err := ParseTLSBlock(map[string]any{}, "n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when no tls block, got %+v", cfg)
	}
}

func TestParseTLSBlock_DisabledBlock(t *testing.T) {
	props := map[string]any{"tls": map[string]any{"enabled": false}}
	cfg, err := ParseTLSBlock(props, "n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when enabled=false, got %+v", cfg)
	}
}

func TestParseTLSBlock_InlineCABundleAndClientPair(t *testing.T) {
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	clientCert, clientKey := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))

	props := map[string]any{
		"tls": map[string]any{
			"enabled":    true,
			"serverName": "device.example.com",
			"caBundle":   caPEM,
			"clientCert": clientCert,
			"clientKey":  clientKey,
		},
	}
	cfg, err := ParseTLSBlock(props, "n", nil)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if cfg == nil {
		t.Fatal("config should not be nil")
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
	if cfg.ServerName != "device.example.com" {
		t.Errorf("ServerName = %q", cfg.ServerName)
	}
}

func TestParseTLSBlock_InlineHalfClientPair(t *testing.T) {
	now := time.Now()
	clientCert, _ := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))

	props := map[string]any{
		"tls": map[string]any{
			"enabled":    true,
			"clientCert": clientCert, // missing clientKey
		},
	}
	_, err := ParseTLSBlock(props, "n", nil)
	if err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Errorf("expected 'must be set together' error, got %v", err)
	}
}

func TestParseTLSBlock_InlineMalformedCABundle(t *testing.T) {
	props := map[string]any{
		"tls": map[string]any{
			"enabled":  true,
			"caBundle": "not-a-pem",
		},
	}
	if _, err := ParseTLSBlock(props, "n", nil); err == nil {
		t.Error("expected error on malformed CA bundle PEM")
	}
}

// --- Ref-mode tests -----------------------------------------------------

func TestParseTLSBlock_RefMode_CABundle(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "trusted-ca", Type: credentials.TypeCABundle, Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "trusted-ca",
		},
	}
	cfg, err := ParseTLSBlock(props, "n", store)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set from ref")
	}
}

func TestParseTLSBlock_RefMode_ClientPair(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	cert, key := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "device-cert", Type: credentials.TypeClientPair, Source: credentials.SourceInline,
		CertPEM: cert, KeyPEM: key,
	}); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":       true,
			"clientPairRef": "device-cert",
		},
	}
	cfg, err := ParseTLSBlock(props, "n", store)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

func TestParseTLSBlock_RefMode_FileSource(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca-file", now.Add(-time.Hour), now.Add(time.Hour))

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, []byte(caPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Store(credentials.CertEntry{
		ID: "ca-from-disk", Type: credentials.TypeCABundle, Source: credentials.SourceFile, CertPath: caPath,
	}); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "ca-from-disk",
		},
	}
	cfg, err := ParseTLSBlock(props, "n", store)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set from file-source ref")
	}
}

func TestParseTLSBlock_RefMode_TypeMismatch(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	cert, key := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "client-only", Type: credentials.TypeClientPair, Source: credentials.SourceInline,
		CertPEM: cert, KeyPEM: key,
	}); err != nil {
		t.Fatal(err)
	}

	// caBundleRef pointing at a client-pair entry should fail.
	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "client-only",
		},
	}
	_, err := ParseTLSBlock(props, "n", store)
	if !errors.Is(err, credentials.ErrCertTypeMismatch) {
		t.Errorf("got %v, want ErrCertTypeMismatch", err)
	}
}

func TestParseTLSBlock_RefMode_UnknownID(t *testing.T) {
	store := newTestCertStore(t)
	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "ghost",
		},
	}
	_, err := ParseTLSBlock(props, "n", store)
	if !errors.Is(err, credentials.ErrCertNotFound) {
		t.Errorf("got %v, want ErrCertNotFound", err)
	}
}

func TestParseTLSBlock_ConflictInlineAndRef_CA(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))

	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundle":    caPEM,
			"caBundleRef": "anything",
		},
	}
	_, err := ParseTLSBlock(props, "n", store)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got %v", err)
	}
}

func TestParseTLSBlock_ConflictInlineAndRef_Client(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	cert, key := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))

	props := map[string]any{
		"tls": map[string]any{
			"enabled":       true,
			"clientCert":    cert,
			"clientKey":     key,
			"clientPairRef": "anything",
		},
	}
	_, err := ParseTLSBlock(props, "n", store)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got %v", err)
	}
}

func TestParseTLSBlock_RefSetButNoStore(t *testing.T) {
	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "anything",
		},
	}
	_, err := ParseTLSBlock(props, "n", nil)
	if err == nil || !strings.Contains(err.Error(), "no cert store") {
		t.Errorf("expected 'no cert store' error, got %v", err)
	}
}

func TestParseTLSBlock_FileMode_CABundle(t *testing.T) {
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca-from-file", now.Add(-time.Hour), now.Add(time.Hour))
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, []byte(caPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":      true,
			"caBundleFile": caPath,
		},
	}
	cfg, err := ParseTLSBlock(props, "n", nil)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be populated from file")
	}
}

func TestParseTLSBlock_FileMode_ClientPair(t *testing.T) {
	now := time.Now()
	cert, key := generateTestPair(t, "client-from-file", now.Add(-time.Hour), now.Add(time.Hour))
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, []byte(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":        true,
			"clientCertFile": certPath,
			"clientKeyFile":  keyPath,
		},
	}
	cfg, err := ParseTLSBlock(props, "n", nil)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

func TestParseTLSBlock_FileMode_HalfClientPair(t *testing.T) {
	props := map[string]any{
		"tls": map[string]any{
			"enabled":        true,
			"clientCertFile": "/tmp/cert.pem", // missing keyFile
		},
	}
	_, err := ParseTLSBlock(props, "n", nil)
	if err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Errorf("expected 'must be set together' error, got %v", err)
	}
}

func TestParseTLSBlock_FileMode_MissingFile(t *testing.T) {
	props := map[string]any{
		"tls": map[string]any{
			"enabled":      true,
			"caBundleFile": "/tmp/does-not-exist.pem",
		},
	}
	if _, err := ParseTLSBlock(props, "n", nil); err == nil {
		t.Error("expected error on missing CA file")
	}
}

func TestParseTLSBlock_ConflictAcrossThreeSources(t *testing.T) {
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))

	t.Run("inline + file", func(t *testing.T) {
		props := map[string]any{
			"tls": map[string]any{
				"enabled":      true,
				"caBundle":     caPEM,
				"caBundleFile": "/tmp/ca.pem",
			},
		}
		_, err := ParseTLSBlock(props, "n", nil)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected mutex error, got %v", err)
		}
	})

	t.Run("ref + file", func(t *testing.T) {
		store := newTestCertStore(t)
		props := map[string]any{
			"tls": map[string]any{
				"enabled":      true,
				"caBundleRef":  "anything",
				"caBundleFile": "/tmp/ca.pem",
			},
		}
		_, err := ParseTLSBlock(props, "n", store)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected mutex error, got %v", err)
		}
	})
}

func TestParseTLSBlock_MixedRefAndInline(t *testing.T) {
	// CA via ref, client cert/key inline. Both should compose correctly.
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	clientCert, clientKey := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))

	if _, err := store.Store(credentials.CertEntry{
		ID: "trusted-ca", Type: credentials.TypeCABundle, Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	props := map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "trusted-ca",
			"clientCert":  clientCert,
			"clientKey":   clientKey,
		},
	}
	cfg, err := ParseTLSBlock(props, "n", store)
	if err != nil {
		t.Fatalf("ParseTLSBlock: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set (from ref)")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1 (inline)", len(cfg.Certificates))
	}
}
