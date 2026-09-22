package journal

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

var testTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// fakeService embeds the Service interface so handler tests only need to stub
// the methods under test.
type fakeService struct {
	Service
	createFunc  func(ctx context.Context, userID, content string, moodTags []string, promptUsed *string) (*JournalEntry, error)
	listFunc    func(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error)
	getFunc     func(ctx context.Context, userID, entryID string) (*JournalEntry, error)
	updateFunc  func(ctx context.Context, userID, entryID string, changes Update) (*JournalEntry, error)
	deleteFunc  func(ctx context.Context, userID, entryID string) error
	reflectFunc func(ctx context.Context, userID, entryID string) (*JournalEntry, error)
}

func (f *fakeService) Create(ctx context.Context, userID, content string, moodTags []string, promptUsed *string) (*JournalEntry, error) {
	if f.createFunc == nil {
		return nil, errors.New("createFunc not configured")
	}
	return f.createFunc(ctx, userID, content, moodTags, promptUsed)
}

func (f *fakeService) List(ctx context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx, userID, limit, before)
}

func (f *fakeService) Get(ctx context.Context, userID, entryID string) (*JournalEntry, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, userID, entryID)
}

func (f *fakeService) Update(ctx context.Context, userID, entryID string, changes Update) (*JournalEntry, error) {
	if f.updateFunc == nil {
		return nil, errors.New("updateFunc not configured")
	}
	return f.updateFunc(ctx, userID, entryID, changes)
}

func (f *fakeService) Delete(ctx context.Context, userID, entryID string) error {
	if f.deleteFunc == nil {
		return errors.New("deleteFunc not configured")
	}
	return f.deleteFunc(ctx, userID, entryID)
}

func (f *fakeService) Reflect(ctx context.Context, userID, entryID string) (*JournalEntry, error) {
	if f.reflectFunc == nil {
		return nil, errors.New("reflectFunc not configured")
	}
	return f.reflectFunc(ctx, userID, entryID)
}

// requestRouter builds a router that simulates the AuthRequired middleware by
// stamping UserIDKey into the Gin context, then serves the request.
func requestRouter(t *testing.T, method, path, body, userID string, handler func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set(middleware.UserIDKey, userID)
		}
		c.Next()
	})
	pathOnly := path
	if i := strings.IndexAny(path, "?"); i >= 0 {
		pathOnly = path[:i]
	}
	router.Handle(method, routePattern(pathOnly), handler)

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

// routePattern converts a concrete request path into the Gin route pattern so
// the :id path parameter resolves on the handler. The journal routes have at
// most one dynamic segment (the id), optionally followed by the literal
// "reflect".
func routePattern(path string) string {
	const prefix = "/api/v1/journal"
	if path == prefix {
		return prefix
	}
	rest, _ := strings.CutPrefix(path, prefix+"/")
	if strings.HasSuffix(rest, "/reflect") {
		return prefix + "/:id/reflect"
	}
	return prefix + "/:id"
}

