// Package server — auth_handlers.go provides REST API handlers for
// user registration, login, logout, and user info.
//
// SECURITY:
//   - Passwords are NEVER logged or returned in responses.
//   - Login failures use generic "invalid credentials" — never reveal
//     whether username or password was wrong.
//   - Registration atomically creates users and redeems invite codes to avoid
//     invite reuse races and orphaned accounts.
//
// REF: SPR-018 T-061, T-062, T-063, T-064
package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
)

// usernamePattern validates usernames: 3-32 chars, alphanumeric + underscore + hyphen.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,31}$`)

// emailPattern provides basic email format validation.
var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// sessionMaxAge is the cookie max-age in seconds (24 hours).
const sessionMaxAge = 86400

// handleRegister creates a new user account with invite code validation.
//
// POST /api/v1/register
//
// FLOW: validate input → hash password → atomically create user and redeem invite → issue JWT.
// DECISION: user creation and invite redemption are performed in one store
// transaction so concurrent registration attempts cannot reuse an invite or
// leave orphaned users.
func (a *AdminAPI) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	// Validate all fields.
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.InviteCode = strings.TrimSpace(strings.ToUpper(req.InviteCode))

	if req.Username == "" || req.Email == "" || req.Password == "" || req.InviteCode == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "All fields are required")
		return
	}
	if !usernamePattern.MatchString(req.Username) {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error",
			"Username must be 3-32 characters, alphanumeric, underscore, or hyphen")
		return
	}
	if !emailPattern.MatchString(req.Email) {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid email format")
		return
	}
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Password must be at least 8 characters")
		return
	}

	// Hash password.
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		a.logger.Error("register: hash password", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Registration failed")
		return
	}

	// Create user and redeem invite code atomically.
	user, err := a.store.RegisterUserWithInviteCode(req.Username, req.Email, hash, req.InviteCode)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			a.writeAdminError(w, http.StatusConflict, "conflict", "Username or email already taken")
			return
		}
		if strings.Contains(err.Error(), "invite code") {
			a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid or expired invite code")
			return
		}
		a.logger.Error("register: create user with invite", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Registration failed")
		return
	}

	// Issue JWT.
	token, err := a.jwt.CreateToken(auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	if err != nil {
		a.logger.Error("register: create JWT", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Registration failed")
		return
	}

	setSessionCookie(w, token, sessionMaxAge)
	a.logger.Info("user registered", "user_id", user.ID, "username", user.Username)

	a.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user": map[string]interface{}{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"created_at": user.CreatedAt.Format(time.RFC3339),
		},
		"token": token,
	})
}

// handleLogin authenticates a user and returns a JWT session.
//
// POST /api/v1/login
//
// SECURITY: Accepts username OR email in the "username" field.
// Uses bcrypt compare (constant-time) to prevent timing oracles.
// Never reveals whether username or password was wrong.
func (a *AdminAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
		return
	}

	// Try username first, then email.
	user, err := a.store.GetUserByUsername(req.Username)
	if err != nil {
		user, err = a.store.GetUserByEmail(req.Username)
	}
	if err != nil || user == nil {
		// SECURITY: Do a dummy bcrypt compare to prevent timing oracle
		// that reveals whether the username exists.
		auth.VerifyPassword(req.Password, "$2a$12$invalidhashpaddingtomakelengthenough")
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
		return
	}

	if !user.IsActive {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
		return
	}

	// Verify password (constant-time via bcrypt).
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Invalid credentials")
		return
	}

	// Update last login.
	a.store.UpdateLastLogin(user.ID)

	// Issue JWT.
	token, err := a.jwt.CreateToken(auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	if err != nil {
		a.logger.Error("login: create JWT", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Login failed")
		return
	}

	setSessionCookie(w, token, sessionMaxAge)
	a.logger.Info("user logged in", "user_id", user.ID, "username", user.Username)

	a.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
		"token": token,
	})
}

// handleLogout clears the session cookie.
//
// POST /api/v1/logout
//
// DECISION: Stateless JWT — no server-side invalidation. Cookie
// is cleared client-side. Token remains valid until expiry.
func (a *AdminAPI) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	a.writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
}

// handleMe returns the authenticated user's info.
//
// GET /api/v1/me (authenticated)
func (a *AdminAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := a.store.GetUserByID(claims.UserID)
	if err != nil {
		a.writeAdminError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	resp := map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	}
	if user.LastLoginAt != nil {
		resp["last_login_at"] = user.LastLoginAt.Format(time.RFC3339)
	}

	a.writeJSON(w, http.StatusOK, resp)
}

