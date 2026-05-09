// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

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
)

// generateTestPair returns a freshly generated self-signed certificate
// (ed25519, valid for the given window) and its private key, both PEM
// encoded. ed25519 is used in preference to RSA so the test suite stays
// fast even when many certs are created.
func generateTestPair(t *testing.T, cn string, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		Issuer:       pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:         true,
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

func writeTempPEM(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "material.pem")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp PEM: %v", err)
	}
	return path
}

func TestCertEntryValidate_SlugRules(t *testing.T) {
	now := time.Now()
	certPEM, _ := generateTestPair(t, "test-ca", now.Add(-time.Hour), now.Add(time.Hour))

	good := []string{"a", "ca", "ca-internal-root", "ca_2026", "0", "abc123"}
	for _, id := range good {
		e := CertEntry{ID: id, Name: "n", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM}
		if err := e.Validate(); err != nil {
			t.Errorf("expected slug %q to be valid, got %v", id, err)
		}
	}

	bad := []string{
		"",                      // empty
		"-leading-dash",         // leading dash
		"_leading-underscore",   // leading underscore
		"Foo",                   // uppercase
		"with space",            // whitespace
		"with.dot",              // dot
		strings.Repeat("x", 65), // too long
	}
	for _, id := range bad {
		e := CertEntry{ID: id, Name: "n", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertIDInvalid) {
			t.Errorf("expected slug %q to be rejected with ErrCertIDInvalid, got %v", id, err)
		}
	}
}

func TestCertEntryValidate_EnumMembership(t *testing.T) {
	now := time.Now()
	certPEM, keyPEM := generateTestPair(t, "test", now.Add(-time.Hour), now.Add(time.Hour))

	t.Run("invalid type", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: "bogus", Source: SourceInline, CertPEM: certPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertTypeInvalid) {
			t.Errorf("got %v, want ErrCertTypeInvalid", err)
		}
	})

	t.Run("invalid source", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: "weird", CertPEM: certPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertSourceInvalid) {
			t.Errorf("got %v, want ErrCertSourceInvalid", err)
		}
	})

	t.Run("client-pair without key", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeClientPair, Source: SourceInline, CertPEM: certPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertKeyMissing) {
			t.Errorf("got %v, want ErrCertKeyMissing", err)
		}
	})

	t.Run("ca-bundle with key forbidden", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM, KeyPEM: keyPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertKeyForbidden) {
			t.Errorf("got %v, want ErrCertKeyForbidden", err)
		}
	})
}

func TestCertEntryValidate_MutualExclusion(t *testing.T) {
	now := time.Now()
	certPEM, _ := generateTestPair(t, "test", now.Add(-time.Hour), now.Add(time.Hour))
	path := writeTempPEM(t, certPEM)

	t.Run("inline with certPath set", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM, CertPath: path}
		if err := e.Validate(); !errors.Is(err, ErrCertMixedSource) {
			t.Errorf("got %v, want ErrCertMixedSource", err)
		}
	})

	t.Run("file with certPem set", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile, CertPath: path, CertPEM: certPEM}
		if err := e.Validate(); !errors.Is(err, ErrCertMixedSource) {
			t.Errorf("got %v, want ErrCertMixedSource", err)
		}
	})

	t.Run("file with keyPem set on ca-bundle", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile, CertPath: path, KeyPEM: "x"}
		if err := e.Validate(); !errors.Is(err, ErrCertMixedSource) {
			t.Errorf("got %v, want ErrCertMixedSource", err)
		}
	})
}

func TestCertEntryValidate_FilePathRules(t *testing.T) {
	now := time.Now()
	certPEM, keyPEM := generateTestPair(t, "test", now.Add(-time.Hour), now.Add(time.Hour))
	certPath := writeTempPEM(t, certPEM)
	keyPath := writeTempPEM(t, keyPEM)

	t.Run("relative certPath rejected", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile, CertPath: "relative.pem"}
		if err := e.Validate(); !errors.Is(err, ErrCertPathRelative) {
			t.Errorf("got %v, want ErrCertPathRelative", err)
		}
	})

	t.Run("relative keyPath rejected", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeClientPair, Source: SourceFile, CertPath: certPath, KeyPath: "relative.key"}
		if err := e.Validate(); !errors.Is(err, ErrCertPathRelative) {
			t.Errorf("got %v, want ErrCertPathRelative", err)
		}
	})

	t.Run("file source missing certPath", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile}
		if err := e.Validate(); !errors.Is(err, ErrCertCertMissing) {
			t.Errorf("got %v, want ErrCertCertMissing", err)
		}
	})

	t.Run("file source client-pair missing keyPath", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeClientPair, Source: SourceFile, CertPath: certPath}
		if err := e.Validate(); !errors.Is(err, ErrCertKeyMissing) {
			t.Errorf("got %v, want ErrCertKeyMissing", err)
		}
	})

	t.Run("file source ca-bundle with keyPath forbidden", func(t *testing.T) {
		e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile, CertPath: certPath, KeyPath: keyPath}
		if err := e.Validate(); !errors.Is(err, ErrCertKeyForbidden) {
			t.Errorf("got %v, want ErrCertKeyForbidden", err)
		}
	})
}

