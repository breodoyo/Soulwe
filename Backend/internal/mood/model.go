package mood

import (
	"errors"
	"time"
)

// ValidMoods is the canonical mood vocabulary; mood_logs.mood is free-form TEXT.
var ValidMoods = []string{"Heavy", "Okay", "Better", "At peace", "Grateful"}

var validMoodSet = makeSet(ValidMoods)

var (
	// ErrInvalidMood reports a mood value outside the product vocabulary.
	ErrInvalidMood = errors.New("invalid mood value")
	// ErrInvalidOwner is a programming-error guard: HTTP rejects it before the service.
	ErrInvalidOwner = errors.New("mood: request has no single owner")
)

// MoodLog mirrors mood_logs; exactly one of UserID / AnonIdentityID is set.
type MoodLog struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	AnonIdentityID string    `json:"-"`
	Mood           string    `json:"mood"`
	LoggedAt       time.Time `json:"logged_at"`
}

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
