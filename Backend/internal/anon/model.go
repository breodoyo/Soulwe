package anon

import (
	"errors"
	"time"
)

// Sentinel errors returned by the anonymous session domain. ErrIdentityConflict
// only surfaces wrapped, when the service exhausts its retry budget.
var (
	ErrIdentityNotFound = errors.New("anonymous identity not found")
	// ErrIdentityConflict reports a collision on anon_name, token_hash, or device_uuid.
	ErrIdentityConflict = errors.New("anonymous identity conflicts with an existing row")
)

// AnonIdentity mirrors the anon_identities table. TokenHash holds a SHA-256
// hash of the client's raw token, which is never persisted.
type AnonIdentity struct {
	ID         string
	UserID     *string
	DeviceUUID *string
	AnonName   string
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}
