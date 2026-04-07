// Package store — invite_codes.go provides CRUD operations for
// fonzygrok invite codes. Invite codes are single-use registration
// tokens created by admins to control user signups.
//
// Code format: 8 uppercase alphanumeric characters (e.g., ABCD1234).
// ID format: inv_xxxxxxxxxxxx (12 hex chars).
//
// REF: SPR-017 T-057
package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
)

// InviteCodeIDPrefix is the prefix for invite code IDs.
const InviteCodeIDPrefix = "inv_"

// inviteCodeAlphabet is the character set for invite code generation.
// Uppercase alphanumeric for easy typing and sharing.
const inviteCodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// InviteCodeLength is the length of generated invite codes.
const InviteCodeLength = 8

// InviteCode represents a stored invite code.
type InviteCode struct {
	ID        string
	Code      string
	CreatedBy string // user ID of creator
	UsedBy    *string
	UsedAt    *time.Time
	CreatedAt time.Time
	IsActive  bool
}

// CreateInviteCode generates a new 8-char alphanumeric invite code.
//
// PRECONDITION: createdBy must be a valid user ID.
// POSTCONDITION: code is unique and active.
func (s *Store) CreateInviteCode(createdBy string) (*InviteCode, string, error) {
	if createdBy == "" {
		return nil, "", fmt.Errorf("store: createdBy is required")
	}

	id := InviteCodeIDPrefix + auth.RandomHex(12)
	code := generateInviteCode()
	now := time.Now().UTC()

	_, err := s.db.Exec(
		`INSERT INTO invite_codes (id, code, created_by, created_at, is_active)
		 VALUES ($1, $2, $3, $4, TRUE)`,
		id, code, createdBy, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("store: create invite code: %w", err)
	}

	ic := &InviteCode{
		ID:        id,
		Code:      code,
		CreatedBy: createdBy,
		CreatedAt: now,
		IsActive:  true,
	}
	return ic, code, nil
}

// ValidateInviteCode checks if a code is valid (exists, unused, active).
// Returns the InviteCode if valid, error otherwise.
func (s *Store) ValidateInviteCode(code string) (*InviteCode, error) {
	if code == "" {
		return nil, fmt.Errorf("store: invite code is required")
	}

	var ic InviteCode
	var usedBy sql.NullString

	err := s.db.QueryRow(
		`SELECT id, code, created_by, used_by, used_at, created_at, is_active
		 FROM invite_codes WHERE code = $1`,
		code,
	).Scan(&ic.ID, &ic.Code, &ic.CreatedBy, &usedBy, &ic.UsedAt, &ic.CreatedAt, &ic.IsActive)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("store: invalid invite code")
	}
	if err != nil {
		return nil, fmt.Errorf("store: validate invite code: %w", err)
	}

	if !ic.IsActive {
		return nil, fmt.Errorf("store: invite code is deactivated")
	}
	if usedBy.Valid {
		return nil, fmt.Errorf("store: invite code already used")
	}

	return &ic, nil
}

// RedeemInviteCode marks a code as used by the given user.
//
// PRECONDITION: code must be validated first via ValidateInviteCode.
func (s *Store) RedeemInviteCode(codeID, usedBy string) error {
	if codeID == "" || usedBy == "" {
		return fmt.Errorf("store: codeID and usedBy are required")
	}

	now := time.Now().UTC()
	result, err := s.db.Exec(
		`UPDATE invite_codes SET used_by = $1, used_at = $2 WHERE id = $3 AND used_by IS NULL`,
		usedBy, now, codeID,
	)
	if err != nil {
		return fmt.Errorf("store: redeem invite code: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("store: invite code already redeemed or not found")
	}

	return nil
}

// ListInviteCodes returns all invite codes ordered by creation date.
func (s *Store) ListInviteCodes() ([]InviteCode, error) {
	rows, err := s.db.Query(
		`SELECT id, code, created_by, used_by, used_at, created_at, is_active
		 FROM invite_codes ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list invite codes: %w", err)
	}
	defer rows.Close()

	var codes []InviteCode
	for rows.Next() {
		var ic InviteCode
		var usedBy sql.NullString

		if err := rows.Scan(&ic.ID, &ic.Code, &ic.CreatedBy, &usedBy, &ic.UsedAt, &ic.CreatedAt, &ic.IsActive); err != nil {
			return nil, fmt.Errorf("store: scan invite code row: %w", err)
		}

		if usedBy.Valid {
			ic.UsedBy = &usedBy.String
		}

		codes = append(codes, ic)
	}

	return codes, rows.Err()
}

// generateInviteCode creates an 8-character uppercase alphanumeric code.
func generateInviteCode() string {
	b := make([]byte, InviteCodeLength)
	randomBytes := make([]byte, InviteCodeLength)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Sprintf("store: crypto/rand.Read failed: %v", err))
	}
	for i := 0; i < InviteCodeLength; i++ {
		b[i] = inviteCodeAlphabet[int(randomBytes[i])%len(inviteCodeAlphabet)]
	}
	return string(b)
}
