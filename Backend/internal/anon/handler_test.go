package anon

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

// fakeService embeds the Service interface so the handler tests only need to
// stub the methods under test; the remaining methods fall back to their
// zero-value result.
type fakeService struct {
	Service
	createSessionFunc func(ctx context.Context, deviceUUID string) (*Session, error)
	authenticateFunc  func(ctx context.Context, rawToken string) (string, bool, error)
}

func (f *fakeService) CreateSession(ctx context.Context, deviceUUID string) (*Session, error) {
	if f.createSessionFunc == nil {
		return nil, errors.New("createSessionFunc not configured")
	}
	return f.createSessionFunc(ctx, deviceUUID)
}

func (f *fakeService) Authenticate(ctx context.Context, rawToken string) (string, bool, error) {
	if f.authenticateFunc == nil {
		return "", false, errors.New("authenticateFunc not configured")
	}
	return f.authenticateFunc(ctx, rawToken)
}

func performAnonymousCreate(t *testing.T, svc Service, deviceID string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(svc)

	router := gin.New()
	router.POST("/api/v1/auth/anonymous", handler.Create)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if deviceID != "" {
		req.Header.Set("X-Device-ID", deviceID)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandlerAnonymousCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 201 with the token and anonymous id", func(t *testing.T) {
		svc := &fakeService{
			createSessionFunc: func(context.Context, string) (*Session, error) {
				return &Session{Token: "raw-anon-token", AnonymousID: "11111111-1111-1111-1111-111111111111"}, nil
			},
		}
		rec := performAnonymousCreate(t, svc, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if body["anonymous_token"] != "raw-anon-token" {
			t.Errorf("expected anonymous_token in the response, got %v", body["anonymous_token"])
		}
		if body["anonymous_id"] != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("expected anonymous_id in the response, got %v", body["anonymous_id"])
		}
	})

	t.Run("passes a valid X-Device-ID through to the service", func(t *testing.T) {
		var gotDevice string
		svc := &fakeService{
			createSessionFunc: func(_ context.Context, deviceUUID string) (*Session, error) {
				gotDevice = deviceUUID
				return &Session{Token: "t", AnonymousID: "id"}, nil
			},
		}
		rec := performAnonymousCreate(t, svc, "550e8400-e29b-41d4-a716-446655440000")
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotDevice != "550e8400-e29b-41d4-a716-446655440000" {
			t.Errorf("expected device UUID to reach the service, got %q", gotDevice)
		}
	})

	t.Run("rejects a malformed X-Device-ID with 400 and a field hint", func(t *testing.T) {
		svc := &fakeService{}
		rec := performAnonymousCreate(t, svc, "not-a-uuid")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
		if rec.Body.String() != "" && rec.Body.Len() > 0 {
			var body map[string]map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode error JSON: %v", err)
			}
			if body["error"]["field"] != "device_id" {
				t.Errorf("expected field=device_id, got %q", body["error"]["field"])
			}
		}
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			createSessionFunc: func(context.Context, string) (*Session, error) {
				return nil, errors.New("connection pool exhausted")
			},
		}
		rec := performAnonymousCreate(t, svc, "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if body := rec.Body.String(); strings.Contains(body, "connection pool exhausted") {
			t.Errorf("internal error detail leaked to the client: %s", body)
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})

	t.Run("the raw token is never included in any error response", func(t *testing.T) {
		svc := &fakeService{}
		rec := performAnonymousCreate(t, svc, "bad-id")
		if strings.Contains(rec.Body.String(), "raw-anon-token") {
			t.Error("raw token leaked into the 400 response")
		}
	})
}

func TestHandlerAnonymousMe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns the authenticated anonymous id", func(t *testing.T) {
		handler := NewHandler(nil)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.AnonIdentityIDKey, "22222222-2222-2222-2222-222222222222")
			c.Next()
		})
		router.GET("/api/v1/auth/anonymous/me", handler.Me)

		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/anonymous/me", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"anonymous_id":"22222222-2222-2222-2222-222222222222"`) {
			t.Errorf("expected anonymous id in response, got %s", rec.Body.String())
		}
	})

	t.Run("returns 401 when the context has no anonymous id", func(t *testing.T) {
		handler := NewHandler(nil)
		router := gin.New()
		router.GET("/api/v1/auth/anonymous/me", handler.Me)

		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/anonymous/me", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if !strings.Contains(rec.Body.String(), `"code":"`+wantCode+`"`) {
		t.Errorf("expected error code %q in %s", wantCode, rec.Body.String())
	}
}
