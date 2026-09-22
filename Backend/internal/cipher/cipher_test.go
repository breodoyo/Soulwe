package cipher

import (
	"bytes"
	"errors"
	"testing"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, KeySize)
}

func TestNewAESGCMValidatesKeyLength(t *testing.T) {
	for name, key := range map[string][]byte{
		"too short": bytes.Repeat([]byte{1}, KeySize-1),
		"too long":  bytes.Repeat([]byte{1}, KeySize+1),
		"empty":     {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewAESGCM(key); !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("expected ErrInvalidKey, got %v", err)
			}
		})
	}

	t.Run("exactly 32 bytes is accepted", func(t *testing.T) {
		if _, err := NewAESGCM(testKey()); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewAESGCM(testKey())
	if err != nil {
		t.Fatalf("NewAESGCM returned error: %v", err)
	}

	owner := []byte("user-1")
	enc, nonce, err := c.Encrypt([]byte("Today was hard. Mama called again."), owner)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("expected non-empty ciphertext")
	}
	if len(nonce) != NonceSize {
		t.Errorf("expected nonce of length %d, got %d", NonceSize, len(nonce))
	}
	if bytes.Contains(enc, []byte("Mama")) {
		t.Error("ciphertext must not contain plaintext")
	}

	plain, err := c.Decrypt(enc, nonce, owner)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if string(plain) != "Today was hard. Mama called again." {
		t.Errorf("round trip mismatch: %q", plain)
	}
}

func TestEncryptMintsAFreshNoncePerCall(t *testing.T) {
	c, _ := NewAESGCM(testKey())
	owner := []byte("user-1")

	enc1, nonce1, _ := c.Encrypt([]byte("same plaintext"), owner)
	enc2, nonce2, _ := c.Encrypt([]byte("same plaintext"), owner)

	if bytes.Equal(nonce1, nonce2) {
		t.Error("two encryptions reused the same nonce")
	}
	if bytes.Equal(enc1, enc2) {
		t.Error("two encryptions produced identical ciphertext")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	c1, _ := NewAESGCM(testKey())
	c2, _ := NewAESGCM(bytes.Repeat([]byte{0x25}, KeySize))

	enc, nonce, _ := c1.Encrypt([]byte("secret journal"), []byte("user-1"))
	if _, err := c2.Decrypt(enc, nonce, []byte("user-1")); err == nil {
		t.Fatal("expected decryption with a different key to fail")
	}
}

func TestDecryptRejectsWrongOwner(t *testing.T) {
	c, _ := NewAESGCM(testKey())

	enc, nonce, err := c.Encrypt([]byte("secret journal"), []byte("user-1"))
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if _, err := c.Decrypt(enc, nonce, []byte("user-2")); err == nil {
		t.Fatal("expected decryption under a different owner (aad) to fail")
	}
}

func TestDecryptRejectsTamperedCiphertextAndNonce(t *testing.T) {
	c, _ := NewAESGCM(testKey())
	owner := []byte("user-1")

	enc, nonce, _ := c.Encrypt([]byte("secret journal"), owner)

	t.Run("flipped ciphertext byte", func(t *testing.T) {
		tampered := append([]byte(nil), enc...)
		tampered[0] ^= 0x01
		if _, err := c.Decrypt(tampered, nonce, owner); err == nil {
			t.Fatal("expected tampered ciphertext to fail authentication")
		}
	})

	t.Run("flipped nonce byte", func(t *testing.T) {
		badNonce := append([]byte(nil), nonce...)
		badNonce[0] ^= 0x01
		if _, err := c.Decrypt(enc, badNonce, owner); err == nil {
			t.Fatal("expected a wrong nonce to fail authentication")
		}
	})

	t.Run("wrong nonce length", func(t *testing.T) {
		if _, err := c.Decrypt(enc, []byte{1, 2, 3}, owner); err == nil {
			t.Fatal("expected an invalid nonce length to fail")
		}
	})
}
