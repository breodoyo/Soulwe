package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the journal domain's persistence operations.
//
// Every method is scoped by Owner, which carries exactly one identity (a
// registered user or an anonymous session). Ownership predicates compare the
// non-matching owner column against NULL, and the schema guarantees a row has
// exactly one owner, so a caller cannot widen the scope by omitting its own
// identity. Journalling should not require registering.
type Repository interface {
	// Create inserts an entry and fills in the generated fields (id, created_at).
	Create(ctx context.Context, entry *JournalEntry) error

	// ListByOwner returns the owner's entries newest first; the optional before
	// cursor is an exclusive created_at bound. It returns an empty slice when the
	// owner has none.
	ListByOwner(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error)

	// GetByID returns the owner's entry by ID, or ErrJournalEntryNotFound.
	GetByID(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error)

	// Update applies the merged values on entry and returns the refreshed row.
	// contentChanged tells the SQL whether content fields and word_count should be
	// replaced and the stale ai_reflection cleared. Returns
	// ErrJournalEntryNotFound when the entry is not the owner's.
	Update(ctx context.Context, owner middleware.Owner, entryID string, entry *JournalEntry, contentChanged bool) error

	// UpdateReflection stores a reflection on the owner's entry, or returns
	// ErrJournalEntryNotFound when the entry is not the owner's.
	UpdateReflection(ctx context.Context, owner middleware.Owner, entryID, reflection string) error

	// Delete permanently removes the owner's entry, or returns
	// ErrJournalEntryNotFound when the entry is not the owner's.
	Delete(ctx context.Context, owner middleware.Owner, entryID string) error
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Columns for SELECT / RETURNING; decryption is a service-layer concern.
const journalColumns = `id, user_id, anon_identity_id, content_enc, content_iv, mood_tags, prompt_used, ai_reflection, word_count, created_at`

// ownerPredicate scopes a statement to a single identity. The NULL comparison
// is what makes this safe: with exactly-one-owner enforced by the schema, a
// caller holding a UserID can only match rows whose anon_identity_id IS NULL.
const ownerPredicate = `user_id IS NOT DISTINCT FROM $1 AND anon_identity_id IS NOT DISTINCT FROM $2`

const (
	createJournalSQL = `
		INSERT INTO journal_entries (user_id, anon_identity_id, content_enc, content_iv, mood_tags, prompt_used, word_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
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
			mood_tags      = $6,
			prompt_used    = $8,
			ai_reflection  = CASE WHEN $7 THEN NULL ELSE ai_reflection END
		WHERE id = $3 AND ` + ownerPredicate + `
		RETURNING ` + journalColumns

	deleteJournalSQL = `DELETE FROM journal_entries WHERE id = $3 AND ` + ownerPredicate + ` RETURNING id`

	updateReflectionSQL = `UPDATE journal_entries SET ai_reflection = $4 WHERE id = $3 AND ` + ownerPredicate + ` RETURNING id`
)

// ownerArgs converts an owner into the two bind parameters every statement expects.
func ownerArgs(owner middleware.Owner) (any, any, bool) {
	// IdentityFromOwner is the single place the exactly-one-owner invariant is
	// enforced, so an ambiguous owner is rejected here instead of silently
	// resolving to whichever branch came first.
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

	err := r.pool.QueryRow(ctx, createJournalSQL,
		userID, anonID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, entry.PromptUsed, entry.WordCount,
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

func (r *PostgresRepository) GetByID(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("journal get: no owner")
	}

	entry := &JournalEntry{}
	err := scanEntry(r.pool.QueryRow(ctx, getJournalSQL, userID, anonID, entryID).Scan, entry)
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
	err := scanEntry(r.pool.QueryRow(ctx, updateJournalSQL,
		userID, anonID, entryID, entry.ContentEnc, entry.ContentIV, entry.MoodTags, contentChanged, entry.PromptUsed, entry.WordCount,
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

	var updatedID string
	err := r.pool.QueryRow(ctx, updateReflectionSQL, userID, anonID, entryID, reflection).Scan(&updatedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJournalEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("journal update reflection: %w", err)
	}
	return nil
}

// scanEntry shares one column decoder between list/get/update: pgx's Row.Scan and
// Rows.Scan both satisfy the func(...any) error shape. The mutually exclusive
// owner columns decode as **string, since pgx cannot scan NULL into *string.
func scanEntry(s func(dest ...any) error, e *JournalEntry) error {
	var userID, anonID *string
	if err := s(&e.ID, &userID, &anonID, &e.ContentEnc, &e.ContentIV, &e.MoodTags, &e.PromptUsed, &e.AIReflection, &e.WordCount, &e.CreatedAt); err != nil {
		return err
	}
	if userID != nil {
		e.UserID = *userID
	}
	if anonID != nil {
		e.AnonIdentityID = *anonID
	}
	return nil
}
