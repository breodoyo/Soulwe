package anon

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// maxCreateAttempts bounds retries on the UNIQUE constraints. An insert can
// collide on anon_name, token_hash, or device_uuid; a fresh random name and
// token are tried again each loop, making collisions effectively impossible.
const maxCreateAttempts = 5

// Session is the successful outcome of creating an anonymous session: the raw
// token the client must present on subsequent requests, plus the identity ID
// of the underlying anon_identities row (anonymous_id).
type Session struct {
	Token       string
	AnonymousID string
}

// Service is the anonymous-session business-logic boundary. Implementations
// authenticate raw tokens, rotate tokens idempotently per device, and never
// leak the raw token back to the repository.
type Service interface {
	// CreateSession mints a fresh anonymous token and the identity that owns
	// it. When the client re-registers with the same device UUID, the same
	// identity (and hence the same anonymous_id) is reused and its token is
	// rotated, keeping the endpoint idempotent per device.
	CreateSession(ctx context.Context, deviceUUID string) (*Session, error)

	// Authenticate checks a raw anonymous token and returns the identity ID
	// when valid. The boolean reports whether the token matched; ok=false
	// means the token is unknown (not an internal failure).
	Authenticate(ctx context.Context, rawToken string) (identityID string, ok bool, err error)
}

type service struct {
	identities Repository
}

// NewService wires the anonymous session service to an identity repository.
func NewService(identities Repository) *service {
	return &service{identities: identities}
}

func (s *service) CreateSession(ctx context.Context, deviceUUID string) (*Session, error) {
	deviceUUID = strings.TrimSpace(deviceUUID)

	// Idempotency: a repeated call from the same device reuses its existing
	// identity and rotates the token, so anonymous_id stays stable.
	if deviceUUID != "" {
		if existing, err := s.identities.FindByDeviceUUID(ctx, deviceUUID); err == nil {
			session, err := s.rotate(ctx, existing.ID)
			if err != nil {
				return nil, err
			}
			return session, nil
		} else if !errors.Is(err, ErrIdentityNotFound) {
			return nil, fmt.Errorf("anon create session: %w", err)
		}
	}

	var devicePtr *string
	if deviceUUID != "" {
		devicePtr = &deviceUUID
	}

	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
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
			if !errors.Is(err, ErrIdentityConflict) {
				return nil, fmt.Errorf("anon create session: %w", err)
			}

			// A concurrent request may have created this device's identity
			// after our initial lookup, racing our insert on the device_uuid
			// unique index. Re-check before retrying so we rotate the
			// winner's identity instead of exhausting retries with a 500.
			if deviceUUID != "" {
				if existing, lookupErr := s.identities.FindByDeviceUUID(ctx, deviceUUID); lookupErr == nil {
					session, rotateErr := s.rotate(ctx, existing.ID)
					if rotateErr != nil {
						return nil, rotateErr
					}
					return session, nil
				} else if !errors.Is(lookupErr, ErrIdentityNotFound) {
					return nil, fmt.Errorf("anon create session: %w", lookupErr)
				}
			}
			continue // collide on name/hash → retry with fresh values
		}

		return &Session{Token: raw, AnonymousID: identity.ID}, nil
	}

	return nil, fmt.Errorf("anon create session: %w", ErrIdentityConflict)
}

// rotate assigns a fresh token hash to an existing identity without touching
// its anonymous_id or anon_name.
func (s *service) rotate(ctx context.Context, identityID string) (*Session, error) {
	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
		raw, err := GenerateRawToken()
		if err != nil {
			return nil, fmt.Errorf("anon rotate: generate token: %w", err)
		}
		if err := s.identities.RotateToken(ctx, identityID, HashToken(raw)); err != nil {
			if errors.Is(err, ErrIdentityConflict) {
				continue
			}
			return nil, fmt.Errorf("anon rotate: %w", err)
		}
		return &Session{Token: raw, AnonymousID: identityID}, nil
	}
	return nil, fmt.Errorf("anon rotate: %w", ErrIdentityConflict)
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
