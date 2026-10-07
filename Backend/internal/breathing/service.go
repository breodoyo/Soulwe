package breathing

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"
)

// Service is the breathing business-logic boundary: exercise resolution,
// session assembly, and list-page clamping, never SQL.
type Service interface {
	// ListExercises returns the curated catalog in its defined order, clamped to
	// a sane page size.
	ListExercises(ctx context.Context, limit int) ([]Exercise, error)

	// GetExercise returns one catalog exercise, or ErrExerciseNotFound.
	GetExercise(ctx context.Context, exerciseID string) (*Exercise, error)

	// RecordSession records a session for the given owner, which may be a
	// registered user or an anonymous session. The exercise must exist
	// (ErrExerciseNotFound otherwise) and its technique and name are stamped on.
	RecordSession(ctx context.Context, owner middleware.Owner, exerciseID string, breaths, durationS int, completed bool) (*Session, error)

	// ListSessions returns the owner's breathing history newest first, clamped
	// to a sane page size.
	ListSessions(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error)
}

type service struct {
	repo Repository
}

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

func (s *service) RecordSession(ctx context.Context, owner middleware.Owner, exerciseID string, breaths, durationS int, completed bool) (*Session, error) {
	// An ambiguous owner (both identities set) would silently write as one of
	// them, so it is rejected rather than guessed.
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, ErrInvalidOwner
	}

	exercise, err := s.repo.GetExercise(ctx, exerciseID)
	if err != nil {
		if errors.Is(err, ErrExerciseNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("breathing record session: resolve exercise: %w", err)
	}

	name := exercise.Name
	session := &Session{
		ExerciseID: stringPtr(exercise.ID),
		Technique:  exercise.Technique,
		Name:       &name,
		Breaths:    breaths,
		DurationS:  durationS,
		Completed:  completed,
	}
	if owner.Registered() {
		session.UserID = owner.UserID
	} else {
		session.AnonIdentityID = owner.AnonIdentityID
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("breathing record session: %w", err)
	}
	return session, nil
}

func (s *service) ListSessions(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error) {
	sessions, err := s.repo.ListSessionsByOwner(ctx, owner, ClampLimit(limit))
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func stringPtr(s string) *string {
	return &s
}
