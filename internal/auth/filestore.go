// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/loopzedev/loopze-edge/internal/storage"
)

// usersFile is the on-disk JSON envelope for user records. It exists so we
// can add metadata (schema version, etc.) later without breaking existing
// installations.
type usersFile struct {
	Users []User `json:"users"`
}

// FileStore is a UserStore backed by a storage.Storage implementation. It
// keeps an in-memory copy of all users for fast lookups and rewrites the
// entire users file on every mutation. Suitable for the small user counts
// (tens, maybe low hundreds) we expect in a LOOPZE deployment.
type FileStore struct {
	storage storage.Storage

	mu    sync.RWMutex
	byID  map[string]User
	index map[indexKey]string // (provider, normalised-username) → ID
}

// indexKey is the secondary lookup key for GetByUsername.
type indexKey struct {
	provider AuthProvider
	username string
}

// NewFileStore loads existing users from storage and returns a ready-to-use
// FileStore. If no users file exists yet, an empty store is returned.
func NewFileStore(s storage.Storage) (*FileStore, error) {
	fs := &FileStore{
		storage: s,
		byID:    make(map[string]User),
		index:   make(map[indexKey]string),
	}

	data, err := s.LoadUsers()
	if err != nil {
		return nil, fmt.Errorf("auth: failed to load users: %w", err)
	}
	if len(data) == 0 {
		return fs, nil
	}

	var f usersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("auth: failed to parse users file: %w", err)
	}

	for _, u := range f.Users {
		fs.byID[u.ID] = u
		fs.index[indexKey{u.AuthProvider, u.Username}] = u.ID
	}
	return fs, nil
}

// Get returns the user with the given ID.
func (fs *FileStore) Get(id string) (*User, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	u, ok := fs.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	// Return a copy so callers cannot mutate the stored record.
	return &u, nil
}

// GetByUsername looks up a user by the (provider, normalised-username) pair.
func (fs *FileStore) GetByUsername(provider AuthProvider, username string) (*User, error) {
	normalised := NormaliseUsername(username)

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	id, ok := fs.index[indexKey{provider, normalised}]
	if !ok {
		return nil, ErrUserNotFound
	}
	u := fs.byID[id]
	return &u, nil
}

// List returns all stored users in arbitrary order.
func (fs *FileStore) List() ([]User, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	out := make([]User, 0, len(fs.byID))
	for _, u := range fs.byID {
		out = append(out, u)
	}
	return out, nil
}

// Create persists a new user. Returns ErrUsernameTaken if a user with the
// same (provider, normalised-username) already exists.
func (fs *FileStore) Create(u User) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	key := indexKey{u.AuthProvider, u.Username}
	if _, exists := fs.index[key]; exists {
		return ErrUsernameTaken
	}
	if _, exists := fs.byID[u.ID]; exists {
		// ID collision is highly unlikely with 128-bit IDs but treat
		// defensively as a programmer error.
		return fmt.Errorf("auth: user id %q already exists", u.ID)
	}

	// Build the new full state, persist, then commit to memory. Order
	// matters: if SaveUsers fails we must not have changed the in-memory
	// view.
	next := append(fs.snapshotLocked(), u)
	if err := fs.persistLocked(next); err != nil {
		return err
	}

	fs.byID[u.ID] = u
	fs.index[key] = u.ID
	return nil
}

// Update replaces the user record matched by ID. It enforces the last-admin
// invariant: the only remaining active admin cannot be disabled or
// downgraded out of the admin role.
//
// The (provider, username) pair is treated as immutable — Update returns an
// error if the caller tries to change it. Renaming is intentionally not
// supported in V1.
func (fs *FileStore) Update(u User) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	old, ok := fs.byID[u.ID]
	if !ok {
		return ErrUserNotFound
	}
	if old.AuthProvider != u.AuthProvider || old.Username != u.Username {
		return fmt.Errorf("auth: cannot change provider or username on update")
	}

	wasActiveAdmin := old.Role == RoleAdmin && !old.Disabled
	becomesActiveAdmin := u.Role == RoleAdmin && !u.Disabled
	if wasActiveAdmin && !becomesActiveAdmin {
		others := 0
		for id, other := range fs.byID {
			if id == u.ID {
				continue
			}
			if other.Role == RoleAdmin && !other.Disabled {
				others++
			}
		}
		if others == 0 {
			return ErrLastAdmin
		}
	}

	// Build snapshot with the updated record.
	next := make([]User, 0, len(fs.byID))
	for id, existing := range fs.byID {
		if id == u.ID {
			next = append(next, u)
		} else {
			next = append(next, existing)
		}
	}
	if err := fs.persistLocked(next); err != nil {
		return err
	}

	fs.byID[u.ID] = u
	return nil
}

// CountActiveAdmins returns the number of users with role=admin and
// disabled=false.
func (fs *FileStore) CountActiveAdmins() (int, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	n := 0
	for _, u := range fs.byID {
		if u.Role == RoleAdmin && !u.Disabled {
			n++
		}
	}
	return n, nil
}

// snapshotLocked returns a slice of all current users. Caller must hold
// at least an RLock.
func (fs *FileStore) snapshotLocked() []User {
	out := make([]User, 0, len(fs.byID))
	for _, u := range fs.byID {
		out = append(out, u)
	}
	return out
}

// persistLocked marshals the given users slice and writes it through the
// storage backend. Caller must hold the write lock.
func (fs *FileStore) persistLocked(users []User) error {
	data, err := json.MarshalIndent(usersFile{Users: users}, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: failed to marshal users: %w", err)
	}
	if err := fs.storage.SaveUsers(data); err != nil {
		return fmt.Errorf("auth: failed to persist users: %w", err)
	}
	return nil
}
