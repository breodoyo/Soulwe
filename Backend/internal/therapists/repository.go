package therapists

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the therapists domain needs.
// The directory is a public catalog, so reads are never scoped to a user and
// there is no ownership to enforce.
type Repository interface {
	// List returns the directory newest first, limited to limit rows, optionally
	// filtered by language and/or specialty and resumed from an exclusive
	// created_at cursor. Returns an empty slice (not nil) when nothing matches.
	List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error)

	// Get returns a single therapist by id, or ErrTherapistNotFound. Existence is
	// not scoped to is_active, so a previously-listed therapist still resolves.
	Get(ctx context.Context, therapistID string) (*Therapist, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// therapistColumns is the select list shared by list and get. Languages are
// aggregated into a single array and specialties are normalized to a non-null
// array so the scanned Go slice is never nil.
const therapistColumns = `t.id, t.full_name, t.bio,
	COALESCE(t.specialties, ARRAY[]::TEXT[]),
	t.price_kes, t.is_active, t.is_online_only, t.created_at,
	COALESCE((SELECT array_agg(tl.language ORDER BY tl.language)
		FROM therapist_languages tl WHERE tl.therapist_id = t.id), ARRAY[]::TEXT[])`

func (r *PostgresRepository) List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error) {
	query := `
		SELECT ` + therapistColumns + `
		FROM therapists t`
	args := make([]any, 0, 4)
	clauses := make([]string, 0, 3)

	if language := strings.TrimSpace(opts.Language); language != "" {
		args = append(args, "%"+language+"%")
		clauses = append(clauses, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM therapist_languages tl WHERE tl.therapist_id = t.id AND tl.language ILIKE $%d)`, len(args)))
	}
	if specialty := strings.TrimSpace(opts.Specialty); specialty != "" {
		args = append(args, "%"+specialty+"%")
		clauses = append(clauses, fmt.Sprintf(
			`t.specialties IS NOT NULL AND EXISTS (SELECT 1 FROM unnest(t.specialties) s WHERE s ILIKE $%d)`, len(args)))
	}
	if before != nil {
		args = append(args, *before)
		clauses = append(clauses, fmt.Sprintf(`t.created_at < $%d`, len(args)))
	}
	if len(clauses) > 0 {
		query += "\n\tWHERE " + strings.Join(clauses, " AND ")
	}

	// The id tiebreaker makes "newest first" deterministic for same-second inserts.
	query += "\n\tORDER BY t.created_at DESC, t.id DESC"
	args = append(args, limit)
	query += fmt.Sprintf(` LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("therapists list: %w", err)
	}
	defer rows.Close()

	therapists := make([]Therapist, 0)
	for rows.Next() {
		var th Therapist
		if err := scanTherapist(rows.Scan, &th); err != nil {
			return nil, fmt.Errorf("therapists list: scan: %w", err)
		}
		therapists = append(therapists, th)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("therapists list: %w", err)
	}
	return therapists, nil
}

func (r *PostgresRepository) Get(ctx context.Context, therapistID string) (*Therapist, error) {
	th := &Therapist{}
	err := scanTherapist(r.pool.QueryRow(ctx,
		`SELECT `+therapistColumns+`
		FROM therapists t
		WHERE t.id = $1`, therapistID).Scan, th)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTherapistNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("therapists get: %w", err)
	}
	return th, nil
}

// scanTherapist shares one column decoder between list and get; currency is a
// fixed product constant applied at decode time.
func scanTherapist(s func(dest ...any) error, th *Therapist) error {
	th.Currency = SessionCurrency
	return s(&th.ID, &th.DisplayName, &th.Bio, &th.Specialties, &th.SessionPrice,
		&th.IsActive, &th.IsOnlineOnly, &th.CreatedAt, &th.Languages)
}
