//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"Backend/internal/auth"
	"Backend/internal/config"
	"Backend/internal/dashboard"
	"Backend/internal/mood"
	"Backend/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPhase4EndToEndIntegration runs the full Phase 4 stack (profile, mood
// check-ins, dashboard) against a live database through the real router. It is
// excluded from the default build via the "integration" tag and skipped when
// DATABASE_URL is not set.
func TestPhase4EndToEndIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	tokenManager, err := auth.NewManager("integration-test-secret-not-for-production")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}

	userRepo := user.NewPostgresRepository(pool)
	userService := user.NewService(userRepo, tokenManager)
	userHandler := user.NewHandler(userService)

	moodRepo := mood.NewPostgresRepository(pool)
	moodService := mood.NewService(moodRepo)
	moodHandler := mood.NewHandler(moodService)

	dashboardHandler := dashboard.NewHandler(dashboard.NewService(userService, moodService))

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, pool, userHandler, tokenManager, nil, nil, moodHandler, dashboardHandler)

	createdEmails := []string{}
	defer func() {
		for _, e := range createdEmails {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", e); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", e, err)
			}
		}
	}()

	registerAndLogin := func(t *testing.T, prefix string) (token, email string) {
		t.Helper()
		email = fmt.Sprintf("%s-%d@soulwe.local", prefix, time.Now().UnixNano())
		createdEmails = append(createdEmails, email)
		const password = "integration-secret-password"
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("register: expected 201, got %d: %s", w.Code, w.Body.String())
		}

		req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var loginRes struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &loginRes); err != nil {
			t.Fatalf("failed to decode login response: %v", err)
		}
		if loginRes.AccessToken == "" {
			t.Fatal("expected a non-empty access token")
		}
		return loginRes.AccessToken, email
	}

	tokenA, emailA := registerAndLogin(t, "phase4-a")
	tokenB, _ := registerAndLogin(t, "phase4-b")

	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, path, reader)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	type route struct {
		method string
		path   string
		body   string
	}
	protectedRoutes := []route{
		{http.MethodGet, "/api/v1/users/me", ""},
		{http.MethodPatch, "/api/v1/users/me", `{"display_name":"Bree"}`},
		{http.MethodPost, "/api/v1/moods", `{"mood":"Better"}`},
		{http.MethodGet, "/api/v1/moods", ""},
		{http.MethodGet, "/api/v1/dashboard", ""},
	}

	t.Run("missing tokens are rejected on every Phase 4 route", func(t *testing.T) {
		for _, r := range protectedRoutes {
			if w := do(r.method, r.path, "", r.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d: %s", r.method, r.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("anonymous-format tokens are rejected on every Phase 4 route", func(t *testing.T) {
		for _, r := range protectedRoutes {
			if w := do(r.method, r.path, "raw-anonymous-format-token", r.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d: %s", r.method, r.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("a registered user reads their own profile", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/users/me", tokenA, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"email":`+fmt.Sprintf("%q", emailA)) {
			t.Errorf("expected the caller's email in the profile: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), `"password"`) {
			t.Errorf("password or its hash leaked in the profile response: %s", w.Body.String())
		}
	})

	t.Run("a registered user updates their profile", func(t *testing.T) {
		w := do(http.MethodPatch, "/api/v1/users/me", tokenA, `{"display_name":"Bree","language_pref":"sw"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"display_name":"Bree"`) {
			t.Errorf("expected the new display name in the response: %s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"language_pref":"sw"`) {
			t.Errorf("expected the new language preference in the response: %s", w.Body.String())
		}

		reload := do(http.MethodGet, "/api/v1/users/me", tokenA, "")
		if !strings.Contains(reload.Body.String(), `"display_name":"Bree"`) {
			t.Errorf("expected the display name to persist: %s", reload.Body.String())
		}
	})

	t.Run("forbidden account fields in a profile patch are ignored", func(t *testing.T) {
		body := fmt.Sprintf(`{"email":"hijack@soulwe.local","password":"new-secret-password","is_verified":true,"id":"00000000-0000-0000-0000-000000000000"}`)
		w := do(http.MethodPatch, "/api/v1/users/me", tokenA, body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		read := do(http.MethodGet, "/api/v1/users/me", tokenA, "")
		if !strings.Contains(read.Body.String(), `"email":`+fmt.Sprintf("%q", emailA)) {
			t.Errorf("email must not be changeable via the profile patch: %s", read.Body.String())
		}
		if !strings.Contains(read.Body.String(), `"is_verified":false`) {
			t.Errorf("is_verified must not be changeable via the profile patch: %s", read.Body.String())
		}
	})

	t.Run("a registered user creates and lists their mood check-ins", func(t *testing.T) {
		created := do(http.MethodPost, "/api/v1/moods", tokenA, `{"mood":"Better"}`)
		if created.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", created.Code, created.Body.String())
		}
		if !strings.Contains(created.Body.String(), `"mood":"Better"`) {
			t.Errorf("expected the created mood in the response: %s", created.Body.String())
		}

		list := do(http.MethodGet, "/api/v1/moods", tokenA, "")
		if list.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
		}
		var listRes struct {
			Moods []struct {
				Mood string `json:"mood"`
			} `json:"moods"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &listRes); err != nil {
			t.Fatalf("failed to decode moods list: %v", err)
		}
		if len(listRes.Moods) != 1 || listRes.Moods[0].Mood != "Better" {
			t.Errorf("expected exactly the owner's single check-in, got %+v", listRes.Moods)
		}
	})

	t.Run("invalid mood values are rejected", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/moods", tokenA, `{"mood":"Whatever"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"mood"`) {
			t.Errorf("expected the mood field on the validation error: %s", w.Body.String())
		}
	})

	t.Run("dashboard reflects only the owner's data", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/dashboard", tokenA, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"latest_mood`) {
			t.Errorf("expected a latest mood for a user with check-ins: %s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"mood_checkins_count":1`) {
			t.Errorf("expected the owner's check-in count to be 1: %s", w.Body.String())
		}
	})

	t.Run("another user sees none of the first user's data", func(t *testing.T) {
		list := do(http.MethodGet, "/api/v1/moods", tokenB, "")
		if list.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
		}
		if !strings.Contains(list.Body.String(), `"moods":[]`) {
			t.Errorf("user B must have an empty mood list: %s", list.Body.String())
		}

		dash := do(http.MethodGet, "/api/v1/dashboard", tokenB, "")
		if !strings.Contains(dash.Body.String(), `"latest_mood":null`) || !strings.Contains(dash.Body.String(), `"mood_checkins_count":0`) {
			t.Errorf("user B's dashboard must show no moods: %s", dash.Body.String())
		}

		me := do(http.MethodGet, "/api/v1/users/me", tokenB, "")
		if strings.Contains(me.Body.String(), `"email":`+fmt.Sprintf("%q", emailA)) {
			t.Errorf("user B must not see user A's identity: %s", me.Body.String())
		}
	})
}
