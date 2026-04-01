package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	password := "securepassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Hash must not be the plaintext.
	if hash == password {
		t.Fatal("hash must not equal plaintext")
	}

	// Hash must be a bcrypt string (starts with $2).
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("hash should be bcrypt format, got prefix: %q", hash[:4])
	}

	// Verify succeeds with correct password.
	if err := VerifyPassword(password, hash); err != nil {
		t.Fatalf("VerifyPassword with correct password: %v", err)
	}
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	hash, _ := HashPassword("correctpassword1")

	err := VerifyPassword("wrongpassword99", hash)
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	// Error should NOT contain the password.
	if strings.Contains(err.Error(), "wrongpassword99") {
		t.Error("error message must not contain the password")
	}
}

func TestHashPasswordEmpty(t *testing.T) {
	_, err := HashPassword("")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPasswordEmptyPassword(t *testing.T) {
	err := VerifyPassword("", "$2a$12$somehash")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPasswordEmptyHash(t *testing.T) {
	err := VerifyPassword("password123", "")
	if err == nil {
		t.Fatal("expected error for empty hash")
	}
}

func TestValidatePasswordStrengthShort(t *testing.T) {
	err := ValidatePasswordStrength("short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
}

func TestValidatePasswordStrengthValid(t *testing.T) {
	err := ValidatePasswordStrength("longenoughpassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePasswordStrengthExactMinimum(t *testing.T) {
	// Exactly 8 characters should pass.
	err := ValidatePasswordStrength("12345678")
	if err != nil {
		t.Fatalf("8-char password should pass: %v", err)
	}

	// 7 characters should fail.
	err = ValidatePasswordStrength("1234567")
	if err == nil {
		t.Fatal("7-char password should fail")
	}
}

func TestDifferentPasswordsDifferentHashes(t *testing.T) {
	h1, _ := HashPassword("password_one1")
	h2, _ := HashPassword("password_two2")

	if h1 == h2 {
		t.Error("different passwords should produce different hashes")
	}
}

func TestSamePasswordDifferentHashes(t *testing.T) {
	// bcrypt includes a random salt, so same password → different hashes.
	h1, _ := HashPassword("samepassword1")
	h2, _ := HashPassword("samepassword1")

	if h1 == h2 {
		t.Error("same password should produce different hashes due to salt")
	}

	// But both should verify.
	if err := VerifyPassword("samepassword1", h1); err != nil {
		t.Errorf("verify h1: %v", err)
	}
	if err := VerifyPassword("samepassword1", h2); err != nil {
		t.Errorf("verify h2: %v", err)
	}
}
