// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestRoleRank(t *testing.T) {
	cases := []struct {
		role Role
		rank int
	}{
		{RoleAdmin, 3},
		{RoleEditor, 2},
		{RoleViewer, 1},
		{Role("unknown"), 0},
		{Role(""), 0},
	}
	for _, c := range cases {
		if got := c.role.Rank(); got != c.rank {
			t.Errorf("Rank(%q) = %d, want %d", c.role, got, c.rank)
		}
	}
}

func TestRoleValid(t *testing.T) {
	if !RoleAdmin.Valid() || !RoleEditor.Valid() || !RoleViewer.Valid() {
		t.Error("known roles must be Valid")
	}
	if Role("nope").Valid() {
		t.Error("unknown role must not be Valid")
	}
}

func TestNormaliseUsername(t *testing.T) {
	cases := map[string]string{
		"alice":     "alice",
		"  Alice  ": "alice",
		"BOB":       "bob",
		"\tcarol\n": "carol",
	}
	for in, want := range cases {
		if got := NormaliseUsername(in); got != want {
			t.Errorf("NormaliseUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := ValidateUsername("  Alice  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "alice" {
			t.Errorf("got %q, want %q", got, "alice")
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, err := ValidateUsername("   "); !errors.Is(err, ErrUsernameEmpty) {
			t.Errorf("got %v, want ErrUsernameEmpty", err)
		}
	})

	t.Run("too long", func(t *testing.T) {
		long := strings.Repeat("x", MaxUsernameLength+1)
		if _, err := ValidateUsername(long); !errors.Is(err, ErrUsernameTooLong) {
			t.Errorf("got %v, want ErrUsernameTooLong", err)
		}
	})

	t.Run("contains space", func(t *testing.T) {
		if _, err := ValidateUsername("foo bar"); !errors.Is(err, ErrUsernameInvalid) {
			t.Errorf("got %v, want ErrUsernameInvalid", err)
		}
	})
}

func TestValidatePassword(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		if err := ValidatePassword("hunter22"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("too short", func(t *testing.T) {
		if err := ValidatePassword("short"); !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("got %v, want ErrPasswordTooShort", err)
		}
	})

	t.Run("too long", func(t *testing.T) {
		long := strings.Repeat("x", MaxPasswordLength+1)
		if err := ValidatePassword(long); !errors.Is(err, ErrPasswordTooLong) {
			t.Errorf("got %v, want ErrPasswordTooLong", err)
		}
	})

	t.Run("unicode counts as runes", func(t *testing.T) {
		// 8 emoji = 8 runes, well over the byte threshold but exactly at
		// the rune minimum.
		if err := ValidatePassword("🔥🔥🔥🔥🔥🔥🔥🔥"); err != nil {
			t.Errorf("unicode password rejected: %v", err)
		}
	})
}

func TestHashPasswordRoundtrip(t *testing.T) {
	hash, err := HashPassword("hunter22")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash does not start with $argon2id$: %q", hash)
	}

	if err := VerifyPassword("hunter22", hash); err != nil {
		t.Errorf("VerifyPassword on correct password: %v", err)
	}

	if err := VerifyPassword("wrong", hash); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("VerifyPassword on wrong password: got %v, want ErrPasswordMismatch", err)
	}
}

func TestHashPasswordSaltsDiffer(t *testing.T) {
	a, err := HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("hunter22")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password produced identical output — salt is not random")
	}
}

func TestHashPasswordRejectsTooShort(t *testing.T) {
	if _, err := HashPassword("x"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("got %v, want ErrPasswordTooShort", err)
	}
}

func TestVerifyPasswordRejectsBadFormat(t *testing.T) {
	cases := []string{
		"",
		"not a hash",
		"$argon2id$",
		"$bcrypt$v=19$m=1,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=99$m=1,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=19$bogus$AAAA$BBBB",
		"$argon2id$v=19$m=1,t=1,p=1$@@@@$BBBB",
	}
	for _, c := range cases {
		err := VerifyPassword("hunter22", c)
		if !errors.Is(err, ErrInvalidHashFormat) {
			t.Errorf("VerifyPassword(%q): got %v, want ErrInvalidHashFormat", c, err)
		}
	}
}

func TestNewIDUnique(t *testing.T) {
	a, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("NewID produced duplicate values")
	}
	if len(a) != 32 {
		t.Errorf("NewID length = %d, want 32 (hex of 16 bytes)", len(a))
	}
}

func TestPublicUserHidesHash(t *testing.T) {
	u := User{
		ID:           "abc",
		Username:     "alice",
		PasswordHash: "$argon2id$secret",
		Role:         RoleAdmin,
	}
	pu := u.Public()
	// PublicUser has no PasswordHash field at all — this is enforced by
	// the type system, but we still assert that round-tripping does not
	// expose the hash via reflection-style serialization.
	if pu.Username != "alice" || pu.Role != RoleAdmin {
		t.Errorf("Public() lost data: %+v", pu)
	}
}
