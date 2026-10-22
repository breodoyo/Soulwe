package circles

import (
	"strings"
	"testing"
)

const testNamingKey = "test-naming-key"

// assertFriendlyName checks the shape the API promises instead of pinning one
// hard-coded label: at most three fields, the first two curated words and an
// optional number of at most two digits, with nothing of the identity in it.
func assertFriendlyName(t *testing.T, name, identityID string) {
	t.Helper()

	fields := strings.Fields(name)
	if len(fields) < 2 || len(fields) > 3 {
		t.Errorf("expected two words with an optional number, got %q", name)
		return
	}
	for _, word := range fields[:2] {
		if !containsWord(displayWords, word) {
			t.Errorf("expected a curated word in %q, got %q", name, word)
		}
	}
	if len(fields) == 3 {
		if n := fields[2]; len(n) > 2 || !isDigits(n) {
			t.Errorf("expected a number of at most two digits in %q, got %q", name, n)
		}
	}
	if identityID != "" && strings.Contains(name, identityID) {
		t.Errorf("label %q exposes the identity it names", name)
	}
}

func containsWord(words []string, word string) bool {
	for _, w := range words {
		if w == word {
			return true
		}
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// TestNameDeriver_PerCircle pins the privacy contract: a name depends on both
// the identity and the circle, is stable within one circle, changes across
// circles, and gives away no part of the identity.
func TestNameDeriver_PerCircle(t *testing.T) {
	deriver := newNameDeriver(testNamingKey)

	const (
		identity = "44444444-4444-4444-4444-444444444444"
		other    = "55555555-5555-5555-5555-555555555555"
	)

	inA := deriver.derive(identity, circleA)
	inB := deriver.derive(identity, circleB)

	t.Run("the same identity keeps one name inside a circle", func(t *testing.T) {
		if again := deriver.derive(identity, circleA); again != inA {
			t.Errorf("the name must be stable within a circle: got %q then %q", inA, again)
		}
	})

	t.Run("the same identity gets a different name in each circle", func(t *testing.T) {
		if inA == inB {
			t.Errorf("the name must differ per circle, got %q in both", inA)
		}
	})

	t.Run("different identities are told apart inside one circle", func(t *testing.T) {
		if peer := deriver.derive(other, circleA); peer == inA {
			t.Errorf("two members would share the label %q in the same circle", inA)
		}
	})

	t.Run("the name contains no part of the identity", func(t *testing.T) {
		for _, name := range []string{inA, inB} {
			assertFriendlyName(t, name, identity)
			if strings.Contains(name, identity[:8]) || strings.Contains(name, "4444") {
				t.Errorf("label %q leaks identity material", name)
			}
		}
	})

	t.Run("a different key produces a different label", func(t *testing.T) {
		if other := newNameDeriver("rotated-key").derive(identity, circleA); other == inA {
			t.Errorf("without the key the label %q must not be reproducible", inA)
		}
	})
}
