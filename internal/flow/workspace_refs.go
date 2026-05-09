// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

// CertReference describes a single occurrence of a `caBundleRef` /
// `clientPairRef` (or any future cert-store reference field) inside a
// workspace. The HTTP layer surfaces a list of these to the operator
// when a DELETE on the cert store is blocked because flows still depend
// on the entry.
type CertReference struct {
	// FlowID is the ID of the flow containing the node, or "" when the
	// reference lives on a workspace-global config node.
	FlowID string `json:"flowId,omitempty"`

	// NodeID is the ID of the referencing node or config node.
	NodeID string `json:"nodeId"`

	// NodeType is the type identifier of the referencing node
	// (e.g. "tcp-out", "mqtt-broker"). Useful for UI rendering so the
	// operator can recognise where the dependency lives.
	NodeType string `json:"nodeType"`

	// Field is the property path that holds the reference, e.g.
	// "tls.caBundleRef" or "tls.clientPairRef". Disambiguates when a
	// single node references the same cert via multiple slots (rare but
	// possible: CA bundle and client pair both pointing at related IDs).
	Field string `json:"field"`
}

// ScanCertReferences walks the workspace and returns every reference to
// the given cert ID. Used by the cert-store HTTP layer to refuse DELETE
// requests that would leave dangling refs in deployed flows.
//
// Currently inspects:
//
//   - Each flow node's "tls" sub-block for "caBundleRef" / "clientPairRef".
//   - Each workspace-global config node's "tls" sub-block likewise.
//
// When OPC UA gains its own cert reference (Step 9 of the central TLS
// storage rollout), extend the inner check rather than introducing a
// parallel walker.
func ScanCertReferences(ws Workspace, certID string) []CertReference {
	if certID == "" {
		return nil
	}
	var refs []CertReference

	for _, f := range ws.Flows {
		for _, n := range f.Nodes {
			refs = appendTLSRefs(refs, certID, n.Config, f.ID, n.ID, n.Type)
		}
	}
	for _, c := range ws.Configs {
		refs = appendTLSRefs(refs, certID, c.Config, "", c.ID, c.Type)
	}
	return refs
}

// appendTLSRefs extracts cert references from a single node's "tls" block
// and appends a CertReference for each match against certID.
func appendTLSRefs(out []CertReference, certID string, cfg map[string]any, flowID, nodeID, nodeType string) []CertReference {
	tls, ok := cfg["tls"].(map[string]any)
	if !ok {
		return out
	}
	for _, field := range []string{"caBundleRef", "clientPairRef"} {
		if v, _ := tls[field].(string); v == certID {
			out = append(out, CertReference{
				FlowID:   flowID,
				NodeID:   nodeID,
				NodeType: nodeType,
				Field:    "tls." + field,
			})
		}
	}
	return out
}
