package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret-that-must-be-long-enough-for-signing"

// parseAccessToken verifies and decodes a token using the same secret the
// manager signs with, mirroring what a future verification step would do.
func parseAccessToken(t *testing.T, tokenString, secret string) *jwt.RegisteredClaims {
	t.Helper()
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("expected a valid token")
	}
	return claims
}

func TestNewManagerRejectsEmptySecret(t *testing.T) {
	if _, err := NewManager(""); err == nil {
		t.Fatal("expected an error for an empty signing secret")
	}
}

func TestSignAccessTokenReturnsCompactJWT(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token, err := m.SignAccessToken("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}

	claims := parseAccessToken(t, token, testSecret)
	if claims.Subject != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("expected subject to be the user id, got %q", claims.Subject)
	}
	if claims.Issuer != Issuer {
		t.Errorf("expected issuer %q, got %q", Issuer, claims.Issuer)
	}
}

func TestAccessTokenLifetimeIsFifteenMinutes(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token, err := m.SignAccessToken("user-id")
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}

	claims := parseAccessToken(t, token, testSecret)
	if AccessTokenTTL != 15*time.Minute {
		t.Errorf("expected AccessTokenTTL to be 15 minutes, got %v", AccessTokenTTL)
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected an issued-at claim")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected an expires-at claim")
	}
	diff := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if diff != AccessTokenTTL {
		t.Errorf("expected exp - iat to be %v, got %v", AccessTokenTTL, diff)
	}
}

func TestTokenFailsVerificationWithWrongSecret(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token, err := m.SignAccessToken("user-id")
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}

	// A token signed with one secret must not validate under another.
	bad := &jwt.RegisteredClaims{}
	if _, err := jwt.ParseWithClaims(token, bad, func(t *jwt.Token) (interface{}, error) {
		return []byte("a-completely-different-secret"), nil
	}); err == nil {
		t.Error("expected verification failure when signing secrets differ")
	}
}
