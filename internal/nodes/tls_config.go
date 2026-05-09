// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/loopzedev/loopze-edge/internal/credentials"
)

// ParseTLSBlock builds a *tls.Config from a "tls" sub-object on a
// node's properties. Three source modes are supported per field — at
// most one may be set per slot (CA bundle and client pair are
// independent slots):
//
//	{
//	  "enabled": true,
//	  "serverName": "device.example.com",
//
//	  // Inline mode (PEM embedded in the flow JSON):
//	  "caBundle":   "<PEM, optional — defaults to system roots>",
//	  "clientCert": "<PEM, optional>",
//	  "clientKey":  "<PEM, optional>",
//
//	  // Reference mode (resolves against the central cert store):
//	  "caBundleRef":   "<cert-id of a ca-bundle entry>",
//	  "clientPairRef": "<cert-id of a client-pair entry>",
//
//	  // File mode (operator-managed paths on disk, read fresh each
//	  // connection init — works with cert-manager / Let's Encrypt):
//	  "caBundleFile":   "/path/to/ca.pem",
//	  "clientCertFile": "/path/to/client.crt",
//	  "clientKeyFile":  "/path/to/client.key",
//
//	  "insecureSkipVerify": false
//	}
//
// Inline / ref / file for the same logical slot are mutually
// exclusive (setting more than one returns an error). Returns
// (nil, nil) when no tls block is present or enabled=false; callers
// treat that as "plain TCP".
//
// The certs parameter may be nil in tests / engine-only setups; if
// nil and any *Ref field is set, an error is returned so the misconfig
// surfaces at deploy time rather than as a silent fall-through.
//
// nodeID is used for log-line context on the InsecureSkipVerify and
// expired-cert warnings.
func ParseTLSBlock(props map[string]any, nodeID string, certs *credentials.CertStore) (*tls.Config, error) {
	raw, ok := props["tls"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	enabled, _ := raw["enabled"].(bool)
	if !enabled {
		return nil, nil
	}

	serverName := strings.TrimSpace(stringVal(raw, "serverName", ""))
	caBundleInline := strings.TrimSpace(stringVal(raw, "caBundle", ""))
	clientCertInline := strings.TrimSpace(stringVal(raw, "clientCert", ""))
	clientKeyInline := strings.TrimSpace(stringVal(raw, "clientKey", ""))
	caBundleRef := strings.TrimSpace(stringVal(raw, "caBundleRef", ""))
	clientPairRef := strings.TrimSpace(stringVal(raw, "clientPairRef", ""))
	caBundleFile := strings.TrimSpace(stringVal(raw, "caBundleFile", ""))
	clientCertFile := strings.TrimSpace(stringVal(raw, "clientCertFile", ""))
	clientKeyFile := strings.TrimSpace(stringVal(raw, "clientKeyFile", ""))
	insecureSkip, _ := raw["insecureSkipVerify"].(bool)

	caSourcesSet := boolCount(caBundleInline != "", caBundleRef != "", caBundleFile != "")
	if caSourcesSet > 1 {
		return nil, errors.New("tls: caBundle, caBundleRef, and caBundleFile are mutually exclusive")
	}
	clientHasInline := clientCertInline != "" || clientKeyInline != ""
	clientHasFile := clientCertFile != "" || clientKeyFile != ""
	clientSourcesSet := boolCount(clientHasInline, clientPairRef != "", clientHasFile)
	if clientSourcesSet > 1 {
		return nil, errors.New("tls: clientCert/clientKey, clientPairRef, and clientCertFile/clientKeyFile are mutually exclusive")
	}
	if (caBundleRef != "" || clientPairRef != "") && certs == nil {
		return nil, errors.New("tls: cert reference set but no cert store is wired into the engine")
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}

	switch {
	case caBundleRef != "":
		certPEM, _, err := certs.LoadMaterial(caBundleRef, credentials.TypeCABundle)
		if err != nil {
			return nil, fmt.Errorf("tls: caBundleRef %q: %w", caBundleRef, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(certPEM) {
			return nil, fmt.Errorf("tls: caBundleRef %q contained no valid PEM certificates", caBundleRef)
		}
		cfg.RootCAs = pool

	case caBundleFile != "":
		pem, err := os.ReadFile(caBundleFile)
		if err != nil {
			return nil, fmt.Errorf("tls: caBundleFile %q: %w", caBundleFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("tls: caBundleFile %q contained no valid PEM certificates", caBundleFile)
		}
		cfg.RootCAs = pool

	case caBundleInline != "":
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caBundleInline)) {
			return nil, errors.New("tls: caBundle: no valid PEM certificates")
		}
		cfg.RootCAs = pool
	}

	switch {
	case clientPairRef != "":
		certPEM, keyPEM, err := certs.LoadMaterial(clientPairRef, credentials.TypeClientPair)
		if err != nil {
			return nil, fmt.Errorf("tls: clientPairRef %q: %w", clientPairRef, err)
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: clientPairRef %q: %w", clientPairRef, err)
		}
		cfg.Certificates = []tls.Certificate{pair}

	case clientHasFile:
		if clientCertFile == "" || clientKeyFile == "" {
			return nil, errors.New("tls: clientCertFile and clientKeyFile must be set together")
		}
		certPEM, err := os.ReadFile(clientCertFile)
		if err != nil {
			return nil, fmt.Errorf("tls: clientCertFile %q: %w", clientCertFile, err)
		}
		keyPEM, err := os.ReadFile(clientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: clientKeyFile %q: %w", clientKeyFile, err)
		}
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("tls: client cert/key from file: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}

	case clientCertInline != "" || clientKeyInline != "":
		if clientCertInline == "" || clientKeyInline == "" {
			return nil, errors.New("tls: clientCert and clientKey must be set together")
		}
		pair, err := tls.X509KeyPair([]byte(clientCertInline), []byte(clientKeyInline))
		if err != nil {
			return nil, fmt.Errorf("tls: client cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}

	if insecureSkip {
		// nosec G402 — deliberately user-controlled, logged at WARN
		// on every deploy so misconfiguration is loud.
		cfg.InsecureSkipVerify = true
		slog.Warn("tls: certificate verification disabled",
			"node_id", nodeID, "server_name", cfg.ServerName)
	}

	return cfg, nil
}

// boolCount returns the number of true values, used to enforce
// at-most-one-source semantics across the TLS block's parallel input
// modes (inline / ref / file).
func boolCount(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}
