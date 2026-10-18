package anon

import (
	"crypto/rand"
	"encoding/hex"
)

var curatedWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

// randomAnonName returns a display name that is unique with overwhelming
// probability: a curated word plus a crypto/rand hex suffix. A fixed word pool
// alone exhausted the anon_name UNIQUE constraint once every word was taken.
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
