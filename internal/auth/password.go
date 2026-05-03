// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters following the OWASP Password Storage Cheat Sheet
// (2024 recommendation for "second choice" memory-constrained environments,
// which suits LOOPZE's edge-deployable single-binary footprint).
//
// memory = 19 MiB, iterations = 2, parallelism = 1, hash length = 32 bytes,
// salt length = 16 bytes.
const (
	argon2Memory      uint32 = 19 * 1024 // KiB
	argon2Iterations  uint32 = 2
	argon2Parallelism uint8  = 1
	argon2KeyLen      uint32 = 32
	argon2SaltLen            = 16
)

// ErrPasswordMismatch is returned by VerifyPassword when the supplied
// password does not match the stored hash. The hash itself is not exposed
// to callers.
var ErrPasswordMismatch = errors.New("auth: password does not match")

// ErrInvalidHashFormat is returned when a stored hash cannot be parsed.
// This indicates corruption or a hash created by a different scheme.
var ErrInvalidHashFormat = errors.New("auth: invalid password hash format")

// HashPassword derives an Argon2id hash for the plaintext password and
// returns it in the standard PHC string format:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<base64-salt>$<base64-hash>
//
// The salt is generated from crypto/rand. The plaintext is *not* stored.
func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: failed to generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(plain), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLen)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory, argon2Iterations, argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks whether plain matches the previously generated PHC
// hash. It returns nil on match, ErrPasswordMismatch on a clean mismatch,
// or another error if the hash cannot be parsed.
//
// The comparison uses crypto/subtle.ConstantTimeCompare on the derived key
// bytes to avoid timing side-channels that could leak hash contents.
func VerifyPassword(plain, encoded string) error {
	params, salt, want, err := decodeArgon2Hash(encoded)
	if err != nil {
		return err
	}

	got := argon2.IDKey([]byte(plain), salt, params.iterations, params.memory, params.parallelism, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// argon2Params holds the parameters embedded in a PHC hash string. Used
// during verification so we can re-derive a key with the same parameters
// the original hash was generated with.
type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

// decodeArgon2Hash parses a $argon2id$ PHC string into its parameters,
// salt, and key bytes. Only the argon2id variant is accepted.
func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// Expected: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	if parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHashFormat
	}

	return p, salt, hash, nil
}
