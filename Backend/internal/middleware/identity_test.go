package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Backend/internal/auth"
	"Backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// stubVerifier recognizes exactly one token, so a test can drive both branches
// of the middleware by presenting a JWT or a non-JWT.
type stubVerifier struct {
	knownToken string
	userID     string
}

func (s stubVerifier) ParseAccessToken(token string) (string, error) {
	if s.knownToken == "" || token != s.knownToken {
		return "", errors.New("token is not a valid access token")
	}
	return s.userID, nil
}

// stubAnonVerifier has a single known token; err drives the "repository
// failure" branch, which must not be confused with "unknown token".
type stubAnonVerifier struct {
	knownToken string
	identityID string
	err        error
	calls      int
}

func (s *stubAnonVerifier) Authenticate(_ context.Context, rawToken string) (string, bool, error) {
	s.calls++
	if s.err != nil {
		return "", false, s.err
	}
	if rawToken != s.knownToken {
		return "", false, nil
	}
	return s.identityID, true, nil
}

// recordingHandler captures the resolved owner so tests can assert the identity
// type, not just the status code.
type recordingHandler struct {
	owner middleware.Owner
	ok    bool
}

func (h *recordingHandler) handle(c *gin.Context) {
	h.owner, h.ok = middleware.OwnerFromContext(c)
	c.Status(http.StatusOK)
}

func newIdentityRouter(t *testing.T, verifier middleware.RegisteredVerifier, anon middleware.AnonymousVerifier) (*gin.Engine, *recordingHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := &recordingHandler{}
	r := gin.New()
	r.GET("/private", middleware.IdentityRequired(verifier, anon), rec.handle)
	return r, rec
}

const (
	identityTestUser     = "11111111-1111-1111-1111-111111111111"
	identityTestAnon     = "22222222-2222-2222-2222-222222222222"
	identityTestJWTToken = "registered-access-token"
	identityTestToken    = "raw-anonymous-token"
)

func TestIdentityRequiredAcceptsEitherCredential(t *testing.T) {
	anon := &stubAnonVerifier{knownToken: identityTestToken, identityID: identityTestAnon}
	verifier := stubVerifier{knownToken: identityTestJWTToken, userID: identityTestUser}
	router, rec := newIdentityRouter(t, verifier, anon)

	t.Run("a registered access token resolves to a user owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Authorization", "Bearer "+identityTestJWTToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !rec.ok || rec.owner.UserID != identityTestUser || rec.owner.Anonymous() {
			t.Errorf("expected a registered owner %q, got %+v (ok=%t)", identityTestUser, rec.owner, rec.ok)
		}
	})

	t.Run("an anonymous token resolves to an anonymous owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Authorization", "Bearer "+identityTestToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if !rec.ok || rec.owner.AnonIdentityID != identityTestAnon || rec.owner.Registered() {
			t.Errorf("expected an anonymous owner %q, got %+v (ok=%t)", identityTestAnon, rec.owner, rec.ok)
		}
	})
}

func TestIdentityRequiredPrefersTheRegisteredToken(t *testing.T) {
	// A token that is both a valid JWT and a known anonymous token must resolve to
	// the registered user: a signed-in user is never demoted to their anon session.
	anon := &stubAnonVerifier{knownToken: "shared-token", identityID: identityTestAnon}
	verifier := stubVerifier{knownToken: "shared-token", userID: identityTestUser}
	router, rec := newIdentityRouter(t, verifier, anon)

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer shared-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if rec.owner.UserID != identityTestUser || rec.owner.Anonymous() {
		t.Errorf("expected the registered owner to win, got %+v", rec.owner)
	}
	if anon.calls != 0 {
		t.Errorf("the anonymous verifier must not be consulted once a JWT parses, calls=%d", anon.calls)
	}
}

