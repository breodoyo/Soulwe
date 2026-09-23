package bookings

import (
	"errors"
	"time"
)

// Booking statuses, matching the CHECK constraint in migration 014. A booking
// is created pending, can only be cancelled while pending, and conservative
// interpretations of the other statuses (confirmed, completed) are left to
// future phases that add therapist-side flows.
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"
)

// SessionDuration is the assumed length of a booked therapy session. The
// schema stores only a start time, so "overlapping sessions" for the same user
// are detected around this fixed window.
const SessionDuration = 60 * time.Minute

// Sentinel errors returned by the bookings domain. Handlers map each to a safe
// HTTP response; none of them embed database details.
var (
	// ErrBookingNotFound reports a booking lookup or update that matched no
	// booking owned by the caller.
	ErrBookingNotFound = errors.New("booking not found")

	// ErrBookingConflict reports a time that collides with a live booking: an
	// overlapping 60-minute window for the same user, or for the same
	// therapist from any user.
	ErrBookingConflict = errors.New("booking time conflict")

	// ErrBookingStatusConflict reports an illegal status transition such as
	// cancelling a booking that is no longer pending.
	ErrBookingStatusConflict = errors.New("booking is not in a cancellable status")

	// ErrTherapistMissing reports a booking attempt for a therapist id that
	// does not exist.
	ErrTherapistMissing = errors.New("therapist not found")

	// ErrTherapistInactive reports a booking attempt for a therapist that is
	// not currently accepting bookings.
	ErrTherapistInactive = errors.New("therapist is not accepting bookings")

	// ErrScheduledInPast reports a scheduled_at that is not strictly in the
	// future.
	ErrScheduledInPast = errors.New("scheduled_at must be in the future")
)

// Booking is the wire shape of one session booking. The owner is carried on
// the struct for repository writes and scoping but is never serialized; the
// therapist is represented by id plus display_name (from therapists.full_name)
// so list/get responses are self-describing without exposing owner identity.
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
