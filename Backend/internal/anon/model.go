package anon

import (
	"errors"
	"time"
)

// Sentinel errors returned by the anonymous session domain. ErrIdentityConflict
// only surfaces wrapped, when the service exhausts its retry budget.
// ErrDeviceConflict means the supplied device UUID already belongs to another
// identity; because the caller proved no secret, its session must not be
// reissued — the service mints a fresh identity instead.
var (
	ErrIdentityNotFound = errors.New("anonymous identity not found")
	ErrIdentityConflict = errors.New("anonymous identity conflicts with an existing row")
	ErrDeviceConflict   = errors.New("device uuid already bound to another identity")
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
