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
type Repository interface {
	Create(ctx context.Context, u *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
	// Promote atomically creates a user and links the anonymous identity to it.
	Promote(ctx context.Context, identityID, email, passwordHash string, displayName *string, languagePref string) (*User, error)
	// UpdateProfile writes only passed fields; a "" displayName clears it.
	UpdateProfile(ctx context.Context, userID string, displayName, languagePref *string) (*User, error)
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// userColumns lists the columns every SELECT below returns; deleted_at IS NULL hides soft-deletes.
const userColumns = "id, email, password_hash, display_name, language_pref, is_verified, created_at, updated_at, deleted_at"

const (
	createUserSQL = `
		INSERT INTO users (email, password_hash, display_name, language_pref)
		VALUES ($1, $2, $3, COALESCE(NULLIF($4, ''), 'en'))
		RETURNING ` + userColumns

	findUserByEmailSQL = "SELECT " + userColumns + ` FROM users WHERE email = $1 AND deleted_at IS NULL`
	findUserByIDSQL    = "SELECT " + userColumns + ` FROM users WHERE id = $1 AND deleted_at IS NULL`

	// updateProfileSQL writes a column only when asked: nil leaves it, &"" clears it to NULL.
	updateProfileSQL = `
		UPDATE users SET
			display_name = CASE WHEN $2::boolean THEN NULLIF($3, '')::text ELSE display_name END,
			language_pref = CASE WHEN $4::boolean THEN $5::text ELSE language_pref END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING ` + userColumns
)

// Create inserts a new user, binding all input as query parameters.
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

// Promote links a user and an anonymous identity in one FOR UPDATE transaction.
func (r *PostgresRepository) Promote(ctx context.Context, identityID, email, passwordHash string, displayName *string, languagePref string) (*User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("user promote: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Blocking on this lock is what makes a same-identity race yield one winner.
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

// UpdateProfile writes only the fields the caller passed, so a PATCH that
// omits one cannot clobber a concurrent update to it. Returns the refreshed
// user or ErrUserNotFound.
func (r *PostgresRepository) UpdateProfile(ctx context.Context, userID string, displayName, languagePref *string) (*User, error) {
	u := &User{}
	err := r.pool.QueryRow(ctx, updateProfileSQL,
		userID,
		displayName != nil, nullableString(displayName),
		languagePref != nil, nullableString(languagePref),
	).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.LanguagePref, &u.IsVerified, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user update profile: %w", err)
	}
	return u, nil
}

// nullableString returns the value behind p, or "" when p is nil, for the
// positional parameters of updateProfileSQL.
func nullableString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
