//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"Backend/internal/anon"
	"Backend/internal/auth"
	"Backend/internal/bookings"
	"Backend/internal/cipher"
	"Backend/internal/circles"
	"Backend/internal/config"
	"Backend/internal/dashboard"
	"Backend/internal/journal"
	"Backend/internal/mood"
	"Backend/internal/therapists"
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
	}, pool, userHandler, tokenManager, nil, nil, moodHandler, dashboardHandler, nil, nil, nil, nil)

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

// TestPhase5JournalEndToEndIntegration runs the full Phase 5 journal stack
// (encrypt-at-rest, create/list/get/update/delete, ownership isolation) against
// a live database through the real router. The AI reflection generator is wired
// as nil, so the reflect endpoint is expected to return 503 — no real
// Anthropic key is required anywhere in the integration suite.
func TestPhase5JournalEndToEndIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	const testJournalKey = "0123456789abcdef0123456789abcdef"
	codec, err := cipher.NewAESGCM([]byte(testJournalKey))
	if err != nil {
		t.Fatalf("journal codec: %v", err)
	}
	journalHandler := journal.NewHandler(journal.NewService(
		journal.NewPostgresRepository(pool), codec, nil, /* no reflection generator wired */
	))

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, pool, userHandler, tokenManager, nil, nil, nil, nil, journalHandler, nil, nil, nil)

	createdEmails := []string{}
	defer func() {
		for _, e := range createdEmails {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", e); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", e, err)
			}
		}
	}()

	registerAndLogin := func(t *testing.T, prefix string) string {
		t.Helper()
		email := fmt.Sprintf("%s-%d@soulwe.local", prefix, time.Now().UnixNano())
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
		return loginRes.AccessToken
	}

	tokenA := registerAndLogin(t, "phase5-a")
	tokenB := registerAndLogin(t, "phase5-b")

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

	const jContent = "Today the mango tree behind the house finally bore fruit and I cried a little."

	t.Run("missing and anonymous tokens are rejected on every journal route", func(t *testing.T) {
		routes := []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodPost, "/api/v1/journal", `{"content":"x"}`},
			{http.MethodGet, "/api/v1/journal", ""},
			{http.MethodGet, "/api/v1/journal/00000000-0000-0000-0000-000000000000", ""},
			{http.MethodPatch, "/api/v1/journal/00000000-0000-0000-0000-000000000000", `{"content":"x"}`},
			{http.MethodDelete, "/api/v1/journal/00000000-0000-0000-0000-000000000000", ""},
			{http.MethodPost, "/api/v1/journal/00000000-0000-0000-0000-000000000000/reflect", ""},
		}
		for _, r := range routes {
			if w := do(r.method, r.path, "", r.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 without a token, got %d: %s", r.method, r.path, w.Code, w.Body.String())
			}
			if w := do(r.method, r.path, "raw-anonymous-format-token", r.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 for an anonymous token, got %d: %s", r.method, r.path, w.Code, w.Body.String())
			}
		}
	})

	var entryID string

	t.Run("a registered user creates an entry and nothing leaks to the wire", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/journal", tokenA, fmt.Sprintf(`{"content":%q,"mood_tags":["Overwhelmed","Loved"],"prompt_used":"Family & pressure"}`, jContent))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		for _, leak := range []string{"content", "content_enc", "content_iv", "user_id"} {
			if strings.Contains(body, `"`+leak) {
				t.Errorf("create response must not include %s: %s", leak, body)
			}
		}
		var res struct {
			Entry journal.JournalEntry `json:"entry"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode create response: %v", err)
		}
		entryID = res.Entry.ID
		if entryID == "" {
			t.Fatal("expected a created entry id")
		}
		if res.Entry.WordCount != 14 {
			t.Errorf("expected word count 14, got %d", res.Entry.WordCount)
		}
		if res.Entry.AIReflection != nil {
			t.Error("expected a null reflection when no generator is configured")
		}
	})

	t.Run("the database stores ciphertext, never the plaintext", func(t *testing.T) {
		var encrypted bool
		if err := pool.QueryRow(ctx,
			`SELECT content_enc <> convert_to($1, 'UTF8') FROM journal_entries WHERE id = $2`, jContent, entryID).Scan(&encrypted); err != nil {
			t.Fatalf("raw SELECT failed: %v", err)
		}
		if !encrypted {
			t.Error("journal content must be encrypted at rest")
		}
		var iv []byte
		if err := pool.QueryRow(ctx, "SELECT content_iv FROM journal_entries WHERE id = $1", entryID).Scan(&iv); err != nil {
			t.Fatalf("SELECT content_iv failed: %v", err)
		}
		if len(iv) == 0 {
			t.Error("expected a stored per-entry IV")
		}
	})

	t.Run("a registered user reads and lists their own entries", func(t *testing.T) {
		detail := do(http.MethodGet, "/api/v1/journal/"+entryID, tokenA, "")
		if detail.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", detail.Code, detail.Body.String())
		}
		if !strings.Contains(detail.Body.String(), jContent) {
			t.Errorf("expected the decrypted content in the detail response: %s", detail.Body.String())
		}
		if strings.Contains(detail.Body.String(), "content_enc") {
			t.Errorf("ciphertext must never be returned: %s", detail.Body.String())
		}

		list := do(http.MethodGet, "/api/v1/journal", tokenA, "")
		if list.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
		}
		if strings.Contains(list.Body.String(), jContent) {
			t.Errorf("list must not include content: %s", list.Body.String())
		}
		if !strings.Contains(list.Body.String(), entryID) {
			t.Errorf("expected the created entry in the list: %s", list.Body.String())
		}
	})

	t.Run("a registered user updates their own entry", func(t *testing.T) {
		const newText = "The river rose overnight."
		w := do(http.MethodPatch, "/api/v1/journal/"+entryID, tokenA, fmt.Sprintf(`{"content":%q,"mood_tags":["Calm"]}`, newText))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), newText) {
			t.Errorf("expected the updated content: %s", w.Body.String())
		}

		detail := do(http.MethodGet, "/api/v1/journal/"+entryID, tokenA, "")
		if !strings.Contains(detail.Body.String(), newText) {
			t.Errorf("expected the updated content to persist: %s", detail.Body.String())
		}
	})

	t.Run("reflect returns a safe error when no AI generator is configured", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/journal/"+entryID+"/reflect", tokenA, "")
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"AI_REFLECTION_UNAVAILABLE"`) {
			t.Errorf("expected the safe AI-unavailable error: %s", w.Body.String())
		}
	})

	t.Run("another user cannot touch the first user's entry", func(t *testing.T) {
		checks := []struct {
			name string
			w    *httptest.ResponseRecorder
		}{
			{"get", do(http.MethodGet, "/api/v1/journal/"+entryID, tokenB, "")},
			{"update", do(http.MethodPatch, "/api/v1/journal/"+entryID, tokenB, `{"content":"hijack"}`)},
			{"delete", do(http.MethodDelete, "/api/v1/journal/"+entryID, tokenB, "")},
			{"reflect", do(http.MethodPost, "/api/v1/journal/"+entryID+"/reflect", tokenB, "")},
		}
		for _, c := range checks {
			if c.w.Code != http.StatusNotFound {
				t.Errorf("%s: expected 404 for another user, got %d: %s", c.name, c.w.Code, c.w.Body.String())
			}
		}

		list := do(http.MethodGet, "/api/v1/journal", tokenB, "")
		if list.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", list.Code)
		}
		if !strings.Contains(list.Body.String(), `"entries":[]`) {
			t.Errorf("user B's journal must be empty: %s", list.Body.String())
		}

		// The owner's entry is untouched.
		detail := do(http.MethodGet, "/api/v1/journal/"+entryID, tokenA, "")
		if detail.Code != http.StatusOK {
			t.Errorf("owner's entry must survive B's tampering, got %d", detail.Code)
		}
	})

	t.Run("invalid input is rejected", func(t *testing.T) {
		empty := do(http.MethodPost, "/api/v1/journal", tokenA, `{"content":""}`)
		if empty.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty content, got %d: %s", empty.Code, empty.Body.String())
		}
		if !strings.Contains(empty.Body.String(), `"field":"content"`) {
			t.Errorf("expected a content field on the validation error: %s", empty.Body.String())
		}

		malformed := do(http.MethodPost, "/api/v1/journal", tokenA, `{"content":`)
		if malformed.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for malformed JSON, got %d: %s", malformed.Code, malformed.Body.String())
		}
	})

	t.Run("a registered user deletes their own entry", func(t *testing.T) {
		del := do(http.MethodDelete, "/api/v1/journal/"+entryID, tokenA, "")
		if del.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", del.Code, del.Body.String())
		}
		after := do(http.MethodGet, "/api/v1/journal/"+entryID, tokenA, "")
		if after.Code != http.StatusNotFound {
			t.Errorf("expected 404 for the deleted entry, got %d: %s", after.Code, after.Body.String())
		}
	})

	t.Run("recorded content count and deletes leave the database consistent", func(t *testing.T) {
		var count int
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM journal_entries WHERE id = $1", entryID).Scan(&count); err != nil {
			t.Fatalf("raw COUNT failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected the deleted entry to be gone, found %d rows", count)
		}
	})
}

