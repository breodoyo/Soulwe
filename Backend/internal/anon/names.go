package anon

import "math/rand/v2"

// curatedWords is the East African nature-word pool that feeds anonymous
// display names (see Docs/DATABASE.md). Names are unique via the UNIQUE
// constraint on anon_name; the service retries with a fresh name (and fresh
// token) if the insert collides.
var curatedWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

// randomAnonName returns a display name like "Anon Baobab".
func randomAnonName() string {
	return "Anon " + curatedWords[rand.IntN(len(curatedWords))]
}
