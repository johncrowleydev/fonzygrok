// Package auth — jwt.go provides JWT token creation and validation
// for fonzygrok user sessions.
//
// JWT secret is auto-generated on first boot and persisted to disk.
// Tokens use HS256 signing with configurable expiry (default 24h).
//
// REF: SPR-017 T-055
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// JWTSecretLen is the length of the auto-generated JWT signing secret.
	JWTSecretLen = 32

	// DefaultJWTExpiry is the default token lifetime.
	DefaultJWTExpiry = 24 * time.Hour
)

// Claims holds the custom JWT claims for a fonzygrok user session.
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"` // "admin" or "user"
}

// jwtClaims wraps custom Claims with standard JWT registered claims.
type jwtClaims struct {
	Claims
	jwt.RegisteredClaims
}

// JWTManager creates and validates JWT tokens.
//
// THREAD SAFETY: Safe for concurrent use — secret is immutable after init.
type JWTManager struct {
	secret []byte
	expiry time.Duration
}

// NewJWTManager creates a JWT manager. If the secret file at secretPath
// doesn't exist, generates a 32-byte random secret and writes it.
// If secretPath is empty, generates an ephemeral in-memory secret (for testing).
//
// FAILURE MODE: If secret generation fails, the server cannot issue tokens.
// BLAST RADIUS: All authentication is blocked. Fail fast at startup.
func NewJWTManager(secretPath string, expiry time.Duration) (*JWTManager, error) {
	if expiry <= 0 {
		expiry = DefaultJWTExpiry
	}

	var secret []byte

	if secretPath == "" {
		// Ephemeral mode (testing): generate in-memory secret.
		secret = make([]byte, JWTSecretLen)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("auth: generate ephemeral JWT secret: %w", err)
		}
		return &JWTManager{secret: secret, expiry: expiry}, nil
	}

	// Persistent mode: load from file or generate.
	data, err := os.ReadFile(secretPath)
	if err == nil && len(data) >= JWTSecretLen {
		secret = data[:JWTSecretLen]
	} else {
		// Generate new secret.
		secret = make([]byte, JWTSecretLen)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("auth: generate JWT secret: %w", err)
		}
		if err := os.WriteFile(secretPath, secret, 0o600); err != nil {
			return nil, fmt.Errorf("auth: write JWT secret to %s: %w", secretPath, err)
		}
	}

	return &JWTManager{secret: secret, expiry: expiry}, nil
}

// CreateToken generates a signed JWT for the given claims.
//
// PRECONDITION: claims.UserID and claims.Username must be non-empty.
// POSTCONDITION: returned string is a valid signed JWT.
func (j *JWTManager) CreateToken(claims Claims) (string, error) {
	if claims.UserID == "" {
		return "", errors.New("auth: JWT claims.UserID is required")
	}
	if claims.Username == "" {
		return "", errors.New("auth: JWT claims.Username is required")
	}

	now := time.Now()
	tokenClaims := jwtClaims{
		Claims: claims,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.expiry)),
			Issuer:    "fonzygrok",
			Subject:   claims.UserID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims)
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign JWT: %w", err)
	}

	return signed, nil
}

// ValidateToken parses and validates a JWT string.
// Returns claims if valid, error if expired, tampered, or malformed.
//
// FAILURE MODE: Returns error — caller must deny access.
func (j *JWTManager) ValidateToken(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, errors.New("auth: token string is empty")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Verify signing method is HMAC (prevents algorithm-switching attacks).
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("auth: invalid token: %w", err)
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, errors.New("auth: invalid token claims")
	}

	return &claims.Claims, nil
}
