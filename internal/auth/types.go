// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package auth provides user identity, role-based authorization, password
// hashing, and session management for the Flint editor. It is the security
// boundary between the HTTP/WebSocket layer and the rest of the runtime.
//
// V1 supports only local users with Argon2id-hashed passwords. The data model
// includes an AuthProvider field so future SSO providers (OAuth2, Azure AD)
// can be added without schema migration.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Role identifies the permission level of a user. Roles are rank-based
// internally (admin > editor > viewer) so middleware can do simple
// minimum-role comparisons. From the user's perspective the roles are flat
// and named, with no permission matrix to configure.
type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleAdmin  Role = "admin"
)

// Rank returns the integer rank of the role for >= comparisons.
// Unknown roles get rank 0 and never satisfy any RequireRole check.
func (r Role) Rank() int {
	switch r {
	case RoleViewer:
		return 1
	case RoleEditor:
		return 2
	case RoleAdmin:
		return 3
	default:
		return 0
	}
}

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	return r.Rank() > 0
}

// AuthProvider identifies the source that authenticated a user. V1 only
// supports the local provider; future providers (oauth2:google, azure-ad,
// etc.) will share the same User schema.
type AuthProvider string

const (
	ProviderLocal AuthProvider = "local"
)

// MinPasswordLength is the minimum acceptable plaintext password length.
// Follows NIST SP 800-63B: length over complexity.
const MinPasswordLength = 8

// MaxPasswordLength caps plaintext passwords to prevent DoS via huge inputs
// fed through Argon2.
const MaxPasswordLength = 1024

// MinUsernameLength is the minimum acceptable username length after trimming.
const MinUsernameLength = 1

// MaxUsernameLength caps usernames to a sane value.
const MaxUsernameLength = 64

// User represents an authenticated identity. Usernames are stored normalised
// (lower-case, trimmed) so case-insensitive lookups can use direct equality.
type User struct {
	ID           string       `json:"id"`
	Username     string       `json:"username"`
	PasswordHash string       `json:"passwordHash,omitempty"`
	Role         Role         `json:"role"`
	AuthProvider AuthProvider `json:"authProvider"`
	Disabled     bool         `json:"disabled"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// PublicUser is the User shape safe to send over the wire. It strips the
// password hash so it cannot leak through API responses.
type PublicUser struct {
	ID           string       `json:"id"`
	Username     string       `json:"username"`
	Role         Role         `json:"role"`
	AuthProvider AuthProvider `json:"authProvider"`
	Disabled     bool         `json:"disabled"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// Public returns the user as a PublicUser without the password hash.
func (u *User) Public() PublicUser {
	return PublicUser{
		ID:           u.ID,
		Username:     u.Username,
		Role:         u.Role,
		AuthProvider: u.AuthProvider,
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// NormaliseUsername trims whitespace and lower-cases the username. All
// storage and lookup operations use the normalised form so "Alice" and
// "alice" are the same account.
func NormaliseUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Validation errors returned by ValidateUsername / ValidatePassword.
var (
	ErrUsernameEmpty    = errors.New("auth: username is empty")
	ErrUsernameTooLong  = errors.New("auth: username is too long")
	ErrUsernameInvalid  = errors.New("auth: username contains invalid characters")
	ErrPasswordTooShort = fmt.Errorf("auth: password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("auth: password must be at most %d characters", MaxPasswordLength)
	ErrInvalidRole      = errors.New("auth: invalid role")
)

// ValidateUsername checks that the username (after normalisation) has an
// acceptable length and contains only printable, non-whitespace runes.
func ValidateUsername(raw string) (string, error) {
	u := NormaliseUsername(raw)
	if len(u) < MinUsernameLength {
		return "", ErrUsernameEmpty
	}
	if utf8.RuneCountInString(u) > MaxUsernameLength {
		return "", ErrUsernameTooLong
	}
	for _, r := range u {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return "", ErrUsernameInvalid
		}
	}
	return u, nil
}

// ValidatePassword enforces the minimum / maximum length policy on a
// plaintext password. Complexity rules are intentionally absent.
func ValidatePassword(p string) error {
	if utf8.RuneCountInString(p) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(p) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// NewID returns a 128-bit random identifier as a 32-character hex string.
// Suitable for user IDs and session IDs.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("auth: failed to generate id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
