package breathing

import (
	"errors"
	"time"
)

// List pagination defaults, matching the documented convention (default 20,
// maximum 50) shared with the other authenticated collections.
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
)

// Exercise is the public wire shape for breathing-exercise discovery. Only
// public catalog facts are serialized: id, slug, name, description,
// technique, and the guided phase durations. created_at is a schema internal
// and stays off the wire.
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

// Session mirrors a breathing_sessions row (db/migrations/009). UserID is
// never serialized back to the client — ownership is derived from the
// authenticated JWT. Name is the denormalized exercise display name, joined
// from the catalog for history reads and stamped directly when recording a
// session; it is nil for legacy device sessions without an exercise link.
type Session struct {
	ID         string    `json:"id"`
	UserID     string    `json:"-"`
	ExerciseID *string   `json:"exercise_id"`
	Technique  string    `json:"technique"`
	Name       *string   `json:"name,omitempty"`
	Breaths    int       `json:"breaths"`
	DurationS  int       `json:"duration_s"`
	Completed  bool      `json:"completed"`
	CreatedAt  time.Time `json:"created_at"`
}

// ClampLimit applies the documented page-size defaults/ceiling to a raw
// client-supplied limit. Non-positive values fall back to the default; large
// values are capped rather than rejected.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