// TestPhase6CirclesEndToEndIntegration runs the full Phase 6.1 circles stack
// (discovery, detail, join/leave, membership-gated messaging) against a live
// database through the real router, using anonymous sessions exactly as the
// product intends. Registered-user JWT flows are untouched.
func TestPhase6CirclesEndToEndIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	anonService := anon.NewService(anon.NewPostgresRepository(pool))
	anonHandler := anon.NewHandler(anonService)
	circlesHandler := circles.NewHandler(circles.NewService(circles.NewPostgresRepository(pool)))

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, pool, nil, nil, anonHandler, anonService, nil, nil, nil, circlesHandler, nil, nil)

	// Created anonymous identities, cleaned up at the end. Deleting an
	// anon_identities row cascades its circle_members and circle_messages.
	createdIdentities := []string{}
	defer func() {
		for _, id := range createdIdentities {
			if _, err := pool.Exec(context.Background(), "DELETE FROM anon_identities WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE test identity %s: %v", id, err)
			}
		}
	}()

	createSession := func(t *testing.T, label string) (token, identityID string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: expected 201 creating an anonymous session, got %d: %s", label, w.Code, w.Body.String())
		}
		var res struct {
			Token string `json:"anonymous_token"`
			ID    string `json:"anonymous_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("%s: failed to decode session: %v", label, err)
		}
		if res.Token == "" || res.ID == "" {
			t.Fatalf("%s: expected a token and identity id, got %+v", label, res)
		}
		createdIdentities = append(createdIdentities, res.ID)
		return res.Token, res.ID
	}

	tokenA, identityA := createSession(t, "circle-a")
	tokenB, _ := createSession(t, "circle-b")

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

	var circleID string

	t.Run("missing and registered-JWT tokens are rejected on every circle route", func(t *testing.T) {
		routes := []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodGet, "/api/v1/circles", ""},
			{http.MethodGet, "/api/v1/circles/00000000-0000-0000-0000-000000000000", ""},
			{http.MethodPost, "/api/v1/circles/00000000-0000-0000-0000-000000000000/join", ""},
			{http.MethodDelete, "/api/v1/circles/00000000-0000-0000-0000-000000000000/leave", ""},
			{http.MethodGet, "/api/v1/circles/00000000-0000-0000-0000-000000000000/messages", ""},
			{http.MethodPost, "/api/v1/circles/00000000-0000-0000-0000-000000000000/messages", `{"content":"x"}`},
		}
		for _, r := range routes {
			if w := do(r.method, r.path, "", r.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 without a token, got %d: %s", r.method, r.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("a public-ish discovery lists the seeded circles", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/circles", tokenA, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if strings.Contains(body, "anon_identity_id") {
			t.Errorf("discovery must never leak anon_identity_id: %s", body)
		}
		var res struct {
			Circles []struct {
				ID    string `json:"id"`
				Slug  string `json:"slug"`
				Count int    `json:"member_count"`
			} `json:"circles"`
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			t.Fatalf("failed to decode discovery: %v", err)
		}
		foundGrief := false
		for _, c := range res.Circles {
			if c.Count < 0 {
				t.Errorf("member_count must never be negative for %s", c.Slug)
			}
			if c.Slug == "grief" {
				foundGrief = true
				circleID = c.ID
			}
		}
		if !foundGrief {
			t.Errorf("expected the seeded grief circle in discovery: %s", body)
		}
		if circleID == "" {
			t.Fatal("expected the seeded grief circle to be discoverable")
		}
	})

	t.Run("detail reports membership before joining", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/circles/"+circleID, tokenA, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"is_member":false`) {
			t.Errorf("expected is_member false before joining: %s", w.Body.String())
		}
	})

	t.Run("join succeeds and duplicate join conflicts", func(t *testing.T) {
		if w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/join", tokenA, ""); w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
		}
		if w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/join", tokenA, ""); w.Code != http.StatusConflict {
			t.Fatalf("expected 409 on a duplicate join, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("a member sends a message and sees it newest first", func(t *testing.T) {
		const text = "I lost my father last month and this place feels safe."
		w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/messages", tokenA,
			fmt.Sprintf(`{"content":%q}`, text))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		created := w.Body.String()
		if !strings.Contains(created, text) {
			t.Errorf("expected the sent content echoed: %s", created)
		}
		if !strings.Contains(created, `"anon_name":`) {
			t.Errorf("expected an anon_name on the created message: %s", created)
		}
		for _, leak := range []string{"anon_identity_id", identityA, "token_hash"} {
			if strings.Contains(created, leak) {
				t.Errorf("created message must never include %s: %s", leak, created)
			}
		}

		list := do(http.MethodGet, "/api/v1/circles/"+circleID+"/messages", tokenA, "")
		if list.Code != http.StatusOK {
			t.Fatalf("expected 200 listing, got %d: %s", list.Code, list.Body.String())
		}
		if !strings.Contains(list.Body.String(), text) {
			t.Errorf("expected the sent message in the list: %s", list.Body.String())
		}
	})

	t.Run("non-members cannot read or write messages", func(t *testing.T) {
		if w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/messages", tokenB, `{"content":"intruder"}`); w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 sending as a non-member, got %d: %s", w.Code, w.Body.String())
		}
		if w := do(http.MethodGet, "/api/v1/circles/"+circleID+"/messages", tokenB, ""); w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 reading as a non-member, got %d: %s", w.Code, w.Body.String())
		} else if !strings.Contains(w.Body.String(), `"code":"NOT_A_MEMBER"`) {
			t.Errorf("expected the NOT_A_MEMBER code: %s", w.Body.String())
		}
	})

	t.Run("another member can read the same conversation", func(t *testing.T) {
		if w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/join", tokenB, ""); w.Code != http.StatusNoContent {
			t.Fatalf("expected 204 joining as B, got %d: %s", w.Code, w.Body.String())
		}
		w := do(http.MethodGet, "/api/v1/circles/"+circleID+"/messages", tokenB, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "I lost my father") {
			t.Errorf("member B should see member A's message: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), identityA) {
			t.Errorf("messages must never expose the author's identity UUID: %s", w.Body.String())
		}
	})

	t.Run("detail and counts reflect memberships after both joined", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/circles/"+circleID, tokenA, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"is_member":true`) || !strings.Contains(w.Body.String(), `"member_count":2`) {
			t.Errorf("expected is_member true and member_count 2, got: %s", w.Body.String())
		}
	})

	t.Run("leaving revokes messaging access", func(t *testing.T) {
		if w := do(http.MethodDelete, "/api/v1/circles/"+circleID+"/leave", tokenB, ""); w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
		}
		if w := do(http.MethodDelete, "/api/v1/circles/"+circleID+"/leave", tokenB, ""); w.Code != http.StatusNoContent {
			t.Fatalf("expected 204 on an idempotent leave, got %d: %s", w.Code, w.Body.String())
		}
		if w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/messages", tokenB, `{"content":"after leave"}`); w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 after leaving, got %d: %s", w.Code, w.Body.String())
		}
		w := do(http.MethodGet, "/api/v1/circles/"+circleID, tokenB, "")
		if !strings.Contains(w.Body.String(), `"is_member":false`) {
			t.Errorf("expected is_member false after leaving: %s", w.Body.String())
		}
	})

	t.Run("unknown circles return 404 on every route", func(t *testing.T) {
		const ghost = "/api/v1/circles/00000000-0000-0000-0000-000000000000"
		if w := do(http.MethodGet, ghost, tokenA, ""); w.Code != http.StatusNotFound {
			t.Errorf("detail: expected 404, got %d", w.Code)
		}
		if w := do(http.MethodPost, ghost+"/join", tokenA, ""); w.Code != http.StatusNotFound {
			t.Errorf("join: expected 404, got %d", w.Code)
		}
		if w := do(http.MethodDelete, ghost+"/leave", tokenA, ""); w.Code != http.StatusNotFound {
			t.Errorf("leave: expected 404, got %d", w.Code)
		}
		if w := do(http.MethodGet, ghost+"/messages", tokenA, ""); w.Code != http.StatusNotFound {
			t.Errorf("list: expected 404, got %d", w.Code)
		}
		if w := do(http.MethodPost, ghost+"/messages", tokenA, `{"content":"x"}`); w.Code != http.StatusNotFound {
			t.Errorf("send: expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid message content is rejected with the content field", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/circles/"+circleID+"/messages", tokenA, `{"content":"   "}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"content"`) {
			t.Errorf("expected the content field on the validation error: %s", w.Body.String())
		}
	})

	t.Run("the database stores only anonymous authorship", func(t *testing.T) {
		var anonCount int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM circle_messages m
			 JOIN anon_identities a ON a.id = m.anon_identity_id
			 WHERE m.circle_id = $1 AND a.token_hash IS NOT NULL`, circleID).Scan(&anonCount); err != nil {
			t.Fatalf("raw COUNT failed: %v", err)
		}
		if anonCount < 1 {
			t.Errorf("expected the circle's messages to be authored by anonymous identities, got %d", anonCount)
		}
	})
}

// TestPhase6TherapistDiscoveryEndToEndIntegration runs the full Phase 6.2
// therapist discovery stack (directory, filters, pagination, profile) against
// a live database through the real router, using a registered-user JWT exactly
// as the product intends. Anonymous tokens must be rejected on every route.
func TestPhase6TherapistDiscoveryEndToEndIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	anonService := anon.NewService(anon.NewPostgresRepository(pool))
	anonHandler := anon.NewHandler(anonService)

	therapistsHandler := therapists.NewHandler(therapists.NewService(therapists.NewPostgresRepository(pool)))

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, pool, userHandler, tokenManager, anonHandler, anonService, nil, nil, nil, nil, therapistsHandler, nil)

	createdEmails := []string{}
	createdTherapistNames := []string{}
	defer func() {
		for _, e := range createdEmails {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", e); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", e, err)
			}
		}
		for _, name := range createdTherapistNames {
			if _, err := pool.Exec(context.Background(), "DELETE FROM therapists WHERE full_name = $1", name); err != nil {
				t.Errorf("cleanup failed to DELETE therapist %s: %v", name, err)
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

		loginBody := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("login: failed to decode response: %v", err)
		}
		return res.AccessToken, email
	}

	anonToken := func(t *testing.T) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/anonymous", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("anonymous: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Token string `json:"anonymous_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("anonymous: failed to decode response: %v", err)
		}
		return res.Token
	}

	// Seed three therapists with distinct created_at values so newest-first
	// ordering and the before-cursor pagination are deterministic. Languages
	// flow through the child table exactly as the product stores them.
	now := time.Now().UTC().Truncate(time.Second)
	seed := func(name string, langs, specs []string, active bool, createdAt time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO therapists (full_name, credentials, years_exp, bio, location,
				is_online_only, price_kes, free_sessions, specialties, is_active, created_at)
			VALUES ($1, 'MA, PhD', 8, $2, 'Nairobi', $3, $4, 1, $5, $6, $7)
			RETURNING id`,
			name, "Clinical psychologist.", false, 800, specs, active, createdAt).Scan(&id); err != nil {
			t.Fatalf("failed to seed therapist %s: %v", name, err)
		}
		for _, lang := range langs {
			if _, err := pool.Exec(ctx,
				`INSERT INTO therapist_languages (therapist_id, language, proficiency) VALUES ($1, $2, 'fluent')`, id, lang); err != nil {
				t.Fatalf("failed to seed language for %s: %v", name, err)
			}
		}
		createdTherapistNames = append(createdTherapistNames, name)
		return id
	}

	aminahID := seed("Dr. Aminah Korir", []string{"English", "Swahili"}, []string{"Grief", "Trauma"}, true, now.Add(-3*time.Hour))
	seed("Dr. Bora Njoroge", []string{"English", "Kikuyu"}, []string{"Anxiety", "Grief"}, false, now.Add(-2*time.Hour))
	ciciID := seed("Dr. Cici Mwangi", []string{"Swahili"}, []string{"Trauma"}, true, now.Add(-1*time.Hour))

	do := func(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req, _ := http.NewRequest(method, path, strings.NewReader(body))
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

	registeredJWT, _ := registerAndLogin(t, "therapist-browser")
	rawAnon := anonToken(t)

	therapistRoutes := []string{"/api/v1/therapists", "/api/v1/therapists/" + ciciID}

	t.Run("both routes reject missing and anonymous tokens", func(t *testing.T) {
		for _, path := range therapistRoutes {
			if w := do(t, http.MethodGet, path, "", ""); w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 without a token, got %d: %s", path, w.Code, w.Body.String())
			}
			if w := do(t, http.MethodGet, path, rawAnon, ""); w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 for an anonymous token, got %d: %s", path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("registered user lists the directory newest-first with public fields", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/therapists", registeredJWT, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Therapists []struct {
				ID           string   `json:"id"`
				DisplayName  string   `json:"display_name"`
				Bio          *string  `json:"bio"`
				Languages    []string `json:"languages"`
				Specialties  []string `json:"specialties"`
				SessionPrice *int     `json:"session_price"`
				Currency     string   `json:"currency"`
				IsActive     bool     `json:"is_active"`
				IsOnlineOnly bool     `json:"is_online_only"`
			} `json:"therapists"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(res.Therapists) != 3 {
			t.Fatalf("expected 3 therapists, got %d", len(res.Therapists))
		}
		if res.Therapists[0].DisplayName != "Dr. Cici Mwangi" {
			t.Errorf("expected the newest therapist first, got %q", res.Therapists[0].DisplayName)
		}
		if res.Therapists[0].Currency != "KES" {
			t.Errorf("expected the fixed currency KES, got %q", res.Therapists[0].Currency)
		}
		if res.Therapists[0].SessionPrice == nil || *res.Therapists[0].SessionPrice != 800 {
			t.Errorf("expected session_price 800, got %v", res.Therapists[0].SessionPrice)
		}
		if len(res.Therapists[0].Languages) != 1 || res.Therapists[0].Languages[0] != "Swahili" {
			t.Errorf("expected Cici's Swahili language, got %v", res.Therapists[0].Languages)
		}
	})

	t.Run("language and specialty filters narrow the directory", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/therapists?language=swahili", registeredJWT, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Therapists []struct {
				DisplayName string `json:"display_name"`
			} `json:"therapists"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(res.Therapists) != 2 {
			t.Errorf("expected 2 Swahili-speaking therapists, got %d", len(res.Therapists))
		}

		w = do(t, http.MethodGet, "/api/v1/therapists?specialty=anxiety", registeredJWT, "")
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(res.Therapists) != 1 || res.Therapists[0].DisplayName != "Dr. Bora Njoroge" {
			t.Errorf("expected only Dr. Bora Njoroge for Anxiety, got %+v", res.Therapists)
		}

		w = do(t, http.MethodGet, "/api/v1/therapists?language=kirundi", registeredJWT, "")
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(res.Therapists) != 0 {
			t.Errorf("expected no Kirundi-speaking therapists, got %d", len(res.Therapists))
		}
	})

	t.Run("pagination honors limit and the before cursor", func(t *testing.T) {
		pageOne := do(t, http.MethodGet, "/api/v1/therapists?limit=1", registeredJWT, "")
		if pageOne.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", pageOne.Code, pageOne.Body.String())
		}
		var one struct {
			Therapists []struct {
				DisplayName string `json:"display_name"`
			} `json:"therapists"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(pageOne.Body.Bytes(), &one); err != nil {
			t.Fatalf("failed to decode page one: %v", err)
		}
		if len(one.Therapists) != 1 || one.Therapists[0].DisplayName != "Dr. Cici Mwangi" || one.NextCursor == nil {
			t.Fatalf("expected Cici with a next_cursor, got %+v (cursor %v)", one.Therapists, one.NextCursor)
		}

		pageTwo := do(t, http.MethodGet, "/api/v1/therapists?limit=1&before="+url.QueryEscape(*one.NextCursor), registeredJWT, "")
		if pageTwo.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", pageTwo.Code, pageTwo.Body.String())
		}
		var two struct {
			Therapists []struct {
				DisplayName string `json:"display_name"`
			} `json:"therapists"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(pageTwo.Body.Bytes(), &two); err != nil {
			t.Fatalf("failed to decode page two: %v", err)
		}
		if len(two.Therapists) != 1 || two.Therapists[0].DisplayName != "Dr. Bora Njoroge" || two.NextCursor == nil {
			t.Fatalf("expected Bora on page two with a next_cursor, got %+v (cursor %v)", two.Therapists, two.NextCursor)
		}

		pageThree := do(t, http.MethodGet, "/api/v1/therapists?limit=1&before="+url.QueryEscape(*two.NextCursor), registeredJWT, "")
		var three struct {
			Therapists []struct {
				DisplayName string `json:"display_name"`
			} `json:"therapists"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(pageThree.Body.Bytes(), &three); err != nil {
			t.Fatalf("failed to decode page three: %v", err)
		}
		if len(three.Therapists) != 1 || three.Therapists[0].DisplayName != "Dr. Aminah Korir" {
			t.Fatalf("expected Aminah on the last page, got %+v", three.Therapists)
		}
		// The page is full, so per the keyset convention it still carries a
		// cursor; consuming it must yield an empty page with no further cursor.
		if three.NextCursor == nil {
			t.Fatalf("expected a cursor on the full last page, got %+v", three.Therapists)
		}

		pageFour := do(t, http.MethodGet, "/api/v1/therapists?limit=1&before="+url.QueryEscape(*three.NextCursor), registeredJWT, "")
		var four struct {
			Therapists []struct {
				DisplayName string `json:"display_name"`
			} `json:"therapists"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(pageFour.Body.Bytes(), &four); err != nil {
			t.Fatalf("failed to decode page four: %v", err)
		}
		if len(four.Therapists) != 0 || four.NextCursor != nil {
			t.Fatalf("expected an empty terminal page with no cursor, got %+v (cursor %v)", four.Therapists, four.NextCursor)
		}
	})

	t.Run("profile returns the public therapist fields", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/therapists/"+aminahID, registeredJWT, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Therapist struct {
				ID           string   `json:"id"`
				DisplayName  string   `json:"display_name"`
				Bio          *string  `json:"bio"`
				Languages    []string `json:"languages"`
				Specialties  []string `json:"specialties"`
				SessionPrice *int     `json:"session_price"`
				Currency     string   `json:"currency"`
				IsActive     bool     `json:"is_active"`
				IsOnlineOnly bool     `json:"is_online_only"`
			} `json:"therapist"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode profile: %v", err)
		}
		th := res.Therapist
		if th.ID != aminahID || th.DisplayName != "Dr. Aminah Korir" {
			t.Errorf("expected the seeded profile, got %+v", th)
		}
		if !th.IsActive || th.IsOnlineOnly {
			t.Errorf("expected active and in-person flags, got active=%v online=%v", th.IsActive, th.IsOnlineOnly)
		}
		if len(th.Languages) != 2 || len(th.Specialties) != 2 {
			t.Errorf("expected both languages and specialties aggregated, got %v / %v", th.Languages, th.Specialties)
		}
	})

	t.Run("invalid and unknown therapist ids are rejected", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/therapists/not-a-uuid", registeredJWT, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for a malformed id, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"id"`) {
			t.Errorf("expected the id field on the validation error: %s", w.Body.String())
		}

		w = do(t, http.MethodGet, "/api/v1/therapists/00000000-0000-0000-0000-000000000000", registeredJWT, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for an unknown id, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no sensitive material ever appears in therapist responses", func(t *testing.T) {
		for _, path := range therapistRoutes {
			w := do(t, http.MethodGet, path, registeredJWT, "")
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			body := w.Body.String()
			for _, leak := range []string{"email", "password", "password_hash", "price_kes", "full_name", "credentials", "years_exp", "photo_url", "location", "free_sessions"} {
				if strings.Contains(body, leak) {
					t.Errorf("%s: response must never include %q: %s", path, leak, body)
				}
			}
		}
	})

	t.Run("registered auth flows reject the anonymous token while therapist routes reject it too", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/auth/me", rawAnon, "")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for an anonymous token on auth/me, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// TestPhase6BookingEndToEndIntegration runs the Phase 6.3 booking stack
// (create under a therapist, list/get/cancel the caller's own bookings) against
// a live database through the real router. It is excluded from the default
// build via the "integration" tag and skipped when DATABASE_URL is not set.
func TestPhase6BookingEndToEndIntegration(t *testing.T) {
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

	anonService := anon.NewService(anon.NewPostgresRepository(pool))
	anonHandler := anon.NewHandler(anonService)

	therapistsHandler := therapists.NewHandler(therapists.NewService(therapists.NewPostgresRepository(pool)))
	bookingsHandler := bookings.NewHandler(bookings.NewService(bookings.NewPostgresRepository(pool)))

	router := setupRouter(&config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}, pool, userHandler, tokenManager, anonHandler, anonService, nil, nil, nil, nil, therapistsHandler, bookingsHandler)

	createdEmails := []string{}
	createdTherapistNames := []string{}
	defer func() {
		for _, e := range createdEmails {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", e); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", e, err)
			}
		}
		for _, name := range createdTherapistNames {
			if _, err := pool.Exec(context.Background(), "DELETE FROM therapists WHERE full_name = $1", name); err != nil {
				t.Errorf("cleanup failed to DELETE therapist %s: %v", name, err)
			}
		}
	}()

	register := func(t *testing.T, prefix string) (token, email string) {
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

		loginBody := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("login: failed to decode response: %v", err)
		}
		return res.AccessToken, email
	}

	do := func(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req, _ := http.NewRequest(method, path, strings.NewReader(body))
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

	// Seed one active and one inactive therapist, each with a unique name so
	// the run is isolated from the Phase 6.2 e2e's therapists.
	seed := func(name string, active bool) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO therapists (full_name, credentials, years_exp, bio, location,
				is_online_only, price_kes, free_sessions, specialties, is_active, created_at)
			VALUES ($1, 'MA', 5, 'bio', 'Nairobi', FALSE, 800, 1, ARRAY['Grief'], $2, NOW())
			RETURNING id`, name, active).Scan(&id); err != nil {
			t.Fatalf("failed to seed therapist %s: %v", name, err)
		}
		createdTherapistNames = append(createdTherapistNames, name)
		return id
	}

	dayoID := seed("Dr. Dayo Achieng", true)
	inactiveID := seed("Dr. Faith Kamau", false)

	alice, _ := register(t, "booking-alice")
	bob, _ := register(t, "booking-bob")

	// Slots are strictly in the future (48-55h out) so the scheduled_at checks
	// and overlap window never depend on the clock being in a particular day.
	slot := func(hoursFromNow int) string {
		return time.Now().UTC().Add(time.Duration(48+hoursFromNow) * time.Hour).Truncate(time.Second).Format(time.RFC3339Nano)
	}
	slotA := slot(0) // alice's first booking
	slotB := slot(2) // 2h after slotA -> no overlap
	// 30 minutes after slotA, strictly inside the assumed 60-minute session
	// window, so the overlap check must reject it.
	slotOverlap := time.Now().UTC().Add(48*time.Hour + 30*time.Minute).Truncate(time.Second).Format(time.RFC3339Nano)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)

	bookingRoutes := []struct {
		name, method, path, body string
	}{
		{"create", http.MethodPost, "/api/v1/therapists/" + dayoID + "/bookings", `{"scheduled_at":"` + slotA + `"}`},
		{"list", http.MethodGet, "/api/v1/bookings", ""},
		{"get", http.MethodGet, "/api/v1/bookings/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", ""},
		{"cancel", http.MethodPatch, "/api/v1/bookings/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb/cancel", ""},
	}

	t.Run("booking routes reject missing and anonymous tokens", func(t *testing.T) {
		for _, tc := range bookingRoutes {
			if w := do(t, tc.method, tc.path, "", tc.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 without a token, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		}

		rawAnon := func() string {
			w := do(t, http.MethodPost, "/api/v1/auth/anonymous", "", "")
			if w.Code != http.StatusCreated {
				t.Fatalf("anonymous: expected 201, got %d: %s", w.Code, w.Body.String())
			}
			var res struct {
				Token string `json:"anonymous_token"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("anonymous: failed to decode response: %v", err)
			}
			return res.Token
		}()
		for _, tc := range bookingRoutes {
			if w := do(t, tc.method, tc.path, rawAnon, tc.body); w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 for an anonymous token, got %d: %s", tc.name, w.Code, w.Body.String())
			}
		}
	})

	var aliceBookingID, aliceSecondID string

	t.Run("creating a booking returns a pending booking with public fields", func(t *testing.T) {
		w := do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", alice, `{"scheduled_at":"`+slotA+`"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Booking struct {
				ID          string `json:"id"`
				TherapistID string `json:"therapist_id"`
				DisplayName string `json:"display_name"`
				ScheduledAt string `json:"scheduled_at"`
				Status      string `json:"status"`
				CreatedAt   string `json:"created_at"`
				UpdatedAt   string `json:"updated_at"`
			} `json:"booking"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode created booking: %v", err)
		}
		b := res.Booking
		if b.TherapistID != dayoID || b.DisplayName != "Dr. Dayo Achieng" {
			t.Errorf("expected the seeded therapist wired in, got %+v", b)
		}
		if b.Status != "pending" {
			t.Errorf("expected initial status pending, got %q", b.Status)
		}
		if b.ID == "" || b.CreatedAt == "" || b.UpdatedAt == "" {
			t.Errorf("expected generated id and timestamps, got %+v", b)
		}
		if !strings.Contains(b.ScheduledAt, "T") {
			t.Errorf("expected a serialized scheduled_at, got %q", b.ScheduledAt)
		}
		body := w.Body.String()
		for _, leak := range []string{"user_id", "password", "password_hash", "email", "full_name"} {
			if strings.Contains(body, leak) {
				t.Errorf("created booking must never include %q: %s", leak, body)
			}
		}
		aliceBookingID = b.ID
	})

	t.Run("the same slot cannot be double-booked", func(t *testing.T) {
		w := do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", bob, `{"scheduled_at":"`+slotA+`"}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 for bob taking the taken slot, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"BOOKING_CONFLICT"`) {
			t.Errorf("expected BOOKING_CONFLICT code: %s", w.Body.String())
		}

		w = do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", alice, `{"scheduled_at":"`+slotOverlap+`"}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 for an overlapping window, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("past, inactive, and missing targets are rejected", func(t *testing.T) {
		w := do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", alice, `{"scheduled_at":"`+past+`"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for a past scheduled_at, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"scheduled_at"`) {
			t.Errorf("expected the scheduled_at field on the validation error: %s", w.Body.String())
		}

		w = do(t, http.MethodPost, "/api/v1/therapists/"+inactiveID+"/bookings", alice, `{"scheduled_at":"`+slotB+`"}`)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 for an inactive therapist, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"THERAPIST_UNAVAILABLE"`) {
			t.Errorf("expected THERAPIST_UNAVAILABLE code: %s", w.Body.String())
		}

		w = do(t, http.MethodPost, "/api/v1/therapists/00000000-0000-0000-0000-000000000000/bookings", alice, `{"scheduled_at":"`+slotB+`"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for a missing therapist, got %d: %s", w.Code, w.Body.String())
		}

		w = do(t, http.MethodPost, "/api/v1/therapists/not-a-uuid/bookings", alice, `{"scheduled_at":"`+slotB+`"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for a malformed therapist id, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("listing returns only my bookings with therapist display names", func(t *testing.T) {
		w := do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", alice, `{"scheduled_at":"`+slotB+`"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for alice's second booking, got %d: %s", w.Code, w.Body.String())
		}
		var created struct {
			Booking struct {
				ID string `json:"id"`
			} `json:"booking"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("failed to decode second booking: %v", err)
		}
		aliceSecondID = created.Booking.ID

		w = do(t, http.MethodGet, "/api/v1/bookings", alice, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var res struct {
			Bookings []struct {
				ID          string `json:"id"`
				TherapistID string `json:"therapist_id"`
				DisplayName string `json:"display_name"`
				Status      string `json:"status"`
			} `json:"bookings"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode booking list: %v", err)
		}
		if len(res.Bookings) != 2 {
			t.Fatalf("expected alice's 2 bookings, got %d", len(res.Bookings))
		}
		for _, b := range res.Bookings {
			if b.DisplayName != "Dr. Dayo Achieng" {
				t.Errorf("expected the therapist display name on each booking, got %+v", b)
			}
			if b.ID != aliceBookingID && b.ID != aliceSecondID {
				t.Errorf("a foreign booking appeared in alice's list: %+v", b)
			}
		}

		w = do(t, http.MethodGet, "/api/v1/bookings", bob, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"bookings":[]`) {
			t.Errorf("expected bob to have no bookings: %s", w.Body.String())
		}
	})

	t.Run("get returns my booking but never another user's", func(t *testing.T) {
		w := do(t, http.MethodGet, "/api/v1/bookings/"+aliceBookingID, alice, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for the owner, got %d: %s", w.Code, w.Body.String())
		}

		w = do(t, http.MethodGet, "/api/v1/bookings/"+aliceBookingID, bob, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for another user's booking, got %d: %s", w.Code, w.Body.String())
		}

		w = do(t, http.MethodGet, "/api/v1/bookings/not-a-uuid", alice, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for a malformed booking id, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("cancelling a pending booking and its conflict cases", func(t *testing.T) {
		w := do(t, http.MethodPatch, "/api/v1/bookings/"+aliceBookingID+"/cancel", alice, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
			t.Errorf("expected the cancelled status in the response: %s", w.Body.String())
		}

		w = do(t, http.MethodPatch, "/api/v1/bookings/"+aliceBookingID+"/cancel", alice, "")
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 for cancelling a non-pending booking, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"BOOKING_STATUS_CONFLICT"`) {
			t.Errorf("expected BOOKING_STATUS_CONFLICT code: %s", w.Body.String())
		}

		w = do(t, http.MethodPatch, "/api/v1/bookings/"+aliceSecondID+"/cancel", bob, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for cancelling a foreign booking, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("a freed slot can be rebooked by someone else", func(t *testing.T) {
		w := do(t, http.MethodPost, "/api/v1/therapists/"+dayoID+"/bookings", bob, `{"scheduled_at":"`+slotA+`"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 after alice cancelled the slot, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no sensitive material ever appears in booking responses", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/bookings",
			"/api/v1/bookings/" + aliceSecondID,
		} {
			w := do(t, http.MethodGet, path, alice, "")
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			body := w.Body.String()
			for _, leak := range []string{"user_id", "password", "password_hash", "email", "full_name", "credentials", "price_kes"} {
				if strings.Contains(body, leak) {
					t.Errorf("%s: response must never include %q: %s", path, leak, body)
				}
			}
		}
	})
}
