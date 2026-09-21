package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"Backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
)

// fakeRepository is an in-memory Repository used to unit-test the service
// without a real PostgreSQL connection. identities maps an anonymous identity
// ID to its linked user ID (nil means the identity is not yet promoted).
type fakeRepository struct {
	users      map[string]*User
	byID       map[string]*User
	identities map[string]*string
	promoteErr error // injectable failure for the Promote path
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		users:      map[string]*User{},
		byID:       map[string]*User{},
		identities: map[string]*string{},
	}
}

// seedIdentity registers an unlinked anonymous identity so Promote can find it.
func seedIdentity(repo *fakeRepository, identityID string) {
	repo.identities[identityID] = nil
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

func (f *fakeRepository) Promote(_ context.Context, identityID, email, passwordHash string, displayName *string, languagePref string) (*User, error) {
	if f.promoteErr != nil {
		return nil, f.promoteErr
	}
	linked, ok := f.identities[identityID]
	if !ok || linked != nil {
		return nil, ErrIdentityAlreadyPromoted
	}
	if _, exists := f.users[email]; exists {
		return nil, ErrEmailTaken
	}
	nextID := len(f.byID) + 1
	u := &User{
		ID:           fmt.Sprintf("00000000-0000-0000-0000-%012d", nextID),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		LanguagePref: languagePref,
		IsVerified:   false,
	}
	f.users[email] = u
	f.byID[u.ID] = u
	userID := u.ID
	f.identities[identityID] = &userID
	return u, nil
}

// failingTokenManager always fails to sign, for the JWT-signing error path.
type failingTokenManager struct{}

func (failingTokenManager) SignAccessToken(string) (string, error) {
	return "", errors.New("signing service unavailable")
}

// raiseIdentity is a tiny helper to set an identity's linked user ID directly.
func raiseIdentity(repo *fakeRepository, identityID, userID string) {
	repo.identities[identityID] = &userID
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

func TestServicePromote(t *testing.T) {
	t.Run("successful promotion returns the user and a signed access token", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "22222222-2222-2222-2222-222222222222")
		svc := NewService(repo, newTestTokenManager())

		displayName := "Bree"
		result, err := svc.Promote(context.Background(),
			"22222222-2222-2222-2222-222222222222", " Bree@Example.com ", "a-strong-password", &displayName)
		if err != nil {
			t.Fatalf("Promote returned error: %v", err)
		}
		if result.User.Email != "bree@example.com" {
			t.Errorf("email should be normalized, got %q", result.User.Email)
		}
		if result.User.PasswordHash == "" || result.User.PasswordHash == "a-strong-password" {
			t.Error("expected the bcrypt hash to be stored, never the plaintext")
		}
		if result.User.ID == "" {
			t.Error("expected the repository to populate the user ID")
		}
		if result.User.DisplayName == nil || *result.User.DisplayName != "Bree" {
			t.Errorf("expected display_name to be persisted, got %v", result.User.DisplayName)
		}
		if result.AccessToken == "" {
			t.Fatal("expected a non-empty access token")
		}
		if sub := parseSubject(t, result.AccessToken); sub != result.User.ID {
			t.Errorf("expected token subject %q, got %q", result.User.ID, sub)
		}
		// The anonymous identity must now be linked to the new user.
		if linked := repo.identities["22222222-2222-2222-2222-222222222222"]; linked == nil || *linked != result.User.ID {
			t.Errorf("expected the anonymous identity to be linked to the new user")
		}
	})

	t.Run("normalizes a whitespace-only display name to null", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "ident-1")
		svc := NewService(repo, newTestTokenManager())

		blank := "   "
		result, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "a-strong-password", &blank)
		if err != nil {
			t.Fatalf("Promote returned error: %v", err)
		}
		if result.User.DisplayName != nil {
			t.Errorf("expected a blank display name to be stored as null, got %q", *result.User.DisplayName)
		}
	})

	t.Run("already promoted identity returns ErrIdentityAlreadyPromoted", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "ident-1")
		raiseIdentity(repo, "ident-1", "some-user-id")
		svc := NewService(repo, newTestTokenManager())

		_, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "a-strong-password", nil)
		if !errors.Is(err, ErrIdentityAlreadyPromoted) {
			t.Fatalf("expected ErrIdentityAlreadyPromoted, got %v", err)
		}
		if len(repo.users) != 0 {
			t.Errorf("no user must be created for an already-promoted identity, got %d users", len(repo.users))
		}
	})

	t.Run("duplicate email returns ErrEmailTaken and does not link the identity", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "ident-1")
		svc := NewService(repo, newTestTokenManager())

		if _, err := svc.Register(context.Background(), "bree@example.com", "a-strong-password"); err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
		_, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "another-strong-pw", nil)
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("expected ErrEmailTaken, got %v", err)
		}
		if repo.identities["ident-1"] != nil {
			t.Error("the anonymous identity must stay unlinked when the email is taken")
		}
	})

	t.Run("invalid email returns ErrInvalidEmail", func(t *testing.T) {
		svc := NewService(newFakeRepository(), newTestTokenManager())
		_, err := svc.Promote(context.Background(), "ident-1", "not-an-email", "a-strong-password", nil)
		if !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("expected ErrInvalidEmail, got %v", err)
		}
	})

	t.Run("invalid password returns ErrInvalidPassword", func(t *testing.T) {
		svc := NewService(newFakeRepository(), newTestTokenManager())
		_, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "short", nil)
		if !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("expected ErrInvalidPassword, got %v", err)
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "ident-1")
		repo.promoteErr = errors.New("connection lost mid-transaction")
		svc := NewService(repo, newTestTokenManager())

		_, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "a-strong-password", nil)
		if err == nil || errors.Is(err, ErrEmailTaken) || errors.Is(err, ErrIdentityAlreadyPromoted) {
			t.Fatalf("expected a wrapped repository error, got %v", err)
		}
	})

	t.Run("JWT signing failure is wrapped", func(t *testing.T) {
		repo := newFakeRepository()
		seedIdentity(repo, "ident-1")
		svc := NewService(repo, failingTokenManager{})

		_, err := svc.Promote(context.Background(), "ident-1", "bree@example.com", "a-strong-password", nil)
		if err == nil || errors.Is(err, ErrEmailTaken) || errors.Is(err, ErrIdentityAlreadyPromoted) {
			t.Fatalf("expected a wrapped signing error, got %v", err)
		}
	})
}
