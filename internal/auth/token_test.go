package auth

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateTokenFormat(t *testing.T) {
	id, raw := GenerateToken()

	// Token ID: "tok_" + 12 hex chars.
	if !strings.HasPrefix(id, TokenIDPrefix) {
		t.Errorf("token ID missing prefix %q: got %q", TokenIDPrefix, id)
	}
	idSuffix := strings.TrimPrefix(id, TokenIDPrefix)
	if len(idSuffix) != TokenIDRandomLen {
		t.Errorf("token ID suffix length: got %d, want %d", len(idSuffix), TokenIDRandomLen)
	}
	hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)
	if !hexPattern.MatchString(idSuffix) {
		t.Errorf("token ID suffix not hex: got %q", idSuffix)
	}

	// Raw token: "fgk_" + 32 lowercase alphanumeric chars.
	if !strings.HasPrefix(raw, TokenPrefix) {
		t.Errorf("raw token missing prefix %q: got %q", TokenPrefix, raw)
	}
	rawSuffix := strings.TrimPrefix(raw, TokenPrefix)
	if len(rawSuffix) != TokenRandomLen {
		t.Errorf("raw token suffix length: got %d, want %d", len(rawSuffix), TokenRandomLen)
	}
	alnumPattern := regexp.MustCompile(`^[a-z0-9]+$`)
	if !alnumPattern.MatchString(rawSuffix) {
		t.Errorf("raw token suffix not lowercase alphanumeric: got %q", rawSuffix)
	}
}

func TestGenerateTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		_, raw := GenerateToken()
		if seen[raw] {
			t.Fatalf("duplicate token generated on iteration %d: %q", i, raw)
		}
		seen[raw] = true
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	raw := "fgk_abcdefghijklmnopqrstuvwxyz012345"
	hash1 := HashToken(raw)
	hash2 := HashToken(raw)
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}
}

func TestHashTokenDifferentInputs(t *testing.T) {
	hash1 := HashToken("fgk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hash2 := HashToken("fgk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if hash1 == hash2 {
		t.Error("different tokens produced same hash")
	}
}

func TestHashTokenLength(t *testing.T) {
	_, raw := GenerateToken()
	hash := HashToken(raw)
	// SHA-256 hex digest is 64 characters.
	if len(hash) != 64 {
		t.Errorf("hash length: got %d, want 64", len(hash))
	}
}

func TestGenerateTokenIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, _ := GenerateToken()
		if seen[id] {
			t.Fatalf("duplicate token ID on iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}
