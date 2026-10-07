package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// AccessTokenTTL is how long a signed access token stays valid (Docs/API.md).
	AccessTokenTTL = 15 * time.Minute

	// Issuer identifies Soulwe as the party that issued a token.
	Issuer = "soulwe-api"
)

// Sentinel errors returned by ParseAccessToken. The middleware maps all of them
// to a generic 401; keeping them distinct helps callers diagnose failures
// without exposing details to clients.
var (
	// ErrInvalidToken covers empty, malformed, expired, or unparseable tokens.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrUnexpectedSigningMethod indicates an algorithm other than HS256.
	ErrUnexpectedSigningMethod = errors.New("unexpected JWT signing method")
	// ErrMissingSubject indicates a valid token carrying no subject.
	ErrMissingSubject = errors.New("access token missing subject")
)

// Manager signs JWT access tokens using the application's JWT_SECRET,
// which never leaves this package.
type Manager struct {
	secret []byte
}

func NewManager(secret string) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT signing secret is empty")
	}
	return &Manager{secret: []byte(secret)}, nil
}

// SignAccessToken issues a signed compact JWT (header.payload.signature) for userID.
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

// ParseAccessToken validates an HS256 access token signed with the manager's
// secret and returns the user ID from its `sub` claim. It requires a present,
// unexpired expiration claim and the Soulwe issuer.
func (m *Manager) ParseAccessToken(tokenString string) (string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", ErrInvalidToken
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (interface{}, error) {
			// Pin the algorithm: reject the sibling HMAC variants too.
			if t.Method != jwt.SigningMethodHS256 {
				return nil, ErrUnexpectedSigningMethod
			}
			return m.secret, nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(Issuer),
	)
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", ErrInvalidToken
	}

	if strings.TrimSpace(claims.Subject) == "" {
		return "", ErrMissingSubject
	}
	return claims.Subject, nil
}
