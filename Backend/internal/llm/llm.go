// Package llm abstracts the server-side language model that writes journal
// reflections. Claude is the default provider and Gemini is available through
// the LLM_PROVIDER environment variable.
//
// Only the journal text a reflection needs is sent; credentials, database
// details and user identity never leave the backend, and journal content is
// never logged. A failed call logs only the provider, model, latency, a coarse
// error category, the HTTP status when there is one, and our own error message
// capped at 200 characters.
package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
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
	// Gemini 2.5 Flash is retired for new API keys, so default to a current
	// Flash model that is still served.
	DefaultGeminiModel = "gemini-3.5-flash"

	// GeminiBaseURL prefixes the Gemini generateContent endpoint. The model name
	// and the ":generateContent" method are appended to it.
	GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

	// MaxTokens bounds the reflection length so requests stay fast and cheap.
	MaxTokens = 220

	// GeminiMaxTokens leaves room for Gemini's thinking tokens, which otherwise
	// consume the whole output budget and return an empty candidate.
	GeminiMaxTokens = 1024

	// RequestTimeout bounds one reflection call; a shorter caller deadline wins.
	RequestTimeout = 45 * time.Second

	// ProviderClaude and ProviderGemini are the accepted LLM_PROVIDER values.
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
)

// ErrNotConfigured reports that a provider has no API key, so reflections are
// unavailable. It is safe to surface; it reveals no key material.
var ErrNotConfigured = errors.New("llm: provider is not configured")

// ErrBlocked reports that a provider refused to answer for a safety reason.
// Callers may substitute a safe fallback.
var ErrBlocked = errors.New("llm: response was blocked")

// ErrEmpty reports that a provider answered successfully but produced no text
// (for example its output budget was exhausted or it returned no candidates).
// It is distinct from ErrBlocked so failures can be diagnosed from the log.
var ErrEmpty = errors.New("llm: response contained no text")

// maxLoggedError caps the provider error text in a log line, so an upstream
// message can never grow one log entry into a payload.
const maxLoggedError = 200

// statusError is a non-200 answer from a provider. Its message carries the
// status code only: the response body is drained and dropped, never echoed.
type statusError struct {
	provider string
	status   int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("llm: %s returned status %d", e.provider, e.status)
}

// statusOf reports the HTTP status behind an error, or 0 when there was none.
func statusOf(err error) int {
	var statusErr *statusError
	if errors.As(err, &statusErr) {
		return statusErr.status
	}
	return 0
}

// statusMeaning translates the statuses worth acting on into plain words, so an
// operator reading a log line knows what to do without decoding an HTTP code.
// It returns "" for every status that needs no explanation.
func statusMeaning(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "401 unauthorized: the provider rejected the API key"
	case http.StatusForbidden:
		return "403 forbidden: the provider denied access to this account or model"
	case http.StatusNotFound:
		return "404 not found: the provider has no such endpoint or model"
	case http.StatusTooManyRequests:
		return "429 too many requests: the provider rate limit was hit"
	default:
		return ""
	}
}

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
		if model == "" {
			model = DefaultClaudeModel
		}
		key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		if key == "" {
			logStartup(provider, model, false)
			return nil, nil
		}
		logStartup(provider, model, true)
		return NewClaude(key, model), nil
	case ProviderGemini:
		if model == "" {
			model = DefaultGeminiModel
		}
		key := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if key == "" {
			logStartup(provider, model, false)
			return nil, nil
		}
		if isProduction() && os.Getenv("GEMINI_PAID") != "true" {
			return nil, fmt.Errorf("llm: gemini is enabled in production but GEMINI_PAID is not \"true\"")
		}
		logStartup(provider, model, true)
		return NewGemini(key, model), nil
	default:
		return nil, fmt.Errorf("llm: unknown LLM_PROVIDER %q", provider)
	}
}

// logStartup records the provider selection once, at startup. The key is
// reported as a boolean only; its value never reaches a log line.
func logStartup(provider, model string, keyPresent bool) {
	attrs := []any{
		slog.String("provider", provider),
		slog.String("model", model),
		slog.Bool("key_present", keyPresent),
	}
	if !keyPresent {
		slog.Warn("llm reflection provider is not active", append(attrs,
			slog.String("reason", "no API key set for this provider, reflections are disabled"))...)
		return
	}
	slog.Info("llm reflection provider is active", attrs...)
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

// logCall records a failed provider call. It is meant to be deferred with the
// named error return.
//
// It never logs the request or response body, the API key, the journal entry or
// any user identity: the only text it emits is our own error string, which
// carries a status code or a transport failure, capped at maxLoggedError
// characters.
func logCall(provider, model string, start time.Time, err error) {
	if err == nil {
		return
	}
	attrs := []any{
		slog.String("provider", provider),
		slog.String("model", model),
		slog.Duration("latency", time.Since(start)),
		slog.String("category", errorType(err)),
		slog.String("error", truncateForLog(err.Error())),
	}
	if status := statusOf(err); status != 0 {
		attrs = append(attrs, slog.Int("http_status", status))
		if plain := statusMeaning(status); plain != "" {
			attrs = append(attrs, slog.String("http_status_plain", plain))
		}
	}
	var failure *geminiFailure
	if errors.As(err, &failure) {
		if failure.caseName != "" {
			attrs = append(attrs, slog.String("case", failure.caseName))
		}
		if failure.finishReason != "" {
			attrs = append(attrs, slog.String("finish_reason", failure.finishReason))
		}
		if failure.blockReason != "" {
			attrs = append(attrs, slog.String("block_reason", failure.blockReason))
		}
	}
	slog.Warn("llm request failed", attrs...)
}

// truncateForLog caps a message at maxLoggedError characters so a log line
// stays a line.
func truncateForLog(s string) string {
	if utf8.RuneCountInString(s) <= maxLoggedError {
		return s
	}
	return string([]rune(s)[:maxLoggedError]) + "..."
}

// errorType classifies an error into a coarse label, so logs stay useful
// without ever carrying content or raw upstream bodies.
func errorType(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "config missing"
	case statusOf(err) != 0:
		return "http status"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrBlocked):
		return "blocked"
	case errors.Is(err, ErrEmpty):
		return "empty response"
	default:
		return "upstream"
	}
}

// Both providers must satisfy the seam the journal service depends on.
var (
	_ Reflector = (*Claude)(nil)
	_ Reflector = (*Gemini)(nil)
)
