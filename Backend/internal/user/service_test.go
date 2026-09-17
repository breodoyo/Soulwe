package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
)

// fakeRepository is an in-memory Repository used to unit-test the service
// without a real PostgreSQL connection.
type fakeRepository struct {
	users map[string]*User
	byID  map[string]*User
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		users: map[string]*User{},
		byID:  map[string]*User{},
	}
}

func (f *fakeRepository) Create(_ context.Context, u *User) error {
	if _, exists := f.users[u.Email]; exists {
		return ErrEmailTaken
	}
	u.ID = "00000000-0000-0000-0000-000000000001"
	f.users[u.Email] = u
	f.byID[u.ID] = u
	return nil
}

func (f *fakeRepository) FindByEmail(_ context.Context, email string) (*User, error) {
	u, ok := f.users[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (*User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// newTestTokenManager returns a real JWT manager bound to a fixed test-only
// secret, so the service tests exercise actual token signing.
func newTestTokenManager() *auth.Manager {
	m, err := auth.NewManager("unit-test-secret-that-is-not-shared-anywhere")
	if err != nil {
		panic(err)
	}
	return m
}

func TestServiceRegisterCreatesUser(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())

	u, err := svc.Register(context.Background(), "  Bree@Example.com ", "a-strong-password")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if u.Email != "bree@example.com" {
		t.Errorf("email should be normalized and lowercased, got %q", u.Email)
	}
	if u.PasswordHash == "" {
		t.Error("expected a password hash to be stored")
	}
	if u.PasswordHash == "a-strong-password" {
		t.Error("stored password must never be the plaintext")
	}
	if u.ID == "" {
		t.Error("expected the repository to populate the ID")
	}
	if !checkPassword(u.PasswordHash, "a-strong-password") {
		t.Error("stored hash should verify against the original password")
	}
}

func TestServiceRegisterDuplicateEmail(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())

	if _, err := svc.Register(context.Background(), "bree@example.com", "a-strong-password"); err != nil {
		t.Fatalf("first Register returned error: %v", err)
	}
	_, err := svc.Register(context.Background(), "BREE@example.com ", "another-strong-pw")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestServiceRegisterRejectsInvalidEmail(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())

	cases := []string{
		"",
		"not-an-email",
		"@example.com",
		"user @example.com",
		strings.Repeat("a", 320) + "@example.com", // exceeds the 320-char cap
	}
	for _, email := range cases {
		_, err := svc.Register(context.Background(), email, "a-strong-password")
		if !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("email %q: expected ErrInvalidEmail, got %v", email, err)
		}
	}
}

func TestServiceRegisterRejectsInvalidPassword(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())

	cases := []string{
		"",            // empty
		"short",       // 5 characters, below the 12-character minimum
		"elevenchars", // exactly 11 characters
	}
	for _, password := range cases {
		_, err := svc.Register(context.Background(), "bree@example.com", password)
		if !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("password %q: expected ErrInvalidPassword, got %v", password, err)
		}
	}

	// > 72 bytes exceeds bcrypt's input limit and must be rejected, not truncated.
	tooLong := strings.Repeat("x", 73)
	_, err := svc.Register(context.Background(), "bree@example.com", tooLong)
	if !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("73-byte password: expected ErrInvalidPassword, got %v", err)
	}
}

func TestServiceVerifyPassword(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())
	_, err := svc.Register(context.Background(), "bree@example.com", "a-strong-password")
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	t.Run("correct credentials return the user", func(t *testing.T) {
		u, err := svc.VerifyPassword(context.Background(), "bree@example.com", "a-strong-password")
		if err != nil {
			t.Fatalf("VerifyPassword returned error: %v", err)
		}
		if u.Email != "bree@example.com" {
			t.Errorf("expected user email, got %q", u.Email)
		}
	})

	t.Run("wrong password returns ErrBadCredentials", func(t *testing.T) {
		_, err := svc.VerifyPassword(context.Background(), "bree@example.com", "not-the-password")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})

	t.Run("unknown email returns ErrBadCredentials, not enumeration", func(t *testing.T) {
		_, err := svc.VerifyPassword(context.Background(), "nobody@example.com", "a-strong-password")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})
}

func TestServiceFindByEmail(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())
	if _, err := svc.Register(context.Background(), "bree@example.com", "a-strong-password"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	u, err := svc.FindByEmail(context.Background(), " BREE@example.COM ")
	if err != nil {
		t.Fatalf("FindByEmail returned error: %v", err)
	}
	if u.Email != "bree@example.com" {
		t.Errorf("expected normalized email, got %q", u.Email)
	}

	_, err = svc.FindByEmail(context.Background(), "missing@example.com")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func parseSubject(t *testing.T, tokenString string) string {
	t.Helper()
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte("unit-test-secret-that-is-not-shared-anywhere"), nil
	})
	if err != nil {
		t.Fatalf("failed to parse access token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("expected a valid access token")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Fatal("expected issued-at and expires-at claims")
	}
	if diff := claims.ExpiresAt.Sub(claims.IssuedAt.Time); diff != auth.AccessTokenTTL {
		t.Errorf("expected token lifetime %v, got %v", auth.AccessTokenTTL, diff)
	}
	if claims.Issuer != auth.Issuer {
		t.Errorf("expected issuer %q, got %q", auth.Issuer, claims.Issuer)
	}
	return claims.Subject
}

func TestServiceLogin(t *testing.T) {
	svc := NewService(newFakeRepository(), newTestTokenManager())
	if _, err := svc.Register(context.Background(), "bree@example.com", "a-strong-password"); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	t.Run("valid credentials return the user and an access token", func(t *testing.T) {
		result, err := svc.Login(context.Background(), " BREE@example.com ", "a-strong-password")
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
		if result.User.Email != "bree@example.com" {
			t.Errorf("expected normalized email, got %q", result.User.Email)
		}
		if result.AccessToken == "" {
			t.Fatal("expected a non-empty access token")
		}
		if sub := parseSubject(t, result.AccessToken); sub != result.User.ID {
			t.Errorf("expected token subject %q, got %q", result.User.ID, sub)
		}
	})

	t.Run("wrong password returns ErrBadCredentials", func(t *testing.T) {
		_, err := svc.Login(context.Background(), "bree@example.com", "not-the-password")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})

	t.Run("unknown email returns ErrBadCredentials, not enumeration", func(t *testing.T) {
		_, err := svc.Login(context.Background(), "nobody@example.com", "a-strong-password")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})
}
