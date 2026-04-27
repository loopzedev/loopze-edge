// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

package auth

import "errors"

// UserStore-level errors. These are returned across all UserStore
// implementations and are safe to compare with errors.Is.
var (
	// ErrUserNotFound is returned when a Get / GetByUsername lookup
	// finds no matching user.
	ErrUserNotFound = errors.New("auth: user not found")

	// ErrUsernameTaken is returned by Create when a user with the same
	// (provider, normalised-username) already exists.
	ErrUsernameTaken = errors.New("auth: username already taken")

	// ErrLastAdmin is returned by Update when the caller would deactivate
	// the last active admin or downgrade them out of the admin role.
	// Flint refuses this to prevent locking everyone out of user
	// management.
	ErrLastAdmin = errors.New("auth: refuses to remove the last active admin")
)

// UserStore is the persistence interface for user records. The HTTP layer
// and the SessionManager talk only to this interface, never directly to
// the file or NATS backend, so the storage implementation can change
// without touching the rest of the auth package.
type UserStore interface {
	// Get returns the user with the given ID or ErrUserNotFound.
	Get(id string) (*User, error)

	// GetByUsername returns the user with the given (provider, username)
	// pair. The username is normalised with NormaliseUsername before
	// lookup so the call is case-insensitive.
	GetByUsername(provider AuthProvider, username string) (*User, error)

	// List returns all users. The order is not guaranteed.
	List() ([]User, error)

	// Create persists a new user. The user must have ID, Username, Role,
	// AuthProvider, and (for local users) PasswordHash already populated.
	// Returns ErrUsernameTaken if a user with the same (provider,
	// username) already exists.
	Create(u User) error

	// Update replaces an existing user record (matched by ID). It
	// enforces the last-admin invariant: the caller cannot disable or
	// downgrade the only remaining active admin (ErrLastAdmin).
	Update(u User) error

	// CountActiveAdmins returns the number of users with role=admin and
	// disabled=false. Used to enforce the last-admin invariant from
	// outside the store (e.g. before deletion or password reset that
	// invalidates a session).
	CountActiveAdmins() (int, error)
}
