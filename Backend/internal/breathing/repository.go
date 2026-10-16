package breathing

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

type Repository interface {
	
	ListExercises(ctx context.Context, limit int) ([]Exercise, error)

	GetExercise(ctx context.Context, exerciseID string) (*Exercise, error)

	CreateSession(ctx context.Context, session *Session) error

	ListSessionsByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

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
		(user_id, anon_identity_id, technique, breaths, duration_s, completed, exercise_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, anon_identity_id, technique, breaths, duration_s, completed, created_at, exercise_id`

	listSessionsSQL = `SELECT s.id, s.user_id, s.anon_identity_id, s.technique, e.name,
		s.breaths, s.duration_s, s.completed, s.created_at, s.exercise_id
		FROM breathing_sessions s
		LEFT JOIN breathing_exercises e ON e.id = s.exercise_id
		WHERE s.user_id IS NOT DISTINCT FROM $1 AND s.anon_identity_id IS NOT DISTINCT FROM $2
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $3`
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
	userID, anonID, ok := ownerArgs(middleware.Owner{
		UserID:         session.UserID,
		AnonIdentityID: session.AnonIdentityID,
	})
	if !ok {
		return errors.New("breathing session create: no owner")
	}

	if err := scanCreatedSession(r.pool.QueryRow(ctx, createSessionSQL,
		userID, anonID, session.Technique, session.Breaths, session.DurationS,
		session.Completed, session.ExerciseID,
	).Scan, session); err != nil {
		return fmt.Errorf("breathing session create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListSessionsByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("breathing sessions list: no owner")
	}

	rows, err := r.pool.Query(ctx, listSessionsSQL, userID, anonID, limit)
	if err != nil {
		return nil, fmt.Errorf("breathing sessions list: %w", err)
	}
	defer rows.Close()

	sessions := make([]Session, 0)
	for rows.Next() {
		var s Session
		if err := scanListedSession(rows.Scan, &s); err != nil {
			return nil, fmt.Errorf("breathing sessions list: scan: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("breathing sessions list: %w", err)
	}
	return sessions, nil
}

func scanCreatedSession(s func(dest ...any) error, session *Session) error {
	var userID, anonID *string
	if err := s(&session.ID, &userID, &anonID, &session.Technique, &session.Breaths,
		&session.DurationS, &session.Completed, &session.CreatedAt, &session.ExerciseID); err != nil {
		return err
	}
	assignOwner(userID, anonID, &session.UserID, &session.AnonIdentityID)
	return nil
}

func scanListedSession(s func(dest ...any) error, session *Session) error {
	var userID, anonID *string
	if err := s(&session.ID, &userID, &anonID, &session.Technique, &session.Name,
		&session.Breaths, &session.DurationS, &session.Completed, &session.CreatedAt,
		&session.ExerciseID); err != nil {
		return err
	}
	assignOwner(userID, anonID, &session.UserID, &session.AnonIdentityID)
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

// scanExercise shares one column decoder between list and get.
func scanExercise(s func(dest ...any) error, e *Exercise) error {
	return s(&e.ID, &e.Slug, &e.Name, &e.Description, &e.Technique,
		&e.InhaleS, &e.HoldS, &e.ExhaleS, &e.CreatedAt)
}
