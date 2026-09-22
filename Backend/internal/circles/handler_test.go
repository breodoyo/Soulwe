package circles

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

const testIdentityID = "11111111-1111-1111-1111-111111111111"

// fakeService embeds the Service interface so handler tests only need to stub
// the methods under test.
type fakeService struct {
	Service
	listFunc         func(ctx context.Context) ([]Circle, error)
	getFunc          func(ctx context.Context, anonIdentityID, circleID string) (*Circle, error)
	joinFunc         func(ctx context.Context, anonIdentityID, circleID string) error
	leaveFunc        func(ctx context.Context, anonIdentityID, circleID string) error
	listMessagesFunc func(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error)
	sendMessageFunc  func(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error)
}

func (f *fakeService) List(ctx context.Context) ([]Circle, error) {
	if f.listFunc == nil {
		return nil, errors.New("listFunc not configured")
	}
	return f.listFunc(ctx)
}

func (f *fakeService) Get(ctx context.Context, anonIdentityID, circleID string) (*Circle, error) {
	if f.getFunc == nil {
		return nil, errors.New("getFunc not configured")
	}
	return f.getFunc(ctx, anonIdentityID, circleID)
}

func (f *fakeService) Join(ctx context.Context, anonIdentityID, circleID string) error {
	if f.joinFunc == nil {
		return errors.New("joinFunc not configured")
	}
	return f.joinFunc(ctx, anonIdentityID, circleID)
}

func (f *fakeService) Leave(ctx context.Context, anonIdentityID, circleID string) error {
	if f.leaveFunc == nil {
		return errors.New("leaveFunc not configured")
	}
	return f.leaveFunc(ctx, anonIdentityID, circleID)
}

func (f *fakeService) ListMessages(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
	if f.listMessagesFunc == nil {
		return nil, errors.New("listMessagesFunc not configured")
	}
	return f.listMessagesFunc(ctx, anonIdentityID, circleID, limit, before)
}

func (f *fakeService) SendMessage(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
	if f.sendMessageFunc == nil {
		return nil, errors.New("sendMessageFunc not configured")
	}
	return f.sendMessageFunc(ctx, anonIdentityID, circleID, content)
}

// newTestRouter builds a router that simulates the anonymous middleware by
// stamping AnonIdentityIDKey into the Gin context, then registers the circles
// handler on every Phase 6.1 route.
func newTestRouter(t *testing.T, h *Handler, identityID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if identityID != "" {
			c.Set(middleware.AnonIdentityIDKey, identityID)
		}
		c.Next()
	})
	group := router.Group("/api/v1")
	{
		group.GET("/circles", h.List)
		group.GET("/circles/:id", h.Get)
		group.POST("/circles/:id/join", h.Join)
		group.DELETE("/circles/:id/leave", h.Leave)
		group.GET("/circles/:id/messages", h.ListMessages)
		group.POST("/circles/:id/messages", h.SendMessage)
	}
	return router
}

func serve(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
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

func circle() *Circle {
	return &Circle{
		ID:          "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Slug:        "grief",
		Name:        "Grief & Loss",
		Description: strPtr("grief support"),
		Icon:        strPtr("🌿"),
		MemberCount: 1,
		IsMember:    true,
		CreatedAt:   testNow,
	}
}

func message() *CircleMessage {
	return &CircleMessage{
		ID:             "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		AnonName:       "Anon Baobab",
		Content:        "Lost my father last month.",
		ReactionCounts: map[string]int{},
		CreatedAt:      testNow,
	}
}

func TestHandler_UnauthenticatedRequests(t *testing.T) {
	// No identity in the context: every route must answer 401 defensively,
	// matching what the anonymous middleware enforces in the router.
	router := newTestRouter(t, NewHandler(&fakeService{}), "")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"discovery", http.MethodGet, "/api/v1/circles", ""},
		{"detail", http.MethodGet, "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", ""},
		{"join", http.MethodPost, "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/join", ""},
		{"leave", http.MethodDelete, "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/leave", ""},
		{"list messages", http.MethodGet, "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/messages", ""},
		{"send message", http.MethodPost, "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/messages", `{"content":"hi"}`},
	}
	for _, tc := range cases {
		w := serve(t, router, tc.method, tc.path, tc.body)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d: %s", tc.name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"UNAUTHORIZED"`) {
			t.Errorf("%s: expected the safe error envelope: %s", tc.name, w.Body.String())
		}
	}
}

