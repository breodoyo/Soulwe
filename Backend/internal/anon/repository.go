package anon

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines the persistence operations the anonymous session domain
// needs. Implementations only touch the database; they contain no business
// logic. All token_hash arguments are already SHA-256 hashes — never raw
// tokens.
type Repository interface {
	// Create inserts a new anonymous identity and fills in the database
	// generated fields (id, created_at, last_seen_at) on the passed value.
	// It returns ErrIdentityConflict when anon_name, token_hash, or
	// device_uuid collides with a unique constraint.
	Create(ctx context.Context, identity *AnonIdentity) error

	// FindByTokenHash returns the identity whose token_hash matches, or
	// ErrIdentityNotFound.
	FindByTokenHash(ctx context.Context, tokenHash string) (*AnonIdentity, error)

	// FindByDeviceUUID returns the identity bound to a device UUID, or
	// ErrIdentityNotFound.
	FindByDeviceUUID(ctx context.Context, deviceUUID string) (*AnonIdentity, error)

	// UpdateLastSeen stamps the identity's last_seen_at to now.
	UpdateLastSeen(ctx context.Context, id string) error

	// RotateToken reassigns a fresh token hash to an existing identity and
	// stamps last_seen_at, used when the same device re-registers so its
	// identity and anon_name stay stable while the token rotates.
	RotateToken(ctx context.Context, id, tokenHash string) error
}

// PostgresRepository implements Repository on top of the shared pgx pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository returns a Repository backed by the given pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const identityColumns = "id, user_id, device_uuid, anon_name, token_hash, created_at, last_seen_at"

const (
	createIdentitySQL = `
		INSERT INTO anon_identities (user_id, device_uuid, anon_name, token_hash, last_seen_at)
		VALUES (NULL, $1, $2, $3, NOW())
		RETURNING ` + identityColumns

	findIdentityByTokenHashSQL = "SELECT " + identityColumns + " FROM anon_identities WHERE token_hash = $1"
	findIdentityByDeviceSQL    = "SELECT " + identityColumns + " FROM anon_identities WHERE device_uuid = $1"

	updateLastSeenSQL = "UPDATE anon_identities SET last_seen_at = NOW() WHERE id = $1"
	rotateTokenSQL    = "UPDATE anon_identities SET token_hash = $2, last_seen_at = NOW() WHERE id = $1"
)

// Create inserts a new anonymous identity, leaving user_id NULL. Anonymous
// sessions never carry a registered user; the column stays NULL until a
// future phase links the identity to an account.
func (r *PostgresRepository) Create(ctx context.Context, identity *AnonIdentity) error {
	err := r.pool.QueryRow(ctx, createIdentitySQL,
		identity.DeviceUUID, identity.AnonName, identity.TokenHash,
	).Scan(
		&identity.ID, &identity.UserID, &identity.DeviceUUID, &identity.AnonName,
		&identity.TokenHash, &identity.CreatedAt, &identity.LastSeenAt,
	)
	return translateCreateError(err)
}

func (r *PostgresRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*AnonIdentity, error) {
	identity := &AnonIdentity{}
	err := r.pool.QueryRow(ctx, findIdentityByTokenHashSQL, tokenHash).Scan(
		&identity.ID, &identity.UserID, &identity.DeviceUUID, &identity.AnonName,
		&identity.TokenHash, &identity.CreatedAt, &identity.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("anon find by token hash: %w", err)
	}
	return identity, nil
}

func (r *PostgresRepository) FindByDeviceUUID(ctx context.Context, deviceUUID string) (*AnonIdentity, error) {
	identity := &AnonIdentity{}
	err := r.pool.QueryRow(ctx, findIdentityByDeviceSQL, deviceUUID).Scan(
		&identity.ID, &identity.UserID, &identity.DeviceUUID, &identity.AnonName,
		&identity.TokenHash, &identity.CreatedAt, &identity.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("anon find by device uuid: %w", err)
	}
	return identity, nil
}

func (r *PostgresRepository) UpdateLastSeen(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, updateLastSeenSQL, id); err != nil {
		return fmt.Errorf("anon update last seen: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RotateToken(ctx context.Context, id, tokenHash string) error {
	if _, err := r.pool.Exec(ctx, rotateTokenSQL, id, tokenHash); err != nil {
		return fmt.Errorf("anon rotate token: %w", err)
	}
	return nil
}

// translateCreateError maps a unique-violation result to ErrIdentityConflict.
// anon_name is NOT NULL UNIQUE, so a collision can surface from any of the
// three unique constraints; the service retries with fresh name + fresh token
// regardless of which one fired.
func translateCreateError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return ErrIdentityConflict
	}
	return fmt.Errorf("anon identity create: %w", err)
}
