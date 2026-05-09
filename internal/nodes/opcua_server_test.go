// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// opcuaTestEndpoint returns the configured Deno test-server endpoint or skips
// the test. Centralised so all OPC UA E2E tests share the same gating logic.
func opcuaTestEndpoint(t *testing.T) string {
	t.Helper()
	endpoint := os.Getenv("LOOPZE_OPCUA_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set LOOPZE_OPCUA_TEST_ENDPOINT to run (e.g. opc.tcp://localhost:4840)")
	}
	return endpoint
}

func TestNewOpcuaServerValidation(t *testing.T) {
	if _, err := NewOpcuaServer(flow.ConfigNode{ID: "x", Type: "opcua-server", Config: map[string]any{}}); err == nil {
		t.Error("missing endpointUrl should fail")
	}

	cfg := flow.ConfigNode{
		ID:   "srv-1",
		Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl":    "opc.tcp://localhost:4840",
			"securityPolicy": "None",
			"authMode":       "anonymous",
		},
	}
	inst, err := NewOpcuaServer(cfg)
	if err != nil {
		t.Fatalf("NewOpcuaServer: %v", err)
	}
	srv := inst.(*OpcuaServer)
	if srv.endpointURL != "opc.tcp://localhost:4840" {
		t.Errorf("endpoint: %q", srv.endpointURL)
	}
	fill, text := srv.Status()
	if fill != "grey" || text != "disconnected" {
		t.Errorf("initial status: (%q,%q)", fill, text)
	}
}

func TestOpcuaServerSecurityModeForcedNoneOnNonePolicy(t *testing.T) {
	// Spec: SecurityPolicy=None implies SecurityMode=None.
	cfg := flow.ConfigNode{
		ID: "srv-2", Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl":    "opc.tcp://localhost:4840",
			"securityPolicy": "None",
			"securityMode":   "SignAndEncrypt", // would be invalid; constructor must coerce
		},
	}
	if _, err := NewOpcuaServer(cfg); err != nil {
		t.Fatalf("constructor must accept and coerce: %v", err)
	}
}

func TestOpcuaTestConnectE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	cfg := flow.ConfigNode{
		ID: "srv-e2e", Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl": endpoint,
			"authMode":    "anonymous",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := OpcuaTestConnect(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("OpcuaTestConnect: %v", err)
	}
	if info == nil || info.EndpointURL != endpoint {
		t.Fatalf("unexpected info: %+v", info)
	}
	t.Logf("server time = %q", info.ServerTime)
}

func TestOpcuaServerStartStopE2E(t *testing.T) {
	endpoint := opcuaTestEndpoint(t)

	cfg := flow.ConfigNode{
		ID: "srv-lifecycle", Type: "opcua-server",
		Config: map[string]any{"endpointUrl": endpoint},
	}
	inst, err := NewOpcuaServer(cfg)
	if err != nil {
		t.Fatalf("NewOpcuaServer: %v", err)
	}
	srv := inst.(*OpcuaServer)

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait up to 5 s for the connection to come up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		fill, _ := srv.Status()
		if fill == "green" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	fill, text := srv.Status()
	if fill != "green" {
		t.Fatalf("expected green status, got (%q,%q)", fill, text)
	}
	if srv.Client() == nil {
		t.Error("Client() must return a connected client")
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	fill, _ = srv.Status()
	if fill != "grey" {
		t.Errorf("post-stop fill = %q; want grey", fill)
	}
}
