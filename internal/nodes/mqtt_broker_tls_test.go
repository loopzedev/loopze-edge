// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// newMqttBrokerForTLSTest constructs a broker via the production factory
// and (optionally) wires a cert store, mirroring the engine's lifecycle.
// The broker is NOT started — tests assert on the prepared TLS state by
// calling applyTLSConfigLocked directly so no network I/O happens.
func newMqttBrokerForTLSTest(t *testing.T, props map[string]any, store *credentials.CertStore) *MqttBroker {
	t.Helper()
	if _, ok := props["host"]; !ok {
		props["host"] = "127.0.0.1"
	}
	inst, err := NewMqttBroker(flow.ConfigNode{
		ID: "mqtt-broker-tls-test", Type: "mqtt-broker", Config: props,
	})
	if err != nil {
		t.Fatalf("NewMqttBroker: %v", err)
	}
	b := inst.(*MqttBroker)
	if store != nil {
		b.SetCertStore(store)
	}
	return b
}

// applyTLSForTest takes the broker's lock and runs the same finaliser
// that Start() uses, so tests can inspect b.cfg.TlsCfg without having
// to bring a live MQTT server up.
func applyTLSForTest(t *testing.T, b *MqttBroker) error {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.applyTLSConfigLocked()
}

func TestMqttBroker_LegacyUseTLS_LogsDeprecationWarn(t *testing.T) {
	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	b := newMqttBrokerForTLSTest(t, map[string]any{"useTLS": true}, nil)
	if err := applyTLSForTest(t, b); err != nil {
		t.Fatalf("applyTLSConfigLocked: %v", err)
	}
	if b.cfg.TlsCfg == nil {
		t.Fatal("expected TlsCfg to be set from legacy useTLS")
	}
	if b.cfg.TlsCfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("MinVersion = %#x, want TLS 1.2", b.cfg.TlsCfg.MinVersion)
	}
	if !strings.Contains(buf.String(), "deprecated") {
		t.Errorf("expected deprecation WARN, got %q", buf.String())
	}
	// Scheme should have been picked at factory time.
	if got := b.cfg.ServerUrls[0].Scheme; got != "mqtts" {
		t.Errorf("scheme = %q, want mqtts", got)
	}
}

func TestMqttBroker_NewTLSBlockWithCABundleRef(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "mqtt-ca", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "mqtt-ca", Name: "MQTT CA", Type: credentials.TypeCABundle,
		Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	captured := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	original := slog.Default()
	slog.SetDefault(captured)
	defer slog.SetDefault(original)

	b := newMqttBrokerForTLSTest(t, map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "mqtt-ca",
		},
	}, store)
	if err := applyTLSForTest(t, b); err != nil {
		t.Fatalf("applyTLSConfigLocked: %v", err)
	}
	if b.cfg.TlsCfg == nil {
		t.Fatal("TlsCfg should be set from new tls block")
	}
	if b.cfg.TlsCfg.RootCAs == nil {
		t.Error("RootCAs should be populated from cert ref")
	}
	if strings.Contains(buf.String(), "deprecated") {
		t.Errorf("new tls block must not emit deprecation WARN, got %q", buf.String())
	}
	if got := b.cfg.ServerUrls[0].Scheme; got != "mqtts" {
		t.Errorf("scheme = %q, want mqtts", got)
	}
}

func TestMqttBroker_NewTLSBlockOverridesLegacyBoolean(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "trusted", now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := store.Store(credentials.CertEntry{
		ID: "trusted-ca", Name: "Trusted", Type: credentials.TypeCABundle,
		Source: credentials.SourceInline, CertPEM: caPEM,
	}); err != nil {
		t.Fatal(err)
	}

	b := newMqttBrokerForTLSTest(t, map[string]any{
		"useTLS": true, // legacy
		"tls": map[string]any{
			"enabled":     true,
			"caBundleRef": "trusted-ca",
		},
	}, store)
	if err := applyTLSForTest(t, b); err != nil {
		t.Fatalf("applyTLSConfigLocked: %v", err)
	}
	if b.cfg.TlsCfg == nil {
		t.Fatal("TlsCfg should be set")
	}
	if b.cfg.TlsCfg.RootCAs == nil {
		t.Error("expected RootCAs from new tls block, not the bare legacy fallback")
	}
}

func TestMqttBroker_NoTLS(t *testing.T) {
	b := newMqttBrokerForTLSTest(t, map[string]any{}, nil)
	if err := applyTLSForTest(t, b); err != nil {
		t.Fatalf("applyTLSConfigLocked: %v", err)
	}
	if b.cfg.TlsCfg != nil {
		t.Error("TlsCfg should remain nil when neither legacy nor new TLS is configured")
	}
	if got := b.cfg.ServerUrls[0].Scheme; got != "mqtt" {
		t.Errorf("scheme = %q, want mqtt", got)
	}
}

func TestMqttBroker_ConflictingInlineAndRef_Errors(t *testing.T) {
	store := newTestCertStore(t)
	now := time.Now()
	caPEM, _ := generateTestPair(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	b := newMqttBrokerForTLSTest(t, map[string]any{
		"tls": map[string]any{
			"enabled":     true,
			"caBundle":    caPEM,
			"caBundleRef": "anything",
		},
	}, store)
	err := applyTLSForTest(t, b)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error, got %v", err)
	}
}
