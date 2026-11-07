package anon

import (
	"crypto/rand"
	"encoding/hex"
)

var curatedWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

// randomAnonName returns a curated word plus a crypto/rand hex suffix, keeping
// the name unique with overwhelming probability.
func randomAnonName() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand.Read only fails when the OS CSPRNG is unavailable, which
		// would break token minting too; fall back rather than panic here.
		return "Anon " + curatedWords[0] + "-0000000000"
	}
	word := curatedWords[int(buf[0])%len(curatedWords)]
	return "Anon " + word + "-" + hex.EncodeToString(buf[1:])
}
