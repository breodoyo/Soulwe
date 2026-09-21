package mood

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

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

func (f *fakeRepository) ListByUserID(_ context.Context, userID string, limit int) ([]MoodLog, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	result := make([]MoodLog, 0)
	for _, l := range f.logs {
		if l.UserID == userID {
			result = append(result, l)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepository) LatestByUserID(_ context.Context, userID string) (*MoodLog, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	logs, _ := f.ListByUserID(context.Background(), userID, 1<<20)
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[len(logs)-1], nil
}

func (f *fakeRepository) CountByUserID(_ context.Context, userID string) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	logs, _ := f.ListByUserID(context.Background(), userID, 1<<20)
	return int64(len(logs)), nil
}

func TestServiceCreate(t *testing.T) {
	t.Run("creates a check-in for the authenticated user", func(t *testing.T) {
		repo := &fakeRepository{}
		svc := NewService(repo)

		log, err := svc.Create(context.Background(), "user-1", "Better")
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
		log, err := svc.Create(context.Background(), "user-1", "  Grateful  ")
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
			if _, err := svc.Create(context.Background(), "user-1", invalid); !errors.Is(err, ErrInvalidMood) {
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
			if _, err := svc.Create(context.Background(), "user-1", m); err != nil {
				t.Errorf("mood %q: expected success, got %v", m, err)
			}
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		svc := NewService(&fakeRepository{createErr: errors.New("connection lost")})
		if _, err := svc.Create(context.Background(), "user-1", "Better"); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceList(t *testing.T) {
	seed := func() *fakeRepository {
		repo := &fakeRepository{}
		svc := NewService(repo)
		for _, m := range []string{"Heavy", "Okay", "Better", "At peace", "Grateful"} {
			if _, err := svc.Create(context.Background(), "user-1", m); err != nil {
				t.Fatalf("seed Create returned error: %v", err)
			}
		}
		return repo
	}

	t.Run("returns only the user's logs", func(t *testing.T) {
		repo := seed()
		svc := NewService(repo)
		// A second user's check-in must never leak into user-1's list.
		if _, err := svc.Create(context.Background(), "user-2", "Better"); err != nil {
			t.Fatalf("user-2 Create returned error: %v", err)
		}

		logs, err := svc.List(context.Background(), "user-1", 0)
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

	t.Run("defaults the limit for non-positive values", func(t *testing.T) {
		repo := seed()
		svc := NewService(repo)
		logs, err := svc.List(context.Background(), "user-1", 0)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(logs) != 5 {
			t.Errorf("expected all 5 (limit clamped to default), got %d", len(logs))
		}
	})

	t.Run("caps the limit at the maximum", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		logs, err := svc.List(context.Background(), "user-1", 9999)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(logs) > MaxListLimit {
			t.Errorf("expected at most %d logs, got %d", MaxListLimit, len(logs))
		}
	})

	t.Run("returns an empty slice when the user has no logs", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		logs, err := svc.List(context.Background(), "nobody", 0)
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
		if _, err := NewService(repo).List(context.Background(), "user-1", 0); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceLatest(t *testing.T) {
	t.Run("returns nil when the user has no check-ins", func(t *testing.T) {
		log, err := NewService(&fakeRepository{}).Latest(context.Background(), "user-1")
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
		if _, err := svc.Create(context.Background(), "user-1", "Heavy"); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if _, err := svc.Create(context.Background(), "user-1", "Grateful"); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		latest, err := svc.Latest(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("Latest returned error: %v", err)
		}
		if latest == nil || latest.Mood != "Grateful" {
			t.Errorf("expected the most recent mood 'Grateful', got %+v", latest)
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := &fakeRepository{latestErr: errors.New("connection lost")}
		if _, err := NewService(repo).Latest(context.Background(), "user-1"); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceCount(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	for _, m := range ValidMoods {
		if _, err := svc.Create(context.Background(), "user-1", m); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	count, err := svc.Count(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}

	zero, err := svc.Count(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if zero != 0 {
		t.Errorf("expected count 0 for a user with no check-ins, got %d", zero)
	}

	repo.countErr = errors.New("connection lost")
	if _, err := svc.Count(context.Background(), "user-1"); err == nil {
		t.Fatal("expected a wrapped repository error")
	}
}
