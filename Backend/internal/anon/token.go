package anon

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
)

// tokenByteLength is the entropy backing each token (32 bytes ≈ 256 bits).
const tokenByteLength = 32

// GenerateRawToken returns a fresh URL-safe base64 token from 32 random bytes.
// The client sees the raw token once; only its SHA-256 hash is ever stored.
func GenerateRawToken() (string, error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the lowercase hex SHA-256 digest stored in token_hash.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// deviceUUIDPattern accepts the 8-4-4-4-12 layout; version/variant bits are
// deliberately not enforced so any client-generated UUID stays acceptable.
var deviceUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidDeviceUUID reports whether s is a well-formed UUID. The device UUID is
// idempotency metadata, NOT an auth secret.
func ValidDeviceUUID(s string) bool {
	return deviceUUIDPattern.MatchString(s)
}
