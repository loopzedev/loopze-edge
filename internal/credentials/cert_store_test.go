// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memStorage is an in-memory CertStorage used by the test suite. It mirrors
// the contract of *storage.FileStorage without touching disk so tests stay
// fast and isolated.
type memStorage struct {
	mu       sync.Mutex
	data     []byte
	saveFail atomic.Bool
}

func (m *memStorage) LoadCredentials() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return nil, nil
	}
	out := make([]byte, len(m.data))
	copy(out, m.data)
	return out, nil
}

func (m *memStorage) SaveCredentials(b []byte) error {
	if m.saveFail.Load() {
		return errors.New("forced save failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make([]byte, len(b))
	copy(m.data, b)
	return nil
}

func newTestStore(t *testing.T) (*CertStore, *memStorage) {
	t.Helper()
	dir := t.TempDir()
	cm := NewCredentialManager(filepath.Join(dir, "loopze.key"))
	if err := cm.EnsureKeyFile(); err != nil {
		t.Fatalf("EnsureKeyFile: %v", err)
	}
	storage := &memStorage{}
	store := NewCertStore(cm, storage)
	if err := store.Load(); err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	return store, storage
}

func sampleInlineCABundle(t *testing.T, id string) CertEntry {
	t.Helper()
	now := time.Now()
	certPEM, _ := generateTestPair(t, "test-"+id, now.Add(-time.Hour), now.Add(time.Hour))
	return CertEntry{
		ID:      id,
		Name:    id,
		Type:    TypeCABundle,
		Source:  SourceInline,
		CertPEM: certPEM,
	}
}

func sampleInlineClientPair(t *testing.T, id string) CertEntry {
	t.Helper()
	now := time.Now()
	certPEM, keyPEM := generateTestPair(t, "client-"+id, now.Add(-time.Hour), now.Add(time.Hour))
	return CertEntry{
		ID:      id,
		Name:    id,
		Type:    TypeClientPair,
		Source:  SourceInline,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
}

func TestCertStore_StoreAndGetRoundtrip(t *testing.T) {
	store, _ := newTestStore(t)

	in := sampleInlineCABundle(t, "ca-root")
	stored, err := store.Store(in)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if stored.Fingerprint == "" {
		t.Error("Store should populate Fingerprint")
	}
	if stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Error("Store should set timestamps")
	}

	got, ok := store.Get("ca-root")
	if !ok {
		t.Fatal("Get returned not found")
	}
	if got.Fingerprint != stored.Fingerprint {
		t.Errorf("Fingerprint mismatch: %q vs %q", got.Fingerprint, stored.Fingerprint)
	}
	if got.CertPEM != in.CertPEM {
		t.Error("CertPEM should be preserved for inline source")
	}
}

func TestCertStore_StoreRejectsDuplicateID(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}
	_, err := store.Store(sampleInlineCABundle(t, "ca"))
	if !errors.Is(err, ErrCertExists) {
		t.Errorf("got %v, want ErrCertExists", err)
	}
}

func TestCertStore_StoreReturnsValidationError(t *testing.T) {
	store, _ := newTestStore(t)
	bad := CertEntry{ID: "Bad ID", Type: TypeCABundle, Source: SourceInline, CertPEM: "x"}
	_, err := store.Store(bad)
	if !errors.Is(err, ErrCertIDInvalid) {
		t.Errorf("got %v, want ErrCertIDInvalid", err)
	}
}

func TestCertStore_UpdatePreservesCreatedAt(t *testing.T) {
	store, _ := newTestStore(t)
	first, err := store.Store(sampleInlineCABundle(t, "ca"))
	if err != nil {
		t.Fatal(err)
	}

	// Sleep so UpdatedAt is reliably distinguishable from CreatedAt.
	time.Sleep(2 * time.Millisecond)

	replacement := sampleInlineCABundle(t, "ca")
	replacement.Name = "updated name"
	updated, err := store.Update("ca", replacement)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed: %v vs %v", updated.CreatedAt, first.CreatedAt)
	}
	if !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance: %v vs %v", updated.UpdatedAt, first.UpdatedAt)
	}
	if updated.Name != "updated name" {
		t.Errorf("Name = %q, want %q", updated.Name, "updated name")
	}
}

