// Package auth provides token generation and hashing for fonzygrok
// authentication. Token format conforms to CON-002 §4.3.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	// TokenIDPrefix is the prefix for token IDs.
	TokenIDPrefix = "tok_"
	// TokenIDRandomLen is the length of the random hex portion of a token ID.
	TokenIDRandomLen = 12

	// TokenPrefix is the prefix for raw API tokens.
	TokenPrefix = "fgk_"
	// TokenRandomLen is the length of the random alphanumeric portion of a token.
	TokenRandomLen = 32
)

// tokenAlphabet is the set of characters used for token generation.
// Lowercase alphanumeric only per CON-002 §4.3.
const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// GenerateToken creates a new token ID and raw token string.
// The token ID has the format "tok_" + 12 hex characters.
// The raw token has the format "fgk_" + 32 lowercase alphanumeric characters.
// Returns (id, rawToken).
func GenerateToken() (string, string) {
	id := TokenIDPrefix + randomHex(TokenIDRandomLen)
	raw := TokenPrefix + randomAlphanumeric(TokenRandomLen)
	return id, raw
}

// HashToken returns the SHA-256 hex digest of a raw token string.
// This is the value stored in the database — raw tokens are never persisted.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// randomHex generates n random hex characters using crypto/rand.
func randomHex(n int) string {
	// Each byte produces 2 hex characters, so we need n/2 bytes (rounded up).
	byteLen := (n + 1) / 2
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)[:n]
}

// randomAlphanumeric generates n random characters from tokenAlphabet
// using crypto/rand for cryptographic security.
func randomAlphanumeric(n int) string {
	b := make([]byte, n)
	randomBytes := make([]byte, n)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand.Read failed: %v", err))
	}
	for i := 0; i < n; i++ {
		b[i] = tokenAlphabet[int(randomBytes[i])%len(tokenAlphabet)]
	}
	return string(b)
}
