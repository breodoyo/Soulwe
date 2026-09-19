//go:build integration

package anon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAnonSessionIntegration exercises the anonymous session flow against a
// real PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. It requires the
// 011_add_anon_session_identity migration to have been applied.
func TestAnonSessionIntegration(t *testing.T) {
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
	svc := NewService(repo)

	var anonymousID string

	t.Run("CreateSession persists an identity and returns its id", func(t *testing.T) {
		deviceUUID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%1_000_000_000_000)
		session, err := svc.CreateSession(ctx, deviceUUID)
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if session.Token == "" {
			t.Fatal("expected a non-empty raw token")
		}
		if len(session.AnonymousID) != 36 {
			t.Fatalf("expected a UUID anonymous id, got %q", session.AnonymousID)
		}
		anonymousID = session.AnonymousID

		// The raw token must never be persisted; only its SHA-256 hash.
		var storedHash string
		if err := pool.QueryRow(ctx,
			"SELECT token_hash FROM anon_identities WHERE id = $1", session.AnonymousID,
		).Scan(&storedHash); err != nil {
			t.Fatalf("failed to read stored token_hash: %v", err)
		}
		if storedHash != HashToken(session.Token) {
			t.Error("stored token_hash does not match the SHA-256 of the returned token")
		}
		if storedHash == session.Token {
			t.Error("the raw token was stored verbatim — only a hash should be stored")
		}

		// device_uuid saved on the identity; cleanup later.
		cleanup(t, pool, session.AnonymousID)
	})

	t.Run("repeated device call rotates the token but keeps the same identity", func(t *testing.T) {
		deviceUUID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%1_000_000_000_000)
		first, err := svc.CreateSession(ctx, deviceUUID)
		if err != nil {
			t.Fatalf("first CreateSession returned error: %v", err)
		}
		second, err := svc.CreateSession(ctx, deviceUUID)
		if err != nil {
			t.Fatalf("second CreateSession returned error: %v", err)
		}

		if second.AnonymousID != first.AnonymousID {
			t.Errorf("expected the same anonymous id %q, got %q", first.AnonymousID, second.AnonymousID)
		}
		if second.Token == first.Token {
			t.Error("expected the token to rotate on a repeated device call")
		}

		// The rotated id is now authenticated by the new token only.
		if id, ok, err := svc.Authenticate(ctx, first.Token); err != nil || ok {
			t.Errorf("old token must no longer authenticate: ok=%v err=%v", ok, err)
			_ = id
		}
		if id, ok, err := svc.Authenticate(ctx, second.Token); err != nil || !ok || id != first.AnonymousID {
			t.Errorf("new token must authenticate to the same identity: id=%q ok=%v err=%v", id, ok, err)
		}

		cleanup(t, pool, first.AnonymousID)
	})

	t.Run("Authenticate finds the identity and refreshes last_seen_at", func(t *testing.T) {
		session, err := svc.CreateSession(ctx, "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		defer cleanup(t, pool, session.AnonymousID)

		var before time.Time
		if err := pool.QueryRow(ctx,
			"SELECT last_seen_at FROM anon_identities WHERE id = $1", session.AnonymousID,
		).Scan(&before); err != nil {
			t.Fatalf("failed to read last_seen_at: %v", err)
		}

		time.Sleep(15 * time.Millisecond)
		id, ok, err := svc.Authenticate(ctx, session.Token)
		if err != nil || !ok {
			t.Fatalf("Authenticate failed: ok=%v err=%v", ok, err)
		}
		if id != session.AnonymousID {
			t.Errorf("expected identity id %q, got %q", session.AnonymousID, id)
		}

		var after time.Time
		if err := pool.QueryRow(ctx,
			"SELECT last_seen_at FROM anon_identities WHERE id = $1", session.AnonymousID,
		).Scan(&after); err != nil {
			t.Fatalf("failed to read updated last_seen_at: %v", err)
		}
		if !after.After(before) && !after.Equal(before) {
			t.Errorf("expected last_seen_at to refresh (before=%v after=%v)", before, after)
		}
	})

	t.Run("Authenticate with an unknown token returns ok=false, not a failure", func(t *testing.T) {
		raw, err := GenerateRawToken()
		if err != nil {
			t.Fatalf("GenerateRawToken returned error: %v", err)
		}
		id, ok, err := svc.Authenticate(ctx, raw)
		if err != nil {
			t.Fatalf("Authenticate returned error: %v", err)
		}
		if ok || id != "" {
			t.Errorf("expected ok=false with empty id, got ok=%v id=%q", ok, id)
		}
	})

	t.Run("FindByDeviceUUID on an unknown device returns ErrIdentityNotFound", func(t *testing.T) {
		_, err := repo.FindByDeviceUUID(ctx, "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, ErrIdentityNotFound) {
			t.Fatalf("expected ErrIdentityNotFound, got %v", err)
		}
	})

	// The deferred cleanup intentionally outlives anonymousID only when it was
	// set in the first subtest; later subtests clean up after themselves.
	if anonymousID != "" {
		cleanup(t, pool, anonymousID)
	}
}

// cleanup removes the anon_identities row created for the test. It is safe to
// defer at subtest scope: DELETE by id returns no rows for already-deleted ids.
func cleanup(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"DELETE FROM anon_identities WHERE id = $1", id,
	); err != nil {
		t.Errorf("cleanup failed to DELETE test identity: %v", err)
	}
}
