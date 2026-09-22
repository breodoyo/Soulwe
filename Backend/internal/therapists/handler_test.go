package therapists

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

const testUserID = "11111111-1111-1111-1111-111111111111"

// fakeService embeds the Service interface so handler tests only need to stub
// the methods under test.
type fakeService struct {
	Service
	listFunc func(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error)
	getFunc  func(ctx context.Context, therapistID string) (*Therapist, error)
}

func (f *fakeService) List(ctx context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx, opts, limit, before)
}

func (f *fakeService) Get(ctx context.Context, therapistID string) (*Therapist, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, therapistID)
}

// requestRouter builds a router that simulates the AuthRequired middleware by
// stamping UserIDKey into the Gin context, then serves the request. An empty
// userID leaves the context unstamped, exercising the handler's 401 path.
func requestRouter(t *testing.T, method, path, userID string, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	return requestRouterPath(t, method, path, path, userID, handler)
}

// requestRouterPath is requestRouter with an explicit route pattern and request
// path, so parameterized routes (e.g. /therapists/:id) can be registered while
// concrete requests are served.
func requestRouterPath(t *testing.T, method, route, requestPath, userID string, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set(middleware.UserIDKey, userID)
		}
		c.Next()
	})
	routeOnly := route
	if i := strings.IndexAny(route, "?"); i >= 0 {
		routeOnly = route[:i]
	}
	router.Handle(method, routeOnly, handler)

	req, err := http.NewRequest(method, requestPath, io.Reader(bytes.NewReader(nil)))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if !strings.Contains(rec.Body.String(), `"code":"`+want+`"`) {
		t.Errorf("expected error code %q in body: %s", want, rec.Body.String())
	}
}

func assertErrorField(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if !strings.Contains(rec.Body.String(), `"field":"`+want+`"`) {
		t.Errorf("expected error field %q in body: %s", want, rec.Body.String())
	}
}

func seededTherapist() *Therapist {
	bio := "Clinical psychologist with 8 years of experience."
	price := 800
	return &Therapist{
		ID:           "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DisplayName:  "Dr. Amina Korir",
		Bio:          &bio,
		Languages:    []string{"English", "Swahili"},
		Specialties:  []string{"Grief", "Trauma"},
		SessionPrice: &price,
		Currency:     SessionCurrency,
		IsActive:     true,
		CreatedAt:    time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestHandlerListRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	therapists := []Therapist{*seededTherapist()}

	t.Run("returns 200 with the directory and a fully-populated therapist", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, opts ListOptions, limit int, before *time.Time) ([]Therapist, error) {
				if limit != 25 {
					t.Errorf("expected limit 25 to reach the service, got %d", limit)
				}
				if before != nil {
					t.Errorf("expected no cursor, got %v", before)
				}
				return therapists, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?limit=25", testUserID, NewHandler(svc).List)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"therapists":[`)) {
			t.Errorf("response missing therapists array: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"display_name":"Dr. Amina Korir"`)) {
			t.Errorf("response missing display_name: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"currency":"KES"`)) {
			t.Errorf("response missing the fixed currency: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"session_price":800`)) {
			t.Errorf("response missing session_price: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"full_name"`)) {
			t.Errorf("internal column full_name leaked: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"price_kes"`)) {
			t.Errorf("internal column price_kes leaked: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"credentials"`)) {
			t.Errorf("internal column credentials leaked: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"email"`)) {
			t.Errorf("email leaked into the directory: %s", body)
		}
	})

	t.Run("passes blank filters as absent", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, opts ListOptions, _ int, _ *time.Time) ([]Therapist, error) {
				if opts.Language != "" || opts.Specialty != "" {
					t.Errorf("expected blank filters to be stripped, got %+v", opts)
				}
				return therapists, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?language=%20%20&specialty=%20", testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("passes filters and cursor through to the service", func(t *testing.T) {
		cursor := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
		svc := &fakeService{
			listFunc: func(_ context.Context, opts ListOptions, _ int, before *time.Time) ([]Therapist, error) {
				if opts.Language != "Swahili" || opts.Specialty != "Grief" {
					t.Errorf("expected filters to reach the service, got %+v", opts)
				}
				if before == nil || !before.Equal(cursor) {
					t.Errorf("expected the cursor to reach the service, got %v", before)
				}
				return therapists, nil
			},
		}
		before := cursor.Format(time.RFC3339Nano)
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?language=Swahili&specialty=Grief&before="+before, testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 200 with an empty list when nothing matches", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, ListOptions, int, *time.Time) ([]Therapist, error) {
				return []Therapist{}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists", testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"therapists":[]`)) {
			t.Errorf("expected an empty therapists array: %s", rec.Body.String())
		}
	})

	t.Run("returns 400 for invalid limit with the field set", func(t *testing.T) {
		t.Run("non-integer", func(t *testing.T) {
			rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?limit=abc", testUserID, NewHandler(&fakeService{}).List)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorField(t, rec, "limit")
		})
		t.Run("non-positive", func(t *testing.T) {
			rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?limit=0", testUserID, NewHandler(&fakeService{}).List)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorField(t, rec, "limit")
		})
	})

	t.Run("returns 400 for an invalid before cursor", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists?before=not-a-timestamp", testUserID, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "before")
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists", "", NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, ListOptions, int, *time.Time) ([]Therapist, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists", testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func TestHandlerGetRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 200 with the public profile on success", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(_ context.Context, therapistID string) (*Therapist, error) {
				if therapistID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
					t.Errorf("expected the profile id, got %q", therapistID)
				}
				return seededTherapist(), nil
			},
		}
		rec := requestRouterPath(t, http.MethodGet, "/api/v1/therapists/:id", "/api/v1/therapists/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", testUserID, NewHandler(svc).Get)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"therapist":{`)) {
			t.Errorf("response missing therapist object: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"email"`)) {
			t.Errorf("email leaked into the profile: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"password"`)) {
			t.Errorf("password material leaked into the profile: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"years_exp"`)) {
			t.Errorf("internal column years_exp leaked into the profile: %s", body)
		}
	})

	t.Run("returns 400 for an invalid therapist id", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/therapists/not-a-uuid", testUserID, NewHandler(&fakeService{}).Get)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "id")
	})

	t.Run("returns 404 when the therapist is missing", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string) (*Therapist, error) {
				return nil, ErrTherapistNotFound
			},
		}
		rec := requestRouterPath(t, http.MethodGet, "/api/v1/therapists/:id", "/api/v1/therapists/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", testUserID, NewHandler(svc).Get)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodGet, "/api/v1/therapists/:id", "/api/v1/therapists/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "", NewHandler(&fakeService{}).Get)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string) (*Therapist, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouterPath(t, http.MethodGet, "/api/v1/therapists/:id", "/api/v1/therapists/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", testUserID, NewHandler(svc).Get)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d:", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}
