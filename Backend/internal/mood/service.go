package mood

import (
	"context"
	"fmt"
	"strings"

	"Backend/internal/middleware"
)

// List defaults and caps, matching the pagination conventions documented for
// other authenticated collections (a simple limit with a sane maximum).
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// Service is the mood-check-in business-logic boundary. Implementations own
// mood validation and list-page clamping; they never construct SQL.
//
// Check-ins belong to an Owner, which is either a registered user or an
// anonymous session — checking in on yourself must not require registering.
type Service interface {
	// Create validates the mood value and records a check-in for the given
	// owner. The owner always comes from the authenticated context, never from
	// client input. Returns ErrInvalidMood for a value outside the product
	// vocabulary.
	Create(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error)

	// List returns the owner's check-ins newest first, clamped to a sane page
	// size. It returns an empty slice (not nil) when they have none.
	List(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error)

	// Latest returns the owner's most recent check-in, or nil when they have
	// none. Exposed to the dashboard so it can surface the latest mood without
	// a dedicated /moods/latest endpoint.
	Latest(ctx context.Context, owner middleware.Owner) (*MoodLog, error)

	// Count returns the total number of the owner's check-ins.
	Count(ctx context.Context, owner middleware.Owner) (int64, error)
}

type service struct {
	logs Repository
}

// NewService wires the mood service to a mood repository.
func NewService(logs Repository) *service {
	return &service{logs: logs}
}

func (s *service) Create(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error) {
	mood = strings.TrimSpace(mood)
	if !IsValidMood(mood) {
		return nil, ErrInvalidMood
	}
	// The owner is stamped onto the row as a single column, so an ambiguous
	// owner (both identities set) would silently write as one of them. Reject
	// it here rather than guessing which identity meant the request.
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
