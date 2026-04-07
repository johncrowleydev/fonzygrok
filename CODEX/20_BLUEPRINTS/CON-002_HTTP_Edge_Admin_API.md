---
id: CON-002
title: "HTTP Edge & Admin API Contract"
type: reference
status: STABLE
owner: architect
agents: [all]
tags: [standards, specification, api, http, routing]
related: [BLU-001, CON-001, GOV-004, GOV-008]
created: 2026-03-31
updated: 2026-04-07
version: 2.0.0
---

> **BLUF:** This contract defines HOW the fonzygrok server routes public HTTP requests to tunnels and WHAT the admin REST API looks like. It covers subdomain-based routing, error responses, and all admin endpoints for token and tunnel management.

# HTTP Edge & Admin API — Interface Contract

> **"The contract is truth. The code is an attempt to match it."**

---

## 1. Contract Scope

**What this covers:**
- HTTP edge routing rules (public requests → tunnels)
- Subdomain assignment and format
- Public-facing error responses
- Admin API endpoints, request/response schemas
- Health check endpoint

**What this does NOT cover:**
- SSH transport and control messages (see CON-001)
- Database schema (see BLU-001)
- Deployment and port binding (see GOV-008)

**Parties:**

| Role | Description |
|:-----|:------------|
| **Producer** | Tunnel Server (edge router, admin API) |
| **Consumer (edge)** | Public internet users (browsers, API clients) |
| **Consumer (admin)** | Server operator (human or scripts) |

---

## 2. Version & Stability

| Field | Value |
|:------|:------|
| Contract version | `2.0.0` |
| Stability | `STABLE` |
| Breaking change policy | MAJOR version bump required for any breaking change |
| Backward compatibility | Admin API versioned via URL prefix (`/api/v1/`) |

---

## 3. HTTP Edge Router

### 3.1 Routing Rules

The edge router listens on `:8080` (mapped to public port `80` via Docker).

**v1.0 routing (subdomain-based):**

```
Request: GET http://<tunnel_id>.tunnel.example.com/api/users
                   ^^^^^^^^^^^
                   Extract this from Host header
                   Look up tunnel by ID in Tunnel Manager
```

| Host Header Pattern | Behavior |
|:-------------------|:---------|
| `<tunnel_id>.<base_domain>` | Route to tunnel with matching ID |
| `<base_domain>` (no subdomain) | Return server info page (200 OK, JSON) |
| No matching tunnel | Return 404 |

### 3.2 Request Proxying

The edge router transparently proxies the **entire HTTP request** (method, path, headers, body) through the SSH tunnel to the client. It also transparently proxies the **entire HTTP response** back.

**Headers added by the edge:**

| Header | Value | Direction |
|:-------|:------|:----------|
| `X-Forwarded-For` | Original client IP | Request → Client |
| `X-Forwarded-Host` | Original Host header | Request → Client |
| `X-Forwarded-Proto` | `http` or `https` | Request → Client |
| `X-Fonzygrok-Tunnel-Id` | Tunnel ID | Request → Client |

**Headers the edge MUST NOT modify:**
- `Content-Type`
- `Content-Length`
- `Authorization`
- Any application-specific headers

### 3.3 Public Error Responses

| Scenario | Status | Body |
|:---------|:-------|:-----|
| Tunnel not found | `404 Not Found` | `{"error": "tunnel_not_found", "message": "No tunnel matches this hostname"}` |
| Tunnel disconnected | `502 Bad Gateway` | `{"error": "tunnel_offline", "message": "The tunnel is currently offline"}` |
| Local service unreachable | `502 Bad Gateway` | `{"error": "upstream_unreachable", "message": "The local service did not respond"}` |
| Proxy timeout (30s) | `504 Gateway Timeout` | `{"error": "proxy_timeout", "message": "The local service did not respond in time"}` |
| Server error | `500 Internal Server Error` | `{"error": "internal_error", "message": "An unexpected error occurred"}` |

All error responses include:
```
Content-Type: application/json
X-Fonzygrok-Error: true
```

