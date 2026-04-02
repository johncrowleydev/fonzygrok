---
id: CON-001
title: "Client-Server Protocol Contract"
type: reference
status: STABLE
owner: architect
agents: [all]
tags: [standards, specification, networking, protocol, ssh]
related: [BLU-001, GOV-004, GOV-008]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** This contract defines the SSH-based protocol between the fonzygrok client and server. It covers authentication, control messages, tunnel lifecycle, and data channel semantics. All agents building client or server code MUST conform. No deviation without Human approval.

# Client-Server Protocol — Interface Contract

> **"The contract is truth. The code is an attempt to match it."**

---

## 1. Contract Scope

**What this covers:**
- SSH transport configuration
- Authentication handshake
- Control channel message types and formats
- Data channel (proxy) semantics
- Tunnel lifecycle (request → assigned → active → closed)
- Heartbeat and keepalive behavior

**What this does NOT cover:**
- HTTP edge routing (see CON-002)
- Admin API endpoints (see CON-002)
- SQLite schema details (see BLU-001)
- Deployment configuration (see GOV-008)

**Parties:**

| Role | Description |
|:-----|:------------|
| **Producer** | Tunnel Server (SSH listener, tunnel manager) |
| **Consumer** | Tunnel Client (SSH connector, local proxy) |

---

## 2. Version & Stability

| Field | Value |
|:------|:------|
| Contract version | `1.0.0` |
| Stability | `EXPERIMENTAL` |
| Breaking change policy | MAJOR version bump required for any breaking change to message formats |
| Backward compatibility | Server should support N-1 protocol versions when possible |

---

## 3. SSH Transport

### 3.1 Connection Parameters

| Parameter | Value |
|:----------|:------|
| Server listen address | `:2222` |
| SSH protocol version | SSH-2.0 |
| Host key algorithm | ED25519 (generated on first server start, persisted at `/data/host_key`) |
| Cipher preferences | Default Go `crypto/ssh` negotiation |
| Keepalive interval | 30 seconds (SSH transport level) |
| Keepalive max missed | 3 (connection closed after 90s of silence) |

### 3.2 Authentication

Authentication uses SSH password auth. The "password" field carries the API token.

```
Client → Server: SSH auth request
  username: "fonzygrok"        (fixed, not used for identity)
  password: "<api-token>"      (used for identity + authorization)

Server validates token against SQLite store.
  → Valid: auth success, SSH session established
  → Invalid: auth failure, connection closed
```

**Rationale:** SSH password auth is the simplest mechanism that works with `crypto/ssh`. The username is ignored — identity is determined by the token. SSH key auth may be added in a future version (v1.2+) without breaking this contract.

### 3.3 Host Key Verification

- Client SHOULD verify the server's host key on first connect and cache it
- Client MUST warn (not block) on host key mismatch in v1.0
- Client stores known hosts in `~/.fonzygrok/known_hosts`

---

## 4. Control Channel

### 4.1 Channel Type

After SSH session establishment, the **client** opens a channel:

```
Channel type: "control"
Extra data:   (none)
```

The server accepts exactly one control channel per SSH session. Additional control channel requests are rejected.

### 4.2 Message Format

All messages on the control channel are **newline-delimited JSON** (one JSON object per line, terminated by `\n`).

```
{"type":"<message_type>","payload":{...}}\n
```

### 4.3 Message Types

#### `TunnelRequest` (Client → Server)

```json
{
  "type": "tunnel_request",
  "payload": {
    "local_port": 3000,
    "protocol": "http",
    "subdomain": ""
  }
}
```

| Field | Type | Required | Description | Constraints |
|:------|:-----|:--------:|:------------|:------------|
| `local_port` | `integer` | ✅ | The port on the client's machine to expose | 1–65535 |
| `protocol` | `string` | ✅ | Protocol type | `"http"` (v1.0). `"tcp"` reserved for future. |
| `subdomain` | `string` | ❌ | Requested subdomain (v1.1+). Empty = auto-assign. | `[a-z0-9-]{3,63}` |

#### `TunnelAssignment` (Server → Client)

```json
{
  "type": "tunnel_assignment",
  "payload": {
    "tunnel_id": "a3f8x2",
    "assigned_subdomain": "a3f8x2",
    "public_url": "http://a3f8x2.tunnel.example.com",
    "protocol": "http"
  }
}
```

| Field | Type | Always present | Description |
|:------|:-----|:--------------:|:------------|
| `tunnel_id` | `string` | ✅ | Unique tunnel identifier (6 chars, lowercase alphanumeric) |
| `assigned_subdomain` | `string` | ✅ | The subdomain assigned to this tunnel |
| `public_url` | `string` | ✅ | Full public URL the user can share |
| `protocol` | `string` | ✅ | Confirmed protocol (`"http"`) |

#### `TunnelClose` (Either direction)

```json
{
  "type": "tunnel_close",
  "payload": {
    "tunnel_id": "a3f8x2",
    "reason": "client_disconnect"
  }
}
```

| Field | Type | Required | Description |
|:------|:-----|:--------:|:------------|
| `tunnel_id` | `string` | ✅ | Tunnel to close |
| `reason` | `string` | ✅ | One of: `client_disconnect`, `server_shutdown`, `token_revoked`, `idle_timeout` |

#### `Error` (Server → Client)

