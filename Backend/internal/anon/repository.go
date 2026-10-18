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

type Repository interface {
	Create(ctx context.Context, identity *AnonIdentity) error

	// FindByTokenHash resolves a session by the hash of the token the caller
	// presented; possessing the token is the only proof of ownership.
	FindByTokenHash(ctx context.Context, tokenHash string) (*AnonIdentity, error)

	// UpdateLastSeen stamps the identity's last_seen_at to now.
	UpdateLastSeen(ctx context.Context, id string) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

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

	updateLastSeenSQL = "UPDATE anon_identities SET last_seen_at = NOW() WHERE id = $1"
)

// deviceUUIDIndexName is the partial unique index guarding one identity per
// device; a violation means the device is already claimed by someone else.
const deviceUUIDIndexName = "idx_anon_identities_device_uuid"

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

func (r *PostgresRepository) UpdateLastSeen(ctx context.Context, id string) error {
	if _, err := r.pool.Exec(ctx, updateLastSeenSQL, id); err != nil {
		return fmt.Errorf("anon update last seen: %w", err)
	}
	return nil
}

// translateCreateError maps a failed INSERT to a domain error without ever
// echoing the driver's message: a Postgres DETAIL can contain the failing row,
// which would leak anon_name and token_hash into logs.
func translateCreateError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgerrcode.UniqueViolation {
			if pgErr.ConstraintName == deviceUUIDIndexName {
				return ErrDeviceConflict
			}
			return ErrIdentityConflict
		}
		return fmt.Errorf("anon identity create: sqlstate %s", pgErr.Code)
	}
	return fmt.Errorf("anon identity create: %w", err)
}
