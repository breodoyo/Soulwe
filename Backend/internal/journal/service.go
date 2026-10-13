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

type Service interface {
	Create(ctx context.Context, owner middleware.Owner, content string, moodTags []string, promptUsed *string) (*JournalEntry, error)

	List(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error)

	Get(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)

	Update(ctx context.Context, owner middleware.Owner, entryID string, changes Update) (*JournalEntry, error)

	Delete(ctx context.Context, owner middleware.Owner, entryID string) error

	Reflect(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)
}

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

func NewService(entries Repository, codec *cipher.AESGCM, reflection ReflectionGenerator) *service {
	return &service{entries: entries, codec: codec, reflection: reflection}
}

// encryptionAAD returns the AES-GCM associated data for an owner's entries.
func encryptionAAD(owner middleware.Owner) ([]byte, error) {
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

	entry.AIReflection = s.generateReflection(ctx, content, tags)

	if err := s.entries.Create(ctx, entry); err != nil {
		return nil, err
	}
	entry.ContentEnc = nil
	entry.ContentIV = nil
	entry.UserID = ""
	entry.AnonIdentityID = ""
	return entry, nil
}

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
