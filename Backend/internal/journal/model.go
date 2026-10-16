package journal

import (
	"errors"
	"time"
)

const (
	MaxContentLength = 10000
	MaxMoodTags      = 10
	MaxMoodTagLength = 50
	MaxPromptLength  = 200
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

var (
	ErrJournalEntryNotFound    = errors.New("journal entry not found")
	ErrInvalidContent          = errors.New("invalid journal content")
	ErrInvalidMoodTags         = errors.New("invalid mood tags")
	ErrInvalidPromptUsed       = errors.New("invalid prompt used")
	ErrNothingToUpdate         = errors.New("nothing to update")
	ErrAIReflectionUnavailable = errors.New("ai reflection is unavailable")
)

const FallbackReflection = "Thank you for putting this into words. Noticing how you feel is a gentle act of care for yourself."

type JournalEntry struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	AnonIdentityID string    `json:"-"`
	Content        string    `json:"content,omitempty"`
	ContentEnc     []byte    `json:"-"`
	ContentIV      []byte    `json:"-"`
	MoodTags       []string  `json:"mood_tags"`
	PromptUsed     *string   `json:"prompt_used"`
	AIReflection   *string   `json:"ai_reflection"`
	WordCount      int       `json:"word_count"`
	CreatedAt      time.Time `json:"created_at"`
}
