package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"Backend/internal/cipher"
	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, entry *JournalEntry) error

	ListByOwner(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error)

	GetByID(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)

	Update(ctx context.Context, owner middleware.Owner, entryID string, entry *JournalEntry, contentChanged bool) error

	UpdateReflection(ctx context.Context, owner middleware.Owner, entryID, reflection string) error

	Delete(ctx context.Context, owner middleware.Owner, entryID string) error
}

type PostgresRepository struct {
	pool  *pgxpool.Pool
	codec *cipher.AESGCM
}

// NewPostgresRepository wires the repository. The codec encrypts the AI
// reflection with the same AES-256-GCM key and ownership AAD as the entry
// content, so it is required in production wiring.
func NewPostgresRepository(pool *pgxpool.Pool, codec *cipher.AESGCM) *PostgresRepository {
	return &PostgresRepository{pool: pool, codec: codec}
}

const journalColumns = `id, user_id, anon_identity_id, content_enc, content_iv, mood_tags, prompt_used, ai_reflection, ai_reflection_enc, ai_reflection_iv, word_count, created_at`

const ownerPredicate = `user_id IS NOT DISTINCT FROM $1 AND anon_identity_id IS NOT DISTINCT FROM $2`

const (
	createJournalSQL = `
		INSERT INTO journal_entries (user_id, anon_identity_id, content_enc, content_iv, mood_tags, prompt_used, ai_reflection, ai_reflection_enc, ai_reflection_iv, word_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`

	// The before cursor is an exclusive created_at bound; id breaks ties.
	listJournalsSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE ` + ownerPredicate +
		` ORDER BY created_at DESC, id DESC LIMIT $3`

	listJournalsBeforeSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE ` + ownerPredicate +
		` AND created_at < $3 ORDER BY created_at DESC, id DESC LIMIT $4`

	getJournalSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE id = $3 AND ` + ownerPredicate

	// On a content change, replace ciphertext/nonce/count and drop the stale
	// reflection; otherwise keep both.
	updateJournalSQL = `
		UPDATE journal_entries SET
			content_enc    = CASE WHEN $7 THEN $4 ELSE content_enc END,
			content_iv     = CASE WHEN $7 THEN $5 ELSE content_iv END,
			word_count     = CASE WHEN $7 THEN $9 ELSE word_count END,
			mood_tags         = $6,
			prompt_used       = $8,
			ai_reflection     = CASE WHEN $7 THEN NULL ELSE ai_reflection END,
			ai_reflection_enc = CASE WHEN $7 THEN NULL ELSE ai_reflection_enc END,
			ai_reflection_iv  = CASE WHEN $7 THEN NULL ELSE ai_reflection_iv END
		WHERE id = $3 AND ` + ownerPredicate + `
		RETURNING ` + journalColumns

	deleteJournalSQL = `DELETE FROM journal_entries WHERE id = $3 AND ` + ownerPredicate + ` RETURNING id`

	updateReflectionSQL = `
		UPDATE journal_entries SET
			ai_reflection     = $4,
			ai_reflection_enc = $5,
			ai_reflection_iv  = $6
		WHERE id = $3 AND ` + ownerPredicate + `
		RETURNING id`
)

func ownerArgs(owner middleware.Owner) (any, any, bool) {
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, nil, false
	}
	if owner.Registered() {
		return owner.UserID, nil, true
	}
	return nil, owner.AnonIdentityID, true
}

func entryOwner(entry *JournalEntry) (any, any, bool) {
	return ownerArgs(middleware.Owner{UserID: entry.UserID, AnonIdentityID: entry.AnonIdentityID})
}

func (r *PostgresRepository) Create(ctx context.Context, entry *JournalEntry) error {
	userID, anonID, ok := entryOwner(entry)
	if !ok {
		return errors.New("journal create: no owner")
	}

	owner := middleware.Owner{UserID: entry.UserID, AnonIdentityID: entry.AnonIdentityID}
	legacy, enc, iv, err := r.sealReflection(owner, entry.AIReflection)
	if err != nil {
		return fmt.Errorf("journal create reflection: %w", err)
	}

	err = r.pool.QueryRow(ctx, createJournalSQL,
		userID, anonID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, entry.PromptUsed, legacy, enc, iv, entry.WordCount,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("journal create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByOwner(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("journal list: no owner")
	}

	query := listJournalsSQL
	args := []any{userID, anonID, limit}
	if before != nil {
		query = listJournalsBeforeSQL
		args = []any{userID, anonID, *before, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("journal list: %w", err)
	}
	defer rows.Close()

	entries := make([]JournalEntry, 0)
	for rows.Next() {
		var e JournalEntry
		if err := r.scanEntry(rows.Scan, &e, owner); err != nil {
			return nil, fmt.Errorf("journal list: scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal list: %w", err)
	}
	return entries, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("journal get: no owner")
	}

	entry := &JournalEntry{}
	err := r.scanEntry(r.pool.QueryRow(ctx, getJournalSQL, userID, anonID, entryID).Scan, entry, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJournalEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("journal get: %w", err)
	}
	return entry, nil
}

func (r *PostgresRepository) Update(ctx context.Context, owner middleware.Owner, entryID string, entry *JournalEntry, contentChanged bool) error {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return errors.New("journal update: no owner")
	}

	scanned := &JournalEntry{}
	err := r.scanEntry(r.pool.QueryRow(ctx, updateJournalSQL,
		userID, anonID, entryID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, contentChanged, entry.PromptUsed, entry.WordCount,
	).Scan, scanned, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal update: %w", err)
	}
	*entry = *scanned
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, owner middleware.Owner, entryID string) error {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return errors.New("journal delete: no owner")
	}

	var deletedID string
	err := r.pool.QueryRow(ctx, deleteJournalSQL, userID, anonID, entryID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal delete: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateReflection(ctx context.Context, owner middleware.Owner, entryID, reflection string) error {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return errors.New("journal update reflection: no owner")
	}

	legacy, enc, iv, err := r.sealReflection(owner, &reflection)
	if err != nil {
		return fmt.Errorf("journal update reflection: %w", err)
	}

	var updatedID string
	err = r.pool.QueryRow(ctx, updateReflectionSQL, userID, anonID, entryID, legacy, enc, iv).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal update reflection: %w", err)
	}
	return nil
}

// sealReflection prepares an AI reflection for storage. The text is sealed
// into fresh ciphertext and nonce columns, and the legacy plaintext column is
// cleared. A missing codec is an error rather than a plaintext write.
func (r *PostgresRepository) sealReflection(owner middleware.Owner, reflection *string) (legacy *string, enc, iv []byte, err error) {
	if reflection == nil || *reflection == "" {
		return nil, nil, nil, nil
	}
	if r.codec == nil {
		return nil, nil, nil, errors.New("journal: reflection encryption codec is not configured")
	}

	aad, err := encryptionAAD(owner)
	if err != nil {
		return nil, nil, nil, err
	}
	enc, iv, err = r.codec.Encrypt([]byte(*reflection), aad)
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, enc, iv, nil
}

// openReflection returns the plaintext reflection, decrypting the sealed
// columns when present. A legacy plaintext value is returned unchanged so rows
// written before encryption was added keep working without a crash.
func (r *PostgresRepository) openReflection(owner middleware.Owner, legacy *string, enc, iv []byte) (*string, error) {
	if len(enc) == 0 {
		return legacy, nil
	}
	if r.codec == nil {
		return nil, errors.New("journal: reflection is encrypted but no codec is configured")
	}

	aad, err := encryptionAAD(owner)
	if err != nil {
		return nil, err
	}
	plain, err := r.codec.Decrypt(enc, iv, aad)
	if err != nil {
		return nil, fmt.Errorf("journal decrypt reflection: %w", err)
	}
	reflection := string(plain)
	return &reflection, nil
}

func (r *PostgresRepository) scanEntry(s func(dest ...any) error, e *JournalEntry, owner middleware.Owner) error {
	var userID, anonID, legacy *string
	var enc, iv []byte
	if err := s(&e.ID, &userID, &anonID, &e.ContentEnc, &e.ContentIV, &e.MoodTags, &e.PromptUsed, &legacy, &enc, &iv, &e.WordCount, &e.CreatedAt); err != nil {
		return err
	}
	if userID != nil {
		e.UserID = *userID
	}
	if anonID != nil {
		e.AnonIdentityID = *anonID
	}

	reflection, err := r.openReflection(owner, legacy, enc, iv)
	if err != nil {
		return err
	}
	e.AIReflection = reflection
	return nil
}
