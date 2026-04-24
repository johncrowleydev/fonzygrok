package store

import (
	"strings"
	"testing"
)

func TestInviteCodeLifecycle(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	// Create a user to be the code creator.
	user, _ := st.CreateUser("admin1", "admin@test.com", "$2a$12$hash", "admin")

	// Create invite code.
	ic, code, err := st.CreateInviteCode(user.ID)
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}

	if !strings.HasPrefix(ic.ID, "inv_") {
		t.Errorf("ID should start with inv_, got %q", ic.ID)
	}
	if len(code) != InviteCodeLength {
		t.Errorf("code length: got %d, want %d", len(code), InviteCodeLength)
	}
	if ic.CreatedBy != user.ID {
		t.Errorf("CreatedBy: got %q, want %q", ic.CreatedBy, user.ID)
	}

	// Validate code.
	validated, err := st.ValidateInviteCode(code)
	if err != nil {
		t.Fatalf("ValidateInviteCode: %v", err)
	}
	if validated.ID != ic.ID {
		t.Errorf("validated ID mismatch")
	}

	// Create another user to redeem the code.
	user2, _ := st.CreateUser("newuser", "new@test.com", "$2a$12$hash2", "user")

	// Redeem code.
	if err := st.RedeemInviteCode(ic.ID, user2.ID); err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}

	// Validate again — should fail (already used).
	_, err = st.ValidateInviteCode(code)
	if err == nil {
		t.Fatal("expected error for already-used code")
	}
}

func TestInviteCodeReuseRejected(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("admin2", "admin2@test.com", "$2a$12$hash", "admin")
	ic, _, _ := st.CreateInviteCode(user.ID)

	user2, _ := st.CreateUser("redeemer1", "r1@test.com", "$2a$12$hash", "user")
	user3, _ := st.CreateUser("redeemer2", "r2@test.com", "$2a$12$hash", "user")

	// First redeem succeeds.
	if err := st.RedeemInviteCode(ic.ID, user2.ID); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	// Second redeem fails.
	err := st.RedeemInviteCode(ic.ID, user3.ID)
	if err == nil {
		t.Fatal("expected error for double redeem")
	}
}

func TestInviteCodeInvalidCode(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.ValidateInviteCode("BADCODE1")
	if err == nil {
		t.Fatal("expected error for non-existent code")
	}
}

func TestInviteCodeEmptyCode(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.ValidateInviteCode("")
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestInviteCodeEmptyCreatedBy(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, _, err := st.CreateInviteCode("")
	if err == nil {
		t.Fatal("expected error for empty createdBy")
	}
}

func TestListInviteCodes(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("admin3", "admin3@test.com", "$2a$12$hash", "admin")
	st.CreateInviteCode(user.ID)
	st.CreateInviteCode(user.ID)

	codes, err := st.ListInviteCodes()
	if err != nil {
		t.Fatalf("ListInviteCodes: %v", err)
	}
	if len(codes) != 2 {
		t.Errorf("expected 2 codes, got %d", len(codes))
	}
}

func TestListInviteCodesEmpty(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	codes, err := st.ListInviteCodes()
	if err != nil {
		t.Fatalf("ListInviteCodes: %v", err)
	}
	if len(codes) != 0 {
		t.Errorf("expected 0 codes, got %d", len(codes))
	}
}

func TestInviteCodeFormat(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("admin4", "admin4@test.com", "$2a$12$hash", "admin")
	_, code, _ := st.CreateInviteCode(user.ID)

	// Code should be 8 uppercase alphanumeric characters.
	if len(code) != 8 {
		t.Errorf("code length: got %d, want 8", len(code))
	}
	for _, c := range code {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("code contains invalid character: %c", c)
		}
	}
}

func TestRegisterUserWithInviteCodeRollsBackUserWhenRedeemFails(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	admin, _ := st.CreateUser("admin5", "admin5@test.com", "$2a$12$hash", "admin")
	ic, code, _ := st.CreateInviteCode(admin.ID)
	redeemer, _ := st.CreateUser("firstredeemer", "firstredeemer@test.com", "$2a$12$hash", "user")

	// Simulate a race: code validates initially, then is redeemed before registration commits.
	if _, err := st.ValidateInviteCode(code); err != nil {
		t.Fatalf("ValidateInviteCode: %v", err)
	}
	if err := st.RedeemInviteCode(ic.ID, redeemer.ID); err != nil {
		t.Fatalf("RedeemInviteCode: %v", err)
	}

	_, err := st.RegisterUserWithInviteCode("raceuser", "race@test.com", "$2a$12$hash", code)
	if err == nil {
		t.Fatal("expected registration to fail when invite is concurrently redeemed")
	}
	if _, err := st.GetUserByUsername("raceuser"); err == nil {
		t.Fatal("registration failure must roll back user creation")
	}
}
