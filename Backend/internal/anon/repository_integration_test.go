//go:build integration

package anon

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

		cleanup(t, pool, session.AnonymousID)
	})

	t.Run("a repeated device call creates a separate identity and never reissues the first", func(t *testing.T) {
		deviceUUID := fmt.Sprintf("550e8400-e29b-41d4-a716-%012d", time.Now().UnixNano()%1_000_000_000_000)
		first, err := svc.CreateSession(ctx, deviceUUID)
		if err != nil {
			t.Fatalf("first CreateSession returned error: %v", err)
		}
		defer cleanup(t, pool, first.AnonymousID)

		second, err := svc.CreateSession(ctx, deviceUUID)
		if err != nil {
			t.Fatalf("second CreateSession returned error: %v", err)
		}
		defer cleanup(t, pool, second.AnonymousID)

		if second.AnonymousID == first.AnonymousID {
			t.Errorf("a known device UUID must not reissue identity %q", first.AnonymousID)
		}

		// The first session must keep working; the second call never touched it.
		if id, ok, err := svc.Authenticate(ctx, first.Token); err != nil || !ok || id != first.AnonymousID {
			t.Errorf("first token must keep authenticating its identity: id=%q ok=%v err=%v", id, ok, err)
		}
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

	// Only needed when the first subtest set anonymousID; later ones clean up after themselves.
	if anonymousID != "" {
		cleanup(t, pool, anonymousID)
	}
}

// cleanup is safe to defer at subtest scope: DELETE by id is a no-op once gone.
func cleanup(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"DELETE FROM anon_identities WHERE id = $1", id,
	); err != nil {
		t.Errorf("cleanup failed to DELETE test identity: %v", err)
	}
}
