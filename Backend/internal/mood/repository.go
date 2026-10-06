package mood

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	// Create inserts a check-in and fills in generated fields (id, logged_at) on log.
	Create(ctx context.Context, log *MoodLog) error

	// ListByOwner returns the owner's check-ins newest first; empty slice, not nil.
	ListByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error)

	// LatestByOwner returns the owner's most recent check-in, or nil.
	LatestByOwner(ctx context.Context, owner middleware.Owner) (*MoodLog, error)

	CountByOwner(ctx context.Context, owner middleware.Owner) (int64, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const moodColumns = "id, user_id, anon_identity_id, mood, logged_at"

// The NULL comparison makes scoping safe given the schema's exactly-one-owner rule.
const ownerPredicate = `user_id IS NOT DISTINCT FROM $1 AND anon_identity_id IS NOT DISTINCT FROM $2`

const (
	createMoodSQL = `
		INSERT INTO mood_logs (user_id, anon_identity_id, mood)
		VALUES ($1, $2, $3)
		RETURNING ` + moodColumns

	// id DESC breaks same-timestamp ties so "newest first" is deterministic.
	listMoodsSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE ` + ownerPredicate +
		` ORDER BY logged_at DESC, id DESC LIMIT $3`

	latestMoodSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE ` + ownerPredicate +
		` ORDER BY logged_at DESC, id DESC LIMIT 1`

	countMoodsSQL = "SELECT COUNT(*) FROM mood_logs WHERE " + ownerPredicate
)

func ownerArgs(owner middleware.Owner) (any, any, bool) {
	// IdentityFromOwner enforces exactly-one-owner, so ambiguous owners are rejected here.
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
// Owner columns are decoded as *string because pgx cannot scan NULL into *string.
func scanLog(s func(dest ...any) error, log *MoodLog) error {
	var userID, anonID *string
	if err := s(&log.ID, &userID, &anonID, &log.Mood, &log.LoggedAt); err != nil {
		return err
	}
	assignOwner(userID, anonID, &log.UserID, &log.AnonIdentityID)
	return nil
}

func assignOwner(userID, anonID *string, dstUser, dstAnon *string) {
	if userID != nil {
		*dstUser = *userID
	}
	if anonID != nil {
		*dstAnon = *anonID
	}
}

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
