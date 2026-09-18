package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"Backend/internal/auth"
	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const middlewareTestSecret = "unit-test-secret-that-must-be-long-enough-for-signing"

// newProtectedRouter builds a Gin engine with a single protected route. The
// handler echoes the authenticated user ID from the context, which is how the
// tests assert that AuthRequired stores it.
func newProtectedRouter(t *testing.T, tokenManager *auth.Manager) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/protected", middleware.AuthRequired(tokenManager), func(c *gin.Context) {
		userID, ok := middleware.UserIDFromContext(c)
		c.JSON(http.StatusOK, gin.H{
			"ok":      ok,
			"user_id": userID,
		})
	})
	return r
}

// signWithSecret signs a token with an explicit method and secret, so tests
// can build both valid tokens and invalid ones (expired, wrong algorithm,
// wrong secret, missing subject).
func signWithSecret(t *testing.T, secret string, method jwt.SigningMethod, claims jwt.RegisteredClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func validClaims(subject string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   subject,
		Issuer:    auth.Issuer,
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
}

func validToken(t *testing.T, subject string) string {
	t.Helper()
	return signWithSecret(t, middlewareTestSecret, jwt.SigningMethodHS256, validClaims(subject))
}

func performProtectedRequest(t *testing.T, tokenManager *auth.Manager, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	router := newProtectedRouter(t, tokenManager)

	req, err := http.NewRequest(http.MethodGet, "/protected", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}
	if body["error"]["code"] != "UNAUTHORIZED" {
		t.Errorf("expected error code UNAUTHORIZED, got %q", body["error"]["code"])
	}
	if body["error"]["message"] == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestAuthRequired(t *testing.T) {
	manager, err := auth.NewManager(middlewareTestSecret)
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		assertUnauthorized(t, performProtectedRequest(t, manager, ""))
	})

	t.Run("non-Bearer scheme returns 401", func(t *testing.T) {
		rec := performProtectedRequest(t, manager, "Basic dXNlcjpwYXNz")
		assertUnauthorized(t, rec)
	})

	t.Run("empty Bearer token returns 401", func(t *testing.T) {
		rec := performProtectedRequest(t, manager, "Bearer")
		assertUnauthorized(t, rec)
		rec = performProtectedRequest(t, manager, "Bearer    ")
		assertUnauthorized(t, rec)
	})

	t.Run("malformed JWT returns 401", func(t *testing.T) {
		rec := performProtectedRequest(t, manager, "Bearer not-a-jwt")
		assertUnauthorized(t, rec)
	})

	t.Run("expired JWT returns 401", func(t *testing.T) {
		claims := validClaims("user-1")
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
		token := signWithSecret(t, middlewareTestSecret, jwt.SigningMethodHS256, claims)
		assertUnauthorized(t, performProtectedRequest(t, manager, "Bearer "+token))
	})

	t.Run("JWT signed with a wrong secret returns 401", func(t *testing.T) {
		token := signWithSecret(t, "a-completely-different-secret", jwt.SigningMethodHS256, validClaims("user-1"))
		assertUnauthorized(t, performProtectedRequest(t, manager, "Bearer "+token))
	})

	t.Run("wrong HMAC algorithm returns 401", func(t *testing.T) {
		token := signWithSecret(t, middlewareTestSecret, jwt.SigningMethodHS384, validClaims("user-1"))
		assertUnauthorized(t, performProtectedRequest(t, manager, "Bearer "+token))
	})

	t.Run("JWT missing subject returns 401", func(t *testing.T) {
		token := signWithSecret(t, middlewareTestSecret, jwt.SigningMethodHS256, validClaims(""))
		assertUnauthorized(t, performProtectedRequest(t, manager, "Bearer "+token))
	})

	t.Run("valid JWT reaches the handler with the user id from context", func(t *testing.T) {
		token := validToken(t, "11111111-1111-1111-1111-111111111111")
		rec := performProtectedRequest(t, manager, "Bearer "+token)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if body["ok"] != true {
			t.Error("expected the user id to be present in the context")
		}
		if body["user_id"] != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("expected user id in response, got %v", body["user_id"])
		}
	})

	t.Run("401 responses never leak the token back to the client", func(t *testing.T) {
		token := signWithSecret(t, "a-completely-different-secret", jwt.SigningMethodHS256, validClaims("user-1"))
		rec := performProtectedRequest(t, manager, "Bearer "+token)
		assertUnauthorized(t, rec)
		if bytes.Contains(rec.Body.Bytes(), []byte(token)) {
			t.Error("the rejected token leaked into the 401 response body")
		}
	})
}
