// Package cipher provides the AES-256-GCM encryption protecting journal content at rest.
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

// NonceSize is the per-entry random GCM nonce length, stored in content_iv.
const NonceSize = 12

// ErrInvalidKey reports a key that is not exactly KeySize bytes.
var ErrInvalidKey = errors.New("invalid encryption key: must be 32 bytes")

// AESGCM wraps AES-256-GCM, minting a fresh random nonce per Encrypt call.
type AESGCM struct {
	gcm cipher.AEAD
}

// NewAESGCM builds an AESGCM from a 32-byte key, rejecting any other length.
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

// Encrypt seals plaintext with a fresh random nonce; aad binds it to its owner.
func (c *AESGCM) Encrypt(plaintext, aad []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("cipher: generate nonce: %w", err)
	}
	sealed := c.gcm.Seal(nil, nonce, plaintext, aad)
	return sealed, nonce, nil
}

// Decrypt opens ciphertext, failing on tamper, wrong nonce or mismatched aad.
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
