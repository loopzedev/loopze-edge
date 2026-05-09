// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

import "testing"

func TestScanCertReferences_FindsTLSBlockRefs(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "flow-1",
			Nodes: []Node{
				{
					ID: "tcp-out-a", Type: "tcp-out",
					Config: map[string]any{
						"tls": map[string]any{
							"enabled":     true,
							"caBundleRef": "trusted-ca",
						},
					},
				},
				{
					ID: "tcp-req-b", Type: "tcp-request",
					Config: map[string]any{
						"tls": map[string]any{
							"enabled":       true,
							"clientPairRef": "trusted-ca", // intentionally same ID, different field
						},
					},
				},
				{
					ID: "noop", Type: "debug",
					Config: map[string]any{},
				},
			},
		}},
		Configs: []ConfigNode{{
			ID: "mqtt-broker-1", Type: "mqtt-broker",
			Config: map[string]any{
				"tls": map[string]any{
					"caBundleRef": "trusted-ca",
				},
			},
		}},
	}

	refs := ScanCertReferences(ws, "trusted-ca")
	if len(refs) != 3 {
		t.Fatalf("len = %d, want 3 (two flow nodes + one config node), got %+v", len(refs), refs)
	}

	wantFields := map[string]string{
		"tcp-out-a":     "tls.caBundleRef",
		"tcp-req-b":     "tls.clientPairRef",
		"mqtt-broker-1": "tls.caBundleRef",
	}
	for _, r := range refs {
		if want, ok := wantFields[r.NodeID]; !ok || want != r.Field {
			t.Errorf("ref %+v: unexpected entry", r)
		}
	}

	// FlowID is set on flow nodes, empty for config nodes.
	for _, r := range refs {
		if r.NodeID == "mqtt-broker-1" && r.FlowID != "" {
			t.Errorf("config-node ref should have empty FlowID, got %q", r.FlowID)
		}
		if r.NodeID == "tcp-out-a" && r.FlowID != "flow-1" {
			t.Errorf("flow-node ref FlowID = %q, want flow-1", r.FlowID)
		}
	}
}

func TestScanCertReferences_IgnoresUnrelatedIDs(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "f", Nodes: []Node{{
				ID: "n", Type: "tcp-out",
				Config: map[string]any{
					"tls": map[string]any{"caBundleRef": "other-ca"},
				},
			}},
		}},
	}
	if refs := ScanCertReferences(ws, "trusted-ca"); len(refs) != 0 {
		t.Errorf("expected 0 refs, got %+v", refs)
	}
}

func TestScanCertReferences_EmptyID(t *testing.T) {
	ws := Workspace{Flows: []Flow{{ID: "f"}}}
	if refs := ScanCertReferences(ws, ""); refs != nil {
		t.Errorf("empty cert ID should return nil, got %+v", refs)
	}
}

func TestScanCertReferences_NoTLSBlock(t *testing.T) {
	ws := Workspace{
		Flows: []Flow{{
			ID: "f", Nodes: []Node{{
				ID: "n", Type: "debug", Config: map[string]any{"complete": true},
			}},
		}},
	}
	if refs := ScanCertReferences(ws, "anything"); len(refs) != 0 {
		t.Errorf("expected 0 refs, got %+v", refs)
	}
}
