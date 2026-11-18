package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
// thinkingBudget=0. Lite, image and audio variants reject it.
func supportsZeroThinkingBudget(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if !strings.Contains(m, "flash") {
		return false
	}
	switch {
	case strings.Contains(m, "lite"),
		strings.Contains(m, "image"), strings.Contains(m, "tts"),
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
	apiKey      string
	model       string
	fallback    string
	url         string
	fallbackURL string
	http        *http.Client
}

// NewGemini returns a Gemini client for the given API key and model. An empty
// key still produces a usable client whose calls fail with ErrNotConfigured.
func NewGemini(apiKey, model string) *Gemini {
	return NewGeminiWithFallback(apiKey, model, "")
}

// NewGeminiWithFallback adds the optional fallback model used after repeated
// 429 or 503 responses.
func NewGeminiWithFallback(apiKey, model, fallback string) *Gemini {
	return newGeminiWithFallback(apiKey, model, fallback, GeminiBaseURL+model+":generateContent")
}

func newGemini(apiKey, model, url string) *Gemini {
	return newGeminiWithFallback(apiKey, model, "", url)
}

func newGeminiWithFallback(apiKey, model, fallback, url string) *Gemini {
	g := &Gemini{
		apiKey:   apiKey,
		model:    model,
		fallback: fallback,
		url:      url,
		http:     &http.Client{},
	}
	if fallback != "" && fallback != model {
		g.fallbackURL = strings.Replace(url, model, fallback, 1)
	}
	return g
}

// Reflect returns the trimmed reflection, or ErrNotConfigured when no API key is
// set. A safety block is reported as ErrBlocked; any other empty answer (an
// exhausted output budget, no candidates, blank text) is reported as ErrEmpty.
// Journal content never appears in the returned error.
func (g *Gemini) Reflect(ctx context.Context, entry string, moodTags []string) (string, error) {
	return runReflection(ctx, ProviderGemini, g.model, g.fallback, func(callCtx context.Context, model string) (string, error) {
		url := g.url
		if model == g.fallback && g.fallbackURL != "" {
			url = g.fallbackURL
		}
		return g.reflectOnce(callCtx, model, url, entry, moodTags)
	})
}

func (g *Gemini) reflectOnce(ctx context.Context, model, url, entry string, moodTags []string) (string, error) {
	if strings.TrimSpace(g.apiKey) == "" {
		return "", ErrNotConfigured
	}

	out, err, includedThinking := g.reflectWithThinking(ctx, model, url, entry, moodTags, true)
	if includedThinking && statusOf(err) == http.StatusBadRequest {
		slog.Info("llm gemini retried without thinkingConfig",
			slog.String("provider", ProviderGemini),
			slog.String("model", model),
			slog.String("note", "thinkingConfig dropped after HTTP 400"),
		)
		out, err, _ = g.reflectWithThinking(ctx, model, url, entry, moodTags, false)
		return out, err
	}
	return out, err
}

func (g *Gemini) reflectWithThinking(ctx context.Context, model, url, entry string, moodTags []string, allowThinking bool) (out string, err error, includedThinking bool) {
	includedThinking = allowThinking && supportsZeroThinkingBudget(model)
	generationConfig := map[string]any{
		"maxOutputTokens": GeminiMaxTokens,
	}
	if includedThinking {
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
		return "", fmt.Errorf("llm: build gemini request: %w", err), includedThinking
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: create gemini request: %w", err), includedThinking
	}
	req.Header.Set("Content-Type", "application/json")
	// The key travels in a header, never in the URL, so it cannot leak via logs.
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: gemini request failed: %w", err), includedThinking
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", newStatusErrorFromBody(ProviderGemini, resp.StatusCode, resp.Header, resp.Body, g.apiKey), includedThinking
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", newStatusError(ProviderGemini, resp.StatusCode, resp.Header), includedThinking
	}

	var parsed geminiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm: decode gemini response: %w", err), includedThinking
	}
	if parsed.PromptFeedback.BlockReason != "" {
		return "", &geminiFailure{
			err:         ErrBlocked,
			caseName:    "prompt_blocked",
			blockReason: parsed.PromptFeedback.BlockReason,
		}, includedThinking
	}
	if len(parsed.Candidates) == 0 {
		return "", &geminiFailure{err: ErrEmpty, caseName: "no_candidates"}, includedThinking
	}

	var finishReasons []string
	for _, candidate := range parsed.Candidates {
		if candidate.FinishReason != "" {
			finishReasons = append(finishReasons, candidate.FinishReason)
		}
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return strings.TrimSpace(part.Text), nil, includedThinking
			}
		}
	}

	for _, reason := range finishReasons {
		if reason == "SAFETY" {
			return "", &geminiFailure{err: ErrBlocked, caseName: "finish_safety", finishReason: reason}, includedThinking
		}
	}
	for _, reason := range finishReasons {
		if reason == "MAX_TOKENS" {
			return "", &geminiFailure{err: ErrEmpty, caseName: "max_tokens_empty", finishReason: reason}, includedThinking
		}
	}

	finishReason := ""
	if len(finishReasons) > 0 {
		finishReason = finishReasons[0]
	}
	return "", &geminiFailure{err: ErrEmpty, caseName: "empty_text", finishReason: finishReason}, includedThinking
}
