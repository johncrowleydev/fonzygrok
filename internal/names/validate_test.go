package names

import (
	"testing"
)

func TestValidateValidNames(t *testing.T) {
	valid := []string{
		"my-api",
		"calm-tiger",
		"abc",
		"test-app-123",
		"a0b",
		"my-long-subdomain-name-here",
		"12345",
		"a-b",
	}

	for _, name := range valid {
		if err := Validate(name); err != nil {
			t.Errorf("Validate(%q) = %v, expected nil", name, err)
		}
	}
}

func TestValidateTooShort(t *testing.T) {
	short := []string{"", "a", "ab"}
	for _, name := range short {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) = nil, expected error (too short)", name)
		}
	}
}

func TestValidateTooLong(t *testing.T) {
	// 33 characters.
	long := "abcdefghijklmnopqrstuvwxyz1234567"
	if len(long) != 33 {
		t.Fatalf("test setup: expected 33 chars, got %d", len(long))
	}
	if err := Validate(long); err == nil {
		t.Errorf("Validate(%q) = nil, expected error (too long)", long)
	}
}

func TestValidateExact32(t *testing.T) {
	// Exactly 32 characters — should be valid.
	exact := "abcdefghijklmnopqrstuvwxyz123456"
	if len(exact) != 32 {
		t.Fatalf("test setup: expected 32 chars, got %d", len(exact))
	}
	if err := Validate(exact); err != nil {
		t.Errorf("Validate(%q) = %v, expected nil (exactly 32 chars)", exact, err)
	}
}

func TestValidateBadChars(t *testing.T) {
	bad := []string{
		"My-Api",        // uppercase
		"my_api",        // underscore
		"my api",        // space
		"my.api",        // dot
		"my@api",        // special char
		"café",          // unicode
		"ALLCAPS",       // uppercase
	}
	for _, name := range bad {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) = nil, expected error (bad chars)", name)
		}
	}
}

func TestValidateLeadingTrailingHyphen(t *testing.T) {
	bad := []string{
		"-my-api",
		"my-api-",
		"-my-api-",
		"---",
	}
	for _, name := range bad {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) = nil, expected error (leading/trailing hyphen)", name)
		}
	}
}

func TestValidateReservedNames(t *testing.T) {
	reserved := []string{
		"www", "api", "admin", "app", "mail", "ftp", "ssh", "dns",
		"ns1", "ns2", "smtp", "imap", "pop", "cdn", "static", "assets",
		"docs", "blog", "status", "health", "tunnel",
	}
	for _, name := range reserved {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) = nil, expected error (reserved)", name)
		}
	}
}

func TestIsReserved(t *testing.T) {
	if !IsReserved("www") {
		t.Error("IsReserved(\"www\") = false, want true")
	}
	if !IsReserved("admin") {
		t.Error("IsReserved(\"admin\") = false, want true")
	}
	if IsReserved("my-api") {
		t.Error("IsReserved(\"my-api\") = true, want false")
	}
	if IsReserved("calm-tiger") {
		t.Error("IsReserved(\"calm-tiger\") = true, want false")
	}
}

func TestIsReservedAllEntries(t *testing.T) {
	// Verify all entries in the reserved list are detected.
	expectedCount := 21
	count := 0
	for name := range reservedNames {
		if !IsReserved(name) {
			t.Errorf("IsReserved(%q) = false, want true", name)
		}
		count++
	}
	if count != expectedCount {
		t.Errorf("reserved names count: got %d, want %d", count, expectedCount)
	}
}
