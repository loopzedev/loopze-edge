// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package server

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// migrateOpcuaCertConfigs walks every opcua-server config node in the
// workspace and migrates legacy clientCertFile / clientKeyFile entries
// into proper cert-store references with Source: file. The original
// fields are removed once migration succeeds so the deprecation WARN
// does not fire on every subsequent deploy.
//
// The function is idempotent: a config that already carries a certRef
// is skipped, and a deterministic per-config cert ID
// ("opcua-<configID>") means a partial-success state on an earlier run
// converges cleanly the next time.
//
// Returns the number of configs that were migrated. The caller is
// responsible for persisting the workspace (the cert entries are saved
// by certs.Store internally).
func migrateOpcuaCertConfigs(ws *flow.Workspace, certs *credentials.CertStore) (int, error) {
	if ws == nil || certs == nil {
		return 0, nil
	}
	migrated := 0
	for i := range ws.Configs {
		cfg := &ws.Configs[i]
		if cfg.Type != "opcua-server" {
			continue
		}
		if cfg.Config == nil {
			continue
		}

		certPath, _ := cfg.Config["clientCertFile"].(string)
		keyPath, _ := cfg.Config["clientKeyFile"].(string)
		if certPath == "" || keyPath == "" {
			continue
		}
		if existing, _ := cfg.Config["certRef"].(string); existing != "" {
			continue
		}

		certID := opcuaConfigCertID(cfg.ID)
		entry := credentials.CertEntry{
			ID:       certID,
			Name:     opcuaConfigCertName(cfg.Name, cfg.ID),
			Type:     credentials.TypeClientPair,
			Source:   credentials.SourceFile,
			CertPath: certPath,
			KeyPath:  keyPath,
			Notes:    "Migrated from opcua-server " + cfg.ID,
		}

		if _, err := certs.Store(entry); err != nil {
			if errors.Is(err, credentials.ErrCertExists) {
				slog.Debug("opcua migration: cert entry already exists, reusing",
					"config_id", cfg.ID, "cert_id", certID)
			} else {
				return migrated, fmt.Errorf("opcua migration: store cert for %q: %w", cfg.ID, err)
			}
		}

		cfg.Config["certRef"] = certID
		delete(cfg.Config, "clientCertFile")
		delete(cfg.Config, "clientKeyFile")
		migrated++

		slog.Info("opcua migration: config node moved to cert store",
			"config_id", cfg.ID, "cert_id", certID)
	}
	return migrated, nil
}

// opcuaConfigCertID returns the deterministic cert-store ID assigned to
// the client-pair we synthesise from a legacy opcua-server config.
func opcuaConfigCertID(configID string) string {
	return "opcua-" + configID
}

// opcuaConfigCertName returns a human-readable display name for the
// migrated entry, used when the operator browses the cert store UI.
func opcuaConfigCertName(configName, configID string) string {
	if configName == "" {
		return "OPC UA: " + configID
	}
	return "OPC UA: " + configName
}
