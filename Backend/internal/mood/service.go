package mood

import (
	"context"
	"fmt"
	"strings"

	"Backend/internal/middleware"
)

// List defaults and caps, shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// Service owns mood validation and list-page clamping; it never constructs SQL.
type Service interface {
	// Create validates the mood and records a check-in for the owner's identity.
	Create(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error)

	// List returns the owner's check-ins newest first, clamped to a sane page size.
	List(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error)

	// Latest returns the owner's most recent check-in, or nil.
	Latest(ctx context.Context, owner middleware.Owner) (*MoodLog, error)

	Count(ctx context.Context, owner middleware.Owner) (int64, error)
}

type service struct {
	logs Repository
}

func NewService(logs Repository) *service {
	return &service{logs: logs}
}

func (s *service) Create(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error) {
	mood = strings.TrimSpace(mood)
	if !IsValidMood(mood) {
		return nil, ErrInvalidMood
	}
	// Reject an ambiguous owner: one column is stamped, so it would silently pick one.
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, ErrInvalidOwner
	}

	log := &MoodLog{Mood: mood}
	if owner.Registered() {
		log.UserID = owner.UserID
	} else {
		log.AnonIdentityID = owner.AnonIdentityID
	}
	if err := s.logs.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("mood create: %w", err)
	}
	return log, nil
}

func (s *service) List(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
	logs, err := s.logs.ListByOwner(ctx, owner, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *service) Latest(ctx context.Context, owner middleware.Owner) (*MoodLog, error) {
	return s.logs.LatestByOwner(ctx, owner)
}

func (s *service) Count(ctx context.Context, owner middleware.Owner) (int64, error) {
	return s.logs.CountByOwner(ctx, owner)
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
