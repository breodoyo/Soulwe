package anon

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// maxCreateAttempts bounds retries on the UNIQUE constraints; a fresh random
// name and token are tried each loop, making collisions effectively impossible.
const maxCreateAttempts = 5

// Session is a successful session creation: the raw token the client presents
// on subsequent requests, plus the identity ID (anonymous_id).
type Session struct {
	Token       string
	AnonymousID string
}

// Service is the anonymous-session business-logic boundary.
type Service interface {
	// CreateSession mints a token and its identity; a repeat call with the same
	// device UUID reuses that identity and rotates the token, keeping the
	// endpoint idempotent per device.
	CreateSession(ctx context.Context, deviceUUID string) (*Session, error)

	// Authenticate returns the identity ID for a valid raw token. ok=false
	// means the token is unknown, not an internal failure.
	Authenticate(ctx context.Context, rawToken string) (identityID string, ok bool, err error)
}

type service struct {
	identities Repository
}

func NewService(identities Repository) *service {
	return &service{identities: identities}
}

func (s *service) CreateSession(ctx context.Context, deviceUUID string) (*Session, error) {
	deviceUUID = strings.TrimSpace(deviceUUID)

	// A repeat call from the same device reuses its identity, so the
	// anonymous_id stays stable.
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
			// after our lookup, racing us on the device_uuid unique index.
			// Re-check so we rotate the winner instead of exhausting retries.
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

// rotate assigns a fresh token hash, leaving anonymous_id and anon_name alone.
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
