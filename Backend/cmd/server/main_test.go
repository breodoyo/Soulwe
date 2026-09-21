package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"Backend/internal/anon"
	"Backend/internal/auth"
	"Backend/internal/config"
	"Backend/internal/middleware"
	"Backend/internal/user"

	"github.com/gin-gonic/gin"
)

func TestHealthEndpoints(t *testing.T) {
	cfg := &config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}

	// The pool is only required for the /readyz endpoint, and the auth handler
	// is only used for /api/v1/auth/* routes. Neither is exercised by these
	// unit tests, so both are omitted here.
	router := setupRouter(cfg, nil, nil, nil, nil, nil)

	t.Run("Root /health returns 200 OK", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["status"] != "ok" {
			t.Errorf("Expected status 'ok', got %v", res["status"])
		}

		if res["service"] != "soulwe-api" {
			t.Errorf("Expected service 'soulwe-api', got %v", res["service"])
		}
	})

	t.Run("API v1 /api/v1/health returns 200 OK", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["version"] != "v1" {
			t.Errorf("Expected version 'v1', got %v", res["version"])
		}
	})

	t.Run("CORS preflight OPTIONS returns 204 No Content", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, "/health", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("Expected status %d for OPTIONS, got %d", http.StatusNoContent, w.Code)
		}

		originHeader := w.Header().Get("Access-Control-Allow-Origin")
		if originHeader != "http://localhost:5173" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://localhost:5173', got '%s'", originHeader)
		}
	})

	t.Run("Panic recovery middleware catches panic and returns 500 JSON", func(t *testing.T) {
		// Register a test route that deliberately panics
		router.GET("/test-panic", func(c *gin.Context) {
			panic("something went wrong inside a handler")
		})

		req, _ := http.NewRequest(http.MethodGet, "/test-panic", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("Expected status 500 for panic, got %d", w.Code)
		}

		var res map[string]map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["error"]["code"] != "INTERNAL_SERVER_ERROR" {
			t.Errorf("Expected error code 'INTERNAL_SERVER_ERROR', got %v", res["error"]["code"])
		}
	})
}

// TestAuthMeEndpoint exercises the Phase 3.3 wiring: /api/v1/auth/me is
// registered through setupRouter and only responds when a valid Bearer token
// is present. The handler used here is built with a nil service because the
// Me endpoint never touches the service layer.
func TestAuthMeEndpoint(t *testing.T) {
	tokenManager, err := auth.NewManager("unit-test-secret-that-must-be-long-enough-for-signing")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, nil, user.NewHandler(nil), tokenManager, nil, nil)

	t.Run("without a token returns 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("with a valid token returns the user id", func(t *testing.T) {
		token, err := tokenManager.SignAccessToken("11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("SignAccessToken returned error: %v", err)
		}

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != `{"user_id":"11111111-1111-1111-1111-111111111111"}` {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("health endpoints remain unprotected", func(t *testing.T) {
		// /readyz is intentionally excluded: it needs a live database pool to
		// return 200, and this router is built with a nil pool.
		for _, path := range []string{"/health", "/api/v1/health"} {
			req, _ := http.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", path, w.Code)
			}
		}
	})

	t.Run("register and login routes remain reachable", func(t *testing.T) {
		// Existence check: without a body these return 400, not 401/404.
		for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/login"} {
			req, _ := http.NewRequest(http.MethodPost, path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400 (route exists, unauthenticated), got %d", path, w.Code)
			}
		}
	})

	t.Run("middleware user id key is used consistently", func(t *testing.T) {
		if key := middleware.UserIDKey; key == "" {
			t.Fatal("expected a non-empty user id context key")
		}
	})
}

// fakeAnonService is a minimal anon.Service used by the router wiring tests. It
// recognises exactly one anonymous token (by its SHA-256 hash), mimicking the
// real service's Authenticate without a database.
type fakeAnonService struct {
	knownHashes map[string]string // token SHA-256 hash -> anonymous identity id
}

func newFakeAnonService(rawToken, identityID string) *fakeAnonService {
	return &fakeAnonService{knownHashes: map[string]string{anon.HashToken(rawToken): identityID}}
}

func (f *fakeAnonService) CreateSession(context.Context, string) (*anon.Session, error) {
	return nil, errors.New("not exercised by these tests")
}

func (f *fakeAnonService) Authenticate(_ context.Context, rawToken string) (string, bool, error) {
	id, ok := f.knownHashes[anon.HashToken(rawToken)]
	return id, ok, nil
}

// TestAnonymousPromoteEndpoint verifies the Phase 3.5 wiring: the promote route
// exists, is protected by the anonymous middleware (registered JWTs are
// rejected), and the pre-existing anonymous and registered auth flows are
// untouched.
func TestAnonymousPromoteEndpoint(t *testing.T) {
	tokenManager, err := auth.NewManager("unit-test-secret-that-must-be-long-enough-for-signing")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	const rawAnonToken = "raw-anonymous-token-v3-5"
	anonSvc := newFakeAnonService(rawAnonToken, "22222222-2222-2222-2222-222222222222")
	anonH := anon.NewHandler(anonSvc)

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, nil, user.NewHandler(nil), tokenManager, anonH, anonSvc)

	t.Run("promote without a token returns 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous/promote", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("promote with a registered JWT is rejected by the anonymous middleware", func(t *testing.T) {
		jwt, err := tokenManager.SignAccessToken("11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("SignAccessToken returned error: %v", err)
		}

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous/promote", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for a registered JWT, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("registered auth/me rejects the anonymous token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+rawAnonToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for an anonymous token on auth/me, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("anonymous/me still accepts the anonymous token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/anonymous/me", nil)
		req.Header.Set("Authorization", "Bearer "+rawAnonToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Body.String() != `{"anonymous_id":"22222222-2222-2222-2222-222222222222"}` {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("promote route is not registered without the anonymous stack", func(t *testing.T) {
		sparseRouter := setupRouter(&config.Config{
			Env:         "test",
			GinMode:     "test",
			FrontendURL: "http://localhost:5173",
		}, nil, user.NewHandler(nil), tokenManager, nil, nil)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous/promote", nil)
		w := httptest.NewRecorder()
		sparseRouter.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 when the anonymous stack is absent, got %d", w.Code)
		}
	})
}
