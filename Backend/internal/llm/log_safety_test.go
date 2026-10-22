package llm

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// captureLogs points the default logger at a buffer for the duration of fn and
// returns everything fn logged.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)
	fn()
	return buf.String()
}

// TestReflectFailureLogHasNoEntryTextAndNoKey is the guard on what a failure
// line may carry: provider, model, category, status and our own error message
// are allowed; the journal entry and the API key never are.
func TestReflectFailureLogHasNoEntryTextAndNoKey(t *testing.T) {
	const (
		apiKey = "sk-ant-api03-do-not-log-this-key"
		entry  = "I felt hollow after my father died and could not sleep"
		model  = "claude-3-5-haiku-latest"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	client := newClaude(apiKey, model, srv.URL)
	logs := captureLogs(t, func() {
		if _, err := client.Reflect(context.Background(), entry, []string{"Grief"}); err == nil {
			t.Fatal("expected the 401 to surface as an error")
		}
	})

	for _, forbidden := range []string{entry, "my father", "could not sleep", apiKey, "sk-ant-api03"} {
		if strings.Contains(logs, forbidden) {
			t.Errorf("log must never contain %q: %s", forbidden, logs)
		}
	}
	for _, want := range []string{
		"provider=" + ProviderClaude,
		"model=" + model,
		`category="http status"`,
		"http_status=401",
		"401 unauthorized: the provider rejected the API key",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q: %s", want, logs)
		}
	}
}

// TestLogCallTruncatesTheProviderErrorMessage pins the 200-character cap on the
// upstream message.
func TestLogCallTruncatesTheProviderErrorMessage(t *testing.T) {
	long := strings.Repeat("z", maxLoggedError+90)
	logs := captureLogs(t, func() {
		logCall(ProviderClaude, DefaultClaudeModel, time.Now(), errString(long))
	})
	if strings.Contains(logs, long) {
		t.Errorf("the provider message was logged untruncated: %s", logs)
	}
	if !strings.Contains(logs, strings.Repeat("z", maxLoggedError)) {
		t.Errorf("expected the first %d characters of the message: %s", maxLoggedError, logs)
	}
}

func TestNewFromEnvLogsTheSelectionWithoutTheKey(t *testing.T) {
	const key = "startup-key-that-must-never-be-logged"
	setEnv(t, map[string]string{"ANTHROPIC_API_KEY": key})

	logs := captureLogs(t, func() { _, _ = NewFromEnv() })

	if strings.Contains(logs, key) {
		t.Errorf("the startup log leaked the API key: %s", logs)
	}
	for _, want := range []string{
		"provider=" + ProviderClaude,
		"model=" + DefaultClaudeModel,
		"key_present=true",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("startup log missing %q: %s", want, logs)
		}
	}
}

func TestNewFromEnvLogsWhenNoKeyIsPresent(t *testing.T) {
	setEnv(t, map[string]string{})

	logs := captureLogs(t, func() { _, _ = NewFromEnv() })

	for _, want := range []string{
		"provider=" + ProviderClaude,
		"model=" + DefaultClaudeModel,
		"key_present=false",
		"reflections are disabled",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("startup log missing %q: %s", want, logs)
		}
	}
}
