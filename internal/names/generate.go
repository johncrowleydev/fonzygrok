package names

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Generate returns a random "adjective-noun" name using crypto/rand.
func Generate() string {
	adj := adjectives[cryptoRandInt(len(adjectives))]
	noun := nouns[cryptoRandInt(len(nouns))]
	return adj + "-" + noun
}

// GenerateUnique returns a name that does not already exist according
// to the provided exists function. It retries up to maxAttempts times.
// If all attempts collide, it falls back to "adjective-noun-XXXX" with
// 4 random digits.
func GenerateUnique(exists func(string) bool) string {
	const maxAttempts = 100

	for i := 0; i < maxAttempts; i++ {
		name := Generate()
		if !exists(name) {
			return name
		}
	}

	// Fallback: append 4 random digits.
	for {
		name := fmt.Sprintf("%s-%04d", Generate(), cryptoRandInt(10000))
		if !exists(name) {
			return name
		}
	}
}

// cryptoRandInt returns a cryptographically random int in [0, max).
func cryptoRandInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(fmt.Sprintf("names: crypto/rand failed: %v", err))
	}
	return int(n.Int64())
}
