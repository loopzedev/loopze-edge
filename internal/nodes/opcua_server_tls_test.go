// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"strings"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

func TestNewOpcuaServer_RejectsCertRefAndLegacyFilesTogether(t *testing.T) {
	_, err := NewOpcuaServer(flow.ConfigNode{
		ID: "x", Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl":    "opc.tcp://localhost:4840",
			"certRef":        "stored-cert",
			"clientCertFile": "/etc/legacy.crt",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error, got %v", err)
	}
}

func TestNewOpcuaServer_AcceptsCertRefAlone(t *testing.T) {
	_, err := NewOpcuaServer(flow.ConfigNode{
		ID: "x", Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl":    "opc.tcp://localhost:4840",
			"securityPolicy": "Basic256Sha256",
			"securityMode":   "SignAndEncrypt",
			"authMode":       "certificate",
			"certRef":        "stored-cert",
		},
	})
	if err != nil {
		t.Fatalf("certRef alone should be accepted: %v", err)
	}
}

func TestOpcuaServer_StartFailsWhenCertRefIsSetButStoreIsNil(t *testing.T) {
	inst, err := NewOpcuaServer(flow.ConfigNode{
		ID: "x", Type: "opcua-server",
		Config: map[string]any{
			"endpointUrl": "opc.tcp://localhost:4840",
			"authMode":    "certificate",
			"certRef":     "stored-cert",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := inst.(*OpcuaServer)
	// Engine-side injection is skipped here, simulating a legacy entry-
	// point that forgot to wire the cert store.
	err = srv.Start()
	if err == nil || !strings.Contains(err.Error(), "no cert store is wired") {
		t.Errorf("expected wiring error, got %v", err)
	}
	_ = srv.Stop()
}
