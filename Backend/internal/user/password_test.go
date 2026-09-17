package user

import "testing"

func TestHashPasswordNeverStoresPlaintext(t *testing.T) {
	const plaintext = "a-correct-horse-1"
	hash, err := hashPassword(plaintext)
	if err != nil {
		t.Fatalf("hashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}
	if hash == plaintext {
		t.Fatal("hash must never equal the plaintext password")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := hashPassword("a-correct-horse-1")
	if err != nil {
		t.Fatalf("hashPassword returned error: %v", err)
	}

	if !checkPassword(hash, "a-correct-horse-1") {
		t.Error("expected correct password to verify")
	}
	if checkPassword(hash, "a-wrong-horse-1") {
		t.Error("expected wrong password to be rejected")
	}
	if hash == "a-correct-horse-1" {
		t.Error("hash must differ from the plaintext")
	}
}

func TestHashProducesUniqueSaltPerRun(t *testing.T) {
	const plaintext = "salt-check-password-1"
	h1, err := hashPassword(plaintext)
	if err != nil {
		t.Fatalf("hashPassword returned error: %v", err)
	}
	h2, err := hashPassword(plaintext)
	if err != nil {
		t.Fatalf("hashPassword returned error: %v", err)
	}
	if h1 == h2 {
		t.Error("expected two hashes of the same password to differ (random salt)")
	}
}
