package user

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeService embeds the Service interface so the handler tests only need to
// stub Register; the remaining methods fall back to their zero-value result.
type fakeService struct {
	Service
	registerFunc func(ctx context.Context, email, password string) (*User, error)
}

func (f *fakeService) Register(ctx context.Context, email, password string) (*User, error) {
	if f.registerFunc == nil {
		return nil, errors.New("registerFunc not configured")
	}
	return f.registerFunc(ctx, email, password)
}

func TestHandlerRegisterRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 201 and a safe user object on success", func(t *testing.T) {
		svc := &fakeService{
			registerFunc: func(_ context.Context, email, _ string) (*User, error) {
				return &User{
					ID:           "11111111-1111-1111-1111-111111111111",
					Email:        email,
					LanguagePref: "en",
					IsVerified:   false,
				}, nil
			},
		}
		rec := performRegister(t, svc, `{"email":"bree@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.Bytes()
		if !bytes.Contains(body, []byte(`"email":"bree@example.com"`)) {
			t.Errorf("response missing email: %s", body)
		}
		if !bytes.Contains(body, []byte(`"id":"11111111-1111-1111-1111-111111111111"`)) {
			t.Errorf("response missing id: %s", body)
		}
		if bytes.Contains(body, []byte("password_hash")) || bytes.Contains(body, []byte(`"password`)) {
			t.Errorf("password or its hash leaked in the response: %s", body)
		}
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		svc := &fakeService{}
		rec := performRegister(t, svc, `{"email":`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 400 for invalid email with field set", func(t *testing.T) {
		svc := &fakeService{
			registerFunc: func(context.Context, string, string) (*User, error) {
				return nil, ErrInvalidEmail
			},
		}
		rec := performRegister(t, svc, `{"email":"not-an-email","password":"a-strong-password"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "INVALID_INPUT", "email")
	})

	t.Run("returns 400 for invalid password with field set", func(t *testing.T) {
		svc := &fakeService{
			registerFunc: func(context.Context, string, string) (*User, error) {
				return nil, ErrInvalidPassword
			},
		}
		rec := performRegister(t, svc, `{"email":"bree@example.com","password":"short"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "INVALID_INPUT", "password")
	})

	t.Run("returns 409 for duplicate email", func(t *testing.T) {
		svc := &fakeService{
			registerFunc: func(context.Context, string, string) (*User, error) {
				return nil, ErrEmailTaken
			},
		}
		rec := performRegister(t, svc, `{"email":"taken@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "CONFLICT")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			registerFunc: func(context.Context, string, string) (*User, error) {
				return nil, errors.New("connection pool exhausted")
			},
		}
		rec := performRegister(t, svc, `{"email":"bree@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("connection pool exhausted")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func performRegister(t *testing.T, svc Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(svc)

	router := gin.New()
	router.POST("/api/v1/auth/register", handler.Register)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"`+wantCode+`"`)) {
		t.Errorf("expected error code %q in %s", wantCode, rec.Body.String())
	}
}

func assertErrorField(t *testing.T, rec *httptest.ResponseRecorder, wantCode, wantField string) {
	t.Helper()
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"`+wantCode+`"`)) {
		t.Errorf("expected error code %q in %s", wantCode, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"field":"`+wantField+`"`)) {
		t.Errorf("expected field %q in %s", wantField, rec.Body.String())
	}
}
