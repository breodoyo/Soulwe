package therapists

import (
	"context"
	"time"
)

// Service is the therapist-discovery business-logic boundary. Implementations
// own list-page clamping; they never construct SQL.
type Service interface {
	// List returns the therapist directory newest first, clamped to a sane
	// page size and optionally filtered by language/specialty. It returns an
	// empty slice (not nil) when nothing matches.
	List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error)

	// Get returns one therapist's public profile, or ErrTherapistNotFound.
	Get(ctx context.Context, therapistID string) (*Therapist, error)
}

type service struct {
	therapists Repository
}

// NewService wires the therapist service to a therapist repository.
func NewService(therapists Repository) *service {
	return &service{therapists: therapists}
}

func (s *service) List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error) {
	return s.therapists.List(ctx, opts, ClampLimit(limit), before)
}

func (s *service) Get(ctx context.Context, therapistID string) (*Therapist, error) {
	return s.therapists.Get(ctx, therapistID)
}
