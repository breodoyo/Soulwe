package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenTTL = 15 * time.Minute

	Issuer = "soulwe-api"
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrUnexpectedSigningMethod = errors.New("unexpected JWT signing method")
	ErrMissingSubject = errors.New("access token missing subject")
)

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