func TestCertStore_UpdateInheritsInlinePEMWhenOmitted(t *testing.T) {
	store, _ := newTestStore(t)
	original, err := store.Store(sampleInlineClientPair(t, "client"))
	if err != nil {
		t.Fatal(err)
	}

	// Caller submits a name change but no PEM material — the existing
	// CertPEM / KeyPEM must be inherited so the entry stays valid.
	updated, err := store.Update("client", CertEntry{
		Name:   "Renamed",
		Type:   TypeClientPair,
		Source: SourceInline,
	})
	if err != nil {
		t.Fatalf("Update without PEM: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", updated.Name)
	}
	if updated.CertPEM != original.CertPEM {
		t.Error("CertPEM should be inherited from existing entry")
	}
	if updated.KeyPEM != original.KeyPEM {
		t.Error("KeyPEM should be inherited from existing entry")
	}
	if updated.Fingerprint != original.Fingerprint {
		t.Errorf("Fingerprint changed despite same material: %q vs %q",
			updated.Fingerprint, original.Fingerprint)
	}
}

func TestCertStore_UpdateInheritsFilePathsWhenOmitted(t *testing.T) {
	store, _ := newTestStore(t)
	now := time.Now()
	cert, key := generateTestPair(t, "device", now.Add(-time.Hour), now.Add(time.Hour))
	dir := t.TempDir()
	certPath := filepath.Join(dir, "device.crt")
	keyPath := filepath.Join(dir, "device.key")
	if err := os.WriteFile(certPath, []byte(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Store(CertEntry{
		ID: "device", Type: TypeClientPair, Source: SourceFile,
		CertPath: certPath, KeyPath: keyPath,
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update("device", CertEntry{
		Name:   "Renamed device",
		Type:   TypeClientPair,
		Source: SourceFile,
	})
	if err != nil {
		t.Fatalf("Update without paths: %v", err)
	}
	if updated.CertPath != certPath {
		t.Errorf("CertPath should be inherited, got %q", updated.CertPath)
	}
	if updated.KeyPath != keyPath {
		t.Errorf("KeyPath should be inherited, got %q", updated.KeyPath)
	}
}

func TestCertStore_UpdateAcrossSourcesRequiresFreshMaterial(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}

	// Switching from inline to file with no path is rejected — there's
	// nothing to inherit across the source change.
	_, err := store.Update("ca", CertEntry{
		Type:   TypeCABundle,
		Source: SourceFile,
	})
	if err == nil {
		t.Fatal("expected error when switching source without supplying material")
	}
}

func TestCertStore_UpdateForbidsRename(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}
	replacement := sampleInlineCABundle(t, "different")
	updated, err := store.Update("ca", replacement)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != "ca" {
		t.Errorf("Update should not rename: ID = %q", updated.ID)
	}
}

func TestCertStore_UpdateMissing(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.Update("nope", sampleInlineCABundle(t, "nope"))
	if !errors.Is(err, ErrCertNotFound) {
		t.Errorf("got %v, want ErrCertNotFound", err)
	}
}

func TestCertStore_DeleteMissing(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.Delete("nope"); !errors.Is(err, ErrCertNotFound) {
		t.Errorf("got %v, want ErrCertNotFound", err)
	}
}

func TestCertStore_Delete(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("ca"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.Get("ca"); ok {
		t.Error("Delete did not remove the entry")
	}
}

func TestCertStore_ListExcludesPEMAndIsSorted(t *testing.T) {
	store, _ := newTestStore(t)
	for _, id := range []string{"zeta", "alpha", "mike"} {
		entry := sampleInlineClientPair(t, id)
		if _, err := store.Store(entry); err != nil {
			t.Fatal(err)
		}
	}
	list := store.List()
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	if list[0].ID != "alpha" || list[1].ID != "mike" || list[2].ID != "zeta" {
		t.Errorf("not sorted: %q %q %q", list[0].ID, list[1].ID, list[2].ID)
	}
	// Summary is a typed struct without CertPEM / KeyPEM fields, so we
	// instead encode it to JSON and assert those keys are absent.
	data, err := json.Marshal(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); containsAny(got, []string{`"certPem"`, `"keyPem"`}) {
		t.Errorf("summary JSON leaks PEM material: %s", got)
	}
}

func TestCertStore_PersistenceRoundtrip(t *testing.T) {
	store, storage := newTestStore(t)

	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Store(sampleInlineClientPair(t, "client")); err != nil {
		t.Fatal(err)
	}

	// Build a fresh store backed by the same storage + key. After Load
	// the in-memory cache must match the previous store's contents.
	dir := t.TempDir()
	_ = dir // silence unused if linter complains; key file lives elsewhere already
	cm := store.cm
	store2 := NewCertStore(cm, storage)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := store2.List(); len(got) != 2 {
		t.Fatalf("after Load len = %d, want 2", len(got))
	}
	if e, ok := store2.Get("client"); !ok || e.KeyPEM == "" {
		t.Error("inline client-pair lost KeyPEM through persistence")
	}
}

func TestCertStore_StoreRollbackOnSaveFailure(t *testing.T) {
	store, storage := newTestStore(t)
	storage.saveFail.Store(true)

	_, err := store.Store(sampleInlineCABundle(t, "ca"))
	if err == nil {
		t.Fatal("expected Store to fail when SaveCredentials errors")
	}
	if _, ok := store.Get("ca"); ok {
		t.Error("cache should be rolled back on save failure")
	}
}

func TestCertStore_PreservesCredentialsHalfThroughRoundtrip(t *testing.T) {
	store, storage := newTestStore(t)

	// Pre-populate the credentials half via a manual envelope write so we
	// can confirm CertStore preserves it on subsequent saves.
	env := newEnvelope()
	env.Credentials["node-xyz"] = json.RawMessage(`{"password":"hunter2"}`)
	plain, err := encodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := store.cm.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SaveCredentials(enc); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}

	// Add a cert. The credentials half must survive the save.
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}

	// Reload from storage and verify both halves are intact.
	store2 := NewCertStore(store.cm, storage)
	if err := store2.Load(); err != nil {
		t.Fatal(err)
	}
	if got := string(store2.creds["node-xyz"]); got != `{"password":"hunter2"}` {
		t.Errorf("credentials half lost: %q", got)
	}
	if _, ok := store2.Get("ca"); !ok {
		t.Error("cert half lost")
	}
}

func TestCertStore_ConcurrentSafety(t *testing.T) {
	store, _ := newTestStore(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, err := store.Store(sampleInlineCABundle(t, id)); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.List()
			_, _ = store.Get("a")
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids := []string{"a", "b", "c", "d"}
			id := ids[i%len(ids)]
			replacement := sampleInlineCABundle(t, id)
			_, _ = store.Update(id, replacement)
		}(i)
	}
	wg.Wait()
}

