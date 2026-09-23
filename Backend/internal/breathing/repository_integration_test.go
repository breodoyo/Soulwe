//go:build integration

package breathing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"Backend/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

const missingID = "00000000-0000-0000-0000-000000000000"

func strPtr(s string) *string { return &s }

// TestPostgresRepositoryIntegration exercises the breathing repository against
// a running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. Two users are
// created so ownership isolation can be verified; sessions cascade-delete with
// their user.
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

	seededUserIDs := make([]string, 0, 3)
	seedUser := func() string {
		u := &user.User{
			Email:        fmt.Sprintf("breathe-owner-%d@soulwe.local", time.Now().UnixNano()),
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

	// Resolve one seeded catalog exercise to drive the session tests.
	var exerciseID, slug string
	if err := pool.QueryRow(ctx,
		`SELECT id, slug FROM breathing_exercises ORDER BY created_at, id LIMIT 1`).Scan(&exerciseID, &slug); err != nil {
		t.Fatalf("failed to resolve a seeded exercise: %v", err)
	}
	if slug == "" || exerciseID == "" {
		t.Fatalf("expected a seeded exercise, got %q / %q", exerciseID, slug)
	}

	t.Run("ListExercises returns the seeded catalog in defined order", func(t *testing.T) {
		exercises, err := repo.ListExercises(ctx, 100)
		if err != nil {
			t.Fatalf("ListExercises returned error: %v", err)
		}
		if len(exercises) < 2 {
			t.Fatalf("expected the two seeded exercises, got %d", len(exercises))
		}
		if exercises[0].ID == "" || exercises[0].Name == "" {
			t.Errorf("exercise public fields not populated: %+v", exercises[0])
		}
	})

	t.Run("ListExercises respects the limit", func(t *testing.T) {
		exercises, err := repo.ListExercises(ctx, 1)
		if err != nil {
			t.Fatalf("ListExercises returned error: %v", err)
		}
		if len(exercises) != 1 {
			t.Errorf("expected 1 exercise, got %d", len(exercises))
		}
	})

	t.Run("GetExercise returns the exercise with public fields only", func(t *testing.T) {
		ex, err := repo.GetExercise(ctx, exerciseID)
		if err != nil {
			t.Fatalf("GetExercise returned error: %v", err)
		}
		if ex.ID != exerciseID || ex.Slug != slug {
			t.Errorf("expected the resolved exercise %q/%q, got %q/%q", exerciseID, slug, ex.ID, ex.Slug)
		}
		if ex.Technique == "" || ex.InhaleS <= 0 || ex.ExhaleS <= 0 {
			t.Errorf("exercise guide data not populated: %+v", ex)
		}
		if ex.CreatedAt.IsZero() {
			t.Errorf("created_at not populated: %+v", ex)
		}
	})

	t.Run("GetExercise returns ErrExerciseNotFound for a missing exercise", func(t *testing.T) {
		if _, err := repo.GetExercise(ctx, missingID); !errors.Is(err, ErrExerciseNotFound) {
			t.Fatalf("expected ErrExerciseNotFound, got %v", err)
		}
	})

	t.Run("CreateSession scopes the session to its user and exercise", func(t *testing.T) {
		name := "seeded"
		session := &Session{
			UserID:     ownerID,
			ExerciseID: &exerciseID,
			Technique:  "478",
			Name:       &name,
			Breaths:    5,
			DurationS:  95,
			Completed:  true,
		}
		if err := repo.CreateSession(ctx, session); err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if len(session.ID) != 36 {
			t.Errorf("expected a UUID id, got %q", session.ID)
		}
		if session.CreatedAt.IsZero() {
			t.Error("expected created_at to be populated")
		}
		if session.ExerciseID == nil || *session.ExerciseID != exerciseID {
			t.Errorf("expected the exercise id echoed back, got %v", session.ExerciseID)
		}
	})

	t.Run("CreateSession rejects a session for an unknown exercise", func(t *testing.T) {
		session := &Session{
			UserID:     ownerID,
			ExerciseID: strPtr(missingID),
			Technique:  "478",
			Breaths:    5,
			DurationS:  95,
			Completed:  true,
		}
		if err := repo.CreateSession(ctx, session); err == nil {
			t.Fatal("expected a foreign-key error for an unknown exercise")
		}
		var fkCount int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_constraint
			WHERE conrelid = 'breathing_sessions'::regclass
			  AND contype = 'f'
			  AND confrelid = 'breathing_exercises'::regclass`).Scan(&fkCount); err != nil {
			t.Fatalf("failed to check foreign keys: %v", err)
		}
		if fkCount != 1 {
			t.Errorf("expected breathing_sessions to carry an exercise foreign key, got %d", fkCount)
		}
		sessionLines, err := repo.ListSessionsByUserID(ctx, ownerID, 100)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		if len(sessionLines) != 1 {
			t.Errorf("the rejected insert must not persist; expected 1 session, got %d", len(sessionLines))
		}
	})

	t.Run("ListSessionsByUserID returns the user's sessions, newest first", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			s := &Session{
				UserID:     ownerID,
				ExerciseID: &exerciseID,
				Technique:  "box",
				Breaths:    4 + i,
				DurationS:  60,
				Completed:  true,
			}
			if err := repo.CreateSession(ctx, s); err != nil {
				t.Fatalf("CreateSession returned error: %v", err)
			}
			time.Sleep(2 * time.Millisecond) // keep created_at strictly increasing
		}

		sessions, err := repo.ListSessionsByUserID(ctx, ownerID, 100)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		if len(sessions) != 4 {
			t.Fatalf("expected 4 sessions (1 + 3), got %d", len(sessions))
		}
		for i := 1; i < len(sessions); i++ {
			if sessions[i-1].CreatedAt.Before(sessions[i].CreatedAt) {
				t.Errorf("list not newest-first at index %d: %v before %v",
					i, sessions[i-1].CreatedAt, sessions[i].CreatedAt)
			}
		}
		if sessions[0].Breaths != 6 {
			t.Errorf("expected the newest session (breaths 6) first, got breaths %d", sessions[0].Breaths)
		}
		for _, s := range sessions {
			if s.UserID != ownerID {
				t.Errorf("leaked a foreign session: %+v", s)
			}
		}
	})

	t.Run("ListSessionsByUserID never returns another user's sessions", func(t *testing.T) {
		otherID := seedUser()
		s := &Session{
			UserID:     otherID,
			ExerciseID: &exerciseID,
			Technique:  "478",
			Breaths:    5,
			DurationS:  95,
			Completed:  true,
		}
		if err := repo.CreateSession(ctx, s); err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}

		sessions, err := repo.ListSessionsByUserID(ctx, ownerID, 100)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		for _, sess := range sessions {
			if sess.UserID != ownerID {
				t.Errorf("expected only the owner's sessions, found user %s", sess.UserID)
			}
		}
	})

	t.Run("ListSessionsByUserID respects the limit", func(t *testing.T) {
		sessions, err := repo.ListSessionsByUserID(ctx, ownerID, 2)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		if len(sessions) != 2 {
			t.Errorf("expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("ListSessionsByUserID returns an empty slice for a user with none", func(t *testing.T) {
		sessions, err := repo.ListSessionsByUserID(ctx, missingID, 100)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		if sessions == nil {
			t.Fatal("expected a non-nil empty slice")
		}
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(sessions))
		}
	})

	t.Run("deleting the user cascades their breathing sessions", func(t *testing.T) {
		ghostID := seedUser()
		s := &Session{
			UserID:     ghostID,
			ExerciseID: &exerciseID,
			Technique:  "box",
			Breaths:    4,
			DurationS:  60,
			Completed:  true,
		}
		if err := repo.CreateSession(ctx, s); err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", ghostID); err != nil {
			t.Fatalf("failed to DELETE the ghost user: %v", err)
		}

		sessions, err := repo.ListSessionsByUserID(ctx, ghostID, 100)
		if err != nil {
			t.Fatalf("ListSessionsByUserID returned error: %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("expected the ghost's sessions to cascade-delete, got %d", len(sessions))
		}
	})
}
