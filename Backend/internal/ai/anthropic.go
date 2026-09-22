// Package ai provides the server-side Anthropic Claude client used to generate
// journal reflections. Only the minimal journal text needed for a reflection
// is sent; credentials, database details, and user identity are never
// transmitted, and journal content is never logged.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// MessagesURL is the Anthropic Messages API endpoint.
	MessagesURL = "https://api.anthropic.com/v1/messages"

	// APIVersion header required by the Messages API.
	APIVersion = "2023-06-01"

	// DefaultModel is the Claude model used for reflections. It is a code
	// constant (not an environment value) so the key and model stay decoupled.
	DefaultModel = "claude-3-5-haiku-latest"

	// MaxTokens bounds the reflection length so requests stay fast and cheap.
	MaxTokens = 220

	// RequestTimeout bounds a single reflection call. The client also honours
	// the caller's context deadline when it is shorter.
	RequestTimeout = 30 * time.Second

	contentType = "application/json"
)

// ErrNotConfigured reports that the client has no API key, so reflections are
// unavailable. It is safe to surface; it reveals no key material.
var ErrNotConfigured = errors.New("ai reflection is not configured")

// systemPrompt steers Claude toward a warm, brief, non-clinical reflection in
// the language of the journal. It contains no user data.
const systemPrompt = `You are a warm, thoughtful companion for a mental-wellbeing app called Soulwe. ` +
	`Write a brief reflection (1-3 sentences) on the journal entry the user wrote. ` +
	`Acknowledge the feeling, offer gentle perspective, and avoid clinical or prescriptive language. ` +
	`Mirror the language the user wrote in, and do not invent facts from outside the entry.`

// messagesResponse mirrors the slice of the Messages API response we read.
type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// Client calls the Anthropic Messages API. It holds only the API key and model
// name; it never stores or logs journal content.
type Client struct {
	apiKey string
	model  string
	url    string
	http   *http.Client
}

// NewClient returns a Claude client for the given API key and model. An empty
// key still produces a usable client whose calls fail with ErrNotConfigured.
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		url:    MessagesURL,
		http:   &http.Client{},
	}
}

// NewClientForTests returns a client pointed at a custom endpoint; used by the
// unit tests to stub the Anthropic API.
func NewClientForTests(endpoint, apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		url:    endpoint,
		http:   &http.Client{},
	}
}

// GenerateReflection sends the journal text to Claude and returns the trimmed
// reflection. It returns ErrNotConfigured when no API key is set and a generic
// error on any transport or API failure. Journal content never appears in the
// returned error.
func (c *Client) GenerateReflection(ctx context.Context, content string, moodTags []string) (string, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", ErrNotConfigured
	}

	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": MaxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": reflectionPrompt(content, moodTags),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ai: build request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", APIVersion)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain a small amount so the connection can be reused, but never echo
		// the response body (or anything else) into the returned error.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ai: upstream returned status %d", resp.StatusCode)
	}

	var parsed messagesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("ai: decode response: %w", err)
	}
	for _, block := range parsed.Content {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		return strings.TrimSpace(block.Text), nil
	}
	return "", errors.New("ai: upstream returned no text")
}

// reflectionPrompt wraps the journal content with a short instruction. Only the
// user's own words and self-chosen mood tags are included — never credentials,
// database details, or encryption keys.
func reflectionPrompt(content string, moodTags []string) string {
	var b strings.Builder
	b.WriteString("Reflect on the following journal entry.\n\n")
	if len(moodTags) > 0 {
		b.WriteString("The user tagged it: ")
		b.WriteString(strings.Join(moodTags, ", "))
		b.WriteString(".\n\n")
	}
	b.WriteString("Journal entry:\n")
	b.WriteString(content)
	return b.String()
}
