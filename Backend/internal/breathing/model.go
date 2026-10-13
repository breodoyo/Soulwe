package breathing

import (
	"errors"
	"time"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

var (
	ErrExerciseNotFound = errors.New("breathing exercise not found")

	ErrInvalidOwner = errors.New("breathing: request has no single owner")
)


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

func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
