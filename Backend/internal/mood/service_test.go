package mood

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"Backend/internal/middleware"
)

// owner is a registered owner for the given id, used to keep the service
// tests readable now that check-ins are scoped to a registered user OR an
// anonymous session.
func owner(userID string) middleware.Owner { return middleware.Owner{UserID: userID} }

// fakeRepository is an in-memory Repository used to unit-test the service
// without a real PostgreSQL connection.
type fakeRepository struct {
	logs       []MoodLog
	createErr  error
	listErr    error
	latestErr  error
	countErr   error
	createdIDs int
}

func (f *fakeRepository) Create(_ context.Context, log *MoodLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdIDs++
	log.ID = fmt.Sprintf("mood-%03d", f.createdIDs)
	log.LoggedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(f.createdIDs) * time.Minute)
	f.logs = append(f.logs, *log)
	return nil
}

func (f *fakeRepository) ListByOwner(_ context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	result := make([]MoodLog, 0)
	for _, l := range f.logs {
		if l.UserID == owner.UserID && l.AnonIdentityID == owner.AnonIdentityID {
			result = append(result, l)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepository) LatestByOwner(_ context.Context, owner middleware.Owner) (*MoodLog, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	logs, _ := f.ListByOwner(context.Background(), owner, 1<<20)
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[len(logs)-1], nil
}

func (f *fakeRepository) CountByOwner(_ context.Context, owner middleware.Owner) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	logs, _ := f.ListByOwner(context.Background(), owner, 1<<20)
	return int64(len(logs)), nil
}

func TestServiceCreate(t *testing.T) {
	t.Run("creates a check-in for the authenticated user", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)

		log, err := svc.Create(context.Background(), owner("user-1"), "Better")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if log.UserID != "user-1" {
			t.Errorf("expected the authenticated user id, got %q", log.UserID)
		}
		if log.Mood != "Better" {
			t.Errorf("expected mood 'Better', got %q", log.Mood)
		}
		if log.ID == "" || log.LoggedAt.IsZero() {
			t.Error("expected repository-generated fields to be populated")
		}
		if len(repo.logs) != 1 {
			t.Errorf("expected 1 stored log, got %d", len(repo.logs))
		}
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		log, err := svc.Create(context.Background(), owner("user-1"), "  Grateful  ")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if log.Mood != "Grateful" {
			t.Errorf("expected mood 'Grateful', got %q", log.Mood)
		}
	})

	t.Run("rejects a mood outside the product vocabulary", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)

		for _, invalid := range []string{"", "Sad", "happy", "At peace!", "  "} {
			if _, err := svc.Create(context.Background(), owner("user-1"), invalid); !errors.Is(err, ErrInvalidMood) {
				t.Errorf("mood %q: expected ErrInvalidMood, got %v", invalid, err)
			}
		}
		if len(repo.logs) != 0 {
			t.Errorf("no log should be stored for invalid moods, got %d", len(repo.logs))
		}
	})

	t.Run("accepts every documented mood value", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		for _, m := range ValidMoods {
			if _, err := svc.Create(context.Background(), owner("user-1"), m); err != nil {
				t.Errorf("mood %q: expected success, got %v", m, err)
			}
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		svc := NewService(&fakeRepository{createErr: errors.New("connection lost")})
		if _, err := svc.Create(context.Background(), owner("user-1"), "Better"); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})

	t.Run("creates a check-in for an anonymous session", func(t *testing.T) {
		repo := &fakeRepository{}
		anon := middleware.Owner{AnonIdentityID: "anon-1"}

		log, err := NewService(repo).Create(context.Background(), anon, "Better")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if log.AnonIdentityID != "anon-1" {
			t.Errorf("expected the anonymous identity, got %q", log.AnonIdentityID)
		}
		if log.UserID != "" {
			t.Errorf("an anonymous check-in must not carry a user id, got %q", log.UserID)
		}
	})
}

