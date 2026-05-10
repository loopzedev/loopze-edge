// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package nodestest provides shared test fixtures for node subpackages
// (mqtt, opcua, network/http, network/tcp, ...). It must only be imported
// from _test.go files; production code should not depend on it.
package nodestest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
)

// GenerateTLSPair returns a self-signed ed25519 cert/key pair as PEM strings.
// Suitable for unit tests that need a complete chain without external CAs.
func GenerateTLSPair(t *testing.T, cn string, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
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

// NewCertStore returns a CertStore backed by an in-memory storage stub. The
// store is fully initialised (key material seeded) and ready for use in
// tests that exercise CertStore.Add / Get / Decrypt paths.
func NewCertStore(t *testing.T) *credentials.CertStore {
	t.Helper()
	dir := t.TempDir()
	cm := credentials.NewCredentialManager(filepath.Join(dir, "loopze.key"))
	if err := cm.EnsureKeyFile(); err != nil {
		t.Fatalf("EnsureKeyFile: %v", err)
	}
	store := credentials.NewCertStore(cm, &memStorage{})
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

type memStorage struct{ data []byte }

func (s *memStorage) LoadCredentials() ([]byte, error) { return s.data, nil }
func (s *memStorage) SaveCredentials(b []byte) error {
	s.data = make([]byte, len(b))
	copy(s.data, b)
	return nil
}