### 3.4 Server Info (Root Domain)

```
GET http://tunnel.example.com/
```

Response:
```json
{
  "service": "fonzygrok",
  "version": "1.0.0",
  "status": "running",
  "tunnels_active": 3
}
```

---

## 4. Admin API

### 4.1 Base URL

```
http://localhost:9090/api/v1/
```

The admin API is NOT exposed to the public internet. It binds to `127.0.0.1:9090` only.

### 4.2 Authentication

v1.0: No authentication on the admin API (localhost-only access is the security boundary).

v1.2+: JWT session authentication. All mutating endpoints require a valid `Authorization: Bearer <jwt>` header or `session` cookie.

**Auth endpoints (on edge router, port 443):**

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/login` | Login page (HTML) |
| `POST` | `/login` | Authenticate, set session cookie |
| `GET` | `/register` | Registration page (requires invite code) |
| `POST` | `/register` | Create account with invite code |
| `POST` | `/logout` | Clear session cookie |
| `GET` | `/dashboard` | User dashboard (tokens, tunnels) |
| `GET` | `/admin/users` | Admin: user management |
| `GET` | `/admin/invites` | Admin: invite code management |
| `POST` | `/admin/invites` | Admin: generate new invite code |

**Admin API auth (port 9090):**

All `/api/v1/` endpoints require `Authorization: Bearer <jwt>` except `/api/v1/health`.

### 4.3 Endpoints

---

#### `GET /api/v1/health`

Health check endpoint.

**Response (200 OK):**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "tunnels_active": 3,
  "clients_connected": 2
}
```

---

#### `GET /api/v1/tokens`

List all tokens.

**Response (200 OK):**
```json
{
  "tokens": [
    {
      "id": "tok_abc123def456",
      "name": "dev-laptop",
      "created_at": "2026-03-31T18:00:00Z",
      "last_used_at": "2026-03-31T18:30:00Z",
      "is_active": true
    }
  ]
}
```

---

#### `POST /api/v1/tokens`

Create a new token.

**Request:**
```json
{
  "name": "dev-laptop"
}
```

| Field | Type | Required | Constraints |
|:------|:-----|:--------:|:------------|
| `name` | `string` | ✅ | 1–64 chars, alphanumeric + hyphens |

**Response (201 Created):**
```json
{
  "id": "tok_abc123def456",
  "name": "dev-laptop",
  "token": "fgk_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "created_at": "2026-03-31T18:00:00Z"
}
```

> ⚠️ The `token` field is only returned once at creation time. It cannot be retrieved again.

**Token format:** `fgk_` prefix + 32 chars of `[a-z0-9]` (generated from crypto/rand).

---

#### `DELETE /api/v1/tokens/:id`

Revoke a token. Any active tunnels using this token will be disconnected.

**Response (204 No Content):** (empty body)

**Response (404 Not Found):**
```json
{
  "error": "not_found",
  "message": "Token not found"
}
```

---

#### `GET /api/v1/tunnels`

List all active tunnels.

**Response (200 OK):**
```json
{
  "tunnels": [
    {
      "tunnel_id": "a3f8x2",
      "subdomain": "a3f8x2",
      "public_url": "http://a3f8x2.tunnel.example.com",
      "protocol": "http",
      "client_ip": "203.0.113.42",
      "token_id": "tok_abc123def456",
      "local_port": 3000,
      "connected_at": "2026-03-31T18:15:00Z",
      "bytes_in": 102400,
      "bytes_out": 51200,
      "requests_proxied": 47
    }
  ]
}
```

---

#### `DELETE /api/v1/tunnels/:tunnel_id`

Force-close a tunnel.

**Response (204 No Content):** (empty body)

**Response (404 Not Found):**
```json
{
  "error": "not_found",
  "message": "Tunnel not found"
}
```

---

## 5. Error Response Format

All admin API errors follow this format:

```json
{
  "error": "<error_code>",
  "message": "<human_readable_description>"
}
```

