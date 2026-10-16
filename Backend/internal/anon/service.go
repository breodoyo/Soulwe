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

func (s *service) CreateSession(ctx context.Context, deviceUUID string) (*Session, error) {
	deviceUUID = strings.TrimSpace(deviceUUID)

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
