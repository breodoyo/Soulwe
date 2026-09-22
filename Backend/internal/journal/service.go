package journal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"Backend/internal/cipher"
)

// Service is the journal business-logic boundary. It owns the input
// validation, the AES-GCM contract (content encrypted before it ever reaches
// the repository, decrypted only to return the authenticated user's own
// entry), and the server-side AI reflection. It never constructs SQL and it
// never logs journal content.
type Service interface {
	// Create validates input, encrypts content, stores the entry, and — when
	// a reflection generator is configured — attempts a reflection. A failing
	// or missing generator never fails the create: the entry is saved with
	// ai_reflection = nil, exactly as documented. The returned entry never
	// carries plaintext content on the wire.
	Create(ctx context.Context, userID, content string, moodTags []string, promptUsed *string) (*JournalEntry, error)

	// List returns the user's entries newest first, clamped to a sane page
	// size, optionally resuming from a created_at cursor (exclusive).
	List(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error)

	// Get returns the user's entry with its content decrypted server-side, or
	// ErrJournalEntryNotFound.
	Get(ctx context.Context, userID, entryID string) (*JournalEntry, error)

	// Update applies the provided fields to the user's entry. Content changes
	// are re-encrypted and invalidate any stored AI reflection; metadata-only
	// changes preserve it. Returns ErrNothingToUpdate when no field was
	// provided and ErrJournalEntryNotFound when the entry is not the user's.
	Update(ctx context.Context, userID, entryID string, changes Update) (*JournalEntry, error)

	// Delete permanently removes the user's entry. Returns
	// ErrJournalEntryNotFound when the entry is not the user's.
	Delete(ctx context.Context, userID, entryID string) error

	// Reflect decrypts the user's entry, generates a fresh AI reflection and
	// persists it. Returns ErrAIReflectionUnavailable when no generator is
	// configured or Claude fails; ErrJournalEntryNotFound when the entry is
	// not the user's.
	Reflect(ctx context.Context, userID, entryID string) (*JournalEntry, error)
}

// Update carries the optional fields of PATCH /journal/:id. A nil field
// leaves that attribute untouched. PromptUsed follows the profile convention:
// nil keeps the current value, "" clears it to null.
type Update struct {
	Content    *string
	MoodTags   *[]string
	PromptUsed *string
}

type service struct {
	entries    Repository
	codec      *cipher.AESGCM
	reflection ReflectionGenerator
}

// NewService wires the journal service to a repository, the AES-GCM codec and
// an optional reflection generator. The generator may be nil when
// ANTHROPIC_API_KEY is unset.
func NewService(entries Repository, codec *cipher.AESGCM, reflection ReflectionGenerator) *service {
	return &service{entries: entries, codec: codec, reflection: reflection}
}

func (s *service) Create(ctx context.Context, userID, content string, moodTags []string, promptUsed *string) (*JournalEntry, error) {
	content = strings.TrimSpace(content)
	if err := validateContent(content); err != nil {
		return nil, err
	}

	tags, err := cleanMoodTags(moodTags)
	if err != nil {
		return nil, err
	}
	prompt, err := normalizePrompt(promptUsed)
	if err != nil {
		return nil, err
	}

	enc, iv, err := s.codec.Encrypt([]byte(content), []byte(userID))
	if err != nil {
		return nil, fmt.Errorf("journal encrypt: %w", err)
	}

	entry := &JournalEntry{
		UserID:     userID,
		ContentEnc: enc,
		ContentIV:  iv,
		MoodTags:   tags,
		PromptUsed: prompt,
		WordCount:  wordCount(content),
	}

	// Reflection is best-effort on create: a missing or failing Claude call
	// must not lose the user's journal entry. ai_reflection stays null.
	entry.AIReflection = s.generateReflection(ctx, content, tags)

	if err := s.entries.Create(ctx, entry); err != nil {
		return nil, err
	}
	// The create response carries no plaintext, ciphertext, or ownership:
	// the row is already persisted, so drop the wire-sensitive fields.
	entry.ContentEnc = nil
	entry.ContentIV = nil
	entry.UserID = ""
	return entry, nil
}

// generateReflection invokes the configured generator, never letting a failure
// escape. Journal content is intentionally not included in logs.
func (s *service) generateReflection(ctx context.Context, content string, tags []string) *string {
	if s.reflection == nil {
		return nil
	}
	reflection, err := s.reflection.GenerateReflection(ctx, content, tags)
	if err != nil {
		slog.Error("journal reflection skipped", slog.String("error", err.Error()))
		return nil
	}
	reflection = strings.TrimSpace(reflection)
	if reflection == "" {
		return nil
	}
	return &reflection
}

func (s *service) List(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
	entries, err := s.entries.ListByUserID(ctx, userID, ClampLimit(limit), before)
	if err != nil {
		return nil, err
	}
	// Content stays empty in list responses; only the single-entry endpoints
	// decrypt and return it. Ciphertext and the ownership field are also
	// stripped at the boundary so the wire can never accidentally include them.
	cleaned := make([]JournalEntry, 0, len(entries))
	for _, e := range entries {
		e.UserID = ""
		e.ContentEnc = nil
		e.ContentIV = nil
		cleaned = append(cleaned, e)
	}
	return cleaned, nil
}

