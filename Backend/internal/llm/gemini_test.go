package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testGeminiModel = "gemini-2.5-flash"

func newFakeGemini(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if want := "/v1beta/models/" + testGeminiModel + ":generateContent"; r.URL.Path != want {
			t.Errorf("expected path %s, got %s", want, r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != testAPIKey {
			t.Errorf("expected x-goog-api-key %q, got %q", testAPIKey, got)
		}
		if strings.Contains(r.URL.RawQuery, testAPIKey) {
			t.Error("the API key must never travel in the query string")
		}
		var req struct {
			SystemInstruction struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"system_instruction"`
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		if len(req.SystemInstruction.Parts) != 1 || req.SystemInstruction.Parts[0].Text != SystemPrompt {
			t.Errorf("expected the shared system prompt as system_instruction")
		}
		if len(req.Contents) != 1 || req.Contents[0].Role != "user" {
			t.Errorf("expected a single user content entry")
		}
		if len(req.Contents[0].Parts) != 1 || !strings.Contains(req.Contents[0].Parts[0].Text, testContent) {
			t.Errorf("request did not include the journal content")
		}
		if req.GenerationConfig.MaxOutputTokens != MaxTokens {
			t.Errorf("expected maxOutputTokens %d, got %d", MaxTokens, req.GenerationConfig.MaxOutputTokens)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestGeminiReflectSuccess(t *testing.T) {
	srv := newFakeGemini(t, http.StatusOK,
		`{"candidates":[{"content":{"parts":[{"text":"It is brave to sit with that anxiety.\n"}]},"finishReason":"STOP"}]}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	got, err := g.Reflect(context.Background(), testContent, []string{"anxiety"})
	if err != nil {
		t.Fatalf("Reflect returned error: %v", err)
	}
	if got != testReflection {
		t.Errorf("expected %q, got %q", testReflection, got)
	}
}

func TestGeminiReflectNotConfigured(t *testing.T) {
	g := NewGemini("", testGeminiModel)
	_, err := g.Reflect(context.Background(), testContent, nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestGeminiReflectRejectsNonSuccessStatus(t *testing.T) {
	srv := newFakeGemini(t, http.StatusInternalServerError, `{"error":"boom"}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	_, err := g.Reflect(context.Background(), testContent, nil)
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

func TestGeminiReflectRejectsMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL)
	if _, err := g.Reflect(context.Background(), testContent, nil); err == nil {
		t.Fatal("expected error for malformed response")
	}
}

func TestGeminiReflectBlocked(t *testing.T) {
	cases := map[string]string{
		"prompt feedback block": `{"promptFeedback":{"blockReason":"SAFETY"}}`,
		"no candidates":         `{"candidates":[]}`,
		"empty parts":           `{"candidates":[{"content":{"parts":[]}}]}`,
		"blank text":            `{"candidates":[{"content":{"parts":[{"text":"   "}]}}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newFakeGemini(t, http.StatusOK, body)
			defer srv.Close()

			g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
			_, err := g.Reflect(context.Background(), testContent, nil)
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("expected ErrBlocked, got %v", err)
			}
		})
	}
}
