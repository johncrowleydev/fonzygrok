// Package store — users.go provides CRUD operations for fonzygrok
// user accounts. Users are persisted in SQLite with bcrypt password
// hashes. Passwords are NEVER stored or logged in plaintext.
//
// REF: SPR-017 T-056
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
)

// UserIDPrefix is the prefix for user IDs.
const UserIDPrefix = "usr_"

// User represents a stored user account.
type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	Role         string // "admin" or "user"
	CreatedAt    time.Time
	LastLoginAt  *time.Time
	IsActive     bool
}

// CreateUser inserts a new user with bcrypt-hashed password.
// ID format: usr_xxxxxxxxxxxx (12 hex chars).
//
// PRECONDITION: passwordHash must be a bcrypt hash, NOT plaintext.
// FAILURE MODE: Returns error on duplicate username or email.
func (s *Store) CreateUser(username, email, passwordHash, role string) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("store: username is required")
	}
	if email == "" {
		return nil, fmt.Errorf("store: email is required")
	}
	if passwordHash == "" {
		return nil, fmt.Errorf("store: password hash is required")
	}
	if role == "" {
		role = "user"
	}

	id := UserIDPrefix + auth.RandomHex(12)
	now := time.Now().UTC()

	_, err := s.db.Exec(
		`INSERT INTO users (id, username, email, password_hash, role, created_at, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, 1)`,
		id, username, email, passwordHash, role, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("store: create user: %w", err)
	}

	return &User{
		ID:           id,
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    now,
		IsActive:     true,
	}, nil
}

// GetUserByUsername returns a user by username.
// Returns sql.ErrNoRows wrapped error if not found.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	return s.getUserByField("username", username)
}

// GetUserByEmail returns a user by email.
// Returns sql.ErrNoRows wrapped error if not found.
func (s *Store) GetUserByEmail(email string) (*User, error) {
	return s.getUserByField("email", email)
}

// GetUserByID returns a user by ID.
// Returns sql.ErrNoRows wrapped error if not found.
func (s *Store) GetUserByID(id string) (*User, error) {
	return s.getUserByField("id", id)
}

// getUserByField is a DRY helper for single-field user lookups.
func (s *Store) getUserByField(field, value string) (*User, error) {
	query := fmt.Sprintf(
		`SELECT id, username, email, password_hash, role, created_at, last_login_at, is_active
		 FROM users WHERE %s = ?`, field,
	)

	var user User
	var createdAt string
	var lastLoginAt sql.NullString
	var isActive int

	err := s.db.QueryRow(query, value).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.Role, &createdAt, &lastLoginAt, &isActive,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("store: get user by %s: %w", field, err)
	}

	user.IsActive = isActive == 1

	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse created_at: %w", err)
	}
	user.CreatedAt = t

	if lastLoginAt.Valid {
		t, err := time.Parse(time.RFC3339, lastLoginAt.String)
		if err != nil {
			return nil, fmt.Errorf("store: parse last_login_at: %w", err)
		}
		user.LastLoginAt = &t
	}

	return &user, nil
}

// UpdateLastLogin sets last_login_at to now for the given user ID.
func (s *Store) UpdateLastLogin(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("store: update last login: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("store: user %q not found", id)
	}
	return nil
}

// ListUsers returns all users ordered by creation date (newest first).
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, email, password_hash, role, created_at, last_login_at, is_active
		 FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var createdAt string
		var lastLoginAt sql.NullString
		var isActive int

		if err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.PasswordHash,
			&user.Role, &createdAt, &lastLoginAt, &isActive,
		); err != nil {
			return nil, fmt.Errorf("store: scan user row: %w", err)
		}

		user.IsActive = isActive == 1
		t, _ := time.Parse(time.RFC3339, createdAt)
		user.CreatedAt = t

		if lastLoginAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastLoginAt.String)
			user.LastLoginAt = &t
		}

		users = append(users, user)
	}

	return users, rows.Err()
}
