package bookings

import (
	"context"
	"time"
)


type Service interface {
	Create(ctx context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error)

	List(ctx context.Context, userID string) ([]Booking, error)

	Get(ctx context.Context, userID, bookingID string) (*Booking, error)

	Cancel(ctx context.Context, userID, bookingID string) (*Booking, error)
}

type service struct {
	bookings Repository
}

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
