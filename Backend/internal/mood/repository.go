package mood

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the mood domain needs.
// Implementations only touch the database; they contain no business logic.
//
// Every method is scoped by Owner, which carries exactly one identity (a
// registered user or an anonymous session). The owner predicates below are
// written so that a row is only ever returned to the identity that owns it:
// the non-matching owner column is compared against NULL, and because the
// schema forbids a row with both or neither set, a caller can never widen the
// scope by omitting its own identity.
type Repository interface {
	// Create inserts a check-in for the given owner and fills in the database
	// generated fields (id, logged_at) on the passed value.
	Create(ctx context.Context, log *MoodLog) error

	// ListByOwner returns the owner's check-ins, newest first, limited to
	// limit rows. It returns an empty slice (not nil) when they have none.
	ListByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error)

	// LatestByOwner returns the owner's most recent check-in, or nil when they
	// have none.
	LatestByOwner(ctx context.Context, owner middleware.Owner) (*MoodLog, error)

	// CountByOwner returns the total number of the owner's check-ins.
	CountByOwner(ctx context.Context, owner middleware.Owner) (int64, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const moodColumns = "id, user_id, anon_identity_id, mood, logged_at"

// ownerPredicate scopes a statement to a single identity. The NULL comparison
// is what makes this safe: with exactly-one-owner enforced by the schema, a
// caller holding a UserID can only match rows whose anon_identity_id IS NULL,
// and vice versa.
const ownerPredicate = `user_id IS NOT DISTINCT FROM $1 AND anon_identity_id IS NOT DISTINCT FROM $2`

const (
	createMoodSQL = `
		INSERT INTO mood_logs (user_id, anon_identity_id, mood)
		VALUES ($1, $2, $3)
		RETURNING ` + moodColumns

	// logged_at DESC with an id tiebreaker ('id DESC') makes "newest first"
	// deterministic even when several check-ins share the same timestamp.
	listMoodsSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE ` + ownerPredicate +
		` ORDER BY logged_at DESC, id DESC LIMIT $3`

	latestMoodSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE ` + ownerPredicate +
		` ORDER BY logged_at DESC, id DESC LIMIT 1`

	countMoodsSQL = "SELECT COUNT(*) FROM mood_logs WHERE " + ownerPredicate
)

// ownerArgs converts an owner into the two bind parameters every statement
// expects. The boolean reports whether the owner was well-formed; an
// ill-formed owner must never reach the database.
func ownerArgs(owner middleware.Owner) (any, any, bool) {
	// IdentityFromOwner is the single place the exactly-one-owner invariant is
	// enforced, so an ambiguous owner (both IDs set) is rejected here instead
	// of silently resolving to whichever branch came first.
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, nil, false
	}
	if owner.Registered() {
		return owner.UserID, nil, true
	}
	return nil, owner.AnonIdentityID, true
}

// scanLog decodes one mood_logs row.
//
// The two owner columns are mutually exclusive, so exactly one of them is NULL.
// pgx refuses to scan a NULL into a plain *string ("cannot scan NULL into
// *string"), so the owner columns are decoded as **string and then flattened
// into the model's non-pointer fields.
func scanLog(s func(dest ...any) error, log *MoodLog) error {
	var userID, anonID *string
	if err := s(&log.ID, &userID, &anonID, &log.Mood, &log.LoggedAt); err != nil {
		return err
	}
	assignOwner(userID, anonID, &log.UserID, &log.AnonIdentityID)
	return nil
}

// assignOwner copies the two scanned owner columns onto the model, leaving the
// unset side as the empty string.
func assignOwner(userID, anonID *string, dstUser, dstAnon *string) {
	if userID != nil {
		*dstUser = *userID
	}
	if anonID != nil {
		*dstAnon = *anonID
	}
}

// Create inserts a new mood check-in. The mood value is expected to be
// pre-validated by the service layer; the repository stores it verbatim.
func (r *PostgresRepository) Create(ctx context.Context, log *MoodLog) error {
	userID, anonID, ok := ownerArgs(middleware.Owner{
		UserID:         log.UserID,
		AnonIdentityID: log.AnonIdentityID,
	})
	if !ok {
		return errors.New("mood create: no owner")
	}

	if err := scanLog(r.pool.QueryRow(ctx, createMoodSQL, userID, anonID, log.Mood).Scan, log); err != nil {
		return fmt.Errorf("mood create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("mood list: no owner")
	}

	rows, err := r.pool.Query(ctx, listMoodsSQL, userID, anonID, limit)
	if err != nil {
		return nil, fmt.Errorf("mood list: %w", err)
	}
	defer rows.Close()

	logs := make([]MoodLog, 0)
	for rows.Next() {
		var log MoodLog
		if err := scanLog(rows.Scan, &log); err != nil {
			return nil, fmt.Errorf("mood list: scan: %w", err)
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mood list: %w", err)
	}
	return logs, nil
}

func (r *PostgresRepository) LatestByOwner(ctx context.Context, owner middleware.Owner) (*MoodLog, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("mood latest: no owner")
	}

	log := &MoodLog{}
	err := scanLog(r.pool.QueryRow(ctx, latestMoodSQL, userID, anonID).Scan, log)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mood latest: %w", err)
	}
	return log, nil
}

func (r *PostgresRepository) CountByOwner(ctx context.Context, owner middleware.Owner) (int64, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return 0, errors.New("mood count: no owner")
	}

	var count int64
	if err := r.pool.QueryRow(ctx, countMoodsSQL, userID, anonID).Scan(&count); err != nil {
		return 0, fmt.Errorf("mood count: %w", err)
	}
	return count, nil
}
