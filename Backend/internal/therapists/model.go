package therapists

import (
	"errors"
	"time"
)

// List pagination defaults, shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// The pricing column is literally price_kes, so no currency column is stored.
const SessionCurrency = "KES"

var (
	// ErrTherapistNotFound reports a profile lookup that matched no therapist.
	ErrTherapistNotFound = errors.New("therapist not found")
)

// Therapist is the public wire shape for therapist discovery and profiles.
// Schema internals that stay off the wire (credentials, years_exp, photo_url,
// location, free_sessions) are never selected into the response.
type Therapist struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	Bio          *string   `json:"bio"`
	Languages    []string  `json:"languages"`
	Specialties  []string  `json:"specialties"`
	SessionPrice *int      `json:"session_price"`
	Currency     string    `json:"currency"`
	IsActive     bool      `json:"is_active"`
	IsOnlineOnly bool      `json:"is_online_only"`
	CreatedAt    time.Time `json:"-"`
}

// ListOptions carries the optional directory filters for therapist discovery.
// A non-empty filter matches case-insensitively against a language row or a
// specialty array element containing the search text.
type ListOptions struct {
	Language  string
	Specialty string
}

// ClampLimit applies the page-size defaults/ceiling to a raw limit.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
