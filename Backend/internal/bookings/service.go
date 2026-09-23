package bookings

import (
	"context"
	"time"
)

// Service is the bookings business-logic boundary. It owns the validation and
// conflict rules (future-times, active therapist, overlap detection); the
// repository owns the SQL. Both layers defend against double-booking: the
// service's overlap pre-check gives the common serial cases a fast path, and
// the database constraints (unique indexes for exact minutes plus GiST EXCLUDE
// guards for overlapping 60-minute windows) make the guarantee hold under any
// concurrency.
type Service interface {
	// Create books a pending session for the user with an active therapist,
	// validating that scheduled_at is in the future and does not collide with
	// another live booking (an overlapping 60-minute window for the same user,
	// or for the same therapist regardless of user).
	Create(ctx context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error)

	// List returns the user's own bookings newest first.
	List(ctx context.Context, userID string) ([]Booking, error)

	// Get returns one of the user's own bookings, or ErrBookingNotFound.
	Get(ctx context.Context, userID, bookingID string) (*Booking, error)

	// Cancel transitions a pending booking to cancelled and returns the
	// refreshed row. A non-pending booking yields ErrBookingStatusConflict.
	Cancel(ctx context.Context, userID, bookingID string) (*Booking, error)
}

type service struct {
	bookings Repository
}

// NewService wires the bookings service to a bookings repository.
func NewService(bookings Repository) *service {
	return &service{bookings: bookings}
}

func (s *service) Create(ctx context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error) {
	if !scheduledAt.After(time.Now()) {
		return nil, ErrScheduledInPast
	}

	found, active, err := s.bookings.TherapistStatus(ctx, therapistID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrTherapistMissing
	}
	if !active {
		return nil, ErrTherapistInactive
	}

	overlaps, err := s.bookings.HasActiveOverlap(ctx, userID, scheduledAt)
	if err != nil {
		return nil, err
	}
	if overlaps {
		return nil, ErrBookingConflict
	}

	b := &Booking{
		UserID:      userID,
		TherapistID: therapistID,
		ScheduledAt: scheduledAt,
		Status:      StatusPending,
	}
	if err := s.bookings.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *service) List(ctx context.Context, userID string) ([]Booking, error) {
	return s.bookings.ListByUserID(ctx, userID)
}

func (s *service) Get(ctx context.Context, userID, bookingID string) (*Booking, error) {
	return s.bookings.GetByID(ctx, userID, bookingID)
}

func (s *service) Cancel(ctx context.Context, userID, bookingID string) (*Booking, error) {
	return s.bookings.SetStatus(ctx, userID, bookingID, StatusPending, StatusCancelled)
}
