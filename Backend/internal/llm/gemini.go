package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// geminiResponse mirrors the slice of the generateContent response we read.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// geminiFailure records why Gemini produced no text. finishReason and
// blockReason are provider enums, never user data, so they are safe to log.
type geminiFailure struct {
	err          error
	caseName     string
	finishReason string
	blockReason  string
}

func (e *geminiFailure) Error() string {
	detail := e.caseName
	if e.finishReason != "" {
		detail += ", finishReason=" + e.finishReason
	}
	if e.blockReason != "" {
		detail += ", blockReason=" + e.blockReason
	}
	return fmt.Sprintf("llm: gemini %s: %v", detail, e.err)
}

func (e *geminiFailure) Unwrap() error { return e.err }

// supportsZeroThinkingBudget reports whether the model accepts
// thinkingBudget=0. Current Flash models document zero; sending the field to
// other models (or to image/audio variants) would make the request fail.
func supportsZeroThinkingBudget(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if !strings.Contains(m, "flash") {
		return false
	}
	switch {
	case strings.Contains(m, "image"), strings.Contains(m, "tts"),
		strings.Contains(m, "audio"), strings.Contains(m, "live"):
		return false
	}
	return strings.HasPrefix(m, "gemini-2.5-flash") ||
		strings.HasPrefix(m, "gemini-3") ||
		strings.HasPrefix(m, "gemini-flash")
}

// Gemini is the Google Gemini implementation of Reflector. It holds only the
// API key and model; it never stores or logs content.
type Gemini struct {
	apiKey string
	model  string
	url    string
	http   *http.Client
}

// NewGemini returns a Gemini client for the given API key and model. An empty
// key still produces a usable client whose calls fail with ErrNotConfigured.
func NewGemini(apiKey, model string) *Gemini {
	return newGemini(apiKey, model, GeminiBaseURL+model+":generateContent")
}

func newGemini(apiKey, model, url string) *Gemini {
	return &Gemini{
		apiKey: apiKey,
		model:  model,
		url:    url,
		http:   &http.Client{},
	}
}

// Reflect returns the trimmed reflection, or ErrNotConfigured when no API key is
// set. A safety block is reported as ErrBlocked; any other empty answer (an
// exhausted output budget, no candidates, blank text) is reported as ErrEmpty.
// Journal content never appears in the returned error.
func (g *Gemini) Reflect(ctx context.Context, entry string, moodTags []string) (out string, err error) {
	start := time.Now()
	defer func() { logCall(ProviderGemini, g.model, start, err) }()

	if strings.TrimSpace(g.apiKey) == "" {
		return "", ErrNotConfigured
	}

	generationConfig := map[string]any{
		"maxOutputTokens": GeminiMaxTokens,
	}
	if supportsZeroThinkingBudget(g.model) {
		generationConfig["thinkingConfig"] = map[string]any{"thinkingBudget": 0}
	}

	body, err := json.Marshal(map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": SystemPrompt}},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": reflectionPrompt(entry, moodTags)}},
			},
		},
		"generationConfig": generationConfig,
	})
	if err != nil {
		return "", fmt.Errorf("llm: build gemini request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The key travels in a header, never in the URL, so it cannot leak via logs.
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", &statusError{provider: ProviderGemini, status: resp.StatusCode}
	}

	var parsed geminiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm: decode gemini response: %w", err)
	}
	if parsed.PromptFeedback.BlockReason != "" {
		return "", &geminiFailure{
			err:         ErrBlocked,
			caseName:    "prompt_blocked",
			blockReason: parsed.PromptFeedback.BlockReason,
		}
	}
	if len(parsed.Candidates) == 0 {
		return "", &geminiFailure{err: ErrEmpty, caseName: "no_candidates"}
	}

	var finishReasons []string
	for _, candidate := range parsed.Candidates {
		if candidate.FinishReason != "" {
			finishReasons = append(finishReasons, candidate.FinishReason)
		}
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text), nil
			}
		}
	}

	for _, reason := range finishReasons {
		if reason == "SAFETY" {
			return "", &geminiFailure{err: ErrBlocked, caseName: "finish_safety", finishReason: reason}
		}
	}
	for _, reason := range finishReasons {
		if reason == "MAX_TOKENS" {
			return "", &geminiFailure{err: ErrEmpty, caseName: "max_tokens_empty", finishReason: reason}
		}
	}

	finishReason := ""
	if len(finishReasons) > 0 {
		finishReason = finishReasons[0]
	}
	return "", &geminiFailure{err: ErrEmpty, caseName: "empty_text", finishReason: finishReason}
}
