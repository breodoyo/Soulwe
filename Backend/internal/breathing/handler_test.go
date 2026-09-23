package breathing

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
const testExerciseID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

// fakeService embeds the Service interface so handler tests only need to
// stub the methods under test.
type fakeService struct {
	Service
	listExercisesFunc func(ctx context.Context, limit int) ([]Exercise, error)
	getExerciseFunc   func(ctx context.Context, exerciseID string) (*Exercise, error)
	recordSessionFunc func(ctx context.Context, userID, exerciseID string, breaths, durationS int, completed bool) (*Session, error)
	listSessionsFunc  func(ctx context.Context, userID string, limit int) ([]Session, error)
}

func (f *fakeService) ListExercises(ctx context.Context, limit int) ([]Exercise, error) {
	if f.listExercisesFunc == nil {
		return nil, errors.New("listExercisesFunc not configured")
	}
	return f.listExercisesFunc(ctx, limit)
}

func (f *fakeService) GetExercise(ctx context.Context, exerciseID string) (*Exercise, error) {
	if f.getExerciseFunc == nil {
		return nil, errors.New("getExerciseFunc not configured")
	}
	return f.getExerciseFunc(ctx, exerciseID)
}

func (f *fakeService) RecordSession(ctx context.Context, userID, exerciseID string, breaths, durationS int, completed bool) (*Session, error) {
	if f.recordSessionFunc == nil {
		return nil, errors.New("recordSessionFunc not configured")
	}
	return f.recordSessionFunc(ctx, userID, exerciseID, breaths, durationS, completed)
}

func (f *fakeService) ListSessions(ctx context.Context, userID string, limit int) ([]Session, error) {
	if f.listSessionsFunc == nil {
		return nil, errors.New("listSessionsFunc not configured")
	}
	return f.listSessionsFunc(ctx, userID, limit)
}

// requestRouter builds a router that simulates the AuthRequired middleware by
// stamping UserIDKey into the Gin context, then serves the request. The route
// pattern (with :id placeholders) and the concrete request path are supplied
// separately.
func requestRouter(t *testing.T, method, routePattern, requestPath, body, userID string, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set(middleware.UserIDKey, userID)
		}
		c.Next()
	})
	router.Handle(method, routePattern, handler)

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, requestPath, reader)
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

func exercise() *Exercise {
	return &Exercise{
		ID:          testExerciseID,
		Slug:        "478",
		Name:        "4-7-8 Breathing",
		Description: "Inhale 4s, hold 7s, exhale 8s",
		Technique:   "478",
		InhaleS:     4,
		HoldS:       7,
		ExhaleS:     8,
	}
}

