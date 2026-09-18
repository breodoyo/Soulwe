package auth

import (
	"errors"
	"strings"
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

// Sentinel errors returned by ParseAccessToken. The middleware treats every
// one of them the same way (a 401), but keeping them distinct helps tests and
// future callers diagnose failures without exposing details to clients.
var (
	// ErrInvalidToken covers empty, malformed, expired, wrongly signed, or
	// otherwise unparseable access tokens.
	ErrInvalidToken = errors.New("invalid access token")
	// ErrUnexpectedSigningMethod indicates the token used an algorithm other
	// than HS256.
	ErrUnexpectedSigningMethod = errors.New("unexpected JWT signing method")
	// ErrMissingSubject indicates a token that passed signature and claim
	// validation but carried no subject.
	ErrMissingSubject = errors.New("access token missing subject")
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

// ParseAccessToken validates a JWT access token and returns the user ID from
// its `sub` claim. It only accepts tokens signed with HS256 using the
// manager's secret, requires a present and unexpired expiration claim that
// matches the Soulwe issuer, and rejects tokens whose subject is empty.
func (m *Manager) ParseAccessToken(tokenString string) (string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", ErrInvalidToken
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (interface{}, error) {
			// Reject every algorithm except HS256, including the sibling
			// HMAC variants HS384 and HS512.
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
