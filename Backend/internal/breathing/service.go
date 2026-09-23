package breathing

import (
	"context"
	"errors"
	"fmt"
)

// Service is the breathing business-logic boundary. Implementations own
// exercise resolution, session assembly, and list-page clamping; they never
// construct SQL.
type Service interface {
	// ListExercises returns the curated catalog in its defined order, clamped
	// to a sane page size. It returns an empty slice (not nil) when the
	// catalog is empty.
	ListExercises(ctx context.Context, limit int) ([]Exercise, error)

	// GetExercise returns one catalog exercise, or ErrExerciseNotFound.
	GetExercise(ctx context.Context, exerciseID string) (*Exercise, error)

	// RecordSession records a completed breathing session for the given user.
	// The exercise must exist (ErrExerciseNotFound otherwise); its technique
	// and name are stamped onto the session. The user ID always comes from the
	// authenticated JWT context, never from client input.
	RecordSession(ctx context.Context, userID, exerciseID string, breaths, durationS int, completed bool) (*Session, error)

	// ListSessions returns the user's breathing history newest first, clamped
	// to a sane page size. It returns an empty slice (not nil) when the user
	// has none.
	ListSessions(ctx context.Context, userID string, limit int) ([]Session, error)
}

type service struct {
	repo Repository
}

// NewService wires the breathing service to a breathing repository.
func NewService(repo Repository) *service {
	return &service{repo: repo}
}

func (s *service) ListExercises(ctx context.Context, limit int) ([]Exercise, error) {
	exercises, err := s.repo.ListExercises(ctx, ClampLimit(limit))
	if err != nil {
		return nil, err
	}
	return exercises, nil
}

func (s *service) GetExercise(ctx context.Context, exerciseID string) (*Exercise, error) {
	return s.repo.GetExercise(ctx, exerciseID)
}

func (s *service) RecordSession(ctx context.Context, userID, exerciseID string, breaths, durationS int, completed bool) (*Session, error) {
	exercise, err := s.repo.GetExercise(ctx, exerciseID)
	if err != nil {
		if errors.Is(err, ErrExerciseNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("breathing record session: resolve exercise: %w", err)
	}

	name := exercise.Name
	session := &Session{
		UserID:     userID,
		ExerciseID: stringPtr(exercise.ID),
		Technique:  exercise.Technique,
		Name:       &name,
		Breaths:    breaths,
		DurationS:  durationS,
		Completed:  completed,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("breathing record session: %w", err)
	}
	return session, nil
}

func (s *service) ListSessions(ctx context.Context, userID string, limit int) ([]Session, error) {
	sessions, err := s.repo.ListSessionsByUserID(ctx, userID, ClampLimit(limit))
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func stringPtr(s string) *string {
	return &s
}
