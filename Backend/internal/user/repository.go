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
