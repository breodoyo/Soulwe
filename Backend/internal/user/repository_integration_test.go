//go:build integration

package user

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"Backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration exercises the real repository against a
// running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set.
func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)

	// Unique email so the test can run repeatedly against the same database.
	email := fmt.Sprintf("repo-%d@soulwe.local", time.Now().UnixNano())
	const passwordHash = "$2a$12$abcdefghijklmnopqrstuv" // arbitrary bcrypt-shaped value

	t.Run("Create populates database-generated fields", func(t *testing.T) {
		u := &User{Email: email, PasswordHash: passwordHash, LanguagePref: "sw"}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if len(u.ID) != 36 {
			t.Errorf("expected a UUID id, got %q", u.ID)
		}
		if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
			t.Error("expected created_at and updated_at to be populated")
		}
		if u.LanguagePref != "sw" {
			t.Errorf("expected language_pref 'sw', got %q", u.LanguagePref)
		}
		if u.IsVerified {
			t.Error("expected is_verified to default to false")
		}
	})

	t.Run("FindByEmail returns the stored user", func(t *testing.T) {
		u, err := repo.FindByEmail(ctx, email)
		if err != nil {
			t.Fatalf("FindByEmail returned error: %v", err)
		}
		if u.Email != email {
			t.Errorf("email mismatch: got %q, want %q", u.Email, email)
		}
		if u.PasswordHash != passwordHash {
			t.Error("password_hash mismatch")
		}
	})

	t.Run("FindByID returns the stored user", func(t *testing.T) {
		created, err := repo.FindByEmail(ctx, email)
		if err != nil {
			t.Fatalf("FindByEmail returned error: %v", err)
		}
		u, err := repo.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID returned error: %v", err)
		}
		if u.Email != email {
			t.Errorf("email mismatch: got %q, want %q", u.Email, email)
		}
	})

	t.Run("Create with duplicate email returns ErrEmailTaken", func(t *testing.T) {
		err := repo.Create(ctx, &User{Email: email, PasswordHash: "another-hash"})
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("expected ErrEmailTaken, got %v", err)
		}
	})

	t.Run("looking up an unknown email returns ErrUserNotFound", func(t *testing.T) {
		_, err := repo.FindByEmail(ctx, "missing-"+email)
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("looking up an unknown id returns ErrUserNotFound", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("cleanup removes the test user", func(t *testing.T) {
		created, err := repo.FindByEmail(ctx, email)
		if err != nil {
			t.Fatalf("FindByEmail returned error: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", created.ID); err != nil {
			t.Fatalf("cleanup failed to DELETE test user: %v", err)
		}
	})
}

// TestServiceLoginIntegration exercises the full login flow against a real
// database: Register persists the user, Login verifies the credentials and
// returns a signed access token whose claims point at that user.
func TestServiceLoginIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	manager, err := auth.NewManager("integration-test-secret-not-for-production")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}
	svc := NewService(repo, manager)

	// Unique email so the test can run repeatedly against the same database.
	email := fmt.Sprintf("login-%d@soulwe.local", time.Now().UnixNano())
	const password = "integration-secret-password"
	if _, err := svc.Register(ctx, email, password); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	defer pool.Exec(ctx, "DELETE FROM users WHERE email = $1", email)

	t.Run("valid login returns the user and a signed access token", func(t *testing.T) {
		result, err := svc.Login(ctx, email, password)
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
		if result.User.Email != email {
			t.Errorf("email mismatch: got %q, want %q", result.User.Email, email)
		}
		if result.AccessToken == "" {
			t.Fatal("expected a non-empty access token")
		}

		claims := &jwt.RegisteredClaims{}
		parsed, err := jwt.ParseWithClaims(result.AccessToken, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte("integration-test-secret-not-for-production"), nil
		})
		if err != nil || !parsed.Valid {
			t.Fatalf("expected access token to verify: err=%v", err)
		}
		if claims.Subject != result.User.ID {
			t.Errorf("expected token subject %q, got %q", result.User.ID, claims.Subject)
		}
		if claims.Issuer != auth.Issuer {
			t.Errorf("expected issuer %q, got %q", auth.Issuer, claims.Issuer)
		}
		if diff := claims.ExpiresAt.Sub(claims.IssuedAt.Time); diff != auth.AccessTokenTTL {
			t.Errorf("expected token lifetime %v, got %v", auth.AccessTokenTTL, diff)
		}
	})

	t.Run("wrong password returns ErrBadCredentials", func(t *testing.T) {
		_, err := svc.Login(ctx, email, "not-the-password")
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})

	t.Run("unknown email returns ErrBadCredentials, not enumeration", func(t *testing.T) {
		_, err := svc.Login(ctx, "missing-"+email, password)
		if !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("expected ErrBadCredentials, got %v", err)
		}
	})
}
