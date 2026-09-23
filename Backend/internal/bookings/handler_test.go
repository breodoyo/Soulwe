package bookings

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

const handlerUserID = "11111111-1111-1111-1111-111111111111"

// fakeService embeds the Service interface so handler tests only stub the
// methods under test.
type fakeService struct {
	Service
	createFunc func(ctx context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error)
	listFunc   func(ctx context.Context, userID string) ([]Booking, error)
	getFunc    func(ctx context.Context, userID, bookingID string) (*Booking, error)
	cancelFunc func(ctx context.Context, userID, bookingID string) (*Booking, error)
}

func (f *fakeService) Create(ctx context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error) {
	if f.createFunc == nil {
		return nil, errors.New("createFunc not configured")
	}
	return f.createFunc(ctx, userID, therapistID, scheduledAt)
}

func (f *fakeService) List(ctx context.Context, userID string) ([]Booking, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx, userID)
}

func (f *fakeService) Get(ctx context.Context, userID, bookingID string) (*Booking, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, userID, bookingID)
}

func (f *fakeService) Cancel(ctx context.Context, userID, bookingID string) (*Booking, error) {
	if f.cancelFunc == nil {
		return nil, errors.New("cancelFunc not configured")
	}
	return f.cancelFunc(ctx, userID, bookingID)
}

// requestRouterPath builds a router that simulates the AuthRequired middleware
// by stamping UserIDKey into the Gin context, then serves the request. An
// empty userID leaves the context unstamped, exercising the handler's 401 path.
// An optional body feeds POST/PATCH routes.
func requestRouterPath(t *testing.T, method, route, requestPath, userID string, body io.Reader, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set(middleware.UserIDKey, userID)
		}
		c.Next()
	})
	router.Handle(method, route, handler)

	req, err := http.NewRequest(method, requestPath, body)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func seededBooking(status string) *Booking {
	return &Booking{
		ID:          "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		UserID:      handlerUserID,
		TherapistID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DisplayName: "Dr. Amina Korir",
		ScheduledAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		Status:      status,
		CreatedAt:   time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
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

func TestHandlerCreateRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/therapists/:id/bookings"
	path := "/api/v1/therapists/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/bookings"
	validBody := `{"scheduled_at":"2026-10-01T10:00:00Z"}`
	emptyBody := io.Reader(bytes.NewReader(nil))

	t.Run("returns 201 with a pending booking and no owner internals", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(_ context.Context, userID, therapistID string, scheduledAt time.Time) (*Booking, error) {
				if userID != handlerUserID {
					t.Errorf("expected user %q, got %q", handlerUserID, userID)
				}
				if therapistID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
					t.Errorf("expected therapist id, got %q", therapistID)
				}
				if !scheduledAt.Equal(time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)) {
					t.Errorf("expected parsed scheduled_at, got %v", scheduledAt)
				}
				return seededBooking(StatusPending), nil
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"booking":{`) {
			t.Errorf("response missing booking object: %s", body)
		}
		for _, want := range []string{`"status":"pending"`, `"display_name":"Dr. Amina Korir"`, `"therapist_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"`} {
			if !strings.Contains(body, want) {
				t.Errorf("response missing %s: %s", want, body)
			}
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"user_id"`)) {
			t.Errorf("owner user_id leaked into the response: %s", body)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"password"`)) {
			t.Errorf("password material leaked into the response: %s", body)
		}
	})

	t.Run("returns 400 for an invalid therapist id", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, "/api/v1/therapists/not-a-uuid/bookings", handlerUserID, strings.NewReader(validBody), NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "id")
	})

	t.Run("returns 400 for a malformed JSON body", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(`{"scheduled_at":`), NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 400 when scheduled_at is missing", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(`{}`), NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "scheduled_at")
	})

	t.Run("returns 400 for a malformed scheduled_at", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(`{"scheduled_at":"not-a-timestamp"}`), NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "scheduled_at")
	})

	t.Run("returns 400 for a past scheduled_at with the field set", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, time.Time) (*Booking, error) {
				return nil, ErrScheduledInPast
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "scheduled_at")
	})

	t.Run("returns 404 for a missing therapist", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, time.Time) (*Booking, error) {
				return nil, ErrTherapistMissing
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 409 for an inactive therapist", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, time.Time) (*Booking, error) {
				return nil, ErrTherapistInactive
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "THERAPIST_UNAVAILABLE")
	})

	t.Run("returns 409 for a slot conflict", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, time.Time) (*Booking, error) {
				return nil, ErrBookingConflict
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "BOOKING_CONFLICT")
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, path, "", strings.NewReader(validBody), NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, time.Time) (*Booking, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, strings.NewReader(validBody), NewHandler(svc).Create)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})

	t.Run("an empty body is rejected as malformed before reaching the service", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPost, route, path, handlerUserID, emptyBody, NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})
}

func TestHandlerListRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/bookings"

	t.Run("returns 200 with the caller's bookings", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, userID string) ([]Booking, error) {
				if userID != handlerUserID {
					t.Errorf("expected user %q, got %q", handlerUserID, userID)
				}
				return []Booking{*seededBooking(StatusPending)}, nil
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, route, handlerUserID, nil, NewHandler(svc).List)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"bookings":[{`) {
			t.Errorf("response missing bookings array: %s", body)
		}
		if !strings.Contains(body, `"display_name":"Dr. Amina Korir"`) {
			t.Errorf("response missing display_name: %s", body)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"user_id"`)) {
			t.Errorf("owner user_id leaked into the response: %s", body)
		}
	})

	t.Run("returns 200 with an empty list when the user has none", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, string) ([]Booking, error) {
				return []Booking{}, nil
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, route, handlerUserID, nil, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"bookings":[]`)) {
			t.Errorf("expected an empty bookings array: %s", rec.Body.String())
		}
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodGet, route, route, "", nil, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, string) ([]Booking, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, route, handlerUserID, nil, NewHandler(svc).List)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}

func TestHandlerGetRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/bookings/:id"
	path := "/api/v1/bookings/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	t.Run("returns 200 with the booking", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(_ context.Context, userID, bookingID string) (*Booking, error) {
				if userID != handlerUserID {
					t.Errorf("expected scoped user, got %q", userID)
				}
				return seededBooking(StatusPending), nil
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, path, handlerUserID, nil, NewHandler(svc).Get)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"booking":{`) {
			t.Errorf("response missing booking object: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for an unknown or foreign booking", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, ErrBookingNotFound
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, path, handlerUserID, nil, NewHandler(svc).Get)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 400 for an invalid booking id", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodGet, route, "/api/v1/bookings/not-a-uuid", handlerUserID, nil, NewHandler(&fakeService{}).Get)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "id")
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodGet, route, path, "", nil, NewHandler(&fakeService{}).Get)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouterPath(t, http.MethodGet, route, path, handlerUserID, nil, NewHandler(svc).Get)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}

func TestHandlerCancelRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/bookings/:id/cancel"
	path := "/api/v1/bookings/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb/cancel"

	t.Run("returns 200 with the cancelled booking", func(t *testing.T) {
		svc := &fakeService{
			cancelFunc: func(_ context.Context, userID, bookingID string) (*Booking, error) {
				if userID != handlerUserID {
					t.Errorf("expected scoped user, got %q", userID)
				}
				return seededBooking(StatusCancelled), nil
			},
		}
		rec := requestRouterPath(t, http.MethodPatch, route, path, handlerUserID, nil, NewHandler(svc).Cancel)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"status":"cancelled"`) {
			t.Errorf("expected cancelled booking: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for an unknown or foreign booking", func(t *testing.T) {
		svc := &fakeService{
			cancelFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, ErrBookingNotFound
			},
		}
		rec := requestRouterPath(t, http.MethodPatch, route, path, handlerUserID, nil, NewHandler(svc).Cancel)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 409 for a non-pending booking", func(t *testing.T) {
		svc := &fakeService{
			cancelFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, ErrBookingStatusConflict
			},
		}
		rec := requestRouterPath(t, http.MethodPatch, route, path, handlerUserID, nil, NewHandler(svc).Cancel)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "BOOKING_STATUS_CONFLICT")
	})

	t.Run("returns 400 for an invalid booking id", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPatch, route, "/api/v1/bookings/not-a-uuid/cancel", handlerUserID, nil, NewHandler(&fakeService{}).Cancel)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "id")
	})

	t.Run("returns 401 when the user is not authenticated", func(t *testing.T) {
		rec := requestRouterPath(t, http.MethodPatch, route, path, "", nil, NewHandler(&fakeService{}).Cancel)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			cancelFunc: func(context.Context, string, string) (*Booking, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouterPath(t, http.MethodPatch, route, path, handlerUserID, nil, NewHandler(svc).Cancel)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}
