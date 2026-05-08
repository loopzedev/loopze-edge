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
	"strings"
)

// ParseTLSBlock builds a *tls.Config from a "tls" sub-object on a
// node's properties. The expected schema (matches NODE_TCP_UDP.md § 8):
//
//	{
//	  "enabled": true,
//	  "serverName": "device.example.com",
//	  "caBundle":   "<PEM, optional — defaults to system roots>",
//	  "clientCert": "<PEM, optional>",
//	  "clientKey":  "<PEM, optional>",
//	  "insecureSkipVerify": false
//	}
//
// Returns (nil, nil) when no tls block is present or enabled=false —
// callers treat that as "plain TCP". Returns an error for malformed
// PEM / missing-half client cert pair / similar misconfiguration so
// the engine refuses the deploy at Init time.
//
// nodeID is used only for log-line context on the
// insecureSkipVerify=true warning.
func ParseTLSBlock(props map[string]any, nodeID string) (*tls.Config, error) {
	raw, ok := props["tls"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	enabled, _ := raw["enabled"].(bool)
	if !enabled {
		return nil, nil
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if v := strings.TrimSpace(stringVal(raw, "serverName", "")); v != "" {
		cfg.ServerName = v
	}

	if pem := strings.TrimSpace(stringVal(raw, "caBundle", "")); pem != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			return nil, errors.New("tls: caBundle: no valid PEM certificates")
		}
		cfg.RootCAs = pool
	}

	certPEM := strings.TrimSpace(stringVal(raw, "clientCert", ""))
	keyPEM := strings.TrimSpace(stringVal(raw, "clientKey", ""))
	switch {
	case certPEM == "" && keyPEM == "":
		// no client auth
	case certPEM == "" || keyPEM == "":
		return nil, errors.New("tls: clientCert and clientKey must be set together")
	default:
		pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("tls: client cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}

	if v, ok := raw["insecureSkipVerify"].(bool); ok && v {
		// nosec G402 — deliberately user-controlled, logged at WARN
		// on every deploy so misconfiguration is loud.
		cfg.InsecureSkipVerify = true
		slog.Warn("tls: certificate verification disabled",
			"node_id", nodeID, "server_name", cfg.ServerName)
	}

	return cfg, nil
}
