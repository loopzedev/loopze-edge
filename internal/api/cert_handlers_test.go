// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/auth"
	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// generateCertPEM mints a fresh self-signed ed25519 certificate suitable
// for cert-store CRUD tests. Reused across the file so each test gets a
// unique fingerprint without needing to manage fixtures.
func generateCertPEM(t *testing.T, cn string) (certPEM, keyPEM string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		Issuer:                pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

// attachCertStore creates an empty CertStore backed by the test's
// memStorage and attaches it to the deps so the cert handlers light up.
func attachCertStore(t *testing.T, ts *testServer) *credentials.CertStore {
	t.Helper()
	dir := t.TempDir()
	cm := credentials.NewCredentialManager(filepath.Join(dir, "loopze.key"))
	if err := cm.EnsureKeyFile(); err != nil {
		t.Fatalf("EnsureKeyFile: %v", err)
	}
	store := credentials.NewCertStore(cm, ts.deps.Storage)
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ts.deps.Certs = store
	return store
}

// seedEditorAndLogin creates an editor user and logs the test client in.
// Returns nothing; on success the client's cookie jar carries a valid
// session for subsequent requests.
func seedEditorAndLogin(t *testing.T, ts *testServer, c *http.Client) {
	t.Helper()
	hash, err := auth.HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := auth.NewID()
	now := time.Now().UTC()
	if err := ts.users.Create(auth.User{
		ID:           id,
		Username:     "edit",
		PasswordHash: hash,
		Role:         auth.RoleEditor,
		AuthProvider: auth.ProviderLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
	// Setup gate insists at least one admin exists; create one too.
	adminHash, _ := auth.HashPassword("hunter22")
	adminID, _ := auth.NewID()
	if err := ts.users.Create(auth.User{
		ID: adminID, Username: "admin", PasswordHash: adminHash,
		Role: auth.RoleAdmin, AuthProvider: auth.ProviderLocal,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if code := login(t, ts, c, "edit", "hunter22"); code != http.StatusOK {
		t.Fatalf("editor login: got %d, want 200", code)
	}
}

func TestCertAPI_CRUDRoundtrip(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	attachCertStore(t, ts)
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	certPEM, _ := generateCertPEM(t, "test-ca-1")

	// Create
	code, body := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs", map[string]any{
		"id": "ca-one", "name": "Internal CA", "type": "ca-bundle",
		"source": "inline", "certPem": certPEM,
	})
	if code != http.StatusCreated {
		t.Fatalf("POST /certs: got %d, want 201; body=%v", code, body)
	}
	if got := body["fingerprint"]; got == "" {
		t.Errorf("fingerprint should be populated, body=%v", body)
	}

	// List
	code, body = doJSON(t, c, http.MethodGet, ts.url+"/api/v1/certs", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /certs: got %d, want 200", code)
	}
	list, _ := body["certs"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(list))
	}

	// Get single
	code, single := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/certs/ca-one", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /certs/ca-one: got %d, want 200", code)
	}
	if single["id"] != "ca-one" {
		t.Errorf("expected id ca-one, got %v", single["id"])
	}

	// Update — change name; cert material is required again because
	// CertEntry.Validate enforces it.
	code, body = doJSON(t, c, http.MethodPut, ts.url+"/api/v1/certs/ca-one", map[string]any{
		"name": "Renamed", "type": "ca-bundle", "source": "inline", "certPem": certPEM,
	})
	if code != http.StatusOK {
		t.Fatalf("PUT /certs/ca-one: got %d, want 200; body=%v", code, body)
	}
	if body["name"] != "Renamed" {
		t.Errorf("name = %v, want Renamed", body["name"])
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.url+"/api/v1/certs/ca-one", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /certs/ca-one: got %d, want 204", resp.StatusCode)
	}

	// List should be empty
	code, body = doJSON(t, c, http.MethodGet, ts.url+"/api/v1/certs", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /certs after delete: got %d", code)
	}
	if list, _ := body["certs"].([]any); len(list) != 0 {
		t.Errorf("expected empty list after delete, got %d", len(list))
	}
}

