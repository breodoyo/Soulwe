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
				ThinkingConfig  *struct {
					ThinkingBudget *int `json:"thinkingBudget"`
				} `json:"thinkingConfig"`
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
		if req.GenerationConfig.MaxOutputTokens != GeminiMaxTokens {
			t.Errorf("expected maxOutputTokens %d, got %d", GeminiMaxTokens, req.GenerationConfig.MaxOutputTokens)
		}
		if req.GenerationConfig.ThinkingConfig == nil || req.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
			t.Error("expected thinkingConfig.thinkingBudget for gemini-2.5-flash")
		} else if *req.GenerationConfig.ThinkingConfig.ThinkingBudget != 0 {
			t.Errorf("expected thinkingBudget 0, got %d", *req.GenerationConfig.ThinkingConfig.ThinkingBudget)
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

func TestGeminiReflectMaxTokensEmptyIsEmptyNotBlocked(t *testing.T) {
	srv := newFakeGemini(t, http.StatusOK,
		`{"candidates":[{"content":{"parts":[]},"finishReason":"MAX_TOKENS"}]}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	_, err := g.Reflect(context.Background(), testContent, nil)
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
	if errors.Is(err, ErrBlocked) {
		t.Fatalf("MAX_TOKENS with no text must not be reported as ErrBlocked: %v", err)
	}
}

func TestGeminiReflectPromptBlockIsBlocked(t *testing.T) {
	srv := newFakeGemini(t, http.StatusOK, `{"promptFeedback":{"blockReason":"SAFETY"}}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	_, err := g.Reflect(context.Background(), testContent, nil)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestGeminiReflectFinishSafetyIsBlocked(t *testing.T) {
	srv := newFakeGemini(t, http.StatusOK,
		`{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	_, err := g.Reflect(context.Background(), testContent, nil)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

func TestGeminiReflectEmptyAnswersAreEmpty(t *testing.T) {
	cases := map[string]string{
		"no candidates": `{"candidates":[]}`,
		"empty parts":   `{"candidates":[{"content":{"parts":[]}}]}`,
		"blank text":    `{"candidates":[{"content":{"parts":[{"text":"   "}]}}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newFakeGemini(t, http.StatusOK, body)
			defer srv.Close()

			g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
			_, err := g.Reflect(context.Background(), testContent, nil)
			if !errors.Is(err, ErrEmpty) {
				t.Fatalf("expected ErrEmpty, got %v", err)
			}
			if errors.Is(err, ErrBlocked) {
				t.Fatalf("an empty answer must not be reported as ErrBlocked: %v", err)
			}
		})
	}
}

func TestGeminiRequestOmitsThinkingBudgetForUnsupportedModels(t *testing.T) {
	const model = "gemini-1.5-pro"
	var sawThinkingConfig bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GenerationConfig struct {
				ThinkingConfig *struct {
					ThinkingBudget *int `json:"thinkingBudget"`
				} `json:"thinkingConfig"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		sawThinkingConfig = req.GenerationConfig.ThinkingConfig != nil
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	}))
	defer srv.Close()

	g := newGemini(testAPIKey, model, srv.URL)
	if _, err := g.Reflect(context.Background(), testContent, nil); err != nil {
		t.Fatalf("Reflect returned error: %v", err)
	}
	if sawThinkingConfig {
		t.Error("thinkingConfig must not be sent for a model that does not support thinkingBudget 0")
	}
}

func TestGeminiFailureLogNamesTheCaseWithoutContent(t *testing.T) {
	srv := newFakeGemini(t, http.StatusOK,
		`{"candidates":[{"content":{"parts":[]},"finishReason":"MAX_TOKENS"}]}`)
	defer srv.Close()

	g := newGemini(testAPIKey, testGeminiModel, srv.URL+"/v1beta/models/"+testGeminiModel+":generateContent")
	logs := captureLogs(t, func() {
		_, _ = g.Reflect(context.Background(), testContent, nil)
	})

	for _, want := range []string{
		"provider=" + ProviderGemini,
		"model=" + testGeminiModel,
		"finish_reason=MAX_TOKENS",
		"case=max_tokens_empty",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q: %s", want, logs)
		}
	}
	for _, forbidden := range []string{testContent, testAPIKey} {
		if strings.Contains(logs, forbidden) {
			t.Errorf("log must never contain %q: %s", forbidden, logs)
		}
	}
}
