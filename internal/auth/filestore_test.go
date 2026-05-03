// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// memStorage is a minimal storage.Storage implementation that keeps the
// users payload in memory. It is only used by the auth-package tests; it
// covers exactly the methods FileStore touches and panics on the rest so
// accidental coupling shows up immediately.
type memStorage struct {
	users []byte
}

func (m *memStorage) LoadFlows() ([]flow.Flow, error)             { panic("not used") }
func (m *memStorage) SaveFlows([]flow.Flow) error                 { panic("not used") }
func (m *memStorage) LoadWorkspace() (flow.Workspace, error)      { panic("not used") }
func (m *memStorage) SaveWorkspace(flow.Workspace) error          { panic("not used") }
func (m *memStorage) LoadCredentials() ([]byte, error)            { panic("not used") }
func (m *memStorage) SaveCredentials([]byte) error                { panic("not used") }
func (m *memStorage) LoadUsers() ([]byte, error)                  { return m.users, nil }
func (m *memStorage) SaveUsers(data []byte) error                 { m.users = data; return nil }

// makeUser is a test helper that builds a fully-populated User with a
// hashed password. Hashing is slow, so tests share users where possible.
func makeUser(t *testing.T, name string, role Role) User {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	username, err := ValidateUsername(name)
	if err != nil {
		t.Fatal(err)
	}
	return User{
		ID:           id,
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		AuthProvider: ProviderLocal,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	store, err := NewFileStore(&memStorage{})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestFileStoreCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	u := makeUser(t, "alice", RoleAdmin)
	if err := s.Create(u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(u.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Username != "alice" || got.Role != RoleAdmin {
		t.Errorf("got %+v, want alice/admin", got)
	}

	if _, err := s.Get("nonexistent"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("Get(missing): got %v, want ErrUserNotFound", err)
	}
}

func TestFileStoreGetByUsernameCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	u := makeUser(t, "alice", RoleAdmin)
	if err := s.Create(u); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetByUsername(ProviderLocal, "ALICE")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("wrong user returned for case-different lookup")
	}
}

func TestFileStoreUsernameTaken(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(makeUser(t, "alice", RoleAdmin)); err != nil {
		t.Fatal(err)
	}

	dup := makeUser(t, "Alice", RoleEditor) // different ID, normalised name collides
	if err := s.Create(dup); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("got %v, want ErrUsernameTaken", err)
	}
}

func TestFileStoreLastAdminInvariant(t *testing.T) {
	t.Run("disable last admin", func(t *testing.T) {
		s := newTestStore(t)
		admin := makeUser(t, "alice", RoleAdmin)
		if err := s.Create(admin); err != nil {
			t.Fatal(err)
		}

		updated := admin
		updated.Disabled = true
		if err := s.Update(updated); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("disabling last admin: got %v, want ErrLastAdmin", err)
		}
	})

	t.Run("downgrade last admin", func(t *testing.T) {
		s := newTestStore(t)
		admin := makeUser(t, "alice", RoleAdmin)
		if err := s.Create(admin); err != nil {
			t.Fatal(err)
		}

		updated := admin
		updated.Role = RoleEditor
		if err := s.Update(updated); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("downgrading last admin: got %v, want ErrLastAdmin", err)
		}
	})

	t.Run("disable admin when another exists", func(t *testing.T) {
		s := newTestStore(t)
		admin1 := makeUser(t, "alice", RoleAdmin)
		admin2 := makeUser(t, "bob", RoleAdmin)
		if err := s.Create(admin1); err != nil {
			t.Fatal(err)
		}
		if err := s.Create(admin2); err != nil {
			t.Fatal(err)
		}

		updated := admin1
		updated.Disabled = true
		if err := s.Update(updated); err != nil {
			t.Errorf("should be allowed when other admin exists: %v", err)
		}
	})

	t.Run("disabled admin does not count", func(t *testing.T) {
		s := newTestStore(t)
		active := makeUser(t, "alice", RoleAdmin)
		disabled := makeUser(t, "bob", RoleAdmin)
		disabled.Disabled = true
		if err := s.Create(active); err != nil {
			t.Fatal(err)
		}
		if err := s.Create(disabled); err != nil {
			t.Fatal(err)
		}

		updated := active
		updated.Role = RoleEditor
		if err := s.Update(updated); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("disabled admin should not satisfy invariant: got %v", err)
		}
	})
}

func TestFileStoreCannotChangeUsername(t *testing.T) {
	s := newTestStore(t)
	u := makeUser(t, "alice", RoleAdmin)
	if err := s.Create(u); err != nil {
		t.Fatal(err)
	}

	updated := u
	updated.Username = "alice2"
	if err := s.Update(updated); err == nil {
		t.Error("Update should reject username change, got nil")
	}
}

func TestFileStorePersistsAcrossReload(t *testing.T) {
	mem := &memStorage{}
	s1, err := NewFileStore(mem)
	if err != nil {
		t.Fatal(err)
	}
	u := makeUser(t, "alice", RoleAdmin)
	if err := s1.Create(u); err != nil {
		t.Fatal(err)
	}

	// Re-open against the same backing storage.
	s2, err := NewFileStore(mem)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.GetByUsername(ProviderLocal, "alice")
	if err != nil {
		t.Fatalf("GetByUsername after reload: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("reload returned different user")
	}
}

func TestFileStoreCountActiveAdmins(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(makeUser(t, "alice", RoleAdmin)); err != nil {
		t.Fatal(err)
	}
	editor := makeUser(t, "bob", RoleEditor)
	if err := s.Create(editor); err != nil {
		t.Fatal(err)
	}
	disabledAdmin := makeUser(t, "carol", RoleAdmin)
	disabledAdmin.Disabled = true
	if err := s.Create(disabledAdmin); err != nil {
		t.Fatal(err)
	}

	n, err := s.CountActiveAdmins()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("CountActiveAdmins = %d, want 1", n)
	}
}

func TestFileStoreList(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(makeUser(t, "alice", RoleAdmin)); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(makeUser(t, "bob", RoleEditor)); err != nil {
		t.Fatal(err)
	}

	users, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Errorf("List returned %d users, want 2", len(users))
	}
}
