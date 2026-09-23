package breathing

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
	exercises []Exercise
	sessions  []Session
	getErr    error
	listErr   error
	createErr error
	createdID int
}

func (f *fakeRepository) ListExercises(_ context.Context, limit int) ([]Exercise, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit < 1 || limit > len(f.exercises) {
		limit = len(f.exercises)
	}
	return append([]Exercise(nil), f.exercises[:limit]...), nil
}

func (f *fakeRepository) GetExercise(_ context.Context, exerciseID string) (*Exercise, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, e := range f.exercises {
		if e.ID == exerciseID {
			ex := e
			return &ex, nil
		}
	}
	return nil, ErrExerciseNotFound
}

func (f *fakeRepository) CreateSession(_ context.Context, session *Session) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdID++
	session.ID = fmt.Sprintf("session-%03d", f.createdID)
	session.CreatedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(f.createdID) * time.Minute)
	f.sessions = append(f.sessions, *session)
	return nil
}

func (f *fakeRepository) ListSessionsByUserID(_ context.Context, userID string, limit int) ([]Session, error) {
	result := make([]Session, 0)
	for _, s := range f.sessions {
		if s.UserID == userID {
			result = append(result, s)
		}
	}
	// The fake stores newest last; ordering assertions are done via created_at.
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func seedFake(t *testing.T) *fakeRepository {
	t.Helper()
	return &fakeRepository{
		exercises: []Exercise{
			{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Slug: "478", Name: "4-7-8 Breathing", Description: "d", Technique: "478", InhaleS: 4, HoldS: 7, ExhaleS: 8},
			{ID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Slug: "box", Name: "Box Breathing", Description: "d", Technique: "box", InhaleS: 4, HoldS: 4, ExhaleS: 4},
		},
	}
}

func TestServiceListExercises(t *testing.T) {
	t.Run("returns the catalog in defined order", func(t *testing.T) {
		svc := NewService(seedFake(t))
		exercises, err := svc.ListExercises(context.Background(), 0)
		if err != nil {
			t.Fatalf("ListExercises returned error: %v", err)
		}
		if len(exercises) != 2 {
			t.Fatalf("expected 2 exercises, got %d", len(exercises))
		}
		if exercises[0].Slug != "478" || exercises[1].Slug != "box" {
			t.Errorf("unexpected catalog order: got %q, %q", exercises[0].Slug, exercises[1].Slug)
		}
	})

	t.Run("caps the limit at the maximum", func(t *testing.T) {
		exercises, err := NewService(seedFake(t)).ListExercises(context.Background(), 9999)
		if err != nil {
			t.Fatalf("ListExercises returned error: %v", err)
		}
		if len(exercises) > MaxListLimit {
			t.Errorf("expected at most %d exercises, got %d", MaxListLimit, len(exercises))
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := seedFake(t)
		repo.listErr = errors.New("connection lost")
		if _, err := NewService(repo).ListExercises(context.Background(), 0); err == nil {
			t.Fatal("expected a repository error")
		}
	})
}

func TestServiceGetExercise(t *testing.T) {
	t.Run("returns the exercise by id", func(t *testing.T) {
		ex, err := NewService(seedFake(t)).GetExercise(context.Background(), "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
		if err != nil {
			t.Fatalf("GetExercise returned error: %v", err)
		}
		if ex.Slug != "box" {
			t.Errorf("expected the box exercise, got %q", ex.Slug)
		}
	})

	t.Run("returns ErrExerciseNotFound for a missing exercise", func(t *testing.T) {
		_, err := NewService(seedFake(t)).GetExercise(context.Background(), "00000000-0000-0000-0000-000000000000")
		if !errors.Is(err, ErrExerciseNotFound) {
			t.Fatalf("expected ErrExerciseNotFound, got %v", err)
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := seedFake(t)
		repo.getErr = errors.New("connection lost")
		if _, err := NewService(repo).GetExercise(context.Background(), "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"); err == nil {
			t.Fatal("expected a repository error")
		}
	})
}

func TestServiceRecordSession(t *testing.T) {
	t.Run("records a session for the authenticated user", func(t *testing.T) {
		repo := seedFake(t)
		svc := NewService(repo)

		session, err := svc.RecordSession(context.Background(), "user-1",
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 5, 95, true)
		if err != nil {
			t.Fatalf("RecordSession returned error: %v", err)
		}
		if session.UserID != "user-1" {
			t.Errorf("expected the authenticated user id, got %q", session.UserID)
		}
		if session.ExerciseID == nil || *session.ExerciseID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
			t.Errorf("expected the exercise id stamped on the session, got %v", session.ExerciseID)
		}
		if session.Technique != "478" {
			t.Errorf("expected the exercise technique, got %q", session.Technique)
		}
		if session.Name == nil || *session.Name != "4-7-8 Breathing" {
			t.Errorf("expected the exercise display name, got %v", session.Name)
		}
		if session.Breaths != 5 || session.DurationS != 95 || !session.Completed {
			t.Errorf("unexpected session data: %+v", session)
		}
		if session.ID == "" || session.CreatedAt.IsZero() {
			t.Error("expected repository-generated fields to be populated")
		}
	})

	t.Run("rejects an unknown exercise", func(t *testing.T) {
		repo := seedFake(t)
		svc := NewService(repo)
		if _, err := svc.RecordSession(context.Background(), "user-1",
			"00000000-0000-0000-0000-000000000000", 5, 95, true); !errors.Is(err, ErrExerciseNotFound) {
			t.Fatalf("expected ErrExerciseNotFound, got %v", err)
		}
		if len(repo.sessions) != 0 {
			t.Errorf("no session should be stored for an unknown exercise, got %d", len(repo.sessions))
		}
	})

	t.Run("repository failure is wrapped", func(t *testing.T) {
		repo := seedFake(t)
		repo.createErr = errors.New("connection lost")
		if _, err := NewService(repo).RecordSession(context.Background(), "user-1",
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 5, 95, true); err == nil {
			t.Fatal("expected a wrapped repository error")
		}
	})
}

func TestServiceListSessions(t *testing.T) {
	t.Run("defaults the limit for a valid user", func(t *testing.T) {
		repo := seedFake(t)
		svc := NewService(repo)
		if _, err := svc.RecordSession(context.Background(), "user-1",
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 5, 95, true); err != nil {
			t.Fatalf("RecordSession returned error: %v", err)
		}

		sessions, err := svc.ListSessions(context.Background(), "user-1", 0)
		if err != nil {
			t.Fatalf("ListSessions returned error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		for _, s := range sessions {
			if s.UserID != "user-1" {
				t.Errorf("leaked another user's session: %+v", s)
			}
		}
	})

	t.Run("returns an empty slice when the user has no sessions", func(t *testing.T) {
		sessions, err := NewService(seedFake(t)).ListSessions(context.Background(), "nobody", 0)
		if err != nil {
			t.Fatalf("ListSessions returned error: %v", err)
		}
		if sessions == nil {
			t.Fatal("expected a non-nil empty slice so JSON renders []")
		}
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(sessions))
		}
	})

	t.Run("caps the limit at the maximum", func(t *testing.T) {
		sessions, err := NewService(seedFake(t)).ListSessions(context.Background(), "user-1", 9999)
		if err != nil {
			t.Fatalf("ListSessions returned error: %v", err)
		}
		if len(sessions) > MaxListLimit {
			t.Errorf("expected at most %d sessions, got %d", MaxListLimit, len(sessions))
		}
	})
}
