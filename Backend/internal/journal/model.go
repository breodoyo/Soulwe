package journal

import (
	"context"
	"errors"
	"time"
)

// Content/validation limits for journal entries; the database schema is unchanged.
const (
	// MaxContentLength caps content at 10,000 Unicode code points.
	MaxContentLength = 10000
	// MaxMoodTags caps how many free-form tags a single entry may carry.
	MaxMoodTags = 10
	// MaxMoodTagLength caps each tag's length in runes.
	MaxMoodTagLength = 50
	// MaxPromptLength caps the optional prompt identifier text.
	MaxPromptLength = 200
)

// List pagination defaults, shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// ClampLimit applies the page-size defaults; non-positive values fall back to the
// default and large values are capped rather than rejected.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

// Sentinel errors returned by the journal domain; none embed user content.
var (
	ErrJournalEntryNotFound    = errors.New("journal entry not found")
	ErrInvalidContent          = errors.New("invalid journal content")
	ErrInvalidMoodTags         = errors.New("invalid mood tags")
	ErrInvalidPromptUsed       = errors.New("invalid prompt used")
	ErrNothingToUpdate         = errors.New("nothing to update")
	ErrAIReflectionUnavailable = errors.New("ai reflection is unavailable")
)

// JournalEntry mirrors journal_entries. Exactly one of UserID or AnonIdentityID
// is set (journalling must not require registering); ownership, ContentEnc and
// ContentIV are never serialized, and Content stays empty outside detail
// responses, so no plaintext ever reaches the wire by accident.
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

// ReflectionGenerator isolates the Claude client behind a seam testable without a key.
type ReflectionGenerator interface {
	GenerateReflection(ctx context.Context, content string, moodTags []string) (string, error)
}
