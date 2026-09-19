package anon

import (
	"errors"
	"time"
)

// Sentinel errors returned by the anonymous session domain. Handlers map
// these to safe HTTP responses; ErrIdentityConflict only appears as a
// wrapped value when the service exhausts its retry budget.
var (
	// ErrIdentityNotFound reports a lookup that matched no anonymous identity.
	ErrIdentityNotFound = errors.New("anonymous identity not found")
	// ErrIdentityConflict reports an insert that collided with a unique
	// constraint (anon_name, token_hash, or device_uuid).
	ErrIdentityConflict = errors.New("anonymous identity conflicts with an existing row")
)

// AnonIdentity mirrors the anon_identities table. TokenHash holds a SHA-256
// hash of the client's raw token — the raw token itself is never persisted.
type AnonIdentity struct {
	ID         string
	UserID     *string
	DeviceUUID *string
	AnonName   string
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}
