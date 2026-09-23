package breathing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the breathing domain needs.
// Implementations only touch the database; they contain no business logic.
// Every session read/write is scoped by userID so a caller can never observe
// or write another user's sessions.
type Repository interface {
	// ListExercises returns the curated catalog in its defined (insertion)
	// order, limited to limit rows. It returns an empty slice (not nil) when
	// the catalog is empty.
	ListExercises(ctx context.Context, limit int) ([]Exercise, error)

	// GetExercise returns a single catalog exercise by id, or
	// ErrExerciseNotFound.
	GetExercise(ctx context.Context, exerciseID string) (*Exercise, error)

	// CreateSession inserts a session and fills in the database-generated
	// fields (id, created_at) on the passed value.
	CreateSession(ctx context.Context, session *Session) error

	// ListSessionsByUserID returns the user's sessions, newest first, limited
	// to limit rows. It returns an empty slice (not nil) when the user has
	// none.
	ListSessionsByUserID(ctx context.Context, userID string, limit int) ([]Session, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const exerciseColumns = `id, slug, name, description, technique, inhale_s, hold_s, exhale_s, created_at`

const (
	listExercisesSQL = `SELECT ` + exerciseColumns +
		` FROM breathing_exercises ORDER BY created_at, id LIMIT $1`

	getExerciseSQL = `SELECT ` + exerciseColumns +
		` FROM breathing_exercises WHERE id = $1`

	createSessionSQL = `INSERT INTO breathing_sessions
		(user_id, technique, breaths, duration_s, completed, exercise_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, technique, breaths, duration_s, completed, created_at, exercise_id`

	// The LEFT JOIN keeps legacy device sessions (NULL exercise_id) listable
	// with a nil display name. created_at DESC with an id tiebreaker makes
	// "newest first" deterministic even when several sessions share a
	// timestamp.
	listSessionsSQL = `SELECT s.id, s.user_id, s.technique, e.name,
		s.breaths, s.duration_s, s.completed, s.created_at, s.exercise_id
		FROM breathing_sessions s
		LEFT JOIN breathing_exercises e ON e.id = s.exercise_id
		WHERE s.user_id = $1
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $2`
)

func (r *PostgresRepository) ListExercises(ctx context.Context, limit int) ([]Exercise, error) {
	rows, err := r.pool.Query(ctx, listExercisesSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("breathing exercises list: %w", err)
	}
	defer rows.Close()

	exercises := make([]Exercise, 0)
	for rows.Next() {
		var e Exercise
		if err := scanExercise(rows.Scan, &e); err != nil {
			return nil, fmt.Errorf("breathing exercises list: scan: %w", err)
		}
		exercises = append(exercises, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("breathing exercises list: %w", err)
	}
	return exercises, nil
}

func (r *PostgresRepository) GetExercise(ctx context.Context, exerciseID string) (*Exercise, error) {
	e := &Exercise{}
	err := scanExercise(r.pool.QueryRow(ctx, getExerciseSQL, exerciseID).Scan, e)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExerciseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("breathing exercises get: %w", err)
	}
	return e, nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session *Session) error {
	err := r.pool.QueryRow(ctx, createSessionSQL,
		session.UserID, session.Technique, session.Breaths, session.DurationS,
		session.Completed, session.ExerciseID,
	).Scan(
		&session.ID, &session.UserID, &session.Technique, &session.Breaths,
		&session.DurationS, &session.Completed, &session.CreatedAt, &session.ExerciseID,
	)
	if err != nil {
		return fmt.Errorf("breathing session create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListSessionsByUserID(ctx context.Context, userID string, limit int) ([]Session, error) {
	rows, err := r.pool.Query(ctx, listSessionsSQL, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("breathing sessions list: %w", err)
	}
	defer rows.Close()

	sessions := make([]Session, 0)
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Technique, &s.Name,
			&s.Breaths, &s.DurationS, &s.Completed, &s.CreatedAt, &s.ExerciseID); err != nil {
			return nil, fmt.Errorf("breathing sessions list: scan: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("breathing sessions list: %w", err)
	}
	return sessions, nil
}

// scanExercise shares one column decoder between list and get.
func scanExercise(s func(dest ...any) error, e *Exercise) error {
	return s(&e.ID, &e.Slug, &e.Name, &e.Description, &e.Technique,
		&e.InhaleS, &e.HoldS, &e.ExhaleS, &e.CreatedAt)
}
