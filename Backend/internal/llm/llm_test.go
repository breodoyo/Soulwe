package llm

import (
	"strings"
	"testing"
)

// envVars are all the variables NewFromEnv consults. Clearing them first keeps
// the table cases independent of the developer's shell.
var envVars = []string{
	"LLM_PROVIDER", "LLM_MODEL",
	"ANTHROPIC_API_KEY", "GEMINI_API_KEY",
	"GEMINI_PAID", "APP_ENV", "ENV",
}

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range envVars {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestNewFromEnv(t *testing.T) {
	const key = "test-key"

	tests := []struct {
		name      string
		env       map[string]string
		wantErr   bool
		wantNil   bool
		wantType  string
		wantModel string
	}{
		{
			name:      "defaults to claude when LLM_PROVIDER is unset",
			env:       map[string]string{"ANTHROPIC_API_KEY": key},
			wantType:  "claude",
			wantModel: DefaultClaudeModel,
		},
		{
			name:    "claude with no key disables reflections",
			env:     map[string]string{},
			wantNil: true,
		},
		{
			name:      "claude honours an LLM_MODEL override",
			env:       map[string]string{"ANTHROPIC_API_KEY": key, "LLM_MODEL": "claude-3-opus-latest"},
			wantType:  "claude",
			wantModel: "claude-3-opus-latest",
		},
		{
			name:      "gemini in development is selected",
			env:       map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": key},
			wantType:  "gemini",
			wantModel: DefaultGeminiModel,
		},
		{
			name:    "gemini with no key disables reflections",
			env:     map[string]string{"LLM_PROVIDER": "gemini"},
			wantNil: true,
		},
		{
			name:      "provider name is case-insensitive",
			env:       map[string]string{"LLM_PROVIDER": "GEMINI", "GEMINI_API_KEY": key},
			wantType:  "gemini",
			wantModel: DefaultGeminiModel,
		},
		{
			name:      "gemini honours an LLM_MODEL override",
			env:       map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": key, "LLM_MODEL": "gemini-1.5-pro"},
			wantType:  "gemini",
			wantModel: "gemini-1.5-pro",
		},
		{
			name:    "gemini in production without GEMINI_PAID is refused",
			env:     map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": key, "APP_ENV": "production"},
			wantErr: true,
		},
		{
			name:      "gemini in production with GEMINI_PAID is allowed",
			env:       map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": key, "APP_ENV": "production", "GEMINI_PAID": "true"},
			wantType:  "gemini",
			wantModel: DefaultGeminiModel,
		},
		{
			name:    "the production guard also reads ENV",
			env:     map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": key, "ENV": "production"},
			wantErr: true,
		},
		{
			name:    "an unknown provider is an error",
			env:     map[string]string{"LLM_PROVIDER": "llama"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)

			got, err := NewFromEnv()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected a nil reflector, got %T", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected a reflector, got nil")
			}

			switch tc.wantType {
			case "claude":
				c, ok := got.(*Claude)
				if !ok {
					t.Fatalf("expected *Claude, got %T", got)
				}
				if c.model != tc.wantModel {
					t.Errorf("expected model %q, got %q", tc.wantModel, c.model)
				}
			case "gemini":
				g, ok := got.(*Gemini)
				if !ok {
					t.Fatalf("expected *Gemini, got %T", got)
				}
				if g.model != tc.wantModel {
					t.Errorf("expected model %q, got %q", tc.wantModel, g.model)
				}
			}
		})
	}
}

func TestReflectionPrompt(t *testing.T) {
	t.Run("includes the entry and tags", func(t *testing.T) {
		got := reflectionPrompt("I felt calm today.", []string{"Calm", "Grateful"})
		if !strings.Contains(got, "I felt calm today.") {
			t.Errorf("prompt missing the entry: %q", got)
		}
		if !strings.Contains(got, "Calm, Grateful") {
			t.Errorf("prompt missing the tags: %q", got)
		}
	})

	t.Run("omits the tag line when there are none", func(t *testing.T) {
		got := reflectionPrompt("I felt calm today.", nil)
		if strings.Contains(got, "The user tagged it") {
			t.Errorf("prompt should not mention tags: %q", got)
		}
		if !strings.Contains(got, "I felt calm today.") {
			t.Errorf("prompt missing the entry: %q", got)
		}
	})
}

func TestErrorType(t *testing.T) {
	cases := map[string]error{
		"blocked":        ErrBlocked,
		"not_configured": ErrNotConfigured,
	}
	for want, err := range cases {
		if got := errorType(err); got != want {
			t.Errorf("errorType(%v) = %q, want %q", err, got, want)
		}
	}
	if got := errorType(errString("boom")); got != "upstream" {
		t.Errorf("expected upstream, got %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
