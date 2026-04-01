---
id: SPR-017
title: "Sprint 017: User Model + Auth Backend"
type: how-to
status: ACTIVE
owner: architect
agents: [developer-a]
tags: [sprint, auth, users, backend, v1.2]
related: [SPR-018, SPR-019, SPR-020]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-017: User Model + Auth Backend

## Goal

Build the foundational user account system: users table, password hashing,
JWT sessions, invite codes, and admin bootstrap command.

---

## Assigned: Dev A (Server)

Branch: `feature/SPR-017-user-auth`

**Read first:**
- `CODEX/10_GOVERNANCE/GOV-002_TestingProtocol.md` §4.2 (acceptance criteria)
- `CODEX/10_GOVERNANCE/GOV-003_CodingStandard.md`
- `internal/auth/token.go` — existing token system
- `internal/store/tokens.go` — existing store pattern
- `migrations/001_initial.sql` — current schema

---

## Tasks

### T-053: Database Migration

Create `migrations/002_users.sql`:

```sql
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',
    created_at    TEXT NOT NULL,
    last_login_at TEXT,
    is_active     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id          TEXT PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    created_by  TEXT NOT NULL REFERENCES users(id),
    used_by     TEXT REFERENCES users(id),
    used_at     TEXT,
    created_at  TEXT NOT NULL,
    is_active   INTEGER NOT NULL DEFAULT 1
);

ALTER TABLE tokens ADD COLUMN user_id TEXT REFERENCES users(id);
```

Update store initialization to run both migrations in order.

---

### T-054: Password Hashing

Create `internal/auth/password.go`:

```go
// HashPassword hashes a plaintext password using bcrypt with cost 12.
func HashPassword(password string) (string, error)

// VerifyPassword checks a plaintext password against a bcrypt hash.
// Returns nil on success, error on mismatch.
func VerifyPassword(password, hash string) error

// ValidatePasswordStrength enforces minimum requirements.
// Returns error if password is < 8 characters.
func ValidatePasswordStrength(password string) error
```

Use `golang.org/x/crypto/bcrypt` with `bcrypt.DefaultCost` (10) or 12.
**Never log passwords.** Not in debug, not in error messages.

---

### T-055: JWT Implementation

Create `internal/auth/jwt.go`:

```go
type JWTManager struct {
    secret []byte
    expiry time.Duration  // default 24h
}

type Claims struct {
    UserID   string `json:"user_id"`
    Username string `json:"username"`
    Role     string `json:"role"`   // "admin" or "user"
}

// NewJWTManager creates a manager. If secretPath doesn't exist,
// generates a 32-byte random secret and writes it to the file.
func NewJWTManager(secretPath string, expiry time.Duration) (*JWTManager, error)

// CreateToken generates a signed JWT for the given claims.
func (j *JWTManager) CreateToken(claims Claims) (string, error)

// ValidateToken parses and validates a JWT string.
// Returns claims if valid, error if expired/invalid.
func (j *JWTManager) ValidateToken(tokenStr string) (*Claims, error)
```

Use `github.com/golang-jwt/jwt/v5`.

JWT secret auto-generated on first boot, stored at `<data-dir>/jwt_secret`.

---

### T-056: User Store

Create `internal/store/users.go`:

```go
// CreateUser inserts a new user with bcrypt-hashed password.
// ID format: usr_xxxxxxxxxxxx (12 hex chars)
func (s *Store) CreateUser(username, email, passwordHash, role string) (*User, error)

// GetUserByUsername returns a user by username.
func (s *Store) GetUserByUsername(username string) (*User, error)

// GetUserByEmail returns a user by email.
func (s *Store) GetUserByEmail(email string) (*User, error)

// GetUserByID returns a user by ID.
func (s *Store) GetUserByID(id string) (*User, error)

// UpdateLastLogin sets last_login_at to now.
func (s *Store) UpdateLastLogin(id string) error

// ListUsers returns all users (admin only).
func (s *Store) ListUsers() ([]User, error)
```

---

### T-057: Invite Code Store

Create `internal/store/invite_codes.go`:

```go
// CreateInviteCode generates a new 8-char alphanumeric invite code.
// ID format: inv_xxxxxxxxxxxx
func (s *Store) CreateInviteCode(createdBy string) (*InviteCode, string, error)

// ValidateInviteCode checks if a code is valid (exists, unused, active).
func (s *Store) ValidateInviteCode(code string) (*InviteCode, error)

// RedeemInviteCode marks a code as used by the given user.
func (s *Store) RedeemInviteCode(codeID, usedBy string) error

// ListInviteCodes returns all codes (admin view).
func (s *Store) ListInviteCodes() ([]InviteCode, error)
```

Invite code format: 8 characters, uppercase alphanumeric (easy to type/share).
Example: `ABCD1234`

---

### T-058: Admin Bootstrap Command

Add `fonzygrok-server admin create` subcommand:

```
fonzygrok-server admin create --username admin --email admin@example.com --data-dir /data
```

- Prompts for password interactively (no echo)
- Creates user with role=admin
- Fails if a user with that username already exists
- Prints confirmation with user ID

---

### T-059: Wire Token Ownership

Modify `internal/store/tokens.go`:
- `CreateToken` now accepts `userID string` parameter
- `ListTokens` accepts optional `userID` filter
- Existing `CreateToken` callers (admin CLI) pass empty userID (legacy)

---

## Acceptance Criteria

- [ ] Migration runs cleanly on fresh DB and on existing v1.1 DB
- [ ] Passwords hashed with bcrypt, never stored/logged in plaintext
- [ ] JWT creation and validation works with expiry
- [ ] JWT secret auto-generated on first boot
- [ ] Invite codes: create, validate, redeem, single-use enforced
- [ ] Admin bootstrap command creates first admin user
- [ ] All existing 212+ tests pass with -race
- [ ] New tests: password hash/verify, JWT create/validate/expire,
      user CRUD, invite code lifecycle, admin bootstrap
- [ ] User-scenario tests per GOV-007 §4.2

## Governance

| GOV | Requirement | Deliverable |
|:----|:-----------|:------------|
| GOV-002 §24 | Tests alongside code | Tests for every new file |
| GOV-003 | Doc comments | All exported functions documented |
| GOV-004 | Error handling | Structured errors, no panics |
| GOV-006 | Logging | No password logging, structured auth events |