func entry() *JournalEntry {
	return &JournalEntry{
		ID:           "22222222-2222-2222-2222-222222222222",
		UserID:       testUserID,
		Content:      "Today was hard.",
		MoodTags:     []string{"Tired"},
		PromptUsed:   ptr("My day"),
		AIReflection: ptr("That sounds heavy."),
		WordCount:    3,
		CreatedAt:    testTime,
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

func TestJournalHandlerCreate(t *testing.T) {
	t.Run("returns 201 with the entry and no plaintext or ciphertext", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(_ context.Context, userID, content string, tags []string, prompt *string) (*JournalEntry, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if content != "Today was hard. Mama called again..." {
					t.Errorf("unexpected content: %q", content)
				}
				e := entry()
				e.Content = ""
				e.ContentEnc = nil
				e.ContentIV = nil
				return e, nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal",
			`{"content":"Today was hard. Mama called again...","mood_tags":["Overwhelmed","Loved"],"prompt_used":"Family & pressure"}`,
			testUserID, NewHandler(svc).Create)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"entry"`)) {
			t.Errorf("response missing entry wrapper: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"content"`)) {
			t.Errorf("create response must not include content: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"user_id"`)) {
			t.Errorf("create response must not include user_id: %s", body)
		}
	})

	t.Run("ignores an unknown 'type' field", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, []string, *string) (*JournalEntry, error) {
				e := entry()
				e.Content = ""
				return e, nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal",
			`{"content":"hello","type":"prayer"}`, testUserID, NewHandler(svc).Create)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 (type field is not part of the schema), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal", `{"content":`, testUserID, NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 400 for invalid content with the field set", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, []string, *string) (*JournalEntry, error) {
				return nil, ErrInvalidContent
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal", `{"content":""}`, testUserID, NewHandler(svc).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "content")
	})

	t.Run("returns 400 for invalid mood tags", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, []string, *string) (*JournalEntry, error) {
				return nil, ErrInvalidMoodTags
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal", `{"content":"x","mood_tags":["a","b"]}`, testUserID, NewHandler(svc).Create)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "mood_tags")
	})

	t.Run("returns 401 without an authenticated user", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal", `{"content":"x"}`, "", NewHandler(&fakeService{}).Create)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "UNAUTHORIZED")
	})

	t.Run("returns 500 without leaking internals", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(context.Context, string, string, []string, *string) (*JournalEntry, error) {
				return nil, errors.New("secret plaintext is never in this error")
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal", `{"content":"x"}`, testUserID, NewHandler(svc).Create)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		assertErrorCode(t, rec, "INTERNAL_SERVER_ERROR")
	})
}

