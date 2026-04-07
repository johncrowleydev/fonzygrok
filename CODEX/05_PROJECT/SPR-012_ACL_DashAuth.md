---
id: SPR-012
title: "Sprint 012: IP Whitelisting & Dashboard Auth"
type: how-to
status: COMPLETE
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.2, security, acl]
related: [CON-002, BLU-001]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-012: IP Whitelisting & Dashboard Auth

## Goal

Per-tunnel IP access control lists. Dashboard authentication to protect the admin API.

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-037A IP Whitelisting | Server | Dev A | SPR-011 merged |
| T-038B Dashboard Auth | Server | Dev B | SPR-011 merged |

Both tasks are fully parallel.

---

## Task Details

### T-037A: IP Whitelisting

#### New file: `internal/server/acl.go`

```go
type ACL struct {
    AllowCIDRs []net.IPNet
    DenyCIDRs  []net.IPNet
    Mode       string // "allow" (whitelist) or "deny" (blacklist)
}

// Check returns true if the IP is allowed.
// allow mode: IP must match at least one AllowCIDR
// deny mode: IP must NOT match any DenyCIDR
func (a *ACL) Check(ip net.IP) bool
```

#### Modify: `internal/server/tunnel.go`

- Add `ACL *ACL` field to `TunnelEntry`
- Pass through on registration

#### Modify: `internal/server/edge.go`

- Before proxying: extract remote IP, check `tunnel.ACL.Check(ip)`
- If blocked → `403 Forbidden`:
  ```json
  {
    "error": "ip_blocked",
    "message": "Your IP is not allowed to access this tunnel"
  }
  ```
- Add `X-Fonzygrok-Error: ip_blocked` header

#### Modify: `internal/server/tcpedge.go`

- Same IP check for TCP connections

#### Modify: `internal/server/admin.go`

- `PUT /api/v1/tunnels/:id/acl`:
  ```json
  {
    "mode": "allow",
    "cidrs": ["192.168.1.0/24", "10.0.0.5/32"]
  }
  ```
- `DELETE /api/v1/tunnels/:id/acl` — remove ACL (allow all)
- Include ACL in tunnel list response

#### Modify: `cmd/client/main.go`

- Add `--allow-ip` flag (repeatable): set whitelist on tunnel creation
  ```bash
  fonzygrok --port 3000 --allow-ip 192.168.1.0/24 --allow-ip 10.0.0.5/32
  ```
- Send ACL in tunnel request (extend proto message)

#### Modify: `internal/proto/messages.go`

- Add `AllowIPs []string` to `TunnelRequest` (optional)

#### Tests

- `acl_test.go`:
  - Allow mode: allowed IP passes, blocked IP rejected
  - Deny mode: denied IP blocked, others pass
  - CIDR ranges work (/24, /32, /16)
  - Empty ACL allows all
  - Invalid CIDR returns parse error
- `edge_test.go`: 403 response format
- `admin_test.go`: PUT/DELETE ACL endpoints

#### Acceptance Criteria

- `--allow-ip 10.0.0.0/8` → only IPs in 10.x.x.x range can access tunnel
- No `--allow-ip` → all IPs allowed (default, no change from v1.0)
- ACL modifiable via admin API after tunnel creation
- Works for both HTTP and TCP tunnels
- `X-Forwarded-For` is NOT used for ACL (source IP only, prevent spoofing)

---

### T-038B: Dashboard Auth & Polish

#### Modify: `internal/server/admin.go`

Add bearer token auth middleware:

```go
func authMiddleware(adminToken string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Skip auth for health endpoint
            if r.URL.Path == "/api/v1/health" {
                next.ServeHTTP(w, r)
                return
            }
            // Check Authorization header or cookie
            token := extractToken(r)
            if token != adminToken {
                http.Error(w, `{"error":"unauthorized"}`, 401)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

#### Modify: `cmd/server/main.go`

- Add `--admin-token` flag (string, optional)
- If set: wrap admin API + dashboard with auth middleware
- If NOT set: admin API is unauthenticated (backward compatible)

#### Dashboard login page

Add `login.html` to dashboard assets:
- Token input field
- "Log in" button → stores token in `localStorage`
- All subsequent htmx requests include `Authorization: Bearer <token>` header
- On 401 response → redirect to login page

#### Dashboard polish

- Add real-time updates to tunnel list via htmx polling (every 3s)
- Add tunnel metrics charts (simple sparklines using CSS, no chart library)
- Add token "last used" field
- Mobile responsive improvements
- Add "copy to clipboard" buttons for tunnel URLs and tokens

#### Tests

- `admin_test.go`:
  - With `--admin-token`: 401 without token, 200 with correct token
  - Health endpoint accessible without token
  - Dashboard redirect to login when unauthorized
- `dashboard_test.go`:
  - Login page served when not authenticated
  - Dashboard served when authenticated

#### Acceptance Criteria

- `--admin-token mysecret` → admin API requires `Authorization: Bearer mysecret`
- Without `--admin-token` → no auth (backward compatible)
- Dashboard shows login page when token not in localStorage
- After login, all pages work
- Health endpoint always accessible (for Docker healthcheck)
- Copy-to-clipboard works for URLs and tokens

---

## Security Notes

- Admin token is a simple shared secret. This is NOT a user auth system — it protects the admin panel from exposure.
- In production, combined with restricting port 9090 to specific IPs, this provides two layers of access control.
- The admin token should be set via env var `FONZYGROK_ADMIN_TOKEN` in docker-compose.
