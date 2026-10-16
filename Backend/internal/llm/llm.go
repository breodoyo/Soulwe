// Package llm abstracts the server-side language model that writes journal
// reflections. Claude is the default provider and Gemini is available through
// the LLM_PROVIDER environment variable.
//
// Only the journal text a reflection needs is sent; credentials, database
// details and user identity never leave the backend, and journal content is
// never logged. Providers log only the provider name, latency and a coarse
// error type.
package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// ClaudeMessagesURL is the Anthropic Messages API endpoint.
	ClaudeMessagesURL = "https://api.anthropic.com/v1/messages"

	// ClaudeAPIVersion header required by the Messages API.
	ClaudeAPIVersion = "2023-06-01"

	// DefaultClaudeModel is the Claude model used for reflections. It is a code
	// constant (not an environment value) so the key and model stay decoupled.
	DefaultClaudeModel = "claude-3-5-haiku-latest"

	// DefaultGeminiModel is the Gemini model used when LLM_MODEL is unset.
	DefaultGeminiModel = "gemini-2.5-flash"

	// GeminiBaseURL prefixes the Gemini generateContent endpoint. The model name
	// and the ":generateContent" method are appended to it.
	GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

	// MaxTokens bounds the reflection length so requests stay fast and cheap.
	MaxTokens = 220

	// RequestTimeout bounds one reflection call; a shorter caller deadline wins.
	RequestTimeout = 45 * time.Second

	// ProviderClaude and ProviderGemini are the accepted LLM_PROVIDER values.
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
)

// ErrNotConfigured reports that a provider has no API key, so reflections are
// unavailable. It is safe to surface; it reveals no key material.
var ErrNotConfigured = errors.New("llm: provider is not configured")

// ErrBlocked reports that a provider refused to answer (for example a safety
// block or an empty completion). Callers may substitute a safe fallback.
var ErrBlocked = errors.New("llm: response was blocked")

// SystemPrompt steers the model toward a warm, brief, non-clinical reflection in
// the language of the journal. It contains no user data.
const SystemPrompt = `You are a warm, thoughtful companion for a mental-wellbeing app called Soulwe. ` +
	`Write a brief reflection (1-3 sentences) on the journal entry the user wrote. ` +
	`Acknowledge the feeling, offer gentle perspective, and avoid clinical or prescriptive language. ` +
	`Mirror the language the user wrote in, and do not invent facts from outside the entry.`

// Reflector is the seam every provider implements. It isolates the AI client so
// the journal service can be tested without a key or a network.
type Reflector interface {
	// Reflect returns a trimmed reflection for the entry, or an error. The error
	// never contains journal content.
	Reflect(ctx context.Context, entry string, moodTags []string) (string, error)
}

// NewFromEnv selects a provider from the environment. LLM_PROVIDER defaults to
// "claude"; LLM_MODEL optionally overrides the model.
//
// It returns (nil, nil) when the selected provider has no API key, which
// disables reflections without failing startup. It returns an error for an
// unknown provider or for a production Gemini configuration that has not opted
// in to a paid account (GEMINI_PAID=true), because the Gemini free tier may use
// submitted content for training.
func NewFromEnv() (Reflector, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" {
		provider = ProviderClaude
	}
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))

	switch provider {
	case ProviderClaude:
		key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		if key == "" {
			return nil, nil
		}
		if model == "" {
			model = DefaultClaudeModel
		}
		return NewClaude(key, model), nil
	case ProviderGemini:
		key := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if key == "" {
			return nil, nil
		}
		if isProduction() && os.Getenv("GEMINI_PAID") != "true" {
			return nil, fmt.Errorf("llm: gemini is enabled in production but GEMINI_PAID is not \"true\"")
		}
		if model == "" {
			model = DefaultGeminiModel
		}
		return NewGemini(key, model), nil
	default:
		return nil, fmt.Errorf("llm: unknown LLM_PROVIDER %q", provider)
	}
}

// isProduction reads APP_ENV, falling back to ENV, so the guard works whichever
// name the deployment uses.
func isProduction() bool {
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = strings.TrimSpace(os.Getenv("ENV"))
	}
	return strings.EqualFold(env, "production")
}

// reflectionPrompt builds the user message shared by every provider.
func reflectionPrompt(entry string, moodTags []string) string {
	var b strings.Builder
	b.WriteString("Reflect on the following journal entry.\n\n")
	if len(moodTags) > 0 {
		b.WriteString("The user tagged it: ")
		b.WriteString(strings.Join(moodTags, ", "))
		b.WriteString(".\n\n")
	}
	b.WriteString("Journal entry:\n")
	b.WriteString(entry)
	return b.String()
}

// logCall records a failed provider call without ever logging the request or
// response body. It is meant to be deferred with the named error return.
func logCall(provider string, start time.Time, err error) {
	if err == nil {
		return
	}
	slog.Warn("llm request failed",
		slog.String("provider", provider),
		slog.Duration("latency", time.Since(start)),
		slog.String("error_type", errorType(err)),
	)
}

// errorType classifies an error into a coarse label, so logs stay useful without
// ever carrying content or raw upstream messages.
func errorType(err error) string {
	switch {
	case errors.Is(err, ErrBlocked):
		return "blocked"
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "upstream"
	}
}

// Both providers must satisfy the seam the journal service depends on.
var (
	_ Reflector = (*Claude)(nil)
	_ Reflector = (*Gemini)(nil)
)