func TestCertStore_BuildTLSConfig_InlineCABundle(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineCABundle(t, "ca")); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.BuildTLSConfig(TLSRefOptions{
		CABundleRef: "ca",
		ServerName:  "example.com",
		NodeID:      "tcp-out-1",
	})
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set when CABundleRef is provided")
	}
	if cfg.ServerName != "example.com" {
		t.Errorf("ServerName = %q, want example.com", cfg.ServerName)
	}
	if cfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %#x, want TLS 1.2", cfg.MinVersion)
	}
}

func TestCertStore_BuildTLSConfig_InlineClientPair(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineClientPair(t, "client")); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.BuildTLSConfig(TLSRefOptions{ClientPairRef: "client", NodeID: "n"})
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

func TestCertStore_BuildTLSConfig_FileCABundleRotation(t *testing.T) {
	store, _ := newTestStore(t)

	dir := t.TempDir()
	certPath := filepath.Join(dir, "ca.pem")

	now := time.Now()
	pemA, _ := generateTestPair(t, "rotation-a", now.Add(-time.Hour), now.Add(time.Hour))
	if err := os.WriteFile(certPath, []byte(pemA), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Store(CertEntry{
		ID: "ca-file", Name: "rotating", Type: TypeCABundle, Source: SourceFile, CertPath: certPath,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "ca-file", NodeID: "n"}); err != nil {
		t.Fatalf("first BuildTLSConfig: %v", err)
	}

	// Delete the file and confirm the next call fails — proves content is
	// re-read on every call, not cached from the initial Store.
	if err := os.Remove(certPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "ca-file", NodeID: "n"}); err == nil {
		t.Fatal("expected error after deleting cert file, got nil — file contents were cached")
	}

	// Restore with a fresh cert; build must succeed again (no stale cache).
	pemB, _ := generateTestPair(t, "rotation-b", now.Add(-time.Hour), now.Add(time.Hour))
	if err := os.WriteFile(certPath, []byte(pemB), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "ca-file", NodeID: "n"}); err != nil {
		t.Fatalf("BuildTLSConfig after rotation: %v", err)
	}
}