func TestJournalHandlerList(t *testing.T) {
	t.Run("returns 200 with entries and a null cursor on a short page", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if limit != 10 {
					t.Errorf("expected limit 10, got %d", limit)
				}
				if before != nil {
					t.Errorf("expected a nil cursor, got %v", *before)
				}
				e := entry()
				e.Content = ""
				return []JournalEntry{*e}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal?limit=10", "", testUserID, NewHandler(svc).List)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"entries"`)) {
			t.Errorf("response missing entries: %s", body)
		}
		if !strings.Contains(body, `"next_cursor":null`) {
			t.Errorf("expected a null next_cursor, got %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"content"`)) {
			t.Errorf("list must not include content: %s", body)
		}
		if bytes.Contains([]byte(body), []byte(`"user_id"`)) {
			t.Errorf("list must not include user_id: %s", body)
		}
	})

	t.Run("sets next_cursor when the page is full", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
				e1 := entry()
				e1.Content = ""
				e1.CreatedAt = testTime.Add(2 * time.Second)
				e2 := entry()
				e2.Content = ""
				e2.CreatedAt = testTime
				return []JournalEntry{*e1, *e2}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal?limit=2", "", testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"next_cursor":"2026-01-01T12:00:00Z"`)) {
			t.Errorf("expected the cursor to point at the last entry: %s", rec.Body.String())
		}
	})

	t.Run("passes the before cursor to the service", func(t *testing.T) {
		svc := &fakeService{
			listFunc: func(_ context.Context, userID string, limit int, before *time.Time) ([]JournalEntry, error) {
				if before == nil || !before.Equal(testTime) {
					t.Errorf("expected the parsed before cursor, got %v", before)
				}
				return []JournalEntry{}, nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal?before=2026-01-01T12:00:00Z", "", testUserID, NewHandler(svc).List)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 400 for a non-integer or non-positive limit", func(t *testing.T) {
		for _, query := range []string{"limit=abc", "limit=0", "limit=-3"} {
			rec := requestRouter(t, http.MethodGet, "/api/v1/journal?"+query, "", testUserID, NewHandler(&fakeService{}).List)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %s, got %d", query, rec.Code)
			}
			assertErrorField(t, rec, "limit")
		}
	})

	t.Run("returns 400 for a malformed before cursor", func(t *testing.T) {
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal?before=not-a-time", "", testUserID, NewHandler(&fakeService{}).List)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorField(t, rec, "before")
	})

	t.Run("returns 401 and 500", func(t *testing.T) {
		t.Run("401 without a user", func(t *testing.T) {
			rec := requestRouter(t, http.MethodGet, "/api/v1/journal", "", "", NewHandler(&fakeService{}).List)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
		t.Run("500 without leaking internals", func(t *testing.T) {
			svc := &fakeService{
				listFunc: func(context.Context, string, int, *time.Time) ([]JournalEntry, error) {
					return nil, errors.New("database is gone")
				},
			}
			rec := requestRouter(t, http.MethodGet, "/api/v1/journal", "", testUserID, NewHandler(svc).List)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d", rec.Code)
			}
			if bytes.Contains(rec.Body.Bytes(), []byte("database is gone")) {
				t.Error("internal detail leaked")
			}
		})
	})
}

func TestJournalHandlerGet(t *testing.T) {
	t.Run("returns 200 with decrypted content", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(_ context.Context, userID, entryID string) (*JournalEntry, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if entryID != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("unexpected entry id: %q", entryID)
				}
				return entry(), nil
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal/22222222-2222-2222-2222-222222222222", "", testUserID, NewHandler(svc).Get)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"content":"Today was hard."`)) {
			t.Errorf("expected the decrypted content: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for a foreign or missing entry", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(context.Context, string, string) (*JournalEntry, error) {
				return nil, ErrJournalEntryNotFound
			},
		}
		rec := requestRouter(t, http.MethodGet, "/api/v1/journal/some-id", "", testUserID, NewHandler(svc).Get)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 401 and 500", func(t *testing.T) {
		t.Run("401 without a user", func(t *testing.T) {
			rec := requestRouter(t, http.MethodGet, "/api/v1/journal/x", "", "", NewHandler(&fakeService{}).Get)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
		t.Run("500 without leaking internals", func(t *testing.T) {
			svc := &fakeService{
				getFunc: func(context.Context, string, string) (*JournalEntry, error) {
					return nil, errors.New("cipher failed")
				},
			}
			rec := requestRouter(t, http.MethodGet, "/api/v1/journal/x", "", testUserID, NewHandler(svc).Get)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d", rec.Code)
			}
			if bytes.Contains(rec.Body.Bytes(), []byte("cipher failed")) {
				t.Error("internal detail leaked")
			}
		})
	})
}

func TestJournalHandlerUpdate(t *testing.T) {
	t.Run("returns 200 with the updated entry", func(t *testing.T) {
		svc := &fakeService{
			updateFunc: func(_ context.Context, userID, entryID string, changes Update) (*JournalEntry, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if changes.Content == nil || *changes.Content != "New words." {
					t.Errorf("unexpected content change: %v", changes.Content)
				}
				updated := entry()
				updated.Content = "New words."
				updated.AIReflection = nil
				return updated, nil
			},
		}
		rec := requestRouter(t, http.MethodPatch, "/api/v1/journal/22222222-2222-2222-2222-222222222222",
			`{"content":"New words."}`, testUserID, NewHandler(svc).Update)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"content":"New words."`)) {
			t.Errorf("expected the updated content: %s", rec.Body.String())
		}
	})

	t.Run("returns 400 when nothing to update", func(t *testing.T) {
		svc := &fakeService{
			updateFunc: func(context.Context, string, string, Update) (*JournalEntry, error) {
				return nil, ErrNothingToUpdate
			},
		}
		rec := requestRouter(t, http.MethodPatch, "/api/v1/journal/x", `{}`, testUserID, NewHandler(svc).Update)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "INVALID_INPUT")
	})

	t.Run("returns 404 for a foreign or missing entry", func(t *testing.T) {
		svc := &fakeService{
			updateFunc: func(context.Context, string, string, Update) (*JournalEntry, error) {
				return nil, ErrJournalEntryNotFound
			},
		}
		rec := requestRouter(t, http.MethodPatch, "/api/v1/journal/x", `{"content":"y"}`, testUserID, NewHandler(svc).Update)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 400 for invalid content with the field set", func(t *testing.T) {
		svc := &fakeService{
			updateFunc: func(context.Context, string, string, Update) (*JournalEntry, error) {
				return nil, ErrInvalidContent
			},
		}
		rec := requestRouter(t, http.MethodPatch, "/api/v1/journal/x", `{"content":""}`, testUserID, NewHandler(svc).Update)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorField(t, rec, "content")
	})

	t.Run("returns 401 without a user", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPatch, "/api/v1/journal/x", `{"content":"y"}`, "", NewHandler(&fakeService{}).Update)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestJournalHandlerDelete(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := &fakeService{
			deleteFunc: func(_ context.Context, userID, entryID string) error {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				if entryID != "22222222-2222-2222-2222-222222222222" {
					t.Errorf("unexpected entry id: %q", entryID)
				}
				return nil
			},
		}
		rec := requestRouter(t, http.MethodDelete, "/api/v1/journal/22222222-2222-2222-2222-222222222222", "", testUserID, NewHandler(svc).Delete)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Errorf("expected an empty body, got %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for a foreign or missing entry", func(t *testing.T) {
		svc := &fakeService{
			deleteFunc: func(context.Context, string, string) error {
				return ErrJournalEntryNotFound
			},
		}
		rec := requestRouter(t, http.MethodDelete, "/api/v1/journal/x", "", testUserID, NewHandler(svc).Delete)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, rec, "NOT_FOUND")
	})

	t.Run("returns 401 without a user", func(t *testing.T) {
		rec := requestRouter(t, http.MethodDelete, "/api/v1/journal/x", "", "", NewHandler(&fakeService{}).Delete)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestJournalHandlerReflect(t *testing.T) {
	t.Run("returns 200 with the generated reflection", func(t *testing.T) {
		svc := &fakeService{
			reflectFunc: func(_ context.Context, userID, entryID string) (*JournalEntry, error) {
				if userID != testUserID {
					t.Errorf("expected the authenticated user id, got %q", userID)
				}
				e := entry()
				e.AIReflection = ptr("A fresh thought.")
				return e, nil
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal/22222222-2222-2222-2222-222222222222/reflect", "", testUserID, NewHandler(svc).Reflect)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"ai_reflection":"A fresh thought."`)) {
			t.Errorf("expected the reflection: %s", rec.Body.String())
		}
	})

	t.Run("returns 404 for a foreign or missing entry", func(t *testing.T) {
		svc := &fakeService{
			reflectFunc: func(context.Context, string, string) (*JournalEntry, error) {
				return nil, ErrJournalEntryNotFound
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal/x/reflect", "", testUserID, NewHandler(svc).Reflect)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 503 with a safe message when AI is unavailable", func(t *testing.T) {
		svc := &fakeService{
			reflectFunc: func(context.Context, string, string) (*JournalEntry, error) {
				return nil, ErrAIReflectionUnavailable
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal/x/reflect", "", testUserID, NewHandler(svc).Reflect)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("key")) {
			t.Error("the error must not hint at key configuration")
		}
		assertErrorCode(t, rec, "AI_REFLECTION_UNAVAILABLE")
	})

	t.Run("returns 401 without a user", func(t *testing.T) {
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal/x/reflect", "", "", NewHandler(&fakeService{}).Reflect)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 500 safely on an unexpected failure", func(t *testing.T) {
		svc := &fakeService{
			reflectFunc: func(context.Context, string, string) (*JournalEntry, error) {
				return nil, errors.New("something broke")
			},
		}
		rec := requestRouter(t, http.MethodPost, "/api/v1/journal/x/reflect", "", testUserID, NewHandler(svc).Reflect)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("something broke")) {
			t.Error("internal error detail leaked")
		}
	})
}
