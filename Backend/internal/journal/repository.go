package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the journal domain needs.
// Implementations only touch the database; they contain no business logic.
// Every method scopes rows by userID so a caller can never observe another
// user's entries by accident.
type Repository interface {
	// Create inserts an entry and fills in the database generated fields
	// (id, created_at) on the passed value.
	Create(ctx context.Context, entry *JournalEntry) error

	// ListByUserID returns the user's entries, newest first, limited to limit
	// rows. An optional before cursor resumes the page from an earlier
	// created_at (exclusive). It returns an empty slice (not nil) when the
	// user has none.
	ListByUserID(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error)

	// GetByID returns the user's entry by ID, or ErrJournalEntryNotFound.
	GetByID(ctx context.Context, userID, entryID string) (*JournalEntry, error)

	// Update applies the merged values carried on entry (always non-nil for
	// the content fields) and returns the refreshed row. contentChanged tells
	// the SQL whether content fields and word_count should be replaced and the
	// stale ai_reflection cleared. Returns ErrJournalEntryNotFound when the
	// entry is not the user's.
	Update(ctx context.Context, userID, entryID string, entry *JournalEntry, contentChanged bool) error

	// UpdateReflection stores a freshly generated reflection on the user's
	// entry. Returns ErrJournalEntryNotFound when the entry is not the user's.
	UpdateReflection(ctx context.Context, userID, entryID, reflection string) error

	// Delete permanently removes the user's entry. Returns
	// ErrJournalEntryNotFound when the entry is not the user's.
	Delete(ctx context.Context, userID, entryID string) error
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Columns for SELECT / RETURNING. Encrypted content is returned alongside the
// metadata; decryption is a service-layer concern.
const journalColumns = `id, user_id, content_enc, content_iv, mood_tags, prompt_used, ai_reflection, word_count, created_at`

const (
	createJournalSQL = `
		INSERT INTO journal_entries (user_id, content_enc, content_iv, mood_tags, prompt_used, word_count)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	// The before cursor is an exclusive bound on created_at; ORDER BY includes
	// id as a deterministic tiebreaker for identical timestamps.
	listJournalsSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`

	listJournalsBeforeSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE user_id = $1 AND created_at < $3
		  ORDER BY created_at DESC, id DESC LIMIT $2`

	getJournalSQL = "SELECT " + journalColumns +
		` FROM journal_entries WHERE id = $2 AND user_id = $1`

	// When content changed, replace the ciphertext/nonce/count and drop the
	// stale reflection. Otherwise keep the content untouched and preserve the
	// reflection.
	updateJournalSQL = `
		UPDATE journal_entries SET
			content_enc    = CASE WHEN $6 THEN $3 ELSE content_enc END,
			content_iv     = CASE WHEN $6 THEN $4 ELSE content_iv END,
			word_count     = CASE WHEN $6 THEN $8 ELSE word_count END,
			mood_tags      = $5,
			prompt_used    = $7,
			ai_reflection  = CASE WHEN $6 THEN NULL ELSE ai_reflection END
		WHERE id = $2 AND user_id = $1
		RETURNING ` + journalColumns

	deleteJournalSQL = `DELETE FROM journal_entries WHERE id = $2 AND user_id = $1 RETURNING id`

	updateReflectionSQL = `UPDATE journal_entries SET ai_reflection = $3 WHERE id = $2 AND user_id = $1 RETURNING id`
)

func (r *PostgresRepository) Create(ctx context.Context, entry *JournalEntry) error {
	err := r.pool.QueryRow(ctx, createJournalSQL,
		entry.UserID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, entry.PromptUsed, entry.WordCount,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("journal create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByUserID(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
	query := listJournalsSQL
	args := []any{userID, limit}
	if before != nil {
		query = listJournalsBeforeSQL
		args = append(args, *before)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("journal list: %w", err)
	}
	defer rows.Close()

	entries := make([]JournalEntry, 0)
	for rows.Next() {
		var e JournalEntry
		if err := scanEntry(rows.Scan, &e); err != nil {
			return nil, fmt.Errorf("journal list: scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal list: %w", err)
	}
	return entries, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, userID, entryID string) (*JournalEntry, error) {
	entry := &JournalEntry{}
	err := scanEntry(r.pool.QueryRow(ctx, getJournalSQL, userID, entryID).Scan, entry)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJournalEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("journal get: %w", err)
	}
	return entry, nil
}

func (r *PostgresRepository) Update(ctx context.Context, userID, entryID string, entry *JournalEntry, contentChanged bool) error {
	scanned := &JournalEntry{}
	err := scanEntry(r.pool.QueryRow(ctx, updateJournalSQL,
		userID, entryID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, contentChanged, entry.PromptUsed, entry.WordCount,
	).Scan, scanned)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal update: %w", err)
	}
	*entry = *scanned
	return nil
}

func (r *PostgresRepository) Delete(ctx context.Context, userID, entryID string) error {
	var deletedID string
	err := r.pool.QueryRow(ctx, deleteJournalSQL, userID, entryID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal delete: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateReflection(ctx context.Context, userID, entryID, reflection string) error {
	var updatedID string
	err := r.pool.QueryRow(ctx, updateReflectionSQL, userID, entryID, reflection).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal update reflection: %w", err)
	}
	return nil
}

// scanEntry shares one column decoder between list/get/update. pgx's Row.Scan
// and Rows.Scan both satisfy the func(...any) error shape.
func scanEntry(s func(dest ...any) error, e *JournalEntry) error {
	return s(&e.ID, &e.UserID, &e.ContentEnc, &e.ContentIV, &e.MoodTags, &e.PromptUsed, &e.AIReflection, &e.WordCount, &e.CreatedAt)
}
