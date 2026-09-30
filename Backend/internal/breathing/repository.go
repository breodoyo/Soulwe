package breathing

import (
	"context"
	"errors"
	"fmt"

	"Backend/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ownerArgs converts an owner into the two bind parameters every session
// statement expects. The boolean reports whether the owner was well-formed; an
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

// Repository defines the persistence operations the breathing domain needs.
// Implementations only touch the database; they contain no business logic.
// Every session read/write is scoped by Owner, which carries exactly one
// identity (a registered user or an anonymous session), so a caller can never
// observe or write another identity's sessions.
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

	// ListSessionsByOwner returns the owner's sessions, newest first, limited to
	// limit rows. It returns an empty slice (not nil) when the owner has none.
	ListSessionsByOwner(ctx context.Context, owner middleware.Owner, limit int) ([]Session, error)
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
		(user_id, anon_identity_id, technique, breaths, duration_s, completed, exercise_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, anon_identity_id, technique, breaths, duration_s, completed, created_at, exercise_id`

	// The LEFT JOIN keeps legacy device sessions (NULL exercise_id) listable
	// with a nil display name. created_at DESC with an id tiebreaker makes
	// "newest first" deterministic even when several sessions share a
	// timestamp.
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

// scanCreatedSession decodes the RETURNING clause of createSessionSQL, which
// has no exercise name to give back.
//
// The two owner columns are mutually exclusive, so exactly one of them is NULL.
// pgx refuses to scan a NULL into a plain *string ("cannot scan NULL into
// *string"), so they are decoded as **string and then flattened onto the
// session.
func scanCreatedSession(s func(dest ...any) error, session *Session) error {
	var userID, anonID *string
	if err := s(&session.ID, &userID, &anonID, &session.Technique, &session.Breaths,
		&session.DurationS, &session.Completed, &session.CreatedAt, &session.ExerciseID); err != nil {
		return err
	}
	assignOwner(userID, anonID, &session.UserID, &session.AnonIdentityID)
	return nil
}

// scanListedSession decodes a row of listSessionsSQL, which adds the exercise
// name from the LEFT JOIN.
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

// assignOwner copies the two scanned owner columns onto the session, leaving the
// unset side as the empty string.
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
