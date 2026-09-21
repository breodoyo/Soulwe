package mood

import (
	"context"
	"fmt"
	"strings"
)

// List defaults and caps, matching the pagination conventions documented for
// other authenticated collections (a simple limit with a sane maximum).
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// Service is the mood-check-in business-logic boundary. Implementations own
// mood validation and list-page clamping; they never construct SQL.
type Service interface {
	// Create validates the mood value and records a check-in for the given
	// user. The user ID always comes from the authenticated JWT context, never
	// from client input. Returns ErrInvalidMood for a value outside the
	// product vocabulary.
	Create(ctx context.Context, userID, mood string) (*MoodLog, error)

	// List returns the user's check-ins newest first, clamped to a sane page
	// size. It returns an empty slice (not nil) when the user has none.
	List(ctx context.Context, userID string, limit int) ([]MoodLog, error)

	// Latest returns the user's most recent check-in, or nil when they have
	// none. Exposed to the dashboard so it can surface the latest mood without
	// a dedicated /moods/latest endpoint.
	Latest(ctx context.Context, userID string) (*MoodLog, error)

	// Count returns the total number of the user's check-ins.
	Count(ctx context.Context, userID string) (int64, error)
}

type service struct {
	logs Repository
}

// NewService wires the mood service to a mood repository.
func NewService(logs Repository) *service {
	return &service{logs: logs}
}

func (s *service) Create(ctx context.Context, userID, mood string) (*MoodLog, error) {
	mood = strings.TrimSpace(mood)
	if !IsValidMood(mood) {
		return nil, ErrInvalidMood
	}

	log := &MoodLog{UserID: userID, Mood: mood}
	if err := s.logs.Create(ctx, log); err != nil {
		return nil, fmt.Errorf("mood create: %w", err)
	}
	return log, nil
}

func (s *service) List(ctx context.Context, userID string, limit int) ([]MoodLog, error) {
	logs, err := s.logs.ListByUserID(ctx, userID, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *service) Latest(ctx context.Context, userID string) (*MoodLog, error) {
	return s.logs.LatestByUserID(ctx, userID)
}

func (s *service) Count(ctx context.Context, userID string) (int64, error) {
	return s.logs.CountByUserID(ctx, userID)
}

// clampLimit applies the documented page-size defaults/ceiling to a raw
// client-supplied limit. Non-positive values fall back to the default; large
// values are capped rather than rejected.
func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
