package journal

import (
	"context"
	"errors"
	"time"
)

// Content/validation limits for journal entries. These are application-level
// guards; the database schema stays unchanged.
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

// List pagination defaults, matching the documented convention (default 20,
// maximum 50) shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// ClampLimit applies the documented page-size defaults/ceiling to a raw
// client-supplied limit. Non-positive values fall back to the default; large
// values are capped rather than rejected. Exported so the handler can compute
// the next_cursor from the effective page size.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

// Sentinel errors returned by the journal domain. Handlers map these to safe
// HTTP responses; none of them embed user content or secrets.
var (
	ErrJournalEntryNotFound    = errors.New("journal entry not found")
	ErrInvalidContent          = errors.New("invalid journal content")
	ErrInvalidMoodTags         = errors.New("invalid mood tags")
	ErrInvalidPromptUsed       = errors.New("invalid prompt used")
	ErrNothingToUpdate         = errors.New("nothing to update")
	ErrAIReflectionUnavailable = errors.New("ai reflection is unavailable")
)

// JournalEntry mirrors the journal_entries table
// (db/migrations/003_create_journal_entries.up.sql).
//
// Content shipping between the service and the wire:
//   - Content is only populated after a server-side decryption (detail,
//     update and reflect responses). List responses and the create response
//     never carry it — the `omitempty` tag keeps plaintext off the wire.
//   - ContentEnc/ContentIV are the AES-256-GCM ciphertext and its fresh
//     nonce; they are stored in the database and never serialized.
//   - UserID is always derived from the authenticated JWT, never from a
//     request body, and is never serialized.
//
// The schema has no journal-type column: the product defines a single
// regular journal entry. No artificial "type" vocabulary is introduced.
type JournalEntry struct {
	ID           string    `json:"id"`
	UserID       string    `json:"-"`
	Content      string    `json:"content,omitempty"`
	ContentEnc   []byte    `json:"-"`
	ContentIV    []byte    `json:"-"`
	MoodTags     []string  `json:"mood_tags"`
	PromptUsed   *string   `json:"prompt_used"`
	AIReflection *string   `json:"ai_reflection"`
	WordCount    int       `json:"word_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// ReflectionGenerator turns journal content into a short server-side
// reflection. It isolates the Anthropic client behind a tiny seam so the
// service can be fully tested without a real API key.
type ReflectionGenerator interface {
	GenerateReflection(ctx context.Context, content string, moodTags []string) (string, error)
}