func TestIdentityRequiredRejectsBadCredentials(t *testing.T) {
	anon := &stubAnonVerifier{knownToken: identityTestToken, identityID: identityTestAnon}
	verifier := stubVerifier{knownToken: identityTestJWTToken, userID: identityTestUser}
	router, rec := newIdentityRouter(t, verifier, anon)

	for _, tc := range []struct {
		name   string
		header string
	}{
		{"no Authorization header", ""},
		{"a non-bearer scheme", "Basic dXNlcjpwYXNz"},
		{"a bearer header with an empty token", "Bearer "},
		{"an unknown token", "Bearer never-issued"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
			if rec.ok {
				t.Error("no owner may reach the handler for an unauthenticated request")
			}
		})
	}
}

// A repository failure must be a 500 so a client does not discard a valid token
// over a transient database fault.
func TestIdentityRequiredReportsVerifierFailuresAsServerErrors(t *testing.T) {
	anon := &stubAnonVerifier{err: errors.New("database is down")}
	router, rec := newIdentityRouter(t, stubVerifier{}, anon)

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer "+identityTestToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if rec.ok {
		t.Error("no owner may reach the handler when authentication failed")
	}
	if body := w.Body.String(); strings.Contains(body, "database is down") {
		t.Errorf("the internal failure reason must not reach the client: %s", body)
	}
}

// A server without the anonymous service must reject anonymous credentials
// rather than panicking on a nil verifier.
func TestIdentityRequiredWithoutAVerifierRejectsAnonymousTokens(t *testing.T) {
	router, rec := newIdentityRouter(t, stubVerifier{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer "+identityTestToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if rec.ok {
		t.Error("no owner may reach the handler without an anonymous verifier")
	}
}

// The real token manager must satisfy RegisteredVerifier end to end.
func TestIdentityRequiredWithTheRealTokenManager(t *testing.T) {
	manager, err := auth.NewManager("unit-test-secret-that-must-be-long-enough-for-signing")
	if err != nil {
		t.Fatalf("auth.NewManager returned error: %v", err)
	}
	jwt, err := manager.SignAccessToken(identityTestUser)
	if err != nil {
		t.Fatalf("SignAccessToken returned error: %v", err)
	}

	anon := &stubAnonVerifier{knownToken: identityTestToken, identityID: identityTestAnon}
	router, rec := newIdentityRouter(t, manager, anon)

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if rec.owner.UserID != identityTestUser {
		t.Errorf("expected owner %q, got %+v", identityTestUser, rec.owner)
	}
}

func TestIdentityFromOwnerRejectsAmbiguousOwners(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner middleware.Owner
		valid bool
	}{
		{"registered", middleware.Owner{UserID: identityTestUser}, true},
		{"anonymous", middleware.Owner{AnonIdentityID: identityTestAnon}, true},
		{"both identities", middleware.Owner{UserID: identityTestUser, AnonIdentityID: identityTestAnon}, false},
		{"neither identity", middleware.Owner{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := middleware.IdentityFromOwner(tc.owner)
			if ok != tc.valid {
				t.Errorf("IdentityFromOwner(%+v) = %t, want %t", tc.owner, ok, tc.valid)
			}
		})
	}
}

func TestOwnerFromContextRejectsAMalformedStoredOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"a registered owner", middleware.Owner{UserID: identityTestUser}, true},
		{"an ambiguous owner", middleware.Owner{UserID: identityTestUser, AnonIdentityID: identityTestAnon}, false},
		{"the zero owner", middleware.Owner{}, false},
		{"a value of the wrong type", "not an owner", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(middleware.OwnerKey, tc.value)
			_, ok := middleware.OwnerFromContext(c)
			if ok != tc.want {
				t.Errorf("OwnerFromContext with %T = %t, want %t", tc.value, ok, tc.want)
			}
		})
	}

	t.Run("no owner stored at all", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if _, ok := middleware.OwnerFromContext(c); ok {
			t.Error("OwnerFromContext must fail when nothing was stored")
		}
	})
}
