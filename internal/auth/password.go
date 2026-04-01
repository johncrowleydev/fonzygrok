// Package auth — password.go provides bcrypt password hashing and
// validation for fonzygrok user authentication.
//
// SECURITY: Passwords are NEVER logged — not in debug, not in errors,
// not in test output. Only bcrypt hashes are stored.
//
// REF: SPR-017 T-054
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost is the bcrypt work factor. 12 balances security and
	// latency (~250ms per hash on modern hardware).
	// DECISION: Cost 12 over DefaultCost(10) per SPR-017 spec.
	BcryptCost = 12

	// MinPasswordLength is the minimum acceptable password length.
	MinPasswordLength = 8
)

// HashPassword hashes a plaintext password using bcrypt with cost 12.
//
// PRECONDITION: password must pass ValidatePasswordStrength.
// POSTCONDITION: returned hash is a bcrypt string, never the plaintext.
// FAILURE MODE: Returns error if bcrypt fails (extremely rare — OOM).
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}

	return string(hash), nil
}

// VerifyPassword checks a plaintext password against a bcrypt hash.
// Returns nil on success, error on mismatch or invalid hash.
func VerifyPassword(password, hash string) error {
	if password == "" {
		return errors.New("auth: password cannot be empty")
	}
	if hash == "" {
		return errors.New("auth: hash cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("auth: invalid credentials")
	}

	return nil
}

// ValidatePasswordStrength enforces minimum password requirements.
// Returns error if password is shorter than MinPasswordLength (8) characters.
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("auth: password must be at least %d characters", MinPasswordLength)
	}
	return nil
}
