package mood

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
const testAnonID = "33333333-3333-3333-3333-333333333333"

// fakeService embeds the Service interface so handler tests only need to
// stub the methods under test.
type fakeService struct {
	Service
	createFunc func(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error)
	listFunc   func(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error)
}

func (f *fakeService) Create(ctx context.Context, owner middleware.Owner, mood string) (*MoodLog, error) {
	if f.createFunc == nil {
		return nil, errors.New("createFunc not configured")
	}
	return f.createFunc(ctx, owner, mood)
}

func (f *fakeService) List(ctx context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx, owner, limit)
}

// requestRouter builds a router that simulates the IdentityRequired middleware
// by stamping OwnerKey into the Gin context, then serves the request.
func requestRouter(t *testing.T, method, path, body string, owner middleware.Owner, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		// An ill-formed owner is treated as "no identity", mirroring how the
		// real middleware would simply not have set the key.
		if _, ok := middleware.IdentityFromOwner(owner); ok {
			c.Set(middleware.OwnerKey, owner)
		}
		c.Next()
	})
	pathOnly := path
	if i := strings.IndexAny(path, "?"); i >= 0 {
		pathOnly = path[:i]
	}
	router.Handle(method, pathOnly, handler)

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func moodLog(mood string) *MoodLog {
	return &MoodLog{
		ID:       "22222222-2222-2222-2222-222222222222",
		UserID:   testUserID,
		Mood:     mood,
		LoggedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func anonMoodLog(mood string) *MoodLog {
	return &MoodLog{
		ID:             "44444444-4444-4444-4444-444444444444",
		AnonIdentityID: testAnonID,
		Mood:           mood,
		LoggedAt:       time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
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

	registeredOwner := middleware.Owner{UserID: testUserID}
	anonOwner := middleware.Owner{AnonIdentityID: testAnonID}

	t.Run("returns 201 with the created check-in on success", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(_ context.Context, owner middleware.Owner, mood string) (*MoodLog, error) {
				if owner.UserID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", owner.UserID)
				}
				return moodLog(mood), nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`, registeredOwner, NewHandler(svc).Create)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"mood":"Better"`)) {
			t.Errorf("response missing mood: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"logged_at"`)) {
			t.Errorf("response missing logged_at: %s", body)
		}
	})

	t.Run("accepts an anonymous session and records the check-in for it", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(_ context.Context, owner middleware.Owner, mood string) (*MoodLog, error) {
				if !owner.Anonymous() {
					t.Errorf("expected an anonymous owner, got %+v", owner)
				}
				if owner.AnonIdentityID != testAnonID {
					t.Errorf("expected anon identity %q, got %q", testAnonID, owner.AnonIdentityID)
				}
				if owner.UserID != "" {
					t.Errorf("an anonymous owner must not carry a user id, got %q", owner.UserID)
				}
				return anonMoodLog(mood), nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`, anonOwner, NewHandler(svc).Create)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for an anonymous check-in, got %d: %s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(testAnonID)) {
			t.Errorf("anonymous identity must never be serialized to the client: %s", rec.Body.String())
		}
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":`, registeredOwner, NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 400 for an invalid mood with the field set", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, middleware.Owner, string) (*MoodLog, error) {
				return nil, ErrInvalidMood
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":"sad"}`, registeredOwner, NewHandler(svc).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "mood")
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`, middleware.Owner{}, NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, middleware.Owner, string) (*MoodLog, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`, registeredOwner, NewHandler(svc).Create)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func TestHandlerListRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registeredOwner := middleware.Owner{UserID: testUserID}
	anonOwner := middleware.Owner{AnonIdentityID: testAnonID}
	recent := []MoodLog{*moodLog("Grateful"), *moodLog("Better")}

	t.Run("returns 200 with the user's mood list", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
				if owner.UserID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", owner.UserID)
				}
				if limit != 10 {
					t.Errorf("expected limit 10 to reach the service, got %d", limit)
				}
				return recent, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods?limit=10", "", registeredOwner, NewHandler(svc).List)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"moods":[`)) {
			t.Errorf("response missing moods array: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"user_id"`)) {
			t.Errorf("user_id must never appear in the list response: %s", body)
		}
	})

	t.Run("returns 200 with the anonymous session's own list", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, owner middleware.Owner, _ int) ([]MoodLog, error) {
				if !owner.Anonymous() {
					t.Errorf("expected an anonymous owner, got %+v", owner)
				}
				if owner.AnonIdentityID != testAnonID {
					t.Errorf("expected anon identity %q, got %q", testAnonID, owner.AnonIdentityID)
				}
				return []MoodLog{*anonMoodLog("At peace")}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods", "", anonOwner, NewHandler(svc).List)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(`"anon_identity_id"`)) {
			t.Errorf("anon_identity_id must never appear in the list response: %s", rec.Body.String())
		}
	})

	t.Run("returns 200 with an empty list when the user has no check-ins", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, middleware.Owner, int) ([]MoodLog, error) {
				return []MoodLog{}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods", "", registeredOwner, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"moods":[]`)) {
			t.Errorf("expected an empty moods array: %s", rec.Body.String())
		}
	})

	t.Run("returns 400 for a non-integer limit", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods?limit=abc", "", registeredOwner, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "limit")
	})

	t.Run("returns 400 for a non-positive limit", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods?limit=0", "", registeredOwner, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "limit")
	})

	t.Run("passes no limit to the service when the parameter is absent", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, owner middleware.Owner, limit int) ([]MoodLog, error) {
				if limit != 0 {
					t.Errorf("expected limit 0 so the service applies its default %d, got %d", DefaultListLimit, limit)
				}
				return recent, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods", "", registeredOwner, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods", "", middleware.Owner{}, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(context.Context, middleware.Owner, int) ([]MoodLog, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/moods", "", registeredOwner, NewHandler(svc).List)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}
