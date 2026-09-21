//go:build integration

package mood

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"Backend/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresRepositoryIntegration exercises the mood repository against a
// running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. Two users are
// created so ownership isolation can be verified.
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

	defer func() {
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

	t.Run("ListByUserID returns the user's logs, newest first", func(t *testing.T) {
		var moods = []string{"Grateful", "At peace", "Better", "Okay"}
		for _, m := range moods {
			if err := repo.Create(ctx, &MoodLog{UserID: ownerID, Mood: m}); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
		}

		logs, err := repo.ListByUserID(ctx, ownerID, 100)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Fatalf("expected 5 logs, got %d", len(logs))
		}
		// "Heavy" was created first, then Grateful/At peace/Better/Okay. With
		// logged_at ordering (same-second inserts resolved by id DESC) the most
		// recent insert should lead.
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

	t.Run("ListByUserID respects the limit", func(t *testing.T) {
		logs, err := repo.ListByUserID(ctx, ownerID, 2)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(logs) != 2 {
			t.Errorf("expected 2 logs, got %d", len(logs))
		}
	})

	t.Run("ListByUserID never returns another user's logs", func(t *testing.T) {
		otherID := seedUser()
		if err := repo.Create(ctx, &MoodLog{UserID: otherID, Mood: "Better"}); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		logs, err := repo.ListByUserID(ctx, ownerID, 100)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		for _, l := range logs {
			if l.UserID != ownerID {
				t.Errorf("expected only the owner's logs, found user %s", l.UserID)
			}
		}
	})

	t.Run("LatestByUserID returns only the newest log", func(t *testing.T) {
		latest, err := repo.LatestByUserID(ctx, ownerID)
		if err != nil {
			t.Fatalf("LatestByUserID returned error: %v", err)
		}
		if latest == nil {
			t.Fatal("expected a latest log for a user with check-ins")
		}
		if latest.Mood != "Okay" {
			t.Errorf("expected the newest mood 'Okay', got %q", latest.Mood)
		}

		empty, err := repo.LatestByUserID(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("LatestByUserID returned error: %v", err)
		}
		if empty != nil {
			t.Errorf("expected nil for a user with no check-ins, got %+v", empty)
		}
	})

	t.Run("CountByUserID counts only the user's own logs", func(t *testing.T) {
		count, err := repo.CountByUserID(ctx, ownerID)
		if err != nil {
			t.Fatalf("CountByUserID returned error: %v", err)
		}
		if count != 5 {
			t.Errorf("expected count 5 for the owner, got %d", count)
		}

		zero, err := repo.CountByUserID(ctx, "00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("CountByUserID returned error: %v", err)
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

		logs, err := repo.ListByUserID(ctx, ghostID, 100)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(logs) != 0 {
			t.Errorf("expected the ghost's logs to cascade-delete, got %d", len(logs))
		}
	})
}
