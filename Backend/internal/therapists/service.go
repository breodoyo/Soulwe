package therapists

import (
	"context"
	"time"
)

// Service is the therapist-discovery business-logic boundary.
type Service interface {
	// List returns the directory newest first, clamped to a sane page size and
	// optionally filtered by language/specialty. Empty slice (not nil) on no match.
	List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error)

	// Get returns one therapist's public profile, or ErrTherapistNotFound.
	Get(ctx context.Context, therapistID string) (*Therapist, error)
}

type service struct {
	therapists Repository
}

func NewService(therapists Repository) *service {
	return &service{therapists: therapists}
}

func (s *service) List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error) {
	return s.therapists.List(ctx, opts, ClampLimit(limit), before)
}

func (s *service) Get(ctx context.Context, therapistID string) (*Therapist, error) {
	return s.therapists.Get(ctx, therapistID)
}
