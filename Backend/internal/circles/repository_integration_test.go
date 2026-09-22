//go:build integration

package circles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration exercises the circles repository against a
// running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. A throwaway
// circle and two anonymous identities are seeded directly so member counts,
// message authorship, and ownership gating can be verified.
func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepository(pool)
	nonce := time.Now().UnixNano()

	// Seed one circle and two anonymous identities.
	var circleID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO circles (slug, name, description, icon)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		fmt.Sprintf("itest-%d", nonce), "Integration circle", "created by tests", "🌿",
	).Scan(&circleID); err != nil {
		t.Fatalf("failed to seed circle: %v", err)
	}
	seedIdentity := func(name string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO anon_identities (anon_name, token_hash)
			 VALUES ($1, $2) RETURNING id`,
			name, fmt.Sprintf("tok-%d-%s", nonce, name),
		).Scan(&id); err != nil {
			t.Fatalf("failed to seed anon identity: %v", err)
		}
		return id
	}
	identityA := seedIdentity(fmt.Sprintf("Anon A-%d", nonce))
	identityB := seedIdentity(fmt.Sprintf("Anon B-%d", nonce))

	defer func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM circles WHERE id = $1", circleID); err != nil {
			t.Errorf("circle cleanup: %v", err)
		}
		for _, id := range []string{identityA, identityB} {
			if _, err := pool.Exec(context.Background(), "DELETE FROM anon_identities WHERE id = $1", id); err != nil {
				t.Errorf("identity cleanup for %s: %v", id, err)
			}
		}
	}()

	t.Run("ListCircles returns the seeded circle with its member count", func(t *testing.T) {
		got, err := repo.ListCircles(ctx)
		if err != nil {
			t.Fatalf("ListCircles returned error: %v", err)
		}
		found := false
		for _, c := range got {
			if c.ID == circleID {
				found = true
				if c.Name != "Integration circle" {
					t.Errorf("unexpected name: %q", c.Name)
				}
				if c.MemberCount != 0 {
					t.Errorf("expected 0 members before any joins, got %d", c.MemberCount)
				}
			}
		}
		if !found {
			t.Fatal("seeded circle missing from ListCircles")
		}
	})

	t.Run("GetCircle returns the circle or NotFound", func(t *testing.T) {
		c, err := repo.GetCircle(ctx, circleID)
		if err != nil {
			t.Fatalf("GetCircle returned error: %v", err)
		}
		if c.Slug != fmt.Sprintf("itest-%d", nonce) {
			t.Errorf("unexpected slug: %q", c.Slug)
		}
		if _, err := repo.GetCircle(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
	})

	t.Run("membership add, duplicate, and remove", func(t *testing.T) {
		if err := repo.AddMember(ctx, circleID, identityA); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
		if err := repo.AddMember(ctx, circleID, identityA); !errors.Is(err, ErrAlreadyMember) {
			t.Errorf("expected ErrAlreadyMember on a duplicate, got %v", err)
		}
		ok, err := repo.IsMember(ctx, circleID, identityA)
		if err != nil || !ok {
			t.Errorf("expected IsMember true, got %t (%v)", ok, err)
		}
		if ok, err := repo.IsMember(ctx, circleID, identityB); err != nil || ok {
			t.Errorf("expected IsMember false for the other identity, got %t (%v)", ok, err)
		}
		if err := repo.RemoveMember(ctx, circleID, identityA); err != nil {
			t.Fatalf("RemoveMember: %v", err)
		}
		// Idempotent leave: removing again is a no-op.
		if err := repo.RemoveMember(ctx, circleID, identityA); err != nil {
			t.Errorf("second RemoveMember should be a no-op, got %v", err)
		}
	})

	t.Run("CreateMessage attributes the message to the identity's anon_name", func(t *testing.T) {
		if err := repo.AddMember(ctx, circleID, identityA); err != nil {
			t.Fatalf("AddMember: %v", err)
		}
		msg, err := repo.CreateMessage(ctx, circleID, identityA, "I understand this so deeply.")
		if err != nil {
			t.Fatalf("CreateMessage: %v", err)
		}
		if len(msg.ID) != 36 {
			t.Errorf("expected a UUID message id, got %q", msg.ID)
		}
		if msg.AnonName == "" {
			t.Error("expected the author's anon_name to be resolved")
		}
		if msg.Content != "I understand this so deeply." {
			t.Errorf("unexpected content round-trip: %q", msg.Content)
		}
		if msg.CreatedAt.IsZero() {
			t.Error("expected created_at to be populated")
		}
		if msg.ReactionCounts == nil {
			t.Error("expected a non-nil, empty reaction_counts from the column default")
		}

		// The stored author is the seeded anonymous identity, never a real user.
		var authorID string
		if err := pool.QueryRow(ctx,
			"SELECT anon_identity_id FROM circle_messages WHERE id = $1", msg.ID).Scan(&authorID); err != nil {
			t.Fatalf("raw SELECT failed: %v", err)
		}
		if authorID != identityA {
			t.Errorf("expected the message author to be identity A, got %q", authorID)
		}
	})

	t.Run("ListMessages returns newest first with cursoring", func(t *testing.T) {
		for _, text := range []string{"first", "second", "third"} {
			time.Sleep(time.Millisecond)
			if _, err := repo.CreateMessage(ctx, circleID, identityA, text); err != nil {
				t.Fatalf("CreateMessage: %v", err)
			}
		}

		messages, err := repo.ListMessages(ctx, circleID, 100, nil)
		if err != nil {
			t.Fatalf("ListMessages returned error: %v", err)
		}
		if len(messages) < 4 {
			t.Fatalf("expected at least 4 messages, got %d", len(messages))
		}
		for i := 1; i < len(messages); i++ {
			if messages[i-1].CreatedAt.Before(messages[i].CreatedAt) {
				t.Errorf("list not newest-first at index %d", i)
			}
		}

		limited, err := repo.ListMessages(ctx, circleID, 2, nil)
		if err != nil {
			t.Fatalf("ListMessages(limit) returned error: %v", err)
		}
		if len(limited) != 2 {
			t.Errorf("expected 2 messages, got %d", len(limited))
		}

		page2, err := repo.ListMessages(ctx, circleID, 2, &limited[1].CreatedAt)
		if err != nil {
			t.Fatalf("paged ListMessages returned error: %v", err)
		}
		if len(page2) == 0 {
			t.Fatal("expected a second page of messages")
		}
		for _, m := range page2 {
			if !m.CreatedAt.Before(limited[1].CreatedAt) {
				t.Errorf("pagination returned a message not older than the cursor")
			}
		}
	})

	t.Run("member count reflects joined members", func(t *testing.T) {
		if err := repo.AddMember(ctx, circleID, identityB); err != nil {
			t.Fatalf("AddMember B: %v", err)
		}
		c, err := repo.GetCircle(ctx, circleID)
		if err != nil {
			t.Fatalf("GetCircle: %v", err)
		}
		if c.MemberCount < 1 {
			t.Errorf("expected at least 1 active membership, got %d", c.MemberCount)
		}
		if err := repo.RemoveMember(ctx, circleID, identityB); err != nil {
			t.Fatalf("RemoveMember B: %v", err)
		}
	})

	t.Run("deleting the circle cascades memberships and messages", func(t *testing.T) {
		// identityA's membership survived from the create-message subtest;
		// add identityB directly so two memberships exist before the delete.
		var createdID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO circle_members (circle_id, anon_identity_id)
			 VALUES ($1, $2) RETURNING id`, circleID, identityB).Scan(&createdID); err != nil {
			t.Fatalf("direct membership insert: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM circles WHERE id = $1", circleID); err != nil {
			t.Fatalf("circle delete: %v", err)
		}

		var members, messages int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM circle_members WHERE circle_id = $1`, circleID).Scan(&members); err != nil {
			t.Fatalf("membership count: %v", err)
		}
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM circle_messages WHERE circle_id = $1`, circleID).Scan(&messages); err != nil {
			t.Fatalf("message count: %v", err)
		}
		if members != 0 || messages != 0 {
			t.Errorf("expected cascade delete to clear members (%d) and messages (%d)", members, messages)
		}
		if _, err := repo.GetCircle(ctx, circleID); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound after delete, got %v", err)
		}
	})
}
