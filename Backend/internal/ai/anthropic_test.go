package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testAPIKey     = "sk-ant-testing-secret"
	testModel      = "claude-3-5-haiku-latest"
	testContent    = "I felt anxious before the interview today."
	testReflection = "It is brave to sit with that anxiety."
)

func newFakeAnthropic(t *testing.T, status int, response any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected path /v1/messages, got %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != testAPIKey {
			t.Errorf("expected x-api-key %q, got %q", testAPIKey, got)
		}
		if got := r.Header.Get("anthropic-version"); got != APIVersion {
			t.Errorf("expected anthropic-version %q, got %q", APIVersion, got)
		}
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    string `json:"system"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		if body.Model != testModel {
			t.Errorf("expected model %q, got %q", testModel, body.Model)
		}
		if body.MaxTokens != MaxTokens {
			t.Errorf("expected max_tokens %d, got %d", MaxTokens, body.MaxTokens)
		}
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" {
			t.Errorf("expected a single user message")
		}
		if !strings.Contains(body.Messages[0].Content, testContent) {
			t.Errorf("request did not include the journal content")
		}
		if strings.Contains(body.Messages[0].Content, testAPIKey) {
			t.Errorf("request leaked the API key into the prompt")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if response != nil {
			_ = json.NewEncoder(w).Encode(response)
		}
	}))
}

func TestGenerateReflectionSuccess(t *testing.T) {
	srv := newFakeAnthropic(t, http.StatusOK, messagesResponse{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: "It is brave to sit with that anxiety." + "\n"}},
	})
	defer srv.Close()

	c := NewClientForTests(srv.URL+"/v1/messages", testAPIKey, testModel)
	got, err := c.GenerateReflection(context.Background(), testContent, []string{"anxiety"})
	if err != nil {
		t.Fatalf("GenerateReflection returned error: %v", err)
	}
	if got != testReflection {
		t.Errorf("expected %q, got %q", testReflection, got)
	}
}

func TestGenerateReflectionNotConfigured(t *testing.T) {
	c := NewClient("", testModel)
	_, err := c.GenerateReflection(context.Background(), testContent, nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestGenerateReflectionRejectsNonSuccessStatus(t *testing.T) {
	srv := newFakeAnthropic(t, http.StatusInternalServerError, map[string]string{"error": "boom"})
	defer srv.Close()

	c := NewClientForTests(srv.URL+"/v1/messages", testAPIKey, testModel)
	_, err := c.GenerateReflection(context.Background(), testContent, nil)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if strings.Contains(err.Error(), "boom") {
		t.Error("error must not echo the upstream response body")
	}
	if strings.Contains(err.Error(), testContent) {
		t.Error("error must not contain journal content")
	}
}

func TestGenerateReflectionRejectsMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	c := NewClientForTests(srv.URL, testAPIKey, testModel)
	if _, err := c.GenerateReflection(context.Background(), testContent, nil); err == nil {
		t.Fatal("expected error for malformed response")
	}
}

func TestGenerateReflectionRejectsEmptyText(t *testing.T) {
	srv := newFakeAnthropic(t, http.StatusOK, messagesResponse{Content: []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{}})
	defer srv.Close()

	c := NewClientForTests(srv.URL+"/v1/messages", testAPIKey, testModel)
	if _, err := c.GenerateReflection(context.Background(), testContent, nil); err == nil {
		t.Fatal("expected error when no text block is returned")
	}
}
