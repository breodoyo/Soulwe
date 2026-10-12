package bookings

import (
	"errors"
	"time"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"
)

const SessionDuration = 60 * time.Minute

var (
	ErrBookingNotFound = errors.New("booking not found")
	ErrBookingConflict = errors.New("booking time conflict")

	ErrBookingStatusConflict = errors.New("booking is not in a cancellable status")

	ErrTherapistMissing = errors.New("therapist not found")

	ErrTherapistInactive = errors.New("therapist unavailable")

	ErrScheduledInPast = errors.New("scheduled_at must be in the future")
)

type Booking struct {
	ID          string    `json:"id"`
	UserID      string    `json:"-"`
	TherapistID string    `json:"therapist_id"`
	DisplayName string    `json:"display_name"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
