package mood

import (
	"errors"
	"time"
)

// ValidMoods is the product's canonical mood vocabulary. The values match the
// marker text used across the Soulwe app and are stored verbatim in the
// mood_logs.mood column (the column is free-form TEXT; validation happens at
// the application boundary so the schema stays unchanged).
var ValidMoods = []string{"Heavy", "Okay", "Better", "At peace", "Grateful"}

// validMoodSet indexes ValidMoods for O(1) membership checks.
var validMoodSet = makeSet(ValidMoods)

// Sentinel errors returned by the mood domain. Handlers map these to safe,
// client-facing HTTP responses; they never contain user input.
var (
	// ErrInvalidMood reports a mood value outside the product vocabulary.
	ErrInvalidMood = errors.New("invalid mood value")
)

// MoodLog mirrors the mood_logs table (db/migrations/004_create_mood_logs.up.sql).
// UserID is never serialized back to the client — ownership is derived from the
// authenticated JWT and the response carries only id, mood, and logged_at.
type MoodLog struct {
	ID       string    `json:"id"`
	UserID   string    `json:"-"`
	Mood     string    `json:"mood"`
	LoggedAt time.Time `json:"logged_at"`
}

// IsValidMood reports whether mood is one of the documented values.
func IsValidMood(mood string) bool {
	return validMoodSet[mood]
}

func makeSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}
