package store

import (
	"strings"
	"testing"
)

func TestCreateUserAndGetByUsername(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, err := st.CreateUser("testuser", "test@example.com", "$2a$12$fakehash", "user")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if !strings.HasPrefix(user.ID, "usr_") {
		t.Errorf("ID should start with usr_, got %q", user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", user.Username, "testuser")
	}
	if user.Role != "user" {
		t.Errorf("Role: got %q, want %q", user.Role, "user")
	}
	if !user.IsActive {
		t.Error("user should be active")
	}

	// Get by username.
	got, err := st.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, user.ID)
	}
}

func TestCreateUserAndGetByEmail(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("emailuser", "email@test.com", "$2a$12$fakehash", "admin")

	got, err := st.GetUserByEmail("email@test.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID mismatch")
	}
	if got.Role != "admin" {
		t.Errorf("Role: got %q, want %q", got.Role, "admin")
	}
}

func TestCreateUserAndGetByID(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("iduser", "id@test.com", "$2a$12$fakehash", "user")

	got, err := st.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Username != "iduser" {
		t.Errorf("Username: got %q, want %q", got.Username, "iduser")
	}
}

func TestCreateUserDuplicateUsername(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.CreateUser("dupeuser", "one@test.com", "$2a$12$hash1", "user")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = st.CreateUser("dupeuser", "two@test.com", "$2a$12$hash2", "user")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.CreateUser("user1", "same@test.com", "$2a$12$hash1", "user")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = st.CreateUser("user2", "same@test.com", "$2a$12$hash2", "user")
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestCreateUserEmptyFields(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.CreateUser("", "e@t.com", "hash", "user")
	if err == nil {
		t.Fatal("expected error for empty username")
	}

	_, err = st.CreateUser("u", "", "hash", "user")
	if err == nil {
		t.Fatal("expected error for empty email")
	}

	_, err = st.CreateUser("u", "e@t.com", "", "user")
	if err == nil {
		t.Fatal("expected error for empty password hash")
	}
}

func TestUpdateLastLogin(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	user, _ := st.CreateUser("loginuser", "login@test.com", "$2a$12$hash", "user")

	if user.LastLoginAt != nil {
		t.Error("last_login_at should be nil initially")
	}

	if err := st.UpdateLastLogin(user.ID); err != nil {
		t.Fatalf("UpdateLastLogin: %v", err)
	}

	got, _ := st.GetUserByID(user.ID)
	if got.LastLoginAt == nil {
		t.Fatal("last_login_at should be set after update")
	}
}

func TestUpdateLastLoginNotFound(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	err := st.UpdateLastLogin("usr_nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestListUsers(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	st.CreateUser("user1", "u1@test.com", "$2a$12$h1", "user")
	st.CreateUser("user2", "u2@test.com", "$2a$12$h2", "admin")

	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestListUsersEmpty(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestGetUserNotFound(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	_, err := st.GetUserByUsername("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestUserDefaultRole(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	// Empty role defaults to "user".
	user, err := st.CreateUser("defaultrole", "dr@test.com", "$2a$12$hash", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Role != "user" {
		t.Errorf("Role: got %q, want %q", user.Role, "user")
	}
}