func TestCertAPI_GetDoesNotLeakPEM(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	attachCertStore(t, ts)
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	certPEM, _ := generateCertPEM(t, "leak-test")
	if _, err := ts.deps.Certs.Store(credentials.CertEntry{
		ID: "ca", Name: "ca", Type: credentials.TypeCABundle, Source: credentials.SourceInline, CertPEM: certPEM,
	}); err != nil {
		t.Fatal(err)
	}

	// List
	code, body := doJSON(t, c, http.MethodGet, ts.url+"/api/v1/certs", nil)
	if code != http.StatusOK {
		t.Fatal(code)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "BEGIN CERTIFICATE") || strings.Contains(string(raw), "BEGIN PRIVATE KEY") {
		t.Errorf("list response leaked PEM material: %s", raw)
	}

	// Single
	code, body = doJSON(t, c, http.MethodGet, ts.url+"/api/v1/certs/ca", nil)
	if code != http.StatusOK {
		t.Fatal(code)
	}
	raw, _ = json.Marshal(body)
	if strings.Contains(string(raw), "BEGIN CERTIFICATE") || strings.Contains(string(raw), "BEGIN PRIVATE KEY") {
		t.Errorf("single-get response leaked PEM material: %s", raw)
	}
}

func TestCertAPI_DeleteBlockedByWorkspaceReferences(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	attachCertStore(t, ts)
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	certPEM, _ := generateCertPEM(t, "in-use")
	if _, err := ts.deps.Certs.Store(credentials.CertEntry{
		ID: "in-use-ca", Name: "ca", Type: credentials.TypeCABundle,
		Source: credentials.SourceInline, CertPEM: certPEM,
	}); err != nil {
		t.Fatal(err)
	}

	// Inject a workspace that references the cert. memStorage.SaveWorkspace
	// is a no-op in the auth test harness, so we go directly through the
	// LoadWorkspace path by replacing the storage method.
	mem, ok := ts.deps.Storage.(*memStorage)
	if !ok {
		t.Fatalf("unexpected storage type %T", ts.deps.Storage)
	}
	mem.workspace = &flow.Workspace{
		Flows: []flow.Flow{{
			ID: "f", Nodes: []flow.Node{{
				ID: "tcp-out-1", Type: "tcp-out",
				Config: map[string]any{
					"tls": map[string]any{
						"enabled":     true,
						"caBundleRef": "in-use-ca",
					},
				},
			}},
		}},
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.url+"/api/v1/certs/in-use-ca", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE blocked: got %d, want 409", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	refs, _ := body["references"].([]any)
	if len(refs) != 1 {
		t.Errorf("expected 1 reference, got %v", refs)
	}

	// Cert must still exist after the rejected delete.
	if _, ok := ts.deps.Certs.Get("in-use-ca"); !ok {
		t.Error("cert should still be present after blocked delete")
	}
}

func TestCertAPI_ValidateDoesNotPersist(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	attachCertStore(t, ts)
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	certPEM, _ := generateCertPEM(t, "validate-only")
	code, body := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs/validate", map[string]any{
		"type": "ca-bundle", "source": "inline", "certPem": certPEM,
	})
	if code != http.StatusOK {
		t.Fatalf("validate: got %d, want 200; body=%v", code, body)
	}
	if body["fingerprint"] == "" {
		t.Errorf("fingerprint should be populated, body=%v", body)
	}
	// The store must be empty — validate is read-only.
	if list := ts.deps.Certs.List(); len(list) != 0 {
		t.Errorf("validate should not persist; store has %d entries", len(list))
	}
}

func TestCertAPI_CreateRejectsInvalidEntry(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	attachCertStore(t, ts)
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	t.Run("bad slug", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs", map[string]any{
			"id": "Bad ID", "type": "ca-bundle", "source": "inline", "certPem": "x",
		})
		if code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", code)
		}
	})

	t.Run("malformed PEM", func(t *testing.T) {
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs", map[string]any{
			"id": "ok-id", "type": "ca-bundle", "source": "inline", "certPem": "not-a-pem",
		})
		if code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", code)
		}
	})

	t.Run("duplicate ID", func(t *testing.T) {
		certPEM, _ := generateCertPEM(t, "dupe")
		entry := map[string]any{
			"id": "dupe", "type": "ca-bundle", "source": "inline", "certPem": certPEM,
		}
		if code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs", entry); code != http.StatusCreated {
			t.Fatalf("first POST: got %d", code)
		}
		code, _ := doJSON(t, c, http.MethodPost, ts.url+"/api/v1/certs", entry)
		if code != http.StatusConflict {
			t.Errorf("duplicate: got %d, want 409", code)
		}
	})
}
