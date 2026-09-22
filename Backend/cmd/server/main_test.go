package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Backend/internal/anon"
	"Backend/internal/auth"
	"Backend/internal/config"
	"Backend/internal/dashboard"
	"Backend/internal/journal"
	"Backend/internal/middleware"
	"Backend/internal/mood"
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
	router := setupRouter(cfg, nil, nil, nil, nil, nil, nil, nil, nil)

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
	}, nil, user.NewHandler(nil), tokenManager, nil, nil, nil, nil, nil)

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
	}, nil, user.NewHandler(nil), tokenManager, anonH, anonSvc, nil, nil, nil)

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
		}, nil, user.NewHandler(nil), tokenManager, nil, nil, nil, nil, nil)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous/promote", nil)
		w := httptest.NewRecorder()
		sparseRouter.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 when the anonymous stack is absent, got %d", w.Code)
		}
	})
}

// stubUserService implements user.Service by embedding the interface and
// overriding only the Phase 4 profile methods, so registered-JWT requests can
// be exercised end to end through the router without a database.
type stubUserService struct {
	user.Service
	profile *user.User
	err     error
}

func (s *stubUserService) GetProfile(context.Context, string) (*user.User, error) {
	return s.profile, s.err
}

func (s *stubUserService) UpdateProfile(context.Context, string, *string, *string) (*user.User, error) {
	return s.profile, s.err
}

// stubMoodService implements mood.Service by embedding the interface and
// overriding only the methods the Phase 4 router tests exercise.
type stubMoodService struct {
	mood.Service
	createLog *mood.MoodLog
	listLogs  []mood.MoodLog
	err       error
}

func (s *stubMoodService) Create(context.Context, string, string) (*mood.MoodLog, error) {
	return s.createLog, s.err
}

func (s *stubMoodService) List(context.Context, string, int) ([]mood.MoodLog, error) {
	return s.listLogs, s.err
}

// stubDashboardService implements dashboard.Service by embedding the interface
// and overriding Get with a canned snapshot.
type stubDashboardService struct {
	dashboard.Service
	getFunc func(ctx context.Context, userID string) (*dashboard.Dashboard, error)
}

func (s *stubDashboardService) Get(ctx context.Context, userID string) (*dashboard.Dashboard, error) {
	return s.getFunc(ctx, userID)
}

// stubJournalService implements journal.Service by embedding the interface and
// overriding every method with canned values, so the Phase 5 router wiring can
// be exercised end to end without a database.
type stubJournalService struct {
	journal.Service
	entry *journal.JournalEntry
	list  []journal.JournalEntry
	err   error
}

func (s *stubJournalService) Create(context.Context, string, string, []string, *string) (*journal.JournalEntry, error) {
	return s.entry, s.err
}

func (s *stubJournalService) List(context.Context, string, int, *time.Time) ([]journal.JournalEntry, error) {
	return s.list, s.err
}

func (s *stubJournalService) Get(context.Context, string, string) (*journal.JournalEntry, error) {
	return s.entry, s.err
}

func (s *stubJournalService) Update(context.Context, string, string, journal.Update) (*journal.JournalEntry, error) {
	return s.entry, s.err
}

func (s *stubJournalService) Delete(context.Context, string, string) error {
	return s.err
}

func (s *stubJournalService) Reflect(context.Context, string, string) (*journal.JournalEntry, error) {
	return s.entry, s.err
}

