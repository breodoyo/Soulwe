package therapists

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

// SessionCurrency is the currency of every therapist's session price. The
// schema's pricing column is literally price_kes, so prices are always
// denominated in Kenyan Shillings and no currency column needs to be stored.
const SessionCurrency = "KES"

// Sentinel errors returned by the therapist domain. Handlers map these to safe
// HTTP responses; they never embed database details.
var (
	// ErrTherapistNotFound reports a profile lookup that matched no therapist.
	ErrTherapistNotFound = errors.New("therapist not found")
)

// Therapist is the public wire shape for therapist discovery and profiles.
// Only public profile facts are serialized: id, display_name (from full_name),
// bio, languages, specialties, session_price, currency, and the availability
// flags. Schema internals that stay off the wire (credentials, years_exp,
// photo_url, location, free_sessions) are deliberately never selected into the
// response.
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
// A filter applies only when non-empty and matches case-insensitively against
// a language row (or a specialty array element) containing the search text.
type ListOptions struct {
	Language  string
	Specialty string
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
