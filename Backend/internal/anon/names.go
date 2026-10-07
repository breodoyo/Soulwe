package anon

import "math/rand/v2"

// curatedWords is the East African nature-word pool behind anonymous display
// names. anon_name is UNIQUE, so the service retries on a collision.
var curatedWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

func randomAnonName() string {
	return "Anon " + curatedWords[rand.IntN(len(curatedWords))]
}
