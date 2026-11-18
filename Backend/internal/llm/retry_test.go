package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClaudeRetriesOnceOnOverloaded(t *testing.T) {
	var calls int32
	start := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"overloaded"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"You are doing better than you think."}]}`))
	}))
	defer srv.Close()

	c := newClaude(testAPIKey, testModel, srv.URL+"/v1/messages")
	var got string
	var err error
	logs := captureLogs(t, func() {
		got, err = c.Reflect(context.Background(), testContent, nil)
	})
	if err != nil {
		t.Fatalf("Reflect returned error: %v", err)
	}
	if want := "You are doing better than you think."; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if calls != 2 {
		t.Errorf("expected 2 attempts, got %d", calls)
	}
	if elapsed := time.Since(start); elapsed < 800*time.Millisecond {
		t.Errorf("Retry-After was not respected: elapsed %s", elapsed)
	}
	for _, want := range []string{"attempt=1", "attempt=2", "http_status=503", "elapsed=", `category="http status"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q: %s", want, logs)
		}
	}
}

func TestGeminiFallsBackAfterRepeatedRateLimits(t *testing.T) {
	const fallbackModel = "gemini-2.5-pro"
	var primaryCalls, fallbackCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/"+testGeminiModel+":"):
			atomic.AddInt32(&primaryCalls, 1)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
		case strings.Contains(r.URL.Path, "/"+fallbackModel+":"):
			atomic.AddInt32(&fallbackCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := newGeminiWithFallback(testAPIKey, testGeminiModel, fallbackModel,
		srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	var got string
	var err error
	logs := captureLogs(t, func() {
		got, err = g.Reflect(context.Background(), testContent, nil)
	})
	if err != nil {
		t.Fatalf("Reflect returned error: %v", err)
	}
	if got != "ok" {
		t.Errorf("expected fallback reflection %q, got %q", "ok", got)
	}
	if primaryCalls != 2 {
		t.Errorf("expected 2 primary calls, got %d", primaryCalls)
	}
	if fallbackCalls != 1 {
		t.Errorf("expected 1 fallback call, got %d", fallbackCalls)
	}
	for _, want := range []string{
		"attempt=3",
		"model=" + fallbackModel,
		"http_status=429",
		"elapsed=",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q: %s", want, logs)
		}
	}
}

func TestNoRetryForClientAndServerErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(status)
			}))
			defer srv.Close()

			c := newClaude(testAPIKey, testModel, srv.URL+"/v1/messages")
			var err error
			logs := captureLogs(t, func() {
				_, err = c.Reflect(context.Background(), testContent, nil)
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if calls != 1 {
				t.Errorf("expected 1 attempt, got %d", calls)
			}
			if strings.Contains(logs, "attempt=2") {
				t.Errorf("unexpected retry in log: %s", logs)
			}
		})
	}
}
