package mood

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the mood domain needs.
// Implementations only touch the database; they contain no business logic.
// Every method scopes rows by userID so a caller can never observe another
// user's check-ins by accident.
type Repository interface {
	// Create inserts a check-in for the user and fills in the database
	// generated fields (id, logged_at) on the passed value.
	Create(ctx context.Context, log *MoodLog) error

	// ListByUserID returns the user's check-ins, newest first, limited to
	// limit rows. It returns an empty slice (not nil) when the user has none.
	ListByUserID(ctx context.Context, userID string, limit int) ([]MoodLog, error)

	// LatestByUserID returns the user's most recent check-in, or nil when they
	// have none.
	LatestByUserID(ctx context.Context, userID string) (*MoodLog, error)

	// CountByUserID returns the total number of the user's check-ins.
	CountByUserID(ctx context.Context, userID string) (int64, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const moodColumns = "id, user_id, mood, logged_at"

const (
	createMoodSQL = `
		INSERT INTO mood_logs (user_id, mood)
		VALUES ($1, $2)
		RETURNING ` + moodColumns

	// logged_at DESC with an id tiebreaker ('id DESC') makes "newest first"
	// deterministic even when several check-ins share the same timestamp.
	listMoodsSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE user_id = $1 ORDER BY logged_at DESC, id DESC LIMIT $2`

	latestMoodSQL = "SELECT " + moodColumns +
		` FROM mood_logs WHERE user_id = $1 ORDER BY logged_at DESC, id DESC LIMIT 1`

	countMoodsSQL = "SELECT COUNT(*) FROM mood_logs WHERE user_id = $1"
)

// Create inserts a new mood check-in. The mood value is expected to be
// pre-validated by the service layer; the repository stores it verbatim.
func (r *PostgresRepository) Create(ctx context.Context, log *MoodLog) error {
	err := r.pool.QueryRow(ctx, createMoodSQL, log.UserID, log.Mood).Scan(
		&log.ID, &log.UserID, &log.Mood, &log.LoggedAt,
	)
	if err != nil {
		return fmt.Errorf("mood create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListByUserID(ctx context.Context, userID string, limit int) ([]MoodLog, error) {
	rows, err := r.pool.Query(ctx, listMoodsSQL, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("mood list: %w", err)
	}
	defer rows.Close()

	logs := make([]MoodLog, 0)
	for rows.Next() {
		var log MoodLog
		if err := rows.Scan(&log.ID, &log.UserID, &log.Mood, &log.LoggedAt); err != nil {
			return nil, fmt.Errorf("mood list: scan: %w", err)
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mood list: %w", err)
	}
	return logs, nil
}

func (r *PostgresRepository) LatestByUserID(ctx context.Context, userID string) (*MoodLog, error) {
	log := &MoodLog{}
	err := r.pool.QueryRow(ctx, latestMoodSQL, userID).Scan(
		&log.ID, &log.UserID, &log.Mood, &log.LoggedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mood latest: %w", err)
	}
	return log, nil
}

func (r *PostgresRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, countMoodsSQL, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("mood count: %w", err)
	}
	return count, nil
}
