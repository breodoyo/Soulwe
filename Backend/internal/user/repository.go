package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the users domain needs.
// Implementations only touch the database; they contain no business logic.
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
	// Promote atomically creates a registered user and links the given
	// anonymous identity to it in a single transaction. See the concrete
	// implementation for the conflict semantics.
	Promote(ctx context.Context, identityID, email, passwordHash string, displayName *string, languagePref string) (*User, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// userColumns lists the columns every SELECT below returns. deleted_at IS NULL
// keeps soft-deleted accounts invisible to lookups (see Docs/DATABASE.md).
const userColumns = "id, email, password_hash, display_name, language_pref, is_verified, created_at, updated_at, deleted_at"

const (
	createUserSQL = `
		INSERT INTO users (email, password_hash, display_name, language_pref)
		VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), 'en'))
		RETURNING ` + userColumns

	findUserByEmailSQL = "SELECT " + userColumns + ` FROM users WHERE email = $1 AND deleted_at IS NULL`
	findUserByIDSQL    = "SELECT " + userColumns + ` FROM users WHERE id = $1 AND deleted_at IS NULL`
)

// Create inserts a new user and fills in the database-generated fields
// (id, defaults, timestamps) on the passed user. Parameterized SQL is used
// throughout — user input is never concatenated into queries.
func (r *PostgresRepository) Create(ctx context.Context, u *User) error {
	err := r.pool.QueryRow(ctx, createUserSQL,
		u.Email, u.PasswordHash, u.DisplayName, u.LanguagePref,
	).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.LanguagePref, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrEmailTaken
		}
		return fmt.Errorf("user create: %w", err)
	}
	return nil
}

// FindByEmail returns the non-deleted user matching email, or ErrUserNotFound.
func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := r.pool.QueryRow(ctx, findUserByEmailSQL, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.LanguagePref, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user find by email: %w", err)
	}
	return u, nil
}

// FindByID returns the non-deleted user matching id, or ErrUserNotFound.
func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.pool.QueryRow(ctx, findUserByIDSQL, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.LanguagePref, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user find by id: %w", err)
	}
	return u, nil
}

// Promote creates a registered user AND links the given anonymous identity to
// it in one PostgreSQL transaction so the two writes can never be split: the
// forbidden half-states (user created but identity unlinked, identity linked
// without a surviving user record) are impossible.
//
// The anon_identities row is locked FOR UPDATE for the duration of the
// transaction, so two concurrent promotion attempts against the same identity
// serialize: the loser observes user_id already set and rolls back with
// ErrIdentityAlreadyPromoted rather than creating a second, orphaned user.
//
// Errors:
//   - ErrIdentityAlreadyPromoted — the identity is already linked to an account
//   - ErrEmailTaken             — the email is already registered
func (r *PostgresRepository) Promote(ctx context.Context, identityID, email, passwordHash string, displayName *string, languagePref string) (*User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("user promote: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the identity row and read its current binding in one statement.
	// Waiting for a concurrent promote's lock is what makes the same-identity
	// race resolve to exactly one winner instead of two created users.
	var linkedUserID *string
	if err := tx.QueryRow(ctx,
		"SELECT user_id FROM anon_identities WHERE id = $1 FOR UPDATE", identityID,
	).Scan(&linkedUserID); err != nil {
		return nil, fmt.Errorf("user promote: lock anonymous identity: %w", err)
	}
	if linkedUserID != nil {
		return nil, ErrIdentityAlreadyPromoted
	}

	u := &User{
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		LanguagePref: languagePref,
	}
	if err := tx.QueryRow(ctx, createUserSQL,
		u.Email, u.PasswordHash, u.DisplayName, u.LanguagePref,
	).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.LanguagePref, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("user promote: create user: %w", err)
	}

	if _, err := tx.Exec(ctx,
		"UPDATE anon_identities SET user_id = $2 WHERE id = $1", identityID, u.ID,
	); err != nil {
		return nil, fmt.Errorf("user promote: link anonymous identity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("user promote: commit transaction: %w", err)
	}
	return u, nil
}
