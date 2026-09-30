package breathing

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"
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

	// RecordSession records a completed breathing session for the given owner,
	// which may be a registered user or an anonymous session. The exercise
	// must exist (ErrExerciseNotFound otherwise); its technique and name are
	// stamped onto the session. The owner always comes from the authenticated
	// identity context, never from client input.
	RecordSession(ctx context.Context, owner middleware.Owner, exerciseID string, breaths, durationS int, completed bool) (*Session, error)

	// ListSessions returns the owner's breathing history newest first, clamped
	// to a sane page size. It returns an empty slice (not nil) when the owner
	// has none.
	ListSessions(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error)
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

func (s *service) RecordSession(ctx context.Context, owner middleware.Owner, exerciseID string, breaths, durationS int, completed bool) (*Session, error) {
	// The owner is stamped onto the row as a single column, so an ambiguous
	// owner (both identities set) would silently write as one of them. Reject
	// it here rather than guessing which identity meant the request.
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