func TestServiceList(t *testing.T) {
	seed := func() *fakeRepository {
		repo := &fakeRepository{}
		svc := NewService(repo)
		for _, m := range []string{"Heavy", "Okay", "Better", "At peace", "Grateful"} {
			if _, err := svc.Create(context.Background(), owner("user-1"), m); err != nil {
				t.Fatalf("seed Create returned error: %v", err)
			}
		}
		return repo
	}

	t.Run("returns only the user's logs", func(t *testing.T) {
		repo := seed()
		svc := NewService(repo)
		// A second user's check-in must never leak into user-1's list.
		if _, err := svc.Create(context.Background(), owner("user-2"), "Better"); err != nil {
			t.Fatalf("user-2 Create returned error: %v", err)
		}

		logs, err := svc.List(context.Background(), owner("user-1"), 0)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Fatalf("expected 5 logs for user-1, got %d", len(logs))
		}
		for _, l := range logs {
			if l.UserID != "user-1" {
				t.Errorf("leaked another user's log: %+v", l)
			}
		}
	})

	t.Run("never mixes an anonymous session's logs into a user's list", func(t *testing.T) {
		repo := seed()
		svc := NewService(repo)
		// An anonymous check-in exists in the same table; a registered user
		// must not see it, and vice versa.
		anon := middleware.Owner{AnonIdentityID: "anon-1"}
		if _, err := svc.Create(context.Background(), anon, "Heavy"); err != nil {
			t.Fatalf("anonymous Create returned error: %v", err)
		}

		userLogs, err := svc.List(context.Background(), owner("user-1"), 0)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		for _, l := range userLogs {
			if l.AnonIdentityID != "" {
				t.Errorf("an anonymous log leaked into a registered user's list: %+v", l)
			}
		}

		anonLogs, err := svc.List(context.Background(), anon, 0)
		if err != nil {
			t.Fatalf("anonymous List returned error: %v", err)
		}
		if len(anonLogs) != 1 {
			t.Fatalf("expected 1 log for the anonymous session, got %d", len(anonLogs))
		}
		if anonLogs[0].UserID != "" {
			t.Errorf("a user's log leaked into an anonymous list: %+v", anonLogs[0])
		}
	})

	t.Run("defaults the limit for non-positive values", func(t *testing.T) {
		repo := seed()
		svc := NewService(repo)
		logs, err := svc.List(context.Background(), owner("user-1"), 0)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Errorf("expected all 5 (limit clamped to default), got %d", len(logs))
		}
	})

	t.Run("caps the limit at the maximum", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		logs, err := svc.List(context.Background(), owner("user-1"), 9999)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(logs) > MaxListLimit {
			t.Errorf("expected at most %d logs, got %d", MaxListLimit, len(logs))
		}
	})

	t.Run("returns an empty slice when the user has no logs", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		logs, err := svc.List(context.Background(), owner("nobody"), 0)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if logs == nil {
			t.Fatal("expected a non-nil empty slice so JSON renders []")
		}
		if len(logs) != 0 {
			t.Errorf("expected 0 logs, got %d", len(logs))
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := &fakeRepository{listErr: errors.New("connection lost")}
		if _, err := NewService(repo).List(context.Background(), owner("user-1"), 0); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceLatest(t *testing.T) {
	t.Run("returns nil when the user has no check-ins", func(t *testing.T) {
		log, err := NewService(&fakeRepository{}).Latest(context.Background(), owner("user-1"))
		if err != nil {
			t.Fatalf("Latest returned error: %v", err)
		}
		if log != nil {
			t.Errorf("expected nil, got %+v", log)
		}
	})

	t.Run("returns the single latest check-in", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)
		if _, err := svc.Create(context.Background(), owner("user-1"), "Heavy"); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if _, err := svc.Create(context.Background(), owner("user-1"), "Grateful"); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		latest, err := svc.Latest(context.Background(), owner("user-1"))
		if err != nil {
			t.Fatalf("Latest returned error: %v", err)
		}
		if latest == nil || latest.Mood != "Grateful" {
			t.Errorf("expected the most recent mood 'Grateful', got %+v", latest)
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := &fakeRepository{latestErr: errors.New("connection lost")}
		if _, err := NewService(repo).Latest(context.Background(), owner("user-1")); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceCount(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	for _, m := range ValidMoods {
		if _, err := svc.Create(context.Background(), owner("user-1"), m); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	count, err := svc.Count(context.Background(), owner("user-1"))
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}

	zero, err := svc.Count(context.Background(), owner("nobody"))
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if zero != 0 {
		t.Errorf("expected count 0 for a user with no check-ins, got %d", zero)
	}

	repo.countErr = errors.New("connection lost")
	if _, err := svc.Count(context.Background(), owner("user-1")); err == nil {
		t.Fatal("expected a wrapped repository error")
	}
}
