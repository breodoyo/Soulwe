package anon

import (
	"errors"
	"time"
)

// Sentinel errors returned by the anonymous session domain. ErrIdentityConflict
// only surfaces wrapped, when the service exhausts its retry budget.
var (
	ErrIdentityNotFound = errors.New("anonymous identity not found")
	ErrIdentityConflict = errors.New("anonymous identity conflicts with an existing row")
)

type AnonIdentity struct {
	ID         string
	UserID     *string
	DeviceUUID *string
	AnonName   string
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}
