package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
)

// Token represents a stored authentication token.
type Token struct {
	ID         string
	Name       string
	TokenHash  string
	UserID     *string // nullable — legacy tokens have no user
	CreatedAt  time.Time
	LastUsedAt *time.Time
	IsActive   bool
}

// CreateToken generates a new token, stores it, and returns the Token
// record along with the raw token string. The raw token is returned
// exactly once — it is not stored in plaintext.
// userID may be empty for legacy/unowned tokens.
func (s *Store) CreateToken(name string, userID ...string) (*Token, string, error) {
	id, rawToken := auth.GenerateToken()
	hash := auth.HashToken(rawToken)
	now := time.Now().UTC()

	// Resolve optional userID parameter.
	var uid *string
	if len(userID) > 0 && userID[0] != "" {
		uid = &userID[0]
	}

	_, err := s.db.Exec(
		`INSERT INTO tokens (id, name, token_hash, user_id, created_at, is_active) VALUES (?, ?, ?, ?, ?, 1)`,
		id, name, hash, uid, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, "", fmt.Errorf("store: create token: %w", err)
	}

	tok := &Token{
		ID:        id,
		Name:      name,
		TokenHash: hash,
		UserID:    uid,
		CreatedAt: now,
		IsActive:  true,
	}
	return tok, rawToken, nil
}

// ValidateToken hashes the raw token and looks up the matching record.
// Returns the Token if found and active, or an error otherwise.
func (s *Store) ValidateToken(rawToken string) (*Token, error) {
	hash := auth.HashToken(rawToken)

	var tok Token
	var createdAt string
	var lastUsedAt sql.NullString
	var isActive int

	err := s.db.QueryRow(
		`SELECT id, name, token_hash, created_at, last_used_at, is_active FROM tokens WHERE token_hash = ?`,
		hash,
	).Scan(&tok.ID, &tok.Name, &tok.TokenHash, &createdAt, &lastUsedAt, &isActive)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: invalid token")
	}
	if err != nil {
		return nil, fmt.Errorf("store: validate token: %w", err)
	}

	tok.IsActive = isActive == 1
	if !tok.IsActive {
		return nil, fmt.Errorf("store: token is revoked")
	}

	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse created_at: %w", err)
	}
	tok.CreatedAt = t

	if lastUsedAt.Valid {
		t, err := time.Parse(time.RFC3339, lastUsedAt.String)
		if err != nil {
			return nil, fmt.Errorf("store: parse last_used_at: %w", err)
		}
		tok.LastUsedAt = &t
	}

	return &tok, nil
}

// ListTokens returns all tokens in the store.
func (s *Store) ListTokens() ([]Token, error) {
	rows, err := s.db.Query(
		`SELECT id, name, token_hash, created_at, last_used_at, is_active FROM tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var tok Token
		var createdAt string
		var lastUsedAt sql.NullString
		var isActive int

		if err := rows.Scan(&tok.ID, &tok.Name, &tok.TokenHash, &createdAt, &lastUsedAt, &isActive); err != nil {
			return nil, fmt.Errorf("store: scan token row: %w", err)
		}

		tok.IsActive = isActive == 1
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse created_at: %w", err)
		}
		tok.CreatedAt = t

		if lastUsedAt.Valid {
			t, err := time.Parse(time.RFC3339, lastUsedAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse last_used_at: %w", err)
			}
			tok.LastUsedAt = &t
		}

		tokens = append(tokens, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate tokens: %w", err)
	}

	return tokens, nil
}

// DeleteToken marks a token as inactive (soft delete) by ID.
// Returns an error if the token does not exist.
func (s *Store) DeleteToken(id string) error {
	result, err := s.db.Exec(`UPDATE tokens SET is_active = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete token rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store: token %q not found", id)
	}
	return nil
}

// UpdateLastUsed sets the last_used_at timestamp to now for the given token ID.
func (s *Store) UpdateLastUsed(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE tokens SET last_used_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("store: update last used: %w", err)
	}
	return nil
}
