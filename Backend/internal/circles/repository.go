package circles

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the circles domain needs.
// Implementations only touch the database; they contain no business logic.
// All anonIdentityID arguments are anonymous-identity UUIDs derived from the
// authenticated session — never raw tokens, device UUIDs, or user IDs.
type Repository interface {
	// ListCircles returns all active circles, newest irrelevant, ordered by
	// name, each with its live member count. It returns an empty slice (not
	// nil) when there are none.
	ListCircles(ctx context.Context) ([]Circle, error)

	// GetCircle returns one active circle with its member count, or
	// ErrCircleNotFound.
	GetCircle(ctx context.Context, circleID string) (*Circle, error)

	// IsMember reports whether the identity has joined the circle. A missing
	// circle reports false (existence is checked separately by the service).
	IsMember(ctx context.Context, circleID, anonIdentityID string) (bool, error)

	// AddMember records the identity's membership. Returns ErrAlreadyMember on
	// a duplicate (circle_id, anon_identity_id) row.
	AddMember(ctx context.Context, circleID, anonIdentityID string) error

	// RemoveMember deletes the identity's membership. Deleting a membership
	// that does not exist is a no-op success, keeping leave idempotent.
	RemoveMember(ctx context.Context, circleID, anonIdentityID string) error

	// ListMessages returns the circle's messages newest first, limited to
	// limit rows, optionally resuming from a created_at cursor (exclusive).
	// It returns an empty slice (not nil) when the circle has none.
	ListMessages(ctx context.Context, circleID string, limit int, before *time.Time) ([]CircleMessage, error)

	// CreateMessage stores a message from the identity in the circle and
	// returns it with the author's anon_name resolved server-side.
	CreateMessage(ctx context.Context, circleID, anonIdentityID, content string) (*CircleMessage, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const circleColumns = "c.id, c.slug, c.name, c.description, c.icon, COUNT(cm.id)::int AS member_count"

const (
	listCirclesSQL = `SELECT ` + circleColumns + `
		FROM circles c
		LEFT JOIN circle_members cm ON cm.circle_id = c.id
		WHERE c.is_active = TRUE
		GROUP BY c.id, c.slug, c.name, c.description, c.icon
		ORDER BY c.name ASC`

	getCircleSQL = `SELECT ` + circleColumns + `
		FROM circles c
		LEFT JOIN circle_members cm ON cm.circle_id = c.id
		WHERE c.id = $1 AND c.is_active = TRUE
		GROUP BY c.id, c.slug, c.name, c.description, c.icon`

	isMemberSQL = `SELECT EXISTS (
		SELECT 1 FROM circle_members WHERE circle_id = $1 AND anon_identity_id = $2)`

	addMemberSQL = `INSERT INTO circle_members (circle_id, anon_identity_id)
		VALUES ($1, $2) RETURNING id`

	removeMemberSQL = `DELETE FROM circle_members
		WHERE circle_id = $1 AND anon_identity_id = $2`

	// The before cursor is an exclusive bound on created_at; ORDER BY includes
	// id as a deterministic tiebreaker for identical timestamps.
	listMessagesSQL = `SELECT m.id, a.anon_name, m.content, m.reaction_counts, m.created_at
		FROM circle_messages m
		JOIN anon_identities a ON a.id = m.anon_identity_id
		WHERE m.circle_id = $1
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2`

	listMessagesBeforeSQL = `SELECT m.id, a.anon_name, m.content, m.reaction_counts, m.created_at
		FROM circle_messages m
		JOIN anon_identities a ON a.id = m.anon_identity_id
		WHERE m.circle_id = $1 AND m.created_at < $3
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2`

	// One round trip: insert the row, then join the author name for the wire
	// response. reaction_counts keeps the column default '{}'.
	createMessageSQL = `WITH inserted AS (
			INSERT INTO circle_messages (circle_id, anon_identity_id, content)
			VALUES ($1, $2, $3)
			RETURNING id, content, created_at, reaction_counts
		)
		SELECT i.id, a.anon_name, i.content, i.reaction_counts, i.created_at
		FROM inserted i
		JOIN anon_identities a ON a.id = $2`
)

func (r *PostgresRepository) ListCircles(ctx context.Context) ([]Circle, error) {
	rows, err := r.pool.Query(ctx, listCirclesSQL)
	if err != nil {
		return nil, fmt.Errorf("circles list: %w", err)
	}
	defer rows.Close()

	circles := make([]Circle, 0)
	for rows.Next() {
		var c Circle
		if err := scanCircle(rows.Scan, &c); err != nil {
			return nil, fmt.Errorf("circles list: scan: %w", err)
		}
		circles = append(circles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("circles list: %w", err)
	}
	return circles, nil
}

func (r *PostgresRepository) GetCircle(ctx context.Context, circleID string) (*Circle, error) {
	circle := &Circle{}
	err := scanCircle(r.pool.QueryRow(ctx, getCircleSQL, circleID).Scan, circle)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCircleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("circles get: %w", err)
	}
	return circle, nil
}

func (r *PostgresRepository) IsMember(ctx context.Context, circleID, anonIdentityID string) (bool, error) {
	var isMember bool
	if err := r.pool.QueryRow(ctx, isMemberSQL, circleID, anonIdentityID).Scan(&isMember); err != nil {
		return false, fmt.Errorf("circles is member: %w", err)
	}
	return isMember, nil
}

func (r *PostgresRepository) AddMember(ctx context.Context, circleID, anonIdentityID string) error {
	var id string
	err := r.pool.QueryRow(ctx, addMemberSQL, circleID, anonIdentityID).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrAlreadyMember
		}
		return fmt.Errorf("circles add member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RemoveMember(ctx context.Context, circleID, anonIdentityID string) error {
	if _, err := r.pool.Exec(ctx, removeMemberSQL, circleID, anonIdentityID); err != nil {
		return fmt.Errorf("circles remove member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListMessages(ctx context.Context, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
	query := listMessagesSQL
	args := []any{circleID, limit}
	if before != nil {
		query = listMessagesBeforeSQL
		args = append(args, *before)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("circles list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]CircleMessage, 0)
	for rows.Next() {
		var m CircleMessage
		if err := scanMessage(rows.Scan, &m); err != nil {
			return nil, fmt.Errorf("circles list messages: scan: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("circles list messages: %w", err)
	}
	return messages, nil
}

func (r *PostgresRepository) CreateMessage(ctx context.Context, circleID, anonIdentityID, content string) (*CircleMessage, error) {
	message := &CircleMessage{}
	err := scanMessage(r.pool.QueryRow(ctx, createMessageSQL, circleID, anonIdentityID, content).Scan, message)
	if err != nil {
		return nil, fmt.Errorf("circles create message: %w", err)
	}
	return message, nil
}

// scanCircle shares one column decoder between list/get. IsMember is a
// business property and is never sourced from SQL.
func scanCircle(s func(dest ...any) error, c *Circle) error {
	return s(&c.ID, &c.Slug, &c.Name, &c.Description, &c.Icon, &c.MemberCount)
}

// scanMessage shares one column decoder between list/create.
func scanMessage(s func(dest ...any) error, m *CircleMessage) error {
	return s(&m.ID, &m.AnonName, &m.Content, &m.ReactionCounts, &m.CreatedAt)
}
