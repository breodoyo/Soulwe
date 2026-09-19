package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// fakeAnonymousVerifier stubs the middleware's AnonymousVerifier interface.
type fakeAnonymousVerifier struct {
	authenticateFunc func(ctx context.Context, rawToken string) (string, bool, error)
	capturedToken    string
}

func (f *fakeAnonymousVerifier) Authenticate(ctx context.Context, rawToken string) (string, bool, error) {
	f.capturedToken = rawToken
	if f.authenticateFunc == nil {
		return "", false, nil
	}
	return f.authenticateFunc(ctx, rawToken)
}

// newAnonymousProtectedRouter builds a Gin engine with a single anonymous-
// protected route. The handler echoes the authenticated identity ID from the
// context, which is how the tests assert that AnonymousAuthRequired stored it.
func newAnonymousProtectedRouter(t *testing.T, verifier middleware.AnonymousVerifier) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/protected", middleware.AnonymousAuthRequired(verifier), func(c *gin.Context) {
		identityID, ok := middleware.AnonIdentityIDFromContext(c)
		c.JSON(http.StatusOK, gin.H{
			"ok":           ok,
			"anonymous_id": identityID,
		})
	})
	return r
}

func performAnonymousProtectedRequest(t *testing.T, verifier middleware.AnonymousVerifier, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	router := newAnonymousProtectedRouter(t, verifier)

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

func TestAnonymousAuthRequired(t *testing.T) {
	verifier := &fakeAnonymousVerifier{}

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		rec := performAnonymousProtectedRequest(t, verifier, "")
		assertUnauthorized(t, rec)
		if verifier.capturedToken != "" {
			t.Error("verifier must not be called when the header is missing")
		}
	})

	t.Run("non-Bearer scheme returns 401", func(t *testing.T) {
		rec := performAnonymousProtectedRequest(t, verifier, "Basic dXNlcjpwYXNz")
		assertUnauthorized(t, rec)
	})

	t.Run("empty Bearer token returns 401", func(t *testing.T) {
		rec := performAnonymousProtectedRequest(t, verifier, "Bearer")
		assertUnauthorized(t, rec)
		rec = performAnonymousProtectedRequest(t, verifier, "Bearer    ")
		assertUnauthorized(t, rec)
	})

	t.Run("unknown token returns 401", func(t *testing.T) {
		rec := performAnonymousProtectedRequest(t, verifier, "Bearer unknown-raw-token")
		assertUnauthorized(t, rec)
		if verifier.capturedToken != "unknown-raw-token" {
			t.Errorf("expected the raw token to reach the verifier, got %q", verifier.capturedToken)
		}
	})

	t.Run("verifier failure returns 500 without leaking internals", func(t *testing.T) {
		verifier.authenticateFunc = func(context.Context, string) (string, bool, error) {
			return "", false, errors.New("database connection broke down")
		}
		rec := performAnonymousProtectedRequest(t, verifier, "Bearer some-token")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "database connection broke down") {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})

	t.Run("valid token reaches the handler with the identity id from context", func(t *testing.T) {
		verifier.authenticateFunc = func(context.Context, string) (string, bool, error) {
			return "22222222-2222-2222-2222-222222222222", true, nil
		}
		rec := performAnonymousProtectedRequest(t, verifier, "Bearer valid-raw-token")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if body["ok"] != true {
			t.Error("expected the anonymous id to be present in the context")
		}
		if body["anonymous_id"] != "22222222-2222-2222-2222-222222222222" {
			t.Errorf("expected anonymous id in response, got %v", body["anonymous_id"])
		}
	})

	t.Run("401 responses never leak the token back to the client", func(t *testing.T) {
		verifier.authenticateFunc = func(context.Context, string) (string, bool, error) {
			return "", false, nil
		}
		token := "secret-raw-token-that-must-not-leak"
		rec := performAnonymousProtectedRequest(t, verifier, "Bearer "+token)
		assertUnauthorized(t, rec)
		if strings.Contains(rec.Body.String(), token) {
			t.Error("the rejected raw token leaked into the 401 response body")
		}
	})
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error JSON: %v", err)
	}
	if body["error"]["code"] != wantCode {
		t.Errorf("expected error code %q, got %q", wantCode, body["error"]["code"])
	}
}
