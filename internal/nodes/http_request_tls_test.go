// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"crypto/tls"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// initHTTPReqWithStore creates an HTTPRequestNode, attaches the supplied
// cert store BEFORE Init runs (mirroring what the engine does in
// production), and returns the initialised node.
func initHTTPReqWithStore(t *testing.T, props map[string]any, store *credentials.CertStore) *HTTPRequestNode {
	t.Helper()
	inst, err := NewHTTPRequestNode(flow.NodeConfig{
		ID: t.Name(), Type: "http-request", FlowID: "f1", Properties: props,
	})
	if err != nil {
		t.Fatalf("NewHTTPRequestNode: %v", err)
	}
	n := inst.(*HTTPRequestNode)
	if store != nil {
		n.SetCertStore(store)
	}
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return n
}

func transportTLSConfig(t *testing.T, n *HTTPRequestNode) *tls.Config {
	t.Helper()
	tr, ok := n.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", n.client.Transport)
	}
	return tr.TLSClientConfig
}

func TestHTTPRequest_LegacyTLSInsecure_LogsDeprecationWarn(t *testing.T) {
	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	n := initHTTPReqWithStore(t, map[string]any{
		"method": "GET", "url": "https://example.com",
		"tlsInsecure": true,
	}, nil)

	cfg := transportTLSConfig(t, n)
	if cfg == nil {
		t.Fatal("legacy tlsInsecure should produce a TLSClientConfig")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
	if !strings.Contains(buf.String(), "deprecated") {
		t.Errorf("expected deprecation WARN, got %q", buf.String())
	}
}

func TestHTTPRequest_NewTLSBlockWithCABundleRef(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "internal-ca", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "internal-ca", Name: "Internal", Type: credentials.TypeCABundle,
		Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	n := initHTTPReqWithStore(t, map[string]any{
		"method": "GET", "url": "https://internal.example.com",
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "internal-ca",
		},
	}, store)

	cfg := transportTLSConfig(t, n)
	if cfg == nil {
		t.Fatal("expected non-nil TLSClientConfig")
	}
	if cfg.RootCAs == nil {
		t.Error("RootCAs should be set from cert ref")
	}
	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should NOT be set")
	}
	if strings.Contains(buf.String(), "deprecated") {
		t.Errorf("new tls block must not emit deprecation WARN, got %q", buf.String())
	}
}

func TestHTTPRequest_NewTLSBlockWinsOverLegacyBoolean(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "trusted", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "trusted-ca", Name: "Trusted", Type: credentials.TypeCABundle,
		Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	n := initHTTPReqWithStore(t, map[string]any{
		"method":      "GET",
		"url":         "https://example.com",
		"tlsInsecure": true, // legacy
		"tls": map[string]any{ // new
			"enabled":     true,
			"caBundleRef": "trusted-ca",
		},
	}, store)

	cfg := transportTLSConfig(t, n)
	if cfg == nil {
		t.Fatal("expected non-nil TLSClientConfig")
	}
	if cfg.InsecureSkipVerify {
		t.Error("new tls block should override legacy tlsInsecure (got InsecureSkipVerify=true)")
	}
	if cfg.RootCAs == nil {
		t.Error("expected new tls block RootCAs to be applied")
	}
}

func TestHTTPRequest_DisabledTLSBlock_FallsBackToLegacy(t *testing.T) {
	// tls.enabled=false should NOT short-circuit the legacy path —
	// operators may legitimately disable the block while keeping the
	// old boolean for one release of grace.
	n := initHTTPReqWithStore(t, map[string]any{
		"method":      "GET",
		"url":         "https://example.com",
		"tlsInsecure": true,
		"tls":         map[string]any{"enabled": false},
	}, nil)

	cfg := transportTLSConfig(t, n)
	if cfg == nil || !cfg.InsecureSkipVerify {
		t.Error("legacy tlsInsecure should still apply when tls block is disabled")
	}
}

func TestHTTPRequest_ConflictingInlineAndRef_Errors(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	props := map[string]any{
		"method": "GET", "url": "https://example.com",
		"tls": map[string]any{
			"enabled":     true,
			"caBundle":    caPEM,
			"caBundleRef": "anything",
		},
	}
	inst, err := NewHTTPRequestNode(flow.NodeConfig{
		ID: t.Name(), Type: "http-request", FlowID: "f1", Properties: props,
	})
	if err != nil {
		t.Fatal(err)
	}
	n := inst.(*HTTPRequestNode)
	n.SetCertStore(store)
	if err := n.Init(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error, got %v", err)
	}
}
