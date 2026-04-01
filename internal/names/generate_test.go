package names

import (
	"regexp"
	"testing"
)

// adjNounPattern matches the "adjective-noun" format.
var adjNounPattern = regexp.MustCompile(`^[a-z]+-[a-z]+$`)

func TestGenerateFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		name := Generate()
		if !adjNounPattern.MatchString(name) {
			t.Errorf("Generate() = %q, does not match adjective-noun format", name)
		}
	}
}

func TestGeneratePassesValidation(t *testing.T) {
	for i := 0; i < 100; i++ {
		name := Generate()
		if err := Validate(name); err != nil {
			t.Errorf("Generate() = %q failed Validate: %v", name, err)
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	collisions := 0
	const total = 1000

	for i := 0; i < total; i++ {
		name := Generate()
		if seen[name] {
			collisions++
		}
		seen[name] = true
	}

	// With ~85*85=7225 possible combinations, 1000 samples should have
	// very few collisions. Allow up to 10% (generous margin).
	maxCollisions := total / 10
	if collisions > maxCollisions {
		t.Errorf("too many collisions: %d out of %d (max %d)", collisions, total, maxCollisions)
	}
}

func TestGenerateUniqueNoCollision(t *testing.T) {
	exists := func(name string) bool { return false }
	name := GenerateUnique(exists)
	if !adjNounPattern.MatchString(name) {
		t.Errorf("GenerateUnique() = %q, does not match format", name)
	}
}

func TestGenerateUniqueRetries(t *testing.T) {
	attempts := 0
	exists := func(name string) bool {
		attempts++
		// First 5 names "exist", then accept.
		return attempts <= 5
	}

	name := GenerateUnique(exists)
	if !adjNounPattern.MatchString(name) {
		t.Errorf("GenerateUnique() = %q, does not match format", name)
	}
	if attempts <= 5 {
		t.Errorf("expected at least 6 attempts, got %d", attempts)
	}
}

func TestGenerateUniqueFallback(t *testing.T) {
	// All simple names "exist" — force fallback to adjective-noun-XXXX.
	fallbackPattern := regexp.MustCompile(`^[a-z]+-[a-z]+-\d{4}$`)

	exists := func(name string) bool {
		// Only accept names with digits (the fallback format).
		return !fallbackPattern.MatchString(name)
	}

	name := GenerateUnique(exists)
	if !fallbackPattern.MatchString(name) {
		t.Errorf("GenerateUnique fallback = %q, expected adjective-noun-XXXX", name)
	}
}

func TestGenerateUniqueAllPassValidation(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := GenerateUnique(func(string) bool { return false })
		if err := Validate(name); err != nil {
			t.Errorf("GenerateUnique() = %q failed Validate: %v", name, err)
		}
	}
}
