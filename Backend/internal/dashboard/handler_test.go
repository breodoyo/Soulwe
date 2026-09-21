package dashboard

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"Backend/internal/middleware"
	"Backend/internal/mood"
	"Backend/internal/user"

	"github.com/gin-gonic/gin"
)

// fakeService embeds the Service interface so handler tests only need to stub
// Get.
type fakeService struct {
	Service
	getFunc func(ctx context.Context, userID string) (*Dashboard, error)
}

func (f *fakeService) Get(ctx context.Context, userID string) (*Dashboard, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, userID)
}

func TestHandlerGetRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 200 with the full dashboard snapshot", func(t *testing.T) {
		latest := testMood("Better")
		svc := &fakeService{
			getFunc: func(_ context.Context, userID string) (*Dashboard, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				return &Dashboard{
					User:              testUser(),
					LatestMood:        &latest,
					RecentMoods:       []mood.MoodLog{latest},
					MoodCheckinsCount: 1,
				}, nil
			},
		}
		rec := performDashboardGet(t, svc, testUserID)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.Bytes()
		for _, want := range []string{`"user"`, `"latest_mood"`, `"recent_moods"`, `"mood_checkins_count":1`, `"email":"bree@example.com"`, `"mood":"Better"`} {
			if !bytes.Contains(body, []byte(want)) {
				t.Errorf("response missing %s: %s", want, body)
			}
		}
		if bytes.Contains(body, []byte("password_hash")) || bytes.Contains(body, []byte(`"password`)) {
			t.Errorf("password or its hash leaked in the response: %s", body)
		}
	})

	t.Run("returns null latest, empty recent, and zero count when no check-ins exist", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string) (*Dashboard, error) {
				return &Dashboard{
					User:              testUser(),
					LatestMood:        nil,
					RecentMoods:       []mood.MoodLog{},
					MoodCheckinsCount: 0,
				}, nil
			},
		}
		rec := performDashboardGet(t, svc, testUserID)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"latest_mood":null`)) {
			t.Errorf("expected latest_mood null: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"recent_moods":[]`)) {
			t.Errorf("expected an empty recent_moods array: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"mood_checkins_count":0`)) {
			t.Errorf("expected count 0: %s", body)
		}
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := performDashboardGet(t, &fakeService{}, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"UNAUTHORIZED"`)) {
			t.Errorf("unexpected error body: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 when the authenticated user no longer exists", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string) (*Dashboard, error) {
				return nil, user.ErrUserNotFound
			},
		}
		rec := performDashboardGet(t, svc, testUserID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string) (*Dashboard, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := performDashboardGet(t, svc, testUserID)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
	})
}

func performDashboardGet(t *testing.T, svc Service, userID string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set(middleware.UserIDKey, userID)
		}
		c.Next()
	})
	router.GET("/api/v1/dashboard", NewHandler(svc).Get)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
