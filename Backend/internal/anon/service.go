package anon

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxCreateAttempts = 5

type Session struct {
	Token       string
	AnonymousID string
}

type Service interface {
	CreateSession(ctx context.Context, deviceUUID string) (*Session, error)
	Authenticate(ctx context.Context, rawToken string) (identityID string, ok bool, err error)
}

type service struct {
	identities Repository
}

func NewService(identities Repository) *service {
	return &service{identities: identities}
}

// CreateSession always mints a brand-new anonymous identity. A caller cannot
// claim an existing one by presenting an id or device UUID: only possession of
// the session token proves ownership, which Authenticate checks. A device UUID
// is therefore treated as an optional, non-authoritative binding and dropped if
// it is already taken.
func (s *service) CreateSession(ctx context.Context, deviceUUID string) (*Session, error) {
	deviceUUID = strings.TrimSpace(deviceUUID)

	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
		var devicePtr *string
		if deviceUUID != "" {
			devicePtr = &deviceUUID
		}

		raw, err := GenerateRawToken()
		if err != nil {
			return nil, fmt.Errorf("anon create session: generate token: %w", err)
		}

		identity := &AnonIdentity{
			DeviceUUID: devicePtr,
			AnonName:   randomAnonName(),
			TokenHash:  HashToken(raw),
		}

		if err := s.identities.Create(ctx, identity); err != nil {
			if errors.Is(err, ErrDeviceConflict) && devicePtr != nil {
				// The device already belongs to another identity. Never reissue
				// that identity's session (the caller proved no secret); mint a
				// fresh one without the binding instead.
				deviceUUID = ""
				continue
			}
			if errors.Is(err, ErrIdentityConflict) {
				// A genuine name/token race; retry with fresh values.
				continue
			}
			return nil, fmt.Errorf("anon create session: %w", err)
		}

		return &Session{Token: raw, AnonymousID: identity.ID}, nil
	}

	return nil, fmt.Errorf("anon create session: %w", ErrIdentityConflict)
}

func (s *service) Authenticate(ctx context.Context, rawToken string) (string, bool, error) {
	if strings.TrimSpace(rawToken) == "" {
		return "", false, nil
	}

	identity, err := s.identities.FindByTokenHash(ctx, HashToken(rawToken))
	if errors.Is(err, ErrIdentityNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("anon authenticate: %w", err)
	}

	if err := s.identities.UpdateLastSeen(ctx, identity.ID); err != nil {
		return "", false, fmt.Errorf("anon authenticate: %w", err)
	}

	return identity.ID, true, nil
}