func TestCertEntry_ParseAndPopulate_InlineCABundle(t *testing.T) {
	notBefore := time.Now().Add(-time.Hour).Truncate(time.Second)
	notAfter := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	certPEM, _ := generateTestPair(t, "test-ca", notBefore, notAfter)

	e := CertEntry{ID: "ca", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := e.parseAndPopulate(); err != nil {
		t.Fatalf("parseAndPopulate: %v", err)
	}

	if e.Fingerprint == "" || len(e.Fingerprint) != 64 {
		t.Errorf("Fingerprint should be 64 hex chars, got %q", e.Fingerprint)
	}
	if !strings.Contains(e.Subject, "test-ca") {
		t.Errorf("Subject = %q, expected to contain test-ca", e.Subject)
	}
	if !e.NotBefore.Equal(notBefore) {
		t.Errorf("NotBefore = %v, want %v", e.NotBefore, notBefore)
	}
	if !e.NotAfter.Equal(notAfter) {
		t.Errorf("NotAfter = %v, want %v", e.NotAfter, notAfter)
	}
}

func TestCertEntry_ParseAndPopulate_InlineClientPair(t *testing.T) {
	now := time.Now()
	certPEM, keyPEM := generateTestPair(t, "client", now.Add(-time.Hour), now.Add(time.Hour))

	e := CertEntry{
		ID: "c", Type: TypeClientPair, Source: SourceInline,
		CertPEM: certPEM, KeyPEM: keyPEM,
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := e.parseAndPopulate(); err != nil {
		t.Fatalf("parseAndPopulate: %v", err)
	}
	if e.Fingerprint == "" {
		t.Error("Fingerprint should be populated")
	}
}

func TestCertEntry_ParseAndPopulate_FileSourceRoundtrip(t *testing.T) {
	now := time.Now()
	certPEM, keyPEM := generateTestPair(t, "device", now.Add(-time.Hour), now.Add(time.Hour))
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, []byte(certPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	e := CertEntry{ID: "c", Type: TypeClientPair, Source: SourceFile, CertPath: certPath, KeyPath: keyPath}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := e.parseAndPopulate(); err != nil {
		t.Fatalf("parseAndPopulate: %v", err)
	}
	if e.Fingerprint == "" {
		t.Error("Fingerprint should be populated for file source")
	}
	if e.CertPEM != "" || e.KeyPEM != "" {
		t.Errorf("file-source entry must not retain inline PEM, got cert=%d bytes key=%d bytes", len(e.CertPEM), len(e.KeyPEM))
	}
}

func TestCertEntry_ParseAndPopulate_FingerprintIsStable(t *testing.T) {
	now := time.Now()
	certPEM, _ := generateTestPair(t, "stable", now.Add(-time.Hour), now.Add(time.Hour))

	a := CertEntry{ID: "a", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM}
	b := CertEntry{ID: "b", Type: TypeCABundle, Source: SourceInline, CertPEM: certPEM}
	if err := a.parseAndPopulate(); err != nil {
		t.Fatal(err)
	}
	if err := b.parseAndPopulate(); err != nil {
		t.Fatal(err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Errorf("same PEM produced different fingerprints: %q vs %q", a.Fingerprint, b.Fingerprint)
	}
}

func TestCertEntry_ParseAndPopulate_RejectsMalformedPEM(t *testing.T) {
	e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceInline, CertPEM: "not a pem"}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := e.parseAndPopulate(); !errors.Is(err, ErrCertPEMInvalid) {
		t.Errorf("got %v, want ErrCertPEMInvalid", err)
	}
}

func TestCertEntry_ParseAndPopulate_RejectsMismatchedKeyPair(t *testing.T) {
	now := time.Now()
	certA, _ := generateTestPair(t, "a", now.Add(-time.Hour), now.Add(time.Hour))
	_, keyB := generateTestPair(t, "b", now.Add(-time.Hour), now.Add(time.Hour))

	e := CertEntry{ID: "x", Type: TypeClientPair, Source: SourceInline, CertPEM: certA, KeyPEM: keyB}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := e.parseAndPopulate(); !errors.Is(err, ErrCertKeyMismatch) {
		t.Errorf("got %v, want ErrCertKeyMismatch", err)
	}
}

func TestCertEntry_ParseAndPopulate_FileMissingFails(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.pem")

	e := CertEntry{ID: "x", Type: TypeCABundle, Source: SourceFile, CertPath: missing}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	err := e.parseAndPopulate()
	if err == nil {
		t.Fatal("expected error reading missing file, got nil")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error should mention path %q, got %v", missing, err)
	}
}
