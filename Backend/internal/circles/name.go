package circles

import (
	"crypto/hmac"
	"crypto/sha256"
	"strconv"
)

// namingContext domain-separates the display-name key from any other HMAC
// built on the same secret, so one key can never be reused across purposes.
const namingContext = "soulwe.circle.display-name.v1"

var displayWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

// nameDeriver maps an identity to the label it wears inside one circle.
// The derivation key is server-side only and is never serialized.
type nameDeriver struct {
	key []byte
}

func newNameDeriver(secret string) nameDeriver {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(namingContext))
	return nameDeriver{key: mac.Sum(nil)}
}

// derive returns the friendly label for identityID inside circleID.
//
// It is a pure function of (identity, circle): the same identity keeps one
// name within a circle and gets a different name in every other circle, with
// nothing stored and no path back to the identity without the key. The result
// is two curated words and, most of the time, a number of at most two digits.
func (d nameDeriver) derive(identityID, circleID string) string {
	if identityID == "" {
		return DefaultAuthorName
	}

	mac := hmac.New(sha256.New, d.key)
	mac.Write([]byte(identityID))
	mac.Write([]byte{0})
	mac.Write([]byte(circleID))
	sum := mac.Sum(nil)

	first := displayWords[int(sum[0])%len(displayWords)]
	second := displayWords[int(sum[1])%len(displayWords)]
	if second == first {
		second = displayWords[(int(sum[1])+1)%len(displayWords)]
	}

	name := first + " " + second
	if n := int(sum[2]) % 100; n > 0 {
		name += " " + strconv.Itoa(n)
	}
	return name
}