// TestPhase5JournalRoutes verifies the Phase 5 wiring: every journal route
// exists behind the registered-user AuthRequired middleware, rejects missing
// and anonymous tokens, accepts a valid registered JWT, and is absent when the
// journal stack is not wired.
func TestPhase5JournalRoutes(t *testing.T) {
	tokenManager, err := auth.NewManager("unit-test-secret-that-must-be-long-enough-for-signing")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	const registeredID = "11111111-1111-1111-1111-111111111111"
	const entryID = "22222222-2222-2222-2222-222222222222"
	prompt := "My day"
	jrnlEntry := &journal.JournalEntry{
		ID:           entryID,
		MoodTags:     []string{"Tired"},
		PromptUsed:   &prompt,
		AIReflection: nil,
		WordCount:    3,
		CreatedAt:    time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	journalSvc := &stubJournalService{entry: jrnlEntry, list: []journal.JournalEntry{*jrnlEntry}}

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, nil, user.NewHandler(nil), tokenManager, nil, nil, nil, nil, journal.NewHandler(journalSvc))

	journalCases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"create", http.MethodPost, "/api/v1/journal", `{"content":"Today was hard."}`, http.StatusCreated},
		{"list", http.MethodGet, "/api/v1/journal", "", http.StatusOK},
		{"get", http.MethodGet, "/api/v1/journal/" + entryID, "", http.StatusOK},
		{"update", http.MethodPatch, "/api/v1/journal/" + entryID, `{"content":"New words."}`, http.StatusOK},
		{"delete", http.MethodDelete, "/api/v1/journal/" + entryID, "", http.StatusNoContent},
		{"reflect", http.MethodPost, "/api/v1/journal/" + entryID + "/reflect", "", http.StatusOK},
	}

	jwt, err := tokenManager.SignAccessToken(registeredID)
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}
	const rawAnonToken = "raw-anonymous-token-phase-5"

	t.Run("routes reject missing tokens", func(t *testing.T) {
		for _, tc := range journalCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 without a token, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		}
	})

	t.Run("routes reject anonymous-format tokens", func(t *testing.T) {
		for _, tc := range journalCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+rawAnonToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 for an anonymous token, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		}
	})

	t.Run("registered JWT reaches every journal route", func(t *testing.T) {
		for _, tc := range journalCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Authorization", "Bearer "+jwt)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("%s: expected %d, got %d: %s", tc.name, tc.want, w.Code, w.Body.String())
			}
		}
	})

	t.Run("journal responses keep content and ownership off the wire", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/journal/"+entryID, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		for _, leak := range []string{"content_enc", "content_iv", "user_id"} {
			if bytes.Contains(w.Body.Bytes(), []byte(leak)) {
				t.Errorf("response must never include %s: %s", leak, body)
			}
		}
	})

	t.Run("routes are not registered without the journal stack", func(t *testing.T) {
		sparseRouter := setupRouter(&config.Config{
			Env:         "test",
			GinMode:     "test",
			FrontendURL: "http://localhost:5173",
		}, nil, nil, nil, nil, nil, nil, nil, nil)

		for _, tc := range journalCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			sparseRouter.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s: expected 404 without the journal stack, got %d", tc.name, w.Code)
			}
		}
	})
}

