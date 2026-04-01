package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJWTCreateAndValidate(t *testing.T) {
	mgr, err := NewJWTManager("", 1*time.Hour)
	if err != nil {
		t.Fatalf("NewJWTManager: %v", err)
	}

	claims := Claims{UserID: "usr_abc123", Username: "testuser", Role: "user"}
	tokenStr, err := mgr.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("token string should not be empty")
	}

	got, err := mgr.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if got.UserID != "usr_abc123" {
		t.Errorf("UserID: got %q, want %q", got.UserID, "usr_abc123")
	}
	if got.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", got.Username, "testuser")
	}
	if got.Role != "user" {
		t.Errorf("Role: got %q, want %q", got.Role, "user")
	}
}

func TestJWTExpiredToken(t *testing.T) {
	// Create manager with 1ms expiry.
	mgr, _ := NewJWTManager("", 1*time.Millisecond)

	claims := Claims{UserID: "usr_abc123", Username: "testuser", Role: "user"}
	tokenStr, _ := mgr.CreateToken(claims)

	// Wait for expiry.
	time.Sleep(10 * time.Millisecond)

	_, err := mgr.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error should mention token: %v", err)
	}
}

func TestJWTTamperedToken(t *testing.T) {
	mgr, _ := NewJWTManager("", 1*time.Hour)

	claims := Claims{UserID: "usr_abc123", Username: "testuser", Role: "user"}
	tokenStr, _ := mgr.CreateToken(claims)

	// Tamper with the token by changing a character in the signature.
	tampered := tokenStr[:len(tokenStr)-1] + "X"

	_, err := mgr.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestJWTDifferentSecretRejects(t *testing.T) {
	mgr1, _ := NewJWTManager("", 1*time.Hour)
	mgr2, _ := NewJWTManager("", 1*time.Hour)

	claims := Claims{UserID: "usr_abc123", Username: "testuser", Role: "user"}
	tokenStr, _ := mgr1.CreateToken(claims)

	// Different manager (different secret) should reject.
	_, err := mgr2.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error when validating with different secret")
	}
}

func TestJWTEmptyToken(t *testing.T) {
	mgr, _ := NewJWTManager("", 1*time.Hour)
	_, err := mgr.ValidateToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestJWTMissingUserID(t *testing.T) {
	mgr, _ := NewJWTManager("", 1*time.Hour)
	_, err := mgr.CreateToken(Claims{Username: "test", Role: "user"})
	if err == nil {
		t.Fatal("expected error for missing UserID")
	}
}

func TestJWTMissingUsername(t *testing.T) {
	mgr, _ := NewJWTManager("", 1*time.Hour)
	_, err := mgr.CreateToken(Claims{UserID: "usr_123", Role: "user"})
	if err == nil {
		t.Fatal("expected error for missing Username")
	}
}

func TestJWTSecretPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "jwt_secret")

	// First instance creates the secret.
	mgr1, err := NewJWTManager(secretPath, 1*time.Hour)
	if err != nil {
		t.Fatalf("NewJWTManager (first): %v", err)
	}

	claims := Claims{UserID: "usr_abc123", Username: "testuser", Role: "user"}
	tokenStr, _ := mgr1.CreateToken(claims)

	// Verify secret file was created.
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		t.Fatal("secret file should exist")
	}

	// Second instance loads the same secret.
	mgr2, err := NewJWTManager(secretPath, 1*time.Hour)
	if err != nil {
		t.Fatalf("NewJWTManager (second): %v", err)
	}

	// Token from first instance should be valid on second.
	got, err := mgr2.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("token should be valid across instances: %v", err)
	}
	if got.UserID != "usr_abc123" {
		t.Errorf("UserID: got %q, want %q", got.UserID, "usr_abc123")
	}
}

func TestJWTAdminRole(t *testing.T) {
	mgr, _ := NewJWTManager("", 1*time.Hour)

	claims := Claims{UserID: "usr_admin01", Username: "admin", Role: "admin"}
	tokenStr, _ := mgr.CreateToken(claims)

	got, err := mgr.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got.Role != "admin" {
		t.Errorf("Role: got %q, want %q", got.Role, "admin")
	}
}