```json
{
  "type": "error",
  "payload": {
    "code": "SUBDOMAIN_TAKEN",
    "message": "The requested subdomain 'myapp' is already in use",
    "request_type": "tunnel_request"
  }
}
```

| Field | Type | Always present | Description |
|:------|:-----|:--------------:|:------------|
| `code` | `string` | ✅ | Error code (see §4.4) |
| `message` | `string` | ✅ | Human-readable error description |
| `request_type` | `string` | ✅ | Which request type triggered the error |

### 4.4 Error Codes

| Code | Meaning |
|:-----|:--------|
| `INVALID_PROTOCOL` | Requested protocol not supported |
| `SUBDOMAIN_TAKEN` | Requested subdomain already in use |
| `SUBDOMAIN_INVALID` | Subdomain format invalid |
| `MAX_TUNNELS_EXCEEDED` | Client has too many active tunnels |
| `INTERNAL_ERROR` | Server-side failure |

---

## 5. Data Channel (Proxy)

### 5.1 Channel Type

For each incoming HTTP request, the **server** opens a channel to the client:

```
Channel type: "proxy"
Extra data:   <tunnel_id> (UTF-8 string)
```

### 5.2 Data Flow

```
Server → Client (on new channel):
  1. Write raw HTTP request bytes (method line + headers + body)

Client:
  2. Dial localhost:<local_port>
  3. Write request bytes to local service
  4. Read response bytes from local service
  5. Write response bytes back to SSH channel

Server:
  6. Read response bytes from SSH channel
  7. Write response to the original HTTP client
  8. Close the SSH channel
```

### 5.3 Error Handling

| Scenario | Behavior |
|:---------|:---------|
| Client can't reach local port | Client writes HTTP 502 response, closes channel |
| Client disconnects mid-request | Server writes HTTP 502 to end user, closes channel |
| Request timeout (30s) | Server closes channel, writes HTTP 504 to end user |
| Channel open fails | Server writes HTTP 502 to end user |

### 5.4 Response Format for Error Cases

When the client or server must generate an error response:

```http
HTTP/1.1 502 Bad Gateway
Content-Type: text/plain
X-Fonzygrok-Error: true

fonzygrok: tunnel error - <reason>
```

---

## 6. Tunnel Lifecycle

```
                  ┌───────────┐
          ┌───────│ REQUESTED │
          │       └─────┬─────┘
          │             │ Server validates
          │             ▼
          │       ┌───────────┐
   Error  │       │ ASSIGNED  │
   ◄──────┘       └─────┬─────┘
                        │ Assignment sent to client
                        ▼
                  ┌───────────┐
                  │  ACTIVE   │◄──── Proxying traffic
                  └─────┬─────┘
                        │ Close event
                        ▼
                  ┌───────────┐
                  │  CLOSED   │
                  └───────────┘
```

---

## 7. Performance Requirements

| Metric | Requirement |
|:-------|:------------|
| Tunnel setup latency | < 500ms from request to assignment |
| Proxy overhead per request | < 10ms added latency (excluding network RTT) |
| Max concurrent tunnels per client | 10 (configurable) |
| Max concurrent HTTP requests per tunnel | 100 (limited by SSH channel capacity) |
| Keepalive interval | 30s |

---

## 8. Go Type Definitions

```go
// ControlMessage is the envelope for all control channel messages.
type ControlMessage struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// TunnelRequest is sent by the client to request a new tunnel.
type TunnelRequest struct {
    LocalPort int    `json:"local_port"`
    Protocol  string `json:"protocol"`
    Subdomain string `json:"subdomain,omitempty"`
}

// TunnelAssignment is sent by the server after a successful tunnel setup.
type TunnelAssignment struct {
    TunnelID          string `json:"tunnel_id"`
    AssignedSubdomain string `json:"assigned_subdomain"`
    PublicURL         string `json:"public_url"`
    Protocol          string `json:"protocol"`
}

// TunnelClose is sent by either side to close a tunnel.
type TunnelClose struct {
    TunnelID string `json:"tunnel_id"`
    Reason   string `json:"reason"`
}

// ErrorMessage is sent by the server when a request fails.
type ErrorMessage struct {
    Code        string `json:"code"`
    Message     string `json:"message"`
    RequestType string `json:"request_type"`
}
```

---

## 9. Change Protocol

> **This contract is immutable without Human approval.**

To propose a contract change:
1. Developer or Tester opens `60_EVOLUTION/EVO-NNN.md` describing the proposed change
2. Architect reviews and drafts the contract update
3. Human approves the updated contract
4. Version is bumped. All consuming agents are notified.
5. A transition sprint is opened if the change is breaking

---

## 10. Verification Checklist

Tests validating this contract live in `40_VERIFICATION/`. The following must pass before any sprint referencing this contract can close:

- [ ] SSH connection establishes with valid token (§3.2)
- [ ] SSH connection rejects invalid token (§3.2)
- [ ] Control channel opens successfully (§4.1)
- [ ] TunnelRequest produces TunnelAssignment (§4.3)
- [ ] Proxy channel carries HTTP request/response round-trip (§5.2)
- [ ] Client reconnects after disconnect (§3.1 keepalive)
- [ ] Error responses match format (§5.4)
- [ ] All error codes handled (§4.4)
- [ ] Performance requirements met (§7)