// TestPhase4WellnessRoutes verifies the Phase 4 wiring: the profile, mood, and
// dashboard routes exist behind the registered-user AuthRequired middleware,
// reject missing and anonymous-format tokens, accept a valid registered JWT,
// and are absent when the stacks are not wired.
func TestPhase4WellnessRoutes(t *testing.T) {
	tokenManager, err := auth.NewManager("unit-test-secret-that-must-be-long-enough-for-signing")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	const (
		registeredID = "11111111-1111-1111-1111-111111111111"
		rawAnonToken = "raw-anonymous-token-phase-4"
	)
	displayName := "Bree"
	profile := &user.User{
		ID:           registeredID,
		Email:        "bree@example.com",
		PasswordHash: "must-never-leak",
		DisplayName:  &displayName,
		LanguagePref: "sw",
		IsVerified:   true,
	}
	createdLog := &mood.MoodLog{
		ID:       "22222222-2222-2222-2222-222222222222",
		Mood:     "Better",
		LoggedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	userSvc := &stubUserService{profile: profile}
	moodSvc := &stubMoodService{
		createLog: createdLog,
		listLogs:  []mood.MoodLog{*createdLog},
	}
	dashSvc := &stubDashboardService{
		getFunc: func(_ context.Context, userID string) (*dashboard.Dashboard, error) {
			return &dashboard.Dashboard{
				User:              profile,
				LatestMood:        createdLog,
				RecentMoods:       []mood.MoodLog{*createdLog},
				MoodCheckinsCount: 1,
			}, nil
		},
	}

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, nil,
		user.NewHandler(userSvc), tokenManager, nil, nil,
		mood.NewHandler(moodSvc), dashboard.NewHandler(dashSvc), nil)

	jwt, err := tokenManager.SignAccessToken(registeredID)
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}

	registerCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"profile read", http.MethodGet, "/api/v1/users/me", ""},
		{"profile update", http.MethodPatch, "/api/v1/users/me", `{"display_name":"Bree"}`},
		{"mood create", http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`},
		{"mood list", http.MethodGet, "/api/v1/moods", ""},
		{"dashboard", http.MethodGet, "/api/v1/dashboard", ""},
	}

	t.Run("routes reject missing tokens", func(t *testing.T) {
		for _, tc := range registerCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 without a token, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("routes reject anonymous-format tokens", func(t *testing.T) {
		for _, tc := range registerCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+rawAnonToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 for an anonymous token, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("registered JWT reaches the profile read route", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if bytes.Contains(w.Body.Bytes(), []byte("password_hash")) || bytes.Contains(w.Body.Bytes(), []byte("must-never-leak")) {
			t.Errorf("password hash leaked in the profile response: %s", w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"email":"bree@example.com"`)) {
			t.Errorf("unexpected profile body: %s", w.Body.String())
		}
	})

	t.Run("registered JWT can update the profile", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"display_name":"Bree"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"display_name":"Bree"`)) {
			t.Errorf("expected the updated display name in the response: %s", w.Body.String())
		}
	})

	t.Run("registered JWT creates a mood check-in", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/moods", strings.NewReader(`{"mood":"Better"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"mood":"Better"`)) {
			t.Errorf("unexpected check-in body: %s", w.Body.String())
		}
	})

	t.Run("registered JWT lists mood check-ins", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/moods", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !bytes.Contains(w.Body.Bytes(), []byte(`"moods":[`)) {
			t.Errorf("expected a moods array in the response: %s", w.Body.String())
		}
	})

	t.Run("registered JWT reaches the dashboard", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		for _, key := range []string{`"user"`, `"latest_mood"`, `"recent_moods"`, `"mood_checkins_count":1`} {
			if !bytes.Contains(w.Body.Bytes(), []byte(key)) {
				t.Errorf("dashboard response missing %s: %s", key, w.Body.String())
			}
		}
	})

	t.Run("routes are not registered without handlers or a token manager", func(t *testing.T) {
		sparseRouter := setupRouter(&config.Config{
			Env:         "test",
			GinMode:     "test",
			FrontendURL: "http://localhost:5173",
		}, nil, nil, nil, nil, nil, nil, nil, nil)

		for _, tc := range registerCases {
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			sparseRouter.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: expected 404 without handlers, got %d", tc.method, tc.path, w.Code)
			}
		}
	})

	t.Run("mood and dashboard routes are guarded independently", func(t *testing.T) {
		partial := setupRouter(&config.Config{
			Env:         "test",
			GinMode:     "test",
			FrontendURL: "http://localhost:5173",
		}, nil, user.NewHandler(userSvc), tokenManager, nil, nil, nil, nil, nil)

		for _, tc := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/moods"},
			{http.MethodGet, "/api/v1/dashboard"},
		} {
			req, _ := http.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			partial.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: expected 404 without mood/dashboard handlers, got %d", tc.method, tc.path, w.Code)
			}
		}

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		w := httptest.NewRecorder()
		partial.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("profile route should remain registered and protected (401 without token), got %d", w.Code)
		}
	})
}
