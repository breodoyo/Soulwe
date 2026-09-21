package user

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// fakeService embeds the Service interface so the handler tests only need to
// stub the methods under test; the remaining methods fall back to their
// zero-value result.
type fakeService struct {
	Service
	registerFunc func(ctx context.Context, email, password string) (*User, error)
	loginFunc    func(ctx context.Context, email, password string) (*LoginResult, error)
	promoteFunc  func(ctx context.Context, identityID, email, password string, displayName *string) (*PromotionResult, error)
}

func (f *fakeService) Register(ctx context.Context, email, password string) (*User, error) {
	if f.registerFunc == nil {
		return nil, errors.New("registerFunc not configured")
	}
	return f.registerFunc(ctx, email, password)
}

func (f *fakeService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	if f.loginFunc == nil {
		return nil, errors.New("loginFunc not configured")
	}
	return f.loginFunc(ctx, email, password)
}

func (f *fakeService) Promote(ctx context.Context, identityID, email, password string, displayName *string) (*PromotionResult, error) {
	if f.promoteFunc == nil {
		return nil, errors.New("promoteFunc not configured")
	}
	return f.promoteFunc(ctx, identityID, email, password, displayName)
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

func TestHandlerLoginRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	successUser := &User{
		ID:           "11111111-1111-1111-1111-111111111111",
		Email:        "bree@example.com",
		LanguagePref: "en",
		IsVerified:   false,
	}

	t.Run("returns 200 with the access token and user on success", func(t *testing.T) {
		svc := &fakeService{
			loginFunc: func(_ context.Context, _ string, _ string) (*LoginResult, error) {
				return &LoginResult{User: successUser, AccessToken: "test-access-token"}, nil
			},
		}
		rec := performLogin(t, svc, `{"email":"bree@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.Bytes()
		if !bytes.Contains(body, []byte(`"access_token":"test-access-token"`)) {
			t.Errorf("response missing access token: %s", body)
		}
		if !bytes.Contains(body, []byte(`"token_type":"Bearer"`)) {
			t.Errorf("response missing token type: %s", body)
		}
		if !bytes.Contains(body, []byte(`"expires_in":900`)) {
			t.Errorf("response missing expires_in: %s", body)
		}
		if !bytes.Contains(body, []byte(`"email":"bree@example.com"`)) {
			t.Errorf("response missing user email: %s", body)
		}
		if bytes.Contains(body, []byte("password_hash")) || bytes.Contains(body, []byte(`"password`)) {
			t.Errorf("password or its hash leaked in the response: %s", body)
		}
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		svc := &fakeService{}
		rec := performLogin(t, svc, `{"email":`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 401 without exposing which field failed", func(t *testing.T) {
		svc := &fakeService{
			loginFunc: func(context.Context, string, string) (*LoginResult, error) {
				return nil, ErrBadCredentials
			},
		}
		rec := performLogin(t, svc, `{"email":"nobody@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"UNAUTHORIZED"`)) {
			t.Errorf("expected UNAUTHORIZED code in %s", rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"field"`)) {
			t.Errorf("response must not reveal which field was wrong: %s", rec.Body.String())
		}
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			loginFunc: func(context.Context, string, string) (*LoginResult, error) {
				return nil, errors.New("signing key service unavailable")
			},
		}
		rec := performLogin(t, svc, `{"email":"bree@example.com","password":"a-strong-password"}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("signing key service unavailable")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func performLogin(t *testing.T, svc Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(svc)

	router := gin.New()
	router.POST("/api/v1/auth/login", handler.Login)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
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

func TestHandlerMeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns the authenticated user id", func(t *testing.T) {
		handler := NewHandler(nil)
		router := gin.New()
		// Simulate what the auth middleware does: store the user id in context.
		router.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, "11111111-1111-1111-1111-111111111111")
			c.Next()
		})
		router.GET("/api/v1/auth/me", handler.Me)

		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"user_id":"11111111-1111-1111-1111-111111111111"`)) {
			t.Errorf("expected user id in response, got %s", rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("password_hash")) {
			t.Errorf("response must never include a password hash: %s", rec.Body.String())
		}
	})

	t.Run("returns 401 when the context has no user id", func(t *testing.T) {
		handler := NewHandler(nil)
		router := gin.New()
		router.GET("/api/v1/auth/me", handler.Me)

		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
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

// performPromote runs the promote handler with the anonymous identity stored in
// the request context, exactly as the anonymous auth middleware would. Passing
// an empty identityID simulates a request without anonymous authentication.
func performPromote(t *testing.T, svc Service, body, identityID string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(svc)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if identityID != "" {
			c.Set(middleware.AnonIdentityIDKey, identityID)
		}
		c.Next()
	})
	router.POST("/api/v1/auth/anonymous/promote", handler.Promote)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous/promote", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandlerPromoteRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	successUser := &User{
		ID:           "11111111-1111-1111-1111-111111111111",
		Email:        "bree@example.com",
		DisplayName:  stringPtr("Bree"),
		LanguagePref: "en",
		IsVerified:   false,
	}

	t.Run("returns 201 with the token and user on success", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(_ context.Context, identityID, email, _ string, _ *string) (*PromotionResult, error) {
				if identityID != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("expected the anonymous identity to reach the service, got %q", identityID)
				}
				user := *successUser
				user.Email = email
				return &PromotionResult{User: &user, AccessToken: "promoted-access-token"}, nil
			},
		}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"a-strong-password","display_name":"Bree"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.Bytes()
		if !bytes.Contains(body, []byte(`"access_token":"promoted-access-token"`)) {
			t.Errorf("response missing access token: %s", body)
		}
		if !bytes.Contains(body, []byte(`"token_type":"Bearer"`)) {
			t.Errorf("response missing token type: %s", body)
		}
		if !bytes.Contains(body, []byte(`"expires_in":900`)) {
			t.Errorf("response missing expires_in: %s", body)
		}
		if !bytes.Contains(body, []byte(`"email":"bree@example.com"`)) {
			t.Errorf("response missing user email: %s", body)
		}
		if !bytes.Contains(body, []byte(`"display_name":"Bree"`)) {
			t.Errorf("response missing display name: %s", body)
		}
		if bytes.Contains(body, []byte("password_hash")) || bytes.Contains(body, []byte(`"password`)) {
			t.Errorf("password or its hash leaked in the response: %s", body)
		}
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		svc := &fakeService{}
		rec := performPromote(t, svc, `{"email":`, "22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 400 for invalid email with field set", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				return nil, ErrInvalidEmail
			},
		}
		rec := performPromote(t, svc,
			`{"email":"not-an-email","password":"a-strong-password"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "INVALID_INPUT", "email")
	})

	t.Run("returns 400 for invalid password with field set", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				return nil, ErrInvalidPassword
			},
		}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"short"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "INVALID_INPUT", "password")
	})

	t.Run("returns 401 when the anonymous identity is missing from context", func(t *testing.T) {
		svc := &fakeService{}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"a-strong-password"}`, "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 409 for duplicate email", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				return nil, ErrEmailTaken
			},
		}
		rec := performPromote(t, svc,
			`{"email":"taken@example.com","password":"a-strong-password"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "CONFLICT")
	})

	t.Run("returns 409 for an already promoted identity", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				return nil, ErrIdentityAlreadyPromoted
			},
		}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"a-strong-password"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "CONFLICT")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				return nil, errors.New("database transaction failed")
			},
		}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"a-strong-password"}`,
			"22222222-2222-2222-2222-222222222222")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database transaction failed")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})

	t.Run("no password, hash, or anonymous token is ever returned", func(t *testing.T) {
		svc := &fakeService{
			promoteFunc: func(context.Context, string, string, string, *string) (*PromotionResult, error) {
				user := *successUser
				user.PasswordHash = "$2a$12$abcdefghijklmnopqrstuv"
				return &PromotionResult{User: &user, AccessToken: "promoted-access-token"}, nil
			},
		}
		rec := performPromote(t, svc,
			`{"email":"bree@example.com","password":"a-strong-password"}`,
			"22222222-2222-2222-2222-222222222222")

		body := rec.Body.String()
		if bytes.Contains(rec.Body.Bytes(), []byte("password_hash")) || bytes.Contains(rec.Body.Bytes(), []byte(`"password`)) {
			t.Errorf("password or its hash leaked in the response: %s", body)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("22222222-2222-2222-2222-222222222222")) {
			t.Errorf("the anonymous identity (raw token holder) leaked into the response: %s", body)
		}
	})
}

func stringPtr(s string) *string { return &s }
