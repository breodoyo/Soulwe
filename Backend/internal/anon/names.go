package anon

import "math/rand/v2"

var curatedWords = []string{
	"Baobab", "Acacia", "Savanna", "Kilimanjaro", "Serengeti",
	"Willow", "Marula", "Okavango", "Zambezi", "Simba",
}

func randomAnonName() string {
	return "Anon " + curatedWords[rand.IntN(len(curatedWords))]
}
