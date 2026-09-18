package auth

import (
	"crypto/rand"
	"crypto/rsa"
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

// signCustomToken signs an HS256 token with the given secret and registered
// claims, returning the compact JWT. It is only used by tests.
func signCustomToken(t *testing.T, secret string, claims jwt.RegisteredClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// validClaims returns the registered claims every happy-path test reuses.
func validClaims(subject string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    Issuer,
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenTTL)),
	}
}

func TestParseAccessTokenValid(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token, err := m.SignAccessToken("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}

	userID, err := m.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken returned error: %v", err)
	}
	if userID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("expected user id from sub, got %q", userID)
	}
}

func TestParseAccessTokenRejectsEmpty(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	for _, token := range []string{"", "   ", " \t "} {
		if _, err := m.ParseAccessToken(token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("token %q: expected ErrInvalidToken, got %v", token, err)
		}
	}
}

func TestParseAccessTokenRejectsMalformed(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	for _, token := range []string{"not-a-jwt", "a.b.c", "eyJhbGciOiJub25lIn0."} {
		if _, err := m.ParseAccessToken(token); err == nil {
			t.Errorf("token %q: expected an error, got nil", token)
		}
	}
}

func TestParseAccessTokenRejectsWrongSecret(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token := signCustomToken(t, "a-completely-different-secret", validClaims("user-1"))
	if _, err := m.ParseAccessToken(token); err == nil {
		t.Fatal("expected an error for a token signed with a different secret")
	}
}

func TestParseAccessTokenRejectsExpired(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	claims := validClaims("user-1")
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	token := signCustomToken(t, testSecret, claims)

	if _, err := m.ParseAccessToken(token); err == nil {
		t.Fatal("expected an error for an expired token")
	}
}

func TestParseAccessTokenRejectsOtherHMACAlgorithm(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	// HS384 is an HMAC variant like HS256 but must still be rejected.
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, validClaims("user-1"))
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	if _, err := m.ParseAccessToken(signed); !errors.Is(err, ErrUnexpectedSigningMethod) {
		t.Fatalf("expected ErrUnexpectedSigningMethod, got %v", err)
	}
}

func TestParseAccessTokenRejectsRSAAlgorithm(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate a test RSA key: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims("user-1"))
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	if _, err := m.ParseAccessToken(signed); !errors.Is(err, ErrUnexpectedSigningMethod) {
		t.Fatalf("expected ErrUnexpectedSigningMethod, got %v", err)
	}
}

func TestParseAccessTokenRejectsMissingSubject(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token := signCustomToken(t, testSecret, validClaims(""))
	if _, err := m.ParseAccessToken(token); !errors.Is(err, ErrMissingSubject) {
		t.Fatalf("expected ErrMissingSubject, got %v", err)
	}
}

func TestParseAccessTokenRejectsWrongIssuer(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	claims := validClaims("user-1")
	claims.Issuer = "some-other-app"
	token := signCustomToken(t, testSecret, claims)
	if _, err := m.ParseAccessToken(token); err == nil {
		t.Fatal("expected an error for a token with a foreign issuer")
	}
}

func TestParseAccessTokenRejectsMissingExpiration(t *testing.T) {
	m, err := NewManager(testSecret)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	claims := validClaims("user-1")
	claims.ExpiresAt = nil
	token := signCustomToken(t, testSecret, claims)
	if _, err := m.ParseAccessToken(token); err == nil {
		t.Fatal("expected an error when the expiration claim is absent")
	}
}