func session() *Session {
	name := "4-7-8 Breathing"
	exID := testExerciseID
	return &Session{
		ID:         "22222222-2222-2222-2222-222222222222",
		UserID:     testUserID,
		ExerciseID: &exID,
		Technique:  "478",
		Name:       &name,
		Breaths:    5,
		DurationS:  95,
		Completed:  true,
		CreatedAt:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
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

func TestHandlerListExercises(t *testing.T) {
	t.Run("returns 200 with the catalog", func(t *testing.T) {
		svc := &fakeService{
			listExercisesFunc: func(_ context.Context, limit int) ([]Exercise, error) {
				if limit != 10 {
					t.Errorf("expected limit 10 to reach the service, got %d", limit)
				}
				return []Exercise{*exercise()}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises",
			"/api/v1/breathing/exercises?limit=10", "", testUserID, NewHandler(svc).ListExercises)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"exercises":[`)) {
			t.Errorf("response missing exercises array: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"slug":"478"`)) {
			t.Errorf("response missing exercise slug: %s", body)
		}
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises",
			"/api/v1/breathing/exercises", "", "", NewHandler(&fakeService{}).ListExercises)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 400 for a non-integer limit", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises",
			"/api/v1/breathing/exercises?limit=abc", "", testUserID, NewHandler(&fakeService{}).ListExercises)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "limit")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			listExercisesFunc: func(context.Context, int) ([]Exercise, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises",
			"/api/v1/breathing/exercises", "", testUserID, NewHandler(svc).ListExercises)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func TestHandlerGetExercise(t *testing.T) {
	t.Run("returns 200 with the exercise", func(t *testing.T) {
		svc := &fakeService{
			getExerciseFunc: func(_ context.Context, exerciseID string) (*Exercise, error) {
				if exerciseID != testExerciseID {
					t.Errorf("expected exercise id %q, got %q", testExerciseID, exerciseID)
				}
				return exercise(), nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises/:id",
			"/api/v1/breathing/exercises/"+testExerciseID, "", testUserID, NewHandler(svc).GetExercise)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"exercise":{`)) {
			t.Errorf("response missing exercise object: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for a missing exercise", func(t *testing.T) {
		svc := &fakeService{
			getExerciseFunc: func(context.Context, string) (*Exercise, error) {
				return nil, ErrExerciseNotFound
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises/:id",
			"/api/v1/breathing/exercises/00000000-0000-0000-0000-000000000000", "", testUserID, NewHandler(svc).GetExercise)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 400 for a malformed exercise id", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises/:id",
			"/api/v1/breathing/exercises/not-a-uuid", "", testUserID, NewHandler(&fakeService{}).GetExercise)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "id")
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/exercises/:id",
			"/api/v1/breathing/exercises/"+testExerciseID, "", "", NewHandler(&fakeService{}).GetExercise)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})
}

func TestHandlerRecordSession(t *testing.T) {
	t.Run("returns 201 with the created session", func(t *testing.T) {
		svc := &fakeService{
			recordSessionFunc: func(_ context.Context, userID, exerciseID string, breaths, durationS int, completed bool) (*Session, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if exerciseID != testExerciseID || breaths != 5 || durationS != 95 || !completed {
					t.Errorf("session args not forwarded: exercise=%s breaths=%d duration=%d completed=%v",
						exerciseID, breaths, durationS, completed)
				}
				return session(), nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":5,"duration_s":95,"completed":true}`,
			testUserID, NewHandler(svc).RecordSession)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"session":{`)) {
			t.Errorf("response missing session object: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"user_id"`)) {
			t.Errorf("user_id must never appear in the session response: %s", body)
		}
		if !bytes.Contains([]byte(body), []byte(`"technique":"478"`)) {
			t.Errorf("response missing technique: %s", body)
		}
	})

	t.Run("defaults completed to true when omitted", func(t *testing.T) {
		svc := &fakeService{
			recordSessionFunc: func(_ context.Context, _ string, _ string, _, _ int, completed bool) (*Session, error) {
				if !completed {
					t.Errorf("expected completed to default to true, got false")
				}
				return session(), nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":5,"duration_s":95}`,
			testUserID, NewHandler(svc).RecordSession)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 404 for an unknown exercise", func(t *testing.T) {
		svc := &fakeService{
			recordSessionFunc: func(context.Context, string, string, int, int, bool) (*Session, error) {
				return nil, ErrExerciseNotFound
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"00000000-0000-0000-0000-000000000000","breaths":5,"duration_s":95}`,
			testUserID, NewHandler(svc).RecordSession)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 400 for a malformed exercise id", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"nope","breaths":5,"duration_s":95}`,
			testUserID, NewHandler(&fakeService{}).RecordSession)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "exercise_id")
	})

	t.Run("returns 400 for non-positive breaths", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":0,"duration_s":95}`,
			testUserID, NewHandler(&fakeService{}).RecordSession)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "breaths")
	})

	t.Run("returns 400 for non-positive duration_s", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":5,"duration_s":-1}`,
			testUserID, NewHandler(&fakeService{}).RecordSession)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "duration_s")
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions", `{"exercise_id":`,
			testUserID, NewHandler(&fakeService{}).RecordSession)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":5,"duration_s":95}`,
			"", NewHandler(&fakeService{}).RecordSession)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals on service failure", func(t *testing.T) {
		svc := &fakeService{
			recordSessionFunc: func(context.Context, string, string, int, int, bool) (*Session, error) {
				return nil, errors.New("database connection lost")
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions",
			`{"exercise_id":"`+testExerciseID+`","breaths":5,"duration_s":95}`,
			testUserID, NewHandler(svc).RecordSession)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("database connection lost")) {
			t.Errorf("internal error detail leaked to the client: %s", rec.Body.String())
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func TestHandlerListSessions(t *testing.T) {
	t.Run("returns 200 with the user's history", func(t *testing.T) {
		svc := &fakeService{
			listSessionsFunc: func(_ context.Context, userID string, limit int) ([]Session, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if limit != 10 {
					t.Errorf("expected limit 10 to reach the service, got %d", limit)
				}
				return []Session{*session()}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions?limit=10", "", testUserID, NewHandler(svc).ListSessions)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"sessions":[`)) {
			t.Errorf("response missing sessions array: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"user_id"`)) {
			t.Errorf("user_id must never appear in the history response: %s", body)
		}
	})

	t.Run("returns 200 with an empty list when the user has no sessions", func(t *testing.T) {
		svc := &fakeService{
			listSessionsFunc: func(context.Context, string, int) ([]Session, error) {
				return []Session{}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions", "", testUserID, NewHandler(svc).ListSessions)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"sessions":[]`)) {
			t.Errorf("expected an empty sessions array: %s", rec.Body.String())
		}
	})

	t.Run("returns 400 for a non-integer limit", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions?limit=abc", "", testUserID, NewHandler(&fakeService{}).ListSessions)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "limit")
	})

	t.Run("returns 401 when the user id is missing from context", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/breathing/sessions",
			"/api/v1/breathing/sessions", "", "", NewHandler(&fakeService{}).ListSessions)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})
}