func TestHandler_List(t *testing.T) {
	grief := circle()
	router := newTestRouter(t, NewHandler(&fakeService{
		listFunc: func(ctx context.Context) ([]Circle, error) {
			return []Circle{*grief}, nil
		},
	}), testIdentityID)

	t.Run("returns the circles array with no sensitive fields", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, "/api/v1/circles", "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"circles":[{`) {
			t.Errorf("expected a circles array: %s", body)
		}
		for _, leak := range []string{"anon_identity_id", "device_uuid", "token_hash", "password"} {
			if strings.Contains(body, leak) {
				t.Errorf("discovery response must never include %s: %s", leak, body)
			}
		}
	})

	t.Run("service failure is a safe 500", func(t *testing.T) {
		failing := newTestRouter(t, NewHandler(&fakeService{
			listFunc: func(ctx context.Context) ([]Circle, error) {
				return nil, errors.New("boom")
			},
		}), testIdentityID)
		w := serve(t, failing, http.MethodGet, "/api/v1/circles", "")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"INTERNAL_SERVER_ERROR"`) {
			t.Errorf("expected the safe error envelope: %s", w.Body.String())
		}
	})
}

func TestHandler_Get(t *testing.T) {
	grief := circle()
	router := newTestRouter(t, NewHandler(&fakeService{
		getFunc: func(ctx context.Context, anonIdentityID, circleID string) (*Circle, error) {
			if circleID != grief.ID {
				return nil, ErrCircleNotFound
			}
			return grief, nil
		},
	}), testIdentityID)

	t.Run("returns circle details including membership", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, "/api/v1/circles/"+grief.ID, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"is_member":true`) {
			t.Errorf("expected is_member in the detail response: %s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"member_count":1`) {
			t.Errorf("expected the member count: %s", w.Body.String())
		}
	})

	t.Run("missing circle is a 404", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, "/api/v1/circles/99999999-9999-9999-9999-999999999999", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandler_Join(t *testing.T) {
	const id = "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/join"

	router := newTestRouter(t, NewHandler(&fakeService{
		joinFunc: func(ctx context.Context, anonIdentityID, circleID string) error {
			return nil
		},
	}), testIdentityID)

	joinUnderCaller := func(identityID string, err error) *gin.Engine {
		svc := &fakeService{
			joinFunc: func(ctx context.Context, anonIdentityID, circleID string) error {
				if anonIdentityID != identityID {
					t.Errorf("service must receive the context identity, got %q", anonIdentityID)
				}
				return err
			},
		}
		return newTestRouter(t, NewHandler(svc), identityID)
	}

	t.Run("a successful join is 204", func(t *testing.T) {
		w := serve(t, router, http.MethodPost, id, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing circle is a 404", func(t *testing.T) {
		w := serve(t, joinUnderCaller(testIdentityID, ErrCircleNotFound), http.MethodPost, id, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("duplicate membership is a 409", func(t *testing.T) {
		w := serve(t, joinUnderCaller(testIdentityID, ErrAlreadyMember), http.MethodPost, id, "")
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"ALREADY_MEMBER"`) {
			t.Errorf("expected the ALREADY_MEMBER code: %s", w.Body.String())
		}
	})

	t.Run("service failure is a safe 500", func(t *testing.T) {
		w := serve(t, joinUnderCaller(testIdentityID, errors.New("boom")), http.MethodPost, id, "")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandler_Leave(t *testing.T) {
	const id = "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/leave"

	leaveUnderCaller := func(identityID string, err error) *gin.Engine {
		svc := &fakeService{
			leaveFunc: func(ctx context.Context, anonIdentityID, circleID string) error {
				if anonIdentityID != identityID {
					t.Errorf("service must receive the context identity, got %q", anonIdentityID)
				}
				return err
			},
		}
		return newTestRouter(t, NewHandler(svc), identityID)
	}

	t.Run("a successful leave is 204 and idempotent", func(t *testing.T) {
		w := serve(t, leaveUnderCaller(testIdentityID, nil), http.MethodDelete, id, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing circle is a 404", func(t *testing.T) {
		w := serve(t, leaveUnderCaller(testIdentityID, ErrCircleNotFound), http.MethodDelete, id, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandler_ListMessages(t *testing.T) {
	msg := message()
	msgs := []CircleMessage{*msg}
	const id = "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/messages"

	router := newTestRouter(t, NewHandler(&fakeService{
		listMessagesFunc: func(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
			return msgs, nil
		},
	}), testIdentityID)

	t.Run("returns messages newest first with the cursor", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, id, "")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"anon_name":"Anon Baobab"`) {
			t.Errorf("expected the anon name in the response: %s", body)
		}
		if strings.Contains(body, "anon_identity_id") {
			t.Errorf("list must never leak anon_identity_id: %s", body)
		}
		if !strings.Contains(body, `"next_cursor":`) {
			t.Errorf("expected the next_cursor key: %s", body)
		}
	})

	t.Run("non-members are forbidden", func(t *testing.T) {
		denied := newTestRouter(t, NewHandler(&fakeService{
			listMessagesFunc: func(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
				return nil, ErrNotMember
			},
		}), testIdentityID)
		w := serve(t, denied, http.MethodGet, id, "")
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":"NOT_A_MEMBER"`) {
			t.Errorf("expected the NOT_A_MEMBER code: %s", w.Body.String())
		}
	})

	t.Run("missing circle is a 404", func(t *testing.T) {
		missing := newTestRouter(t, NewHandler(&fakeService{
			listMessagesFunc: func(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
				return nil, ErrCircleNotFound
			},
		}), testIdentityID)
		w := serve(t, missing, http.MethodGet, id, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid limit is a 400", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, id+"?limit=not-a-number", "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"limit"`) {
			t.Errorf("expected the limit field on the error: %s", w.Body.String())
		}
	})

	t.Run("invalid before is a 400", func(t *testing.T) {
		w := serve(t, router, http.MethodGet, id+"?before=not-a-timestamp", "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandler_SendMessage(t *testing.T) {
	msg := message()
	const id = "/api/v1/circles/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/messages"

	router := newTestRouter(t, NewHandler(&fakeService{
		sendMessageFunc: func(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
			return msg, nil
		},
	}), testIdentityID)

	t.Run("a member creates a message with 201", func(t *testing.T) {
		w := serve(t, router, http.MethodPost, id, `{"content":"Lost my father last month."}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"anon_name":"Anon Baobab"`) && !strings.Contains(body, `"content":`) {
			t.Errorf("unexpected message body: %s", body)
		}
		if strings.Contains(body, "anon_identity_id") {
			t.Errorf("send must never leak anon_identity_id: %s", body)
		}
	})

	t.Run("invalid content is a 400 with the content field", func(t *testing.T) {
		rejecting := newTestRouter(t, NewHandler(&fakeService{
			sendMessageFunc: func(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
				return nil, ErrInvalidContent
			},
		}), testIdentityID)
		w := serve(t, rejecting, http.MethodPost, id, `{"content":""}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"field":"content"`) {
			t.Errorf("expected the content field on the error: %s", w.Body.String())
		}
	})

	t.Run("malformed JSON is a 400", func(t *testing.T) {
		w := serve(t, router, http.MethodPost, id, `{"content":`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("non-members are forbidden", func(t *testing.T) {
		denied := newTestRouter(t, NewHandler(&fakeService{
			sendMessageFunc: func(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
				return nil, ErrNotMember
			},
		}), testIdentityID)
		w := serve(t, denied, http.MethodPost, id, `{"content":"hello"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing circle is a 404", func(t *testing.T) {
		missing := newTestRouter(t, NewHandler(&fakeService{
			sendMessageFunc: func(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
				return nil, ErrCircleNotFound
			},
		}), testIdentityID)
		w := serve(t, missing, http.MethodPost, id, `{"content":"hello"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})
}
