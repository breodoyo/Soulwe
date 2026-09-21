//go:build integration

package user

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"Backend/internal/anon"
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

// TestPromoteIntegration exercises the anonymous -> registered promotion
// against a real PostgreSQL. It starts from a token-bound anonymous identity
// (no user_id) and verifies the promotion is atomic, idempotence once linked,
// safe under duplicate emails, and race-free under concurrent attempts.
func TestPromoteIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	identities := anon.NewPostgresRepository(pool)

	const passwordHash = "$2a$12$abcdefghijklmnopqrstuv" // arbitrary bcrypt-shaped value

	email := fmt.Sprintf("promote-%d@soulwe.local", time.Now().UnixNano())

	// A fresh token-bound anonymous identity, unlinked to any user.
	identity := &anon.AnonIdentity{
		AnonName:  fmt.Sprintf("Anon Promote %d", time.Now().UnixNano()%1_000_000_000_000),
		TokenHash: anon.HashToken("promote-integration-token"),
	}
	if err := identities.Create(ctx, identity); err != nil {
		t.Fatalf("failed to create anonymous identity: %v", err)
	}

	originalCreatedAt := identity.CreatedAt

	// Track every row created so cleanup is deterministic even when a subtest
	// fails early. Deleting the promoted user cascades its linked identity.
	var createdEmails []string
	var createdIdentities []string
	createdEmails = append(createdEmails, email)
	createdIdentities = append(createdIdentities, identity.ID)
	defer func() {
		for _, id := range createdIdentities {
			if _, err := pool.Exec(context.Background(),
				"DELETE FROM anon_identities WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE test identity %s: %v", id, err)
			}
		}
		for _, e := range createdEmails {
			if _, err := pool.Exec(context.Background(),
				"DELETE FROM users WHERE email = $1", e); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", e, err)
			}
		}
	}()

	var promotedUserID string

	t.Run("Promote creates the user, links the identity, and preserves identity data", func(t *testing.T) {
		u, err := repo.Promote(ctx, identity.ID, email, passwordHash, nil, "en")
		if err != nil {
			t.Fatalf("Promote returned error: %v", err)
		}
		promotedUserID = u.ID
		if len(u.ID) != 36 {
			t.Errorf("expected a UUID user id, got %q", u.ID)
		}
		if u.Email != email {
			t.Errorf("email mismatch: got %q, want %q", u.Email, email)
		}
		if u.PasswordHash != passwordHash {
			t.Error("password_hash mismatch")
		}
		if u.IsVerified {
			t.Error("expected is_verified to default to false")
		}

		// The anonymous identity must still exist, still carry its own data,
		// and now point at the promoted user (user_id populated).
		var linkedUserID *string
		var storedCreatedAt time.Time
		var storedName string
		if err := pool.QueryRow(ctx,
			"SELECT user_id, created_at, anon_name FROM anon_identities WHERE id = $1",
			identity.ID,
		).Scan(&linkedUserID, &storedCreatedAt, &storedName); err != nil {
			t.Fatalf("failed to read the promoted identity: %v", err)
		}
		if linkedUserID == nil || *linkedUserID != promotedUserID {
			t.Errorf("expected user_id %q on the identity, got %v", promotedUserID, linkedUserID)
		}
		if storedName != identity.AnonName {
			t.Errorf("anon_name must survive promotion: got %q, want %q", storedName, identity.AnonName)
		}
		if !storedCreatedAt.Equal(originalCreatedAt) {
			t.Errorf("created_at must survive promotion: got %v, want %v", storedCreatedAt, originalCreatedAt)
		}
	})

	t.Run("an already promoted identity returns ErrIdentityAlreadyPromoted", func(t *testing.T) {
		otherEmail := fmt.Sprintf("promote-already-%d@soulwe.local", time.Now().UnixNano())
		createdEmails = append(createdEmails, otherEmail)

		_, err := repo.Promote(ctx, identity.ID, otherEmail, passwordHash, nil, "en")
		if !errors.Is(err, ErrIdentityAlreadyPromoted) {
			t.Fatalf("expected ErrIdentityAlreadyPromoted, got %v", err)
		}

		// No second user may be created for the rejected attempt.
		var count int
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM users WHERE email = $1", otherEmail).Scan(&count); err != nil {
			t.Fatalf("failed to count users: %v", err)
		}
		if count != 0 {
			t.Errorf("a user was created for an already-promoted identity: %d row(s)", count)
		}
	})

	t.Run("duplicate email returns ErrEmailTaken and leaves the identity unlinked", func(t *testing.T) {
		second := &anon.AnonIdentity{
			AnonName:  fmt.Sprintf("Anon Dup %d", time.Now().UnixNano()%1_000_000_000_000),
			TokenHash: anon.HashToken("promote-duplicate-email-token"),
		}
		if err := identities.Create(ctx, second); err != nil {
			t.Fatalf("failed to create second identity: %v", err)
		}
		createdIdentities = append(createdIdentities, second.ID)

		_, err := repo.Promote(ctx, second.ID, email, passwordHash, nil, "en")
		if !errors.Is(err, ErrEmailTaken) {
			t.Fatalf("expected ErrEmailTaken, got %v", err)
		}

		// The loser identity must remain unlinked (no partial state).
		var linkedUserID *string
		if err := pool.QueryRow(ctx,
			"SELECT user_id FROM anon_identities WHERE id = $1", second.ID).Scan(&linkedUserID); err != nil {
			t.Fatalf("failed to read identity: %v", err)
		}
		if linkedUserID != nil {
			t.Errorf("identity must stay unlinked after a duplicate-email rejection, got user_id %q", *linkedUserID)
		}
	})

	t.Run("concurrent promotions of the same identity have exactly one winner", func(t *testing.T) {
		raceIdentity := &anon.AnonIdentity{
			AnonName:  fmt.Sprintf("Anon Race %d", time.Now().UnixNano()%1_000_000_000_000),
			TokenHash: anon.HashToken("promote-race-token"),
		}
		if err := identities.Create(ctx, raceIdentity); err != nil {
			t.Fatalf("failed to create race identity: %v", err)
		}
		createdIdentities = append(createdIdentities, raceIdentity.ID)

		const workers = 4
		emails := make([]string, workers)
		results := make([]error, workers)
		for i := 0; i < workers; i++ {
			emails[i] = fmt.Sprintf("promote-race-%d-%d@soulwe.local", i, time.Now().UnixNano())
			createdEmails = append(createdEmails, emails[i])
		}

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, results[i] = repo.Promote(ctx, raceIdentity.ID, emails[i], passwordHash, nil, "en")
			}()
		}
		wg.Wait()

		var winners int
		for i, err := range results {
			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrIdentityAlreadyPromoted):
			default:
				t.Errorf("worker %d: unexpected error: %v", i, err)
			}
		}
		if winners != 1 {
			t.Errorf("expected exactly one winning promote, got %d", winners)
		}

		// Exactly one user may exist across all the race emails.
		var created int
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM users WHERE email = ANY($1)", emails).Scan(&created); err != nil {
			t.Fatalf("failed to count race users: %v", err)
		}
		if created != 1 {
			t.Errorf("expected exactly 1 user created under contention, got %d", created)
		}

		// The identity is linked to exactly the winning user.
		var linkedUserID *string
		if err := pool.QueryRow(ctx,
			"SELECT user_id FROM anon_identities WHERE id = $1", raceIdentity.ID).Scan(&linkedUserID); err != nil {
			t.Fatalf("failed to read race identity link: %v", err)
		}
		if linkedUserID == nil {
			t.Error("expected the race identity to end up linked to a user")
		}
	})
}
