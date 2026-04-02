---
id: SPR-018
title: "Sprint 018: API Endpoints + Auth Middleware"
type: how-to
status: COMPLETE
owner: architect
agents: [developer-a]
tags: [sprint, api, auth, middleware, v1.2]
related: [SPR-017, SPR-019]
created: 2026-04-01
updated: 2026-04-02
version: 1.0.0
---

# SPR-018: API Endpoints + Auth Middleware

## Goal

Build the authenticated REST API: registration, login, token management,
invite codes, and JWT auth middleware that protects all endpoints.

**Depends on:** SPR-017 (user model, bcrypt, JWT — merged)

---

## Assigned: Dev A (Server)

Branch: `feature/SPR-018-api-auth`

**Pull main first** — you need the SPR-017 code.

---

## Tasks

### T-060: Auth Middleware

Create `internal/server/middleware.go`:

```go
// AuthMiddleware extracts and validates JWT from:
// 1. Authorization: Bearer <token> header (API clients)
// 2. HttpOnly cookie named "session" (web dashboard)
//
// On success: adds Claims to request context
// On failure: returns 401 Unauthorized JSON response
func (s *Server) AuthMiddleware(next http.Handler) http.Handler

// RequireRole returns middleware that checks the JWT claims for a
// specific role. Returns 403 if the user doesn't have the role.
func (s *Server) RequireRole(role string, next http.Handler) http.Handler

// OptionalAuth is like AuthMiddleware but doesn't reject unauthenticated
// requests — just passes nil claims in context.
func (s *Server) OptionalAuth(next http.Handler) http.Handler
```

Context helper functions:
```go
func ClaimsFromContext(ctx context.Context) *auth.Claims
func UserIDFromContext(ctx context.Context) string
```

---

### T-061: Registration Endpoint

`POST /api/v1/register`

```json
// Request
{
  "username": "john",
  "email": "john@example.com",
  "password": "securepassword",
  "invite_code": "ABCD1234"
}

// Response 201
{
  "user": {
    "id": "usr_a1b2c3d4e5f6",
    "username": "john",
    "email": "john@example.com",
    "role": "user",
    "created_at": "2026-04-01T19:00:00Z"
  },
  "token": "eyJhbG..."   // JWT session token
}
```

Validation:
- Username: 3-32 chars, alphanumeric + underscore + hyphen
- Email: basic format validation
- Password: 8+ chars (ValidatePasswordStrength)
- Invite code: must be valid, unused, active
- All fields required

On success:
- Create user
- Redeem invite code
- Return JWT as both JSON field and Set-Cookie header

---

### T-062: Login Endpoint

`POST /api/v1/login`

```json
// Request
{
  "username": "john",       // username OR email
  "password": "securepassword"
}

// Response 200
{
  "user": {
    "id": "usr_a1b2c3d4e5f6",
    "username": "john",
    "role": "user"
  },
  "token": "eyJhbG..."
}
```

- Accept username or email in the "username" field
- Constant-time comparison via bcrypt (no timing oracle)
- Update last_login_at on success
- Set-Cookie header for web clients
- Generic error message on failure: "invalid credentials" (don't reveal
  whether username or password was wrong)

---

### T-063: Logout Endpoint

`POST /api/v1/logout`

- Clears the session cookie (Set-Cookie with MaxAge=0)
- Returns 200 OK
- No server-side session invalidation yet (stateless JWT)

---

### T-064: User Info Endpoint

`GET /api/v1/me` (authenticated)

```json
// Response 200
{
  "id": "usr_a1b2c3d4e5f6",
  "username": "john",
  "email": "john@example.com",
  "role": "user",
  "created_at": "2026-04-01T19:00:00Z",
  "last_login_at": "2026-04-01T19:30:00Z"
}
```

---

### T-065: Token Management Endpoints (Authenticated)

`POST /api/v1/tokens` (authenticated)

```json
// Request
{ "name": "dev-laptop" }

// Response 201
{
  "id": "tok_a1b2c3d4e5f6",
  "name": "dev-laptop",
  "token": "fgk_abc123...",  // raw token, shown ONCE
  "created_at": "2026-04-01T19:00:00Z"
}
```

- Token is owned by the authenticated user (user_id set)

`GET /api/v1/tokens` (authenticated)

- Users: returns only THEIR tokens
- Admins: returns all tokens (with user info)

`DELETE /api/v1/tokens/:id` (authenticated)

- Users: can only delete their own tokens
- Admins: can delete any token
- Returns 404 if token doesn't exist or doesn't belong to user

---

### T-066: Invite Code Endpoints (Admin Only)

`POST /api/v1/invite-codes` (admin only)

```json
// Response 201
{
  "id": "inv_a1b2c3d4e5f6",
  "code": "ABCD1234",
  "created_at": "2026-04-01T19:00:00Z"
}
```

`GET /api/v1/invite-codes` (admin only)

- Returns all codes with usage status

---

### T-067: Admin User Management (Admin Only)

`GET /api/v1/users` (admin only)

- List all users with active status, last login, token count

---

### T-068: Wire to Admin Router

Register all new routes on the existing admin API router (`:9090`).
The admin router currently serves `/api/v1/health`, `/api/v1/tokens`,
`/api/v1/tunnels`.

- Existing token endpoints become authenticated (breaking change for
  unauthenticated admin API access)
- Health endpoint stays public (no auth required)
- Add CORS headers for web dashboard access

**Migration note:** The existing unauthenticated `/api/v1/tokens` endpoint
is now behind auth. This is intentional — the admin API was insecure.

---

## Acceptance Criteria

- [ ] All endpoints return proper JSON with correct status codes
- [ ] Auth middleware validates JWT from header and cookie
- [ ] Role-based access: users see own tokens, admins see all
- [ ] Registration validates invite code, creates user, returns JWT
- [ ] Login works with both username and email
- [ ] Generic error on login failure (no info leakage)
- [ ] Existing health endpoint stays public
- [ ] All 251+ tests pass with -race
- [ ] New tests: register, login, auth middleware, each endpoint,
      role enforcement, edge cases (expired JWT, invalid code, etc.)
