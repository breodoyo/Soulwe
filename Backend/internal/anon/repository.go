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

	FindByTokenHash(ctx context.Context, tokenHash string) (*AnonIdentity, error)

	// FindByDeviceUUID returns the identity bound to a device UUID, or ErrIdentityNotFound.
	FindByDeviceUUID(ctx context.Context, deviceUUID string) (*AnonIdentity, error)

	// UpdateLastSeen stamps the identity's last_seen_at to now.
	UpdateLastSeen(ctx context.Context, id string) error

	// RotateToken reassigns a fresh token hash and stamps last_seen_at, so a
	// re-registering device keeps its identity and anon_name.
	RotateToken(ctx context.Context, id, tokenHash string) error
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
	findIdentityByDeviceSQL    = "SELECT " + identityColumns + " FROM anon_identities WHERE device_uuid = $1"

	updateLastSeenSQL = "UPDATE anon_identities SET last_seen_at = NOW() WHERE id = $1"
	rotateTokenSQL    = "UPDATE anon_identities SET token_hash = $2, last_seen_at = NOW() WHERE id = $1"
)

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
