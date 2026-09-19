package anon

import (
	"strings"
	"testing"
)

func TestGenerateRawToken(t *testing.T) {
	tok, err := GenerateRawToken()
	if err != nil {
		t.Fatalf("GenerateRawToken returned error: %v", err)
	}
	if tok == "" {
		t.Fatal("expected a non-empty token")
	}
	if len(tok) != 43 {
		t.Errorf("expected 43 base64url characters (32 bytes), got %d: %q", len(tok), tok)
	}
	if strings.ContainsAny(tok, "+/=") {
		t.Errorf("token must be URL-safe base64 without padding, got %q", tok)
	}
}

func TestGenerateRawTokenUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, err := GenerateRawToken()
		if err != nil {
			t.Fatalf("GenerateRawToken returned error: %v", err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = true
	}
}

func TestHashToken(t *testing.T) {
	raw := "some-raw-token"
	hash := HashToken(raw)
	if len(hash) != 64 {
		t.Errorf("expected a 64-char SHA-256 hex digest, got %d: %q", len(hash), hash)
	}
	if hash == raw {
		t.Error("hash must never equal the raw token")
	}
	if HashToken(raw) != hash {
		t.Error("HashToken must be deterministic")
	}
	if HashToken("different-token") == hash {
		t.Error("different tokens must hash differently")
	}
	for _, c := range hash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("hash contains a non-hex character: %q", hash)
			break
		}
	}
}

func TestValidDeviceUUID(t *testing.T) {
	valid := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"550E8400-E29B-41D4-A716-446655440000",
	}
	for _, uuid := range valid {
		if !ValidDeviceUUID(uuid) {
			t.Errorf("expected %q to be a valid device UUID", uuid)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"550e8400e29b41d4a716446655440000",      // no dashes
		"550e8400-e29b-41d4-a716",               // too short
		"550e8400-e29b-41d4-a716-44665544000",   // truncated group
		"550e8400-e29b-41d4-xxxx-446655440000",  // non-hex
		"550e8400-e29b-41d4-a716-4466554400000", // extra
	}
	for _, uuid := range invalid {
		if ValidDeviceUUID(uuid) {
			t.Errorf("expected %q to be an invalid device UUID", uuid)
		}
	}
}
