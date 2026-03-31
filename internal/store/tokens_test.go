package store

import (
	"strings"
	"testing"
)

func TestCreateToken(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	tok, raw, err := s.CreateToken("test-token")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token")
	}
	if !strings.HasPrefix(tok.ID, "tok_") {
		t.Errorf("token ID missing prefix: %q", tok.ID)
	}
	if tok.Name != "test-token" {
		t.Errorf("name: got %q, want %q", tok.Name, "test-token")
	}
	if !strings.HasPrefix(raw, "fgk_") {
		t.Errorf("raw token missing prefix: %q", raw)
	}
	if !tok.IsActive {
		t.Error("new token should be active")
	}
	if tok.LastUsedAt != nil {
		t.Error("new token should have nil LastUsedAt")
	}
}

func TestValidateToken(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, raw, err := s.CreateToken("validate-test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := s.ValidateToken(raw)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got.Name != "validate-test" {
		t.Errorf("name: got %q, want %q", got.Name, "validate-test")
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, err := s.ValidateToken("fgk_invalidtokeninvalidtokeninvalid")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateTokenRevoked(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	tok, raw, err := s.CreateToken("revoke-test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err := s.DeleteToken(tok.ID); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err = s.ValidateToken(raw)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("error should mention 'revoked': got %q", err.Error())
	}
}

func TestListTokens(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	// Empty list initially.
	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens (empty): %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}

	// Create two tokens.
	s.CreateToken("first")
	s.CreateToken("second")

	tokens, err = s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestDeleteToken(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	tok, _, err := s.CreateToken("delete-test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err := s.DeleteToken(tok.ID); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	// Verify token is inactive.
	tokens, err := s.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	for _, tk := range tokens {
		if tk.ID == tok.ID && tk.IsActive {
			t.Error("deleted token should be inactive")
		}
	}
}

func TestDeleteTokenNotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	err := s.DeleteToken("tok_nonexistent00")
	if err == nil {
		t.Fatal("expected error for non-existent token")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': got %q", err.Error())
	}
}

func TestUpdateLastUsed(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	tok, raw, err := s.CreateToken("lastused-test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err := s.UpdateLastUsed(tok.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}

	got, err := s.ValidateToken(raw)
	if err != nil {
		t.Fatalf("ValidateToken after UpdateLastUsed: %v", err)
	}
	if got.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set after UpdateLastUsed")
	}
}

func TestCreateMultipleTokens(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tok, _, err := s.CreateToken("multi-test")
		if err != nil {
			t.Fatalf("CreateToken[%d]: %v", i, err)
		}
		if ids[tok.ID] {
			t.Fatalf("duplicate token ID: %q", tok.ID)
		}
		ids[tok.ID] = true
	}
}