func TestCertStore_BuildTLSConfig_TypeMismatch(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Store(sampleInlineClientPair(t, "client")); err != nil {
		t.Fatal(err)
	}

	_, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "client", NodeID: "n"})
	if !errors.Is(err, ErrCertTypeMismatch) {
		t.Errorf("got %v, want ErrCertTypeMismatch", err)
	}
}

func TestCertStore_BuildTLSConfig_UnknownRef(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "ghost", NodeID: "n"})
	if !errors.Is(err, ErrCertNotFound) {
		t.Errorf("got %v, want ErrCertNotFound", err)
	}
}

func TestCertStore_BuildTLSConfig_NoRefsUsesSystemRoots(t *testing.T) {
	store, _ := newTestStore(t)
	cfg, err := store.BuildTLSConfig(TLSRefOptions{ServerName: "x", NodeID: "n"})
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	if cfg.RootCAs != nil {
		t.Error("RootCAs should be nil when no CABundleRef is set (lets Go use system roots)")
	}
	if len(cfg.Certificates) != 0 {
		t.Error("Certificates should be empty when no ClientPairRef is set")
	}
	if cfg.ServerName != "x" {
		t.Errorf("ServerName = %q", cfg.ServerName)
	}
}

func TestCertStore_BuildTLSConfig_InsecureSkipVerify_LogsWarn(t *testing.T) {
	store, _ := newTestStore(t)

	// Capture slog output to assert the WARN is emitted.
	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	cfg, err := store.BuildTLSConfig(TLSRefOptions{
		InsecureSkipVerify: true,
		NodeID:             "node-loose",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should propagate to tls.Config")
	}
	if !strings.Contains(buf.String(), "verification disabled") {
		t.Errorf("expected WARN log, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "node-loose") {
		t.Errorf("expected node ID in WARN log, got %q", buf.String())
	}
}

func TestCertStore_BuildTLSConfig_ExpiredCert_WarnsButReturnsConfig(t *testing.T) {
	store, _ := newTestStore(t)

	// Generate a cert that expired an hour ago.
	now := time.Now()
	expiredCert, _ := generateTestPair(t, "expired", now.Add(-2*time.Hour), now.Add(-time.Hour))
	if _, err := store.Store(CertEntry{
		ID: "old-ca", Name: "old", Type: TypeCABundle, Source: SourceInline, CertPEM: expiredCert,
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	cfg, err := store.BuildTLSConfig(TLSRefOptions{CABundleRef: "old-ca", NodeID: "n"})
	if err != nil {
		t.Fatalf("expired cert should not block deploy: %v", err)
	}
	if cfg == nil {
		t.Fatal("config should still be returned for expired cert")
	}
	if !strings.Contains(buf.String(), "past notAfter") {
		t.Errorf("expected expiry WARN, got %q", buf.String())
	}
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		for i := 0; i+len(n) <= len(haystack); i++ {
			if haystack[i:i+len(n)] == n {
				return true
			}
		}
	}
	return false
}
