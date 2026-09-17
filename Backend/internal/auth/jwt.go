package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// AccessTokenTTL is how long a signed access token stays valid.
	// Mirrors the 15-minute lifetime documented in Docs/API.md.
	AccessTokenTTL = 15 * time.Minute

	// Issuer identifies Soulwe as the party that issued a token.
	Issuer = "soulwe-api"
)

// Manager signs JWT access tokens using the application's JWT_SECRET.
// The secret never leaves this package's internals.
type Manager struct {
	secret []byte
}

// NewManager validates the signing secret and returns a ready-to-use Manager.
func NewManager(secret string) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT signing secret is empty")
	}
	return &Manager{secret: []byte(secret)}, nil
}

// SignAccessToken issues a signed JWT for the given user ID with a 15-minute
// lifetime. The returned string is the compact JWT (header.payload.signature).
func (m *Manager) SignAccessToken(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    Issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}
