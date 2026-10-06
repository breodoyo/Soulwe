package circles

import (
	"context"
	"errors"
	"fmt"
	"time"

	"Backend/internal/middleware"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the circles domain needs.
// Implementations only touch the database; they contain no business logic.
//
// Every owner argument is a middleware.Owner resolved from the request's
// credential: a registered user ID or an anonymous-identity UUID. Owner IDs
// never arrive from a request body or path.
type Repository interface {
	// ListCircles returns all active circles ordered by name, each with its
	// live member count. Membership is a per-caller property filled in by the
	// service, so this needs no owner.
	ListCircles(ctx context.Context) ([]Circle, error)

	// GetCircle returns one active circle with its member count, or
	// ErrCircleNotFound.
	GetCircle(ctx context.Context, circleID string) (*Circle, error)

	// IsMember reports whether the owner has joined; a missing circle reports
	// false, since existence is checked separately by the service.
	IsMember(ctx context.Context, circleID string, owner middleware.Owner) (bool, error)

	// AddMember records the owner's membership, or ErrAlreadyMember on a
	// duplicate (circle_id, owner) row for either kind of owner.
	AddMember(ctx context.Context, circleID string, owner middleware.Owner) error

	// RemoveMember deletes the owner's membership; a membership that does not
	// exist is a no-op success, keeping leave idempotent.
	RemoveMember(ctx context.Context, circleID string, owner middleware.Owner) error

	// ListMessages returns the circle's messages newest first, limited to
	// limit rows, resuming from an exclusive created_at cursor when given.
	ListMessages(ctx context.Context, circleID string, limit int, before *time.Time) ([]CircleMessage, error)

	// CreateMessage stores a message from the owner and returns it with the
	// author's display name resolved server-side.
	CreateMessage(ctx context.Context, circleID string, owner middleware.Owner, content string) (*CircleMessage, error)
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

// sqlDefaultAuthorName is the last-resort author label, quoted for inlining
// into the COALESCE, and reuses the exported DefaultAuthorName so the SQL and
// Go-side fallbacks cannot drift.
const sqlDefaultAuthorName = "'" + DefaultAuthorName + "'"

// ownerArgs converts an owner into the two bind parameters every owner-scoped
// statement expects. The boolean reports whether the owner was well-formed; an
// ill-formed owner must never reach the database.
func ownerArgs(owner middleware.Owner) (any, any, bool) {
	// IdentityFromOwner is where the exactly-one-owner invariant is enforced,
	// so an ambiguous owner (both IDs set) is rejected here rather than
	// silently resolving to whichever branch came first.
	if _, ok := middleware.IdentityFromOwner(owner); !ok {
		return nil, nil, false
	}
	if owner.Registered() {
		return owner.UserID, nil, true
	}
	return nil, owner.AnonIdentityID, true
}

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

	// The schema's one-owner CHECK means exactly one column is set per row, so
	// a caller holding a UserID can only match rows whose anon_identity_id IS
	// NULL, and vice versa: an owner cannot widen its scope by omitting an
	// identity. Same predicate journal uses.
	ownerPredicate = `user_id IS NOT DISTINCT FROM $2 AND anon_identity_id IS NOT DISTINCT FROM $3`

	isMemberSQL = `SELECT EXISTS (
		SELECT 1 FROM circle_members WHERE circle_id = $1 AND ` + ownerPredicate + `)`

	addMemberSQL = `INSERT INTO circle_members (circle_id, user_id, anon_identity_id)
		VALUES ($1, $2, $3) RETURNING id`

	removeMemberSQL = `DELETE FROM circle_members
		WHERE circle_id = $1 AND ` + ownerPredicate

	// The before cursor is an exclusive bound on created_at; ORDER BY includes
	// id as a deterministic tiebreaker for identical timestamps.
	//
	// The author name is resolved here rather than in Go to avoid a second
	// round trip. The one-owner CHECK guarantees at most one join matches, so
	// COALESCE can never pick a name from the wrong side, and no owner ID is
	// selected, so user_id cannot leak onto the wire.
	messageSelect = `SELECT m.id,
		COALESCE(NULLIF(BTRIM(u.display_name), ''), a.anon_name, ` + sqlDefaultAuthorName + `),
		m.anon_identity_id IS NOT NULL,
		m.content, m.reaction_counts, m.created_at
		FROM circle_messages m
		LEFT JOIN anon_identities a ON a.id = m.anon_identity_id
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.circle_id = $1`

	listMessagesSQL = messageSelect + `
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2`

	listMessagesBeforeSQL = messageSelect + `
		AND m.created_at < $3
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2`

	// One round trip: insert the row, then resolve the author name for the wire
	// response. reaction_counts keeps the column default '{}'.
	createMessageSQL = `WITH inserted AS (
			INSERT INTO circle_messages (circle_id, user_id, anon_identity_id, content)
			VALUES ($1, $2, $3, $4)
			RETURNING id, user_id, anon_identity_id, content, reaction_counts, created_at
		)
		SELECT i.id,
			COALESCE(NULLIF(BTRIM(u.display_name), ''), a.anon_name, ` + sqlDefaultAuthorName + `),
			i.anon_identity_id IS NOT NULL,
			i.content, i.reaction_counts, i.created_at
		FROM inserted i
		LEFT JOIN anon_identities a ON a.id = i.anon_identity_id
		LEFT JOIN users u ON u.id = i.user_id`
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

func (r *PostgresRepository) IsMember(ctx context.Context, circleID string, owner middleware.Owner) (bool, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return false, errors.New("circles is member: no owner")
	}
	var isMember bool
	if err := r.pool.QueryRow(ctx, isMemberSQL, circleID, userID, anonID).Scan(&isMember); err != nil {
		return false, fmt.Errorf("circles is member: %w", err)
	}
	return isMember, nil
}

func (r *PostgresRepository) AddMember(ctx context.Context, circleID string, owner middleware.Owner) error {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return errors.New("circles add member: no owner")
	}
	var id string
	err := r.pool.QueryRow(ctx, addMemberSQL, circleID, userID, anonID).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		// A partial unique index covers both owner kinds, so a repeat join
		// violates it whether the caller is registered or anonymous.
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrAlreadyMember
		}
		return fmt.Errorf("circles add member: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RemoveMember(ctx context.Context, circleID string, owner middleware.Owner) error {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return errors.New("circles remove member: no owner")
	}
	if _, err := r.pool.Exec(ctx, removeMemberSQL, circleID, userID, anonID); err != nil {
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

func (r *PostgresRepository) CreateMessage(ctx context.Context, circleID string, owner middleware.Owner, content string) (*CircleMessage, error) {
	userID, anonID, ok := ownerArgs(owner)
	if !ok {
		return nil, errors.New("circles create message: no owner")
	}
	message := &CircleMessage{}
	err := scanMessage(r.pool.QueryRow(ctx, createMessageSQL, circleID, userID, anonID, content).Scan, message)
	if err != nil {
		return nil, fmt.Errorf("circles create message: %w", err)
	}
	return message, nil
}

// scanCircle shares one column decoder between list/get. IsMember is never
// sourced from SQL: it is a business property the service resolves.
func scanCircle(s func(dest ...any) error, c *Circle) error {
	return s(&c.ID, &c.Slug, &c.Name, &c.Description, &c.Icon, &c.MemberCount)
}

// scanMessage shares one column decoder between list/create. No owner ID is
// ever scanned into the wire struct, which keeps a member's user_id off the API.
func scanMessage(s func(dest ...any) error, m *CircleMessage) error {
	return s(&m.ID, &m.AuthorName, &m.IsAnonymous, &m.Content, &m.ReactionCounts, &m.CreatedAt)
}