// handleCreateTokenAuth creates a token owned by the authenticated user.
//
// POST /api/v1/tokens (authenticated)
func (a *AdminAPI) handleCreateTokenAuth(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Field 'name' is required")
		return
	}
	if !namePattern.MatchString(req.Name) {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error",
			"Field 'name' must be 1-64 chars, alphanumeric and hyphens only")
		return
	}

	tok, rawToken, err := a.store.CreateToken(req.Name, claims.UserID)
	if err != nil {
		a.logger.Error("api: create token", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}

	a.logger.Info("api: token created", "token_id", tok.ID, "user_id", claims.UserID)

	a.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         tok.ID,
		"name":       tok.Name,
		"token":      rawToken,
		"created_at": tok.CreatedAt.Format(time.RFC3339),
	})
}

// handleListTokensAuth returns tokens based on role:
// - Users: only their own tokens
// - Admins: all tokens
//
// GET /api/v1/tokens (authenticated)
func (a *AdminAPI) handleListTokensAuth(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tokens, err := a.store.ListTokens()
	if err != nil {
		a.logger.Error("api: list tokens", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to list tokens")
		return
	}

	type tokenResp struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		UserID     *string `json:"user_id,omitempty"`
		CreatedAt  string  `json:"created_at"`
		LastUsedAt *string `json:"last_used_at"`
		IsActive   bool    `json:"is_active"`
	}

	items := make([]tokenResp, 0)
	for _, t := range tokens {
		// Non-admin users only see their own tokens.
		if claims.Role != "admin" && (t.UserID == nil || *t.UserID != claims.UserID) {
			continue
		}

		item := tokenResp{
			ID:        t.ID,
			Name:      t.Name,
			UserID:    t.UserID,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			IsActive:  t.IsActive,
		}
		if t.LastUsedAt != nil {
			s := t.LastUsedAt.Format(time.RFC3339)
			item.LastUsedAt = &s
		}
		items = append(items, item)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": items})
}

// handleDeleteTokenAuth revokes a token with ownership check.
//
// DELETE /api/v1/tokens/:id (authenticated)
// Users can only delete their own tokens. Admins can delete any.
func (a *AdminAPI) handleDeleteTokenAuth(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tokenID := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	if tokenID == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Token ID is required")
		return
	}

	if err := a.store.DeleteTokenForUser(tokenID, claims.UserID, claims.Role); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "revoked") {
			a.writeAdminError(w, http.StatusNotFound, "not_found", "Token not found")
			return
		}
		a.logger.Error("api: delete token", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to delete token")
		return
	}

	a.disconnectTunnelsByToken(tokenID)
	a.logger.Info("api: token revoked", "token_id", tokenID, "by_user", claims.UserID)
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateInviteCode creates a new invite code (admin only).
//
// POST /api/v1/invite-codes (admin only)
func (a *AdminAPI) handleCreateInviteCode(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		a.writeAdminError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	ic, code, err := a.store.CreateInviteCode(claims.UserID)
	if err != nil {
		a.logger.Error("api: create invite code", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to create invite code")
		return
	}

	a.logger.Info("api: invite code created", "code_id", ic.ID, "by_user", claims.UserID)

	a.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         ic.ID,
		"code":       code,
		"created_at": ic.CreatedAt.Format(time.RFC3339),
	})
}

// handleListInviteCodes returns all invite codes (admin only).
//
// GET /api/v1/invite-codes (admin only)
func (a *AdminAPI) handleListInviteCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := a.store.ListInviteCodes()
	if err != nil {
		a.logger.Error("api: list invite codes", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to list invite codes")
		return
	}

	type codeResp struct {
		ID        string  `json:"id"`
		Code      string  `json:"code"`
		CreatedBy string  `json:"created_by"`
		UsedBy    *string `json:"used_by,omitempty"`
		UsedAt    *string `json:"used_at,omitempty"`
		CreatedAt string  `json:"created_at"`
		IsActive  bool    `json:"is_active"`
	}

	items := make([]codeResp, 0, len(codes))
	for _, c := range codes {
		item := codeResp{
			ID:        c.ID,
			Code:      c.Code,
			CreatedBy: c.CreatedBy,
			UsedBy:    c.UsedBy,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
			IsActive:  c.IsActive,
		}
		if c.UsedAt != nil {
			s := c.UsedAt.Format(time.RFC3339)
			item.UsedAt = &s
		}
		items = append(items, item)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{"invite_codes": items})
}

// handleListUsers returns all users (admin only).
//
// GET /api/v1/users (admin only)
func (a *AdminAPI) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers()
	if err != nil {
		a.logger.Error("api: list users", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to list users")
		return
	}

	type userResp struct {
		ID          string  `json:"id"`
		Username    string  `json:"username"`
		Email       string  `json:"email"`
		Role        string  `json:"role"`
		IsActive    bool    `json:"is_active"`
		CreatedAt   string  `json:"created_at"`
		LastLoginAt *string `json:"last_login_at,omitempty"`
	}

	items := make([]userResp, 0, len(users))
	for _, u := range users {
		item := userResp{
			ID:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			IsActive:  u.IsActive,
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
		}
		if u.LastLoginAt != nil {
			s := u.LastLoginAt.Format(time.RFC3339)
			item.LastLoginAt = &s
		}
		items = append(items, item)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{"users": items})
}
