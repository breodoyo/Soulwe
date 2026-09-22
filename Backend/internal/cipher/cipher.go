// Package cipher provides the AES-256-GCM encryption used to protect journal
// content at rest. Plaintext never leaves this package unencrypted, and the
// raw key is never exposed to any other layer.
package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize is the required length of the AES-256 key in bytes.
const KeySize = 32

// NonceSize is the length of the per-entry random nonce (the GCM standard
// nonce size). Stored alongside the ciphertext in journal_entries.content_iv.
const NonceSize = 12

// ErrInvalidKey reports a key that is not exactly KeySize bytes.
var ErrInvalidKey = errors.New("invalid encryption key: must be 32 bytes")

// AESGCM wraps an AES-256-GCM cipher. Each call mints a fresh random nonce, so
// encrypting the same plaintext twice never yields the same ciphertext.
type AESGCM struct {
	gcm cipher.AEAD
}

// NewAESGCM builds an AESGCM from a 32-byte key. Anything shorter or longer is
// rejected rather than silently truncated or padded.
func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: create aes block: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher: create gcm: %w", err)
	}
	return &AESGCM{gcm: gcm}, nil
}

// Encrypt seals plaintext into ciphertext with a fresh random nonce. The
// additional authenticated data (aad) binds the ciphertext to its owner (the
// user ID) so a ciphertext can never be decrypted under a different owner.
func (c *AESGCM) Encrypt(plaintext, aad []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("cipher: generate nonce: %w", err)
	}
	sealed := c.gcm.Seal(nil, nonce, plaintext, aad)
	return sealed, nonce, nil
}

// Decrypt opens ciphertext with the given nonce and aad. It returns an error
// for tampered ciphertext, a wrong nonce, or mismatched authenticated data —
// never a partial plaintext.
func (c *AESGCM) Decrypt(ciphertext, nonce, aad []byte) ([]byte, error) {
	if len(nonce) != c.gcm.NonceSize() {
		return nil, errors.New("cipher: invalid nonce length")
	}
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("cipher: decrypt: %w", err)
	}
	return plaintext, nil
}
