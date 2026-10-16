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

// claudeResponse mirrors the slice of the Messages API response we read.
type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Claude is the Anthropic implementation of Reflector. It holds only the API
// key and model; it never stores or logs content.
type Claude struct {
	apiKey string
	model  string
	url    string
	http   *http.Client
}

// NewClaude returns a Claude client for the given API key and model. An empty
// key still produces a usable client whose calls fail with ErrNotConfigured.
func NewClaude(apiKey, model string) *Claude {
	return newClaude(apiKey, model, ClaudeMessagesURL)
}

func newClaude(apiKey, model, url string) *Claude {
	return &Claude{
		apiKey: apiKey,
		model:  model,
		url:    url,
		http:   &http.Client{},
	}
}

// Reflect returns the trimmed reflection, or ErrNotConfigured when no API key is
// set. Journal content never appears in the returned error.
func (c *Claude) Reflect(ctx context.Context, entry string, moodTags []string) (out string, err error) {
	start := time.Now()
	defer func() { logCall(ProviderClaude, start, err) }()

	if strings.TrimSpace(c.apiKey) == "" {
		return "", ErrNotConfigured
	}

	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": MaxTokens,
		"system":     SystemPrompt,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": reflectionPrompt(entry, moodTags),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("llm: build claude request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: create claude request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", ClaudeAPIVersion)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: claude request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain a little so the connection can be reused, but never echo the body.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("llm: claude returned status %d", resp.StatusCode)
	}

	var parsed claudeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm: decode claude response: %w", err)
	}
	for _, block := range parsed.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		return strings.TrimSpace(block.Text), nil
	}
	return "", fmt.Errorf("llm: claude returned no text: %w", ErrBlocked)
}
