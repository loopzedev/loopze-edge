// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import (
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// sessionKeySize is the length in bytes of the HMAC-SHA256 signing key for
// session cookies. 32 bytes matches the SHA-256 block-output size and is
// the common recommendation for HMAC keys.
const sessionKeySize = 32

// EnsureSessionKey returns the contents of the session signing key file,
// creating it with cryptographically random bytes (mode 0600) if it does
// not yet exist.
//
// Deleting the file invalidates every existing session — that is the
// intended manual rotation procedure.
func EnsureSessionKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != sessionKeySize {
			return nil, fmt.Errorf("auth: session key %q has invalid length %d (expected %d)", path, len(data), sessionKeySize)
		}
		slog.Debug("session signing key loaded", "path", path)
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("auth: failed to read session key %q: %w", path, err)
	}

	slog.Info("generating new session signing key", "path", path)

	key := make([]byte, sessionKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("auth: failed to generate session key: %w", err)
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("auth: failed to write session key %q: %w", path, err)
	}
	return key, nil
}