| Scenario | Status | Error Code |
|:---------|:-------|:-----------|
| Missing required field | `400 Bad Request` | `validation_error` |
| Invalid field format | `400 Bad Request` | `validation_error` |
| Resource not found | `404 Not Found` | `not_found` |
| Method not allowed | `405 Method Not Allowed` | `method_not_allowed` |
| Internal error | `500 Internal Server Error` | `internal_error` |

---

## 6. Performance Requirements

| Metric | Requirement |
|:-------|:------------|
| Edge proxy p95 latency overhead | < 10ms (excluding tunnel RTT) |
| Admin API p95 latency | < 50ms |
| Max concurrent proxied requests | 1000 per server |
| Edge timeout | 30s (configurable) |

---

## 7. Go Type Definitions

```go
// --- Edge Router Types ---

// EdgeErrorResponse is returned by the edge router for tunnel errors.
type EdgeErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
}

// ServerInfo is returned when the root domain is accessed.
type ServerInfo struct {
    Service       string `json:"service"`
    Version       string `json:"version"`
    Status        string `json:"status"`
    TunnelsActive int    `json:"tunnels_active"`
}

// --- Admin API Types ---

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
    Status           string `json:"status"`
    Version          string `json:"version"`
    UptimeSeconds    int64  `json:"uptime_seconds"`
    TunnelsActive    int    `json:"tunnels_active"`
    ClientsConnected int    `json:"clients_connected"`
}

// TokenListResponse is returned by GET /api/v1/tokens.
type TokenListResponse struct {
    Tokens []TokenSummary `json:"tokens"`
}

// TokenSummary represents a token in list responses.
type TokenSummary struct {
    ID         string  `json:"id"`
    Name       string  `json:"name"`
    CreatedAt  string  `json:"created_at"`
    LastUsedAt *string `json:"last_used_at,omitempty"`
    IsActive   bool    `json:"is_active"`
}

// CreateTokenRequest is accepted by POST /api/v1/tokens.
type CreateTokenRequest struct {
    Name string `json:"name"`
}

// CreateTokenResponse is returned by POST /api/v1/tokens.
type CreateTokenResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Token     string `json:"token"`
    CreatedAt string `json:"created_at"`
}

// TunnelListResponse is returned by GET /api/v1/tunnels.
type TunnelListResponse struct {
    Tunnels []TunnelInfo `json:"tunnels"`
}

// TunnelInfo represents an active tunnel.
type TunnelInfo struct {
    TunnelID        string `json:"tunnel_id"`
    Subdomain       string `json:"subdomain"`
    PublicURL       string `json:"public_url"`
    Protocol        string `json:"protocol"`
    ClientIP        string `json:"client_ip"`
    TokenID         string `json:"token_id"`
    LocalPort       int    `json:"local_port"`
    ConnectedAt     string `json:"connected_at"`
    BytesIn         int64  `json:"bytes_in"`
    BytesOut        int64  `json:"bytes_out"`
    RequestsProxied int64  `json:"requests_proxied"`
}

// AdminErrorResponse is returned by admin API for errors.
type AdminErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
}
```

---

## 8. Change Protocol

> **This contract is immutable without Human approval.**

To propose a contract change:
1. Developer or Tester opens `60_EVOLUTION/EVO-NNN.md` describing the proposed change
2. Architect reviews and drafts the contract update
3. Human approves the updated contract
4. Version is bumped. All consuming agents are notified.
5. A transition sprint is opened if the change is breaking

---

## 9. Verification Checklist

Tests validating this contract live in `40_VERIFICATION/`. The following must pass before any sprint referencing this contract can close:

- [ ] Edge routes request to correct tunnel by Host header (§3.1)
- [ ] Edge adds X-Forwarded-* headers (§3.2)
- [ ] Edge returns correct error responses for all scenarios (§3.3)
- [ ] Server info endpoint works on root domain (§3.4)
- [ ] All admin API endpoints return correct schemas (§4.3)
- [ ] Token creation returns token only once (§4.3)
- [ ] Token deletion disconnects active tunnels (§4.3)
- [ ] All error responses match format (§5)
- [ ] Performance requirements met (§6)
