package breathing

import (
	"errors"
	"time"
)

// List pagination defaults, matching the convention (default 20, maximum 50)
// shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// Sentinel errors returned by the breathing domain. Handlers map these to
// safe HTTP responses; they never embed database details.
var (
	// ErrExerciseNotFound reports an exercise lookup that matched no catalog
	// exercise.
	ErrExerciseNotFound = errors.New("breathing exercise not found")
	// ErrInvalidOwner reports an owner that is neither exactly a registered user
	// nor exactly an anonymous session. It is a programming-error guard: the
	// HTTP layer refuses such a request before the service is reached.
	ErrInvalidOwner = errors.New("breathing: request has no single owner")
)

// Exercise is the public wire shape for breathing-exercise discovery. Only
// public catalog facts are serialized; created_at stays off the wire.
type Exercise struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Technique   string    `json:"technique"`
	InhaleS     int       `json:"inhale_s"`
	HoldS       int       `json:"hold_s"`
	ExhaleS     int       `json:"exhale_s"`
	CreatedAt   time.Time `json:"-"`
}

// Session mirrors a breathing_sessions row. Exactly one of UserID /
// AnonIdentityID is populated, so a session belongs to a registered user or an
// anonymous one — finishing an exercise need not require registering. Neither
// owner column is serialized. Name is the denormalized exercise display name,
// joined for history reads and stamped when recording; nil for legacy device
// sessions without an exercise link.
type Session struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	AnonIdentityID string    `json:"-"`
	ExerciseID     *string   `json:"exercise_id"`
	Technique      string    `json:"technique"`
	Name           *string   `json:"name,omitempty"`
	Breaths        int       `json:"breaths"`
	DurationS      int       `json:"duration_s"`
	Completed      bool      `json:"completed"`
	CreatedAt      time.Time `json:"created_at"`
}

// ClampLimit applies the page-size defaults/ceiling to a raw client-supplied
// limit: non-positive falls back to the default, large is capped not rejected.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