func (s *service) Get(ctx context.Context, userID, entryID string) (*JournalEntry, error) {
	entry, err := s.entries.GetByID(ctx, userID, entryID)
	if err != nil {
		return nil, err
	}
	if err := s.decrypt(entry, userID); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *service) Update(ctx context.Context, userID, entryID string, changes Update) (*JournalEntry, error) {
	if changes.Content == nil && changes.MoodTags == nil && changes.PromptUsed == nil {
		return nil, ErrNothingToUpdate
	}

	existing, err := s.entries.GetByID(ctx, userID, entryID)
	if err != nil {
		return nil, err
	}

	merged := *existing
	contentChanged := false

	if changes.Content != nil {
		content := strings.TrimSpace(*changes.Content)
		if err := validateContent(content); err != nil {
			return nil, err
		}
		// Re-encrypt only when the text actually changed, so editing without
		// touching the words preserves the stored AI reflection.
		plain, err := s.codec.Decrypt(existing.ContentEnc, existing.ContentIV, []byte(userID))
		if err != nil {
			return nil, fmt.Errorf("journal decrypt: %w", err)
		}
		if string(plain) != content {
			enc, iv, err := s.codec.Encrypt([]byte(content), []byte(userID))
			if err != nil {
				return nil, fmt.Errorf("journal encrypt: %w", err)
			}
			merged.ContentEnc = enc
			merged.ContentIV = iv
			contentChanged = true
		}
		merged.WordCount = wordCount(content)
	}

	if changes.MoodTags != nil {
		tags, err := cleanMoodTags(*changes.MoodTags)
		if err != nil {
			return nil, err
		}
		merged.MoodTags = tags
	}

	if changes.PromptUsed != nil {
		// A pointer to "" clears prompt_used back to null.
		prompt, err := normalizePrompt(changes.PromptUsed)
		if err != nil {
			return nil, err
		}
		merged.PromptUsed = prompt
	}

	if err := s.entries.Update(ctx, userID, entryID, &merged, contentChanged); err != nil {
		return nil, err
	}
	if err := s.decrypt(&merged, userID); err != nil {
		return nil, err
	}
	return &merged, nil
}

func (s *service) Delete(ctx context.Context, userID, entryID string) error {
	return s.entries.Delete(ctx, userID, entryID)
}

func (s *service) Reflect(ctx context.Context, userID, entryID string) (*JournalEntry, error) {
	if s.reflection == nil {
		return nil, ErrAIReflectionUnavailable
	}

	entry, err := s.Get(ctx, userID, entryID)
	if err != nil {
		return nil, err
	}

	reflection, err := s.reflection.GenerateReflection(ctx, entry.Content, entry.MoodTags)
	if err != nil {
		slog.Error("journal reflection failed", slog.String("error", err.Error()))
		return nil, ErrAIReflectionUnavailable
	}
	reflection = strings.TrimSpace(reflection)
	if reflection == "" {
		return nil, ErrAIReflectionUnavailable
	}

	if err := s.entries.UpdateReflection(ctx, userID, entryID, reflection); err != nil {
		return nil, err
	}
	entry.AIReflection = &reflection
	return entry, nil
}

// decrypt replaces the entry's ciphertext with the plaintext content, scoped
// to the owning user's authenticated-data tag.
func (s *service) decrypt(entry *JournalEntry, userID string) error {
	plain, err := s.codec.Decrypt(entry.ContentEnc, entry.ContentIV, []byte(userID))
	if err != nil {
		return fmt.Errorf("journal decrypt: %w", err)
	}
	entry.Content = string(plain)
	return nil
}

func validateContent(content string) error {
	if content == "" {
		return ErrInvalidContent
	}
	if utf8.RuneCountInString(content) > MaxContentLength {
		return ErrInvalidContent
	}
	return nil
}

// cleanMoodTags trims, drops empties and deduplicates while preserving order,
// then enforces the tag count and length caps. An empty result is valid (a
// plain journal entry with no tags).
func cleanMoodTags(tags []string) ([]string, error) {
	cleaned := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > MaxMoodTagLength {
			return nil, ErrInvalidMoodTags
		}
		seen[tag] = true
		cleaned = append(cleaned, tag)
	}
	if len(cleaned) > MaxMoodTags {
		return nil, ErrInvalidMoodTags
	}
	return cleaned, nil
}

// normalizePrompt trims the prompt identifier and clears empty values to null.
func normalizePrompt(prompt *string) (*string, error) {
	if prompt == nil {
		return nil, nil
	}
	promptText := strings.TrimSpace(*prompt)
	if promptText == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(promptText) > MaxPromptLength {
		return nil, ErrInvalidPromptUsed
	}
	return &promptText, nil
}

// wordCount counts space-separated tokens in the trimmed content.
func wordCount(content string) int {
	return len(strings.Fields(content))
}

// ensure the concrete service satisfies the interface.
var _ Service = (*service)(nil)
