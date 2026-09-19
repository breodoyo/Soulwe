package anon

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
)

// tokenByteLength is the cryptographically random entropy backing each
// anonymous session token (32 bytes ≈ 256 bits).
const tokenByteLength = 32

// GenerateRawToken returns a fresh URL-safe base64 anonymous token built from
// 32 random bytes. The raw token is shown to the client exactly once; only
// its SHA-256 hash (see HashToken) is ever stored or looked up.
func GenerateRawToken() (string, error) {
	buf := make([]byte, tokenByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the lowercase hex SHA-256 digest of a raw token. This is
// the value stored in anon_identities.token_hash.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// deviceUUIDPattern accepts the canonical 8-4-4-4-12 UUID layout. Validation
// only checks shape; we deliberately do not enforce version/variant bits so
// any client-generated UUID remains acceptable.
var deviceUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidDeviceUUID reports whether s is a well-formed UUID. The device UUID is
// optional metadata recorded for idempotency; it is NOT an auth secret.
func ValidDeviceUUID(s string) bool {
	return deviceUUIDPattern.MatchString(s)
}
