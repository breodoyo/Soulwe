//go:build integration

package mood

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"Backend/internal/anon"
	"Backend/internal/middleware"
	"Backend/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration needs DATABASE_URL and the "integration" tag.
func TestPostgresRepositoryIntegration(t *testing.T) {
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

	users := user.NewPostgresRepository(pool)
	repo := NewPostgresRepository(pool)

	const passwordHash = "$2a$12$abcdefghijklmnopqrstuv" // arbitrary bcrypt-shaped value

	seededUserIDs := make([]string, 0, 2)
	seedUser := func() string {
		u := &user.User{
			Email:        fmt.Sprintf("mood-owner-%d@soulwe.local", time.Now().UnixNano()),
			PasswordHash: passwordHash,
			LanguagePref: "en",
		}
		if err := users.Create(ctx, u); err != nil {
			t.Fatalf("failed to seed user: %v", err)
		}
		seededUserIDs = append(seededUserIDs, u.ID)
		return u.ID
	}

	// Seeded directly: only the row is needed, not the session-minting service.
	seededAnonIDs := make([]string, 0, 2)
	seedAnon := func() string {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO anon_identities (anon_name, token_hash)
			VALUES ($1, $2)
			RETURNING id`,
			fmt.Sprintf("mood-anon-%d", time.Now().UnixNano()),
			anon.HashToken(fmt.Sprintf("mood-anon-token-%d", time.Now().UnixNano())),
		).Scan(&id)
		if err != nil {
			t.Fatalf("failed to seed anonymous identity: %v", err)
		}
		seededAnonIDs = append(seededAnonIDs, id)
		return id
	}

	defer func() {
		for _, id := range seededAnonIDs {
			if _, err := pool.Exec(context.Background(), "DELETE FROM anon_identities WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE test anonymous identity %s: %v", id, err)
			}
		}
		for _, id := range seededUserIDs {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", id, err)
			}
		}
	}()

	ownerID := seedUser()

	t.Run("Create populates the log with a UUID and timestamp", func(t *testing.T) {
		log := &MoodLog{UserID: ownerID, Mood: "Heavy"}
		if err := repo.Create(ctx, log); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if len(log.ID) != 36 {
			t.Errorf("expected a UUID id, got %q", log.ID)
		}
		if log.LoggedAt.IsZero() {
			t.Error("expected logged_at to be populated")
		}
	})

	t.Run("ListByOwner returns the user's logs, newest first", func(t *testing.T) {
		var moods = []string{"Grateful", "At peace", "Better", "Okay"}
		for _, m := range moods {
			if err := repo.Create(ctx, &MoodLog{UserID: ownerID, Mood: m}); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
		}

		logs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ownerID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Fatalf("expected 5 logs, got %d", len(logs))
		}
		// Same-second inserts are ordered by id DESC, so the last insert leads.
		for i := 1; i < len(logs); i++ {
			if logs[i-1].LoggedAt.Before(logs[i].LoggedAt) {
				t.Errorf("list not newest-first at index %d: %v before %v",
					i, logs[i-1].LoggedAt, logs[i].LoggedAt)
			}
		}
		if logs[0].Mood != "Okay" {
			t.Errorf("expected the most recently inserted mood 'Okay' first, got %q", logs[0].Mood)
		}
		for _, l := range logs {
			if l.UserID != ownerID {
				t.Errorf("leaked a foreign log: %+v", l)
			}
		}
	})

	t.Run("ListByOwner respects the limit", func(t *testing.T) {
		logs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ownerID}, 2)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 2 {
			t.Errorf("expected 2 logs, got %d", len(logs))
		}
	})

	t.Run("ListByOwner never returns another user's logs", func(t *testing.T) {
		otherID := seedUser()
		if err := repo.Create(ctx, &MoodLog{UserID: otherID, Mood: "Better"}); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		logs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ownerID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		for _, l := range logs {
			if l.UserID != ownerID {
				t.Errorf("expected only the owner's logs, found user %s", l.UserID)
			}
		}
	})

	t.Run("LatestByOwner returns only the newest log", func(t *testing.T) {
		latest, err := repo.LatestByOwner(ctx, middleware.Owner{UserID: ownerID})
		if err != nil {
			t.Fatalf("LatestByOwner returned error: %v", err)
		}
		if latest == nil {
			t.Fatal("expected a latest log for a user with check-ins")
		}
		if latest.Mood != "Okay" {
			t.Errorf("expected the newest mood 'Okay', got %q", latest.Mood)
		}

		empty, err := repo.LatestByOwner(ctx, middleware.Owner{UserID: "00000000-0000-0000-0000-000000000000"})
		if err != nil {
			t.Fatalf("LatestByOwner returned error: %v", err)
		}
		if empty != nil {
			t.Errorf("expected nil for a user with no check-ins, got %+v", empty)
		}
	})

	t.Run("CountByOwner counts only the user's own logs", func(t *testing.T) {
		count, err := repo.CountByOwner(ctx, middleware.Owner{UserID: ownerID})
		if err != nil {
			t.Fatalf("CountByOwner returned error: %v", err)
		}
		if count != 5 {
			t.Errorf("expected count 5 for the owner, got %d", count)
		}

		zero, err := repo.CountByOwner(ctx, middleware.Owner{UserID: "00000000-0000-0000-0000-000000000000"})
		if err != nil {
			t.Fatalf("CountByOwner returned error: %v", err)
		}
		if zero != 0 {
			t.Errorf("expected count 0 for a user with no check-ins, got %d", zero)
		}
	})

	t.Run("deleting the user cascades their mood logs", func(t *testing.T) {
		ghostID := seedUser()
		for range 3 {
			if err := repo.Create(ctx, &MoodLog{UserID: ghostID, Mood: "Better"}); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", ghostID); err != nil {
			t.Fatalf("failed to DELETE the ghost user: %v", err)
		}

		logs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ghostID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 0 {
			t.Errorf("expected the ghost's logs to cascade-delete, got %d", len(logs))
		}
	})

	// The only subtests that can catch a scanner that cannot read a NULL owner column.
	t.Run("an anonymous session's check-ins round-trip", func(t *testing.T) {
		anonID := seedAnon()
		owner := middleware.Owner{AnonIdentityID: anonID}

		created := &MoodLog{AnonIdentityID: anonID, Mood: "Grateful"}
		if err := repo.Create(ctx, created); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if created.UserID != "" {
			t.Errorf("an anonymous check-in must not gain a user_id, got %q", created.UserID)
		}
		if created.AnonIdentityID != anonID {
			t.Errorf("expected anon_identity_id %q, got %q", anonID, created.AnonIdentityID)
		}

		logs, err := repo.ListByOwner(ctx, owner, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected the anonymous session's 1 check-in, got %d", len(logs))
		}
		if logs[0].AnonIdentityID != anonID || logs[0].UserID != "" {
			t.Errorf("scanned the wrong owner back: %+v", logs[0])
		}

		latest, err := repo.LatestByOwner(ctx, owner)
		if err != nil {
			t.Fatalf("LatestByOwner returned error: %v", err)
		}
		if latest == nil || latest.Mood != "Grateful" {
			t.Errorf("expected the anonymous latest mood 'Grateful', got %+v", latest)
		}

		count, err := repo.CountByOwner(ctx, owner)
		if err != nil {
			t.Fatalf("CountByOwner returned error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1 for the anonymous owner, got %d", count)
		}
	})

	// A registered row has anon_identity_id NULL, exercising the nullable column.
	t.Run("a registered owner's check-ins survive the anonymous column", func(t *testing.T) {
		logs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ownerID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Fatalf("expected the owner's 5 check-ins, got %d", len(logs))
		}
		for _, l := range logs {
			if l.UserID != ownerID || l.AnonIdentityID != "" {
				t.Errorf("scanned the wrong owner back: %+v", l)
			}
		}
	})

	t.Run("neither owner sees the other's check-ins", func(t *testing.T) {
		anonID := seedAnon()
		if err := repo.Create(ctx, &MoodLog{AnonIdentityID: anonID, Mood: "Heavy"}); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		userLogs, err := repo.ListByOwner(ctx, middleware.Owner{UserID: ownerID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		for _, l := range userLogs {
			if l.AnonIdentityID != "" {
				t.Errorf("a registered owner must never see an anonymous check-in: %+v", l)
			}
		}

		anonLogs, err := repo.ListByOwner(ctx, middleware.Owner{AnonIdentityID: anonID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		for _, l := range anonLogs {
			if l.UserID != "" {
				t.Errorf("an anonymous session must never see a registered check-in: %+v", l)
			}
		}
	})

	t.Run("deleting the anonymous identity cascades its check-ins", func(t *testing.T) {
		ghostAnonID := seedAnon()
		for range 3 {
			if err := repo.Create(ctx, &MoodLog{AnonIdentityID: ghostAnonID, Mood: "Better"}); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, "DELETE FROM anon_identities WHERE id = $1", ghostAnonID); err != nil {
			t.Fatalf("failed to DELETE the ghost anonymous identity: %v", err)
		}

		logs, err := repo.ListByOwner(ctx, middleware.Owner{AnonIdentityID: ghostAnonID}, 100)
		if err != nil {
			t.Fatalf("ListByOwner returned error: %v", err)
		}
		if len(logs) != 0 {
			t.Errorf("expected the ghost's check-ins to cascade-delete, got %d", len(logs))
		}
	})
}
