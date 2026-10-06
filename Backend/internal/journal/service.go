package journal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"Backend/internal/cipher"
	"Backend/internal/middleware"
)

// Service is the journal business-logic boundary: it owns input validation, the
// AES-GCM contract, and the server-side AI reflection. It never builds SQL and
// never logs journal content.
type Service interface {
	// Create validates input, encrypts content, stores the entry, and attempts a
	// reflection. A failing or missing generator never fails the create: the entry
	// is saved with ai_reflection = nil. The returned entry carries no plaintext.
	Create(ctx context.Context, owner middleware.Owner, content string, moodTags []string, promptUsed *string) (*JournalEntry, error)

	// List returns the owner's entries newest first, clamped to a sane page size,
	// optionally resuming from an exclusive created_at cursor.
	List(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error)

	// Get returns the owner's entry with its content decrypted server-side, or
	// ErrJournalEntryNotFound.
	Get(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)

	// Update applies the provided fields to the owner's entry. Content changes are
	// re-encrypted and invalidate any stored AI reflection; metadata-only changes
	// preserve it. Returns ErrNothingToUpdate when no field was provided and
	// ErrJournalEntryNotFound when the entry is not the owner's.
	Update(ctx context.Context, owner middleware.Owner, entryID string, changes Update) (*JournalEntry, error)

	// Delete permanently removes the owner's entry, or returns
	// ErrJournalEntryNotFound when the entry is not the owner's.
	Delete(ctx context.Context, owner middleware.Owner, entryID string) error

	// Reflect decrypts the owner's entry, generates a fresh AI reflection and
	// persists it. Returns ErrAIReflectionUnavailable when no generator is
	// configured or Claude fails; ErrJournalEntryNotFound when the entry is not
	// the owner's.
	Reflect(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)
}

// Update carries the optional fields of PATCH /journal/:id. A nil field leaves
// that attribute untouched; PromptUsed follows the profile convention where ""
// clears it to null.
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

// NewService wires the service; reflection may be nil when ANTHROPIC_API_KEY is unset.
func NewService(entries Repository, codec *cipher.AESGCM, reflection ReflectionGenerator) *service {
	return &service{entries: entries, codec: codec, reflection: reflection}
}

// encryptionAAD returns the AES-GCM associated data for an owner's entries.
//
// A registered user's AAD is the bare user ID, fixed because entries encrypted
// before anonymous support was added already use it; changing it would make
// existing journals undecryptable. An anonymous session is namespaced with an
// "anon:" prefix so it can never share an AAD with a user whose ID happens to be
// the same UUID.
func encryptionAAD(owner middleware.Owner) ([]byte, error) {
	// An ambiguous owner must never pick one identity's AAD and be written as the other.
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, errors.New("journal: no single owner")
	}
	if owner.Registered() {
		return []byte(owner.UserID), nil
	}
	return []byte("anon:" + owner.AnonIdentityID), nil
}

func applyOwner(entry *JournalEntry, owner middleware.Owner) {
	entry.UserID = ""
	entry.AnonIdentityID = ""
	if owner.Registered() {
		entry.UserID = owner.UserID
	} else {
		entry.AnonIdentityID = owner.AnonIdentityID
	}
}

func (s *service) Create(ctx context.Context, owner middleware.Owner, content string, moodTags []string, promptUsed *string) (*JournalEntry, error) {
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

	aad, err := encryptionAAD(owner)
	if err != nil {
		return nil, err
	}

	enc, iv, err := s.codec.Encrypt([]byte(content), aad)
	if err != nil {
		return nil, fmt.Errorf("journal encrypt: %w", err)
	}

	entry := &JournalEntry{
		ContentEnc: enc,
		ContentIV:  iv,
		MoodTags:   tags,
		PromptUsed: prompt,
		WordCount:  wordCount(content),
	}
	applyOwner(entry, owner)

	// Reflection is best-effort: a missing or failing call must not lose the entry.
	entry.AIReflection = s.generateReflection(ctx, content, tags)

	if err := s.entries.Create(ctx, entry); err != nil {
		return nil, err
	}
	// The row is persisted, so drop the wire-sensitive fields from the response.
	entry.ContentEnc = nil
	entry.ContentIV = nil
	entry.UserID = ""
	entry.AnonIdentityID = ""
	return entry, nil
}

// generateReflection never lets a failure escape; journal content is not logged.
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

func (s *service) List(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error) {
	entries, err := s.entries.ListByOwner(ctx, owner, ClampLimit(limit), before)
	if err != nil {
		return nil, err
	}
	// Strip ciphertext and ownership here too, so the wire can never carry them.
	cleaned := make([]JournalEntry, 0, len(entries))
	for _, e := range entries {
		e.UserID = ""
		e.AnonIdentityID = ""
		e.ContentEnc = nil
		e.ContentIV = nil
		cleaned = append(cleaned, e)
	}
	return cleaned, nil
}

func (s *service) Get(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error) {
	entry, err := s.entries.GetByID(ctx, owner, entryID)
	if err != nil {
		return nil, err
	}
	if err := s.decrypt(entry, owner); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *service) Update(ctx context.Context, owner middleware.Owner, entryID string, changes Update) (*JournalEntry, error) {
	if changes.Content == nil && changes.MoodTags == nil && changes.PromptUsed == nil {
		return nil, ErrNothingToUpdate
	}

	existing, err := s.entries.GetByID(ctx, owner, entryID)
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
		aad, err := encryptionAAD(owner)
		if err != nil {
			return nil, err
		}
		// Re-encrypt only when the text changed, so an unchanged edit keeps the reflection.
		plain, err := s.codec.Decrypt(existing.ContentEnc, existing.ContentIV, aad)
		if err != nil {
			return nil, fmt.Errorf("journal decrypt: %w", err)
		}
		if string(plain) != content {
			enc, iv, err := s.codec.Encrypt([]byte(content), aad)
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

	if err := s.entries.Update(ctx, owner, entryID, &merged, contentChanged); err != nil {
		return nil, err
	}
	if err := s.decrypt(&merged, owner); err != nil {
		return nil, err
	}
	return &merged, nil
}

func (s *service) Delete(ctx context.Context, owner middleware.Owner, entryID string) error {
	return s.entries.Delete(ctx, owner, entryID)
}

func (s *service) Reflect(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error) {
	if s.reflection == nil {
		return nil, ErrAIReflectionUnavailable
	}

	entry, err := s.Get(ctx, owner, entryID)
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

	if err := s.entries.UpdateReflection(ctx, owner, entryID, reflection); err != nil {
		return nil, err
	}
	entry.AIReflection = &reflection
	return entry, nil
}

// decrypt replaces the ciphertext with plaintext. Since the AAD is owner-specific,
// a successful decrypt is itself proof the caller owns the row.
func (s *service) decrypt(entry *JournalEntry, owner middleware.Owner) error {
	aad, err := encryptionAAD(owner)
	if err != nil {
		return err
	}
	plain, err := s.codec.Decrypt(entry.ContentEnc, entry.ContentIV, aad)
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

func wordCount(content string) int {
	return len(strings.Fields(content))
}

// ensure the concrete service satisfies the interface.
var _ Service = (*service)(nil)
