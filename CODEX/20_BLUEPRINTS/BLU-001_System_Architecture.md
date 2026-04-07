---
id: BLU-001
title: "Fonzygrok System Architecture Blueprint"
type: explanation
status: APPROVED
owner: architect
agents: [all]
tags: [architecture, design, networking, tunneling, golang]
related: [PRJ-001, CON-001, CON-002, GOV-008, RES-001]
created: 2026-03-31
updated: 2026-04-07
version: 2.0.0
---

> **BLUF:** Fonzygrok is a two-binary system (server + client) that creates SSH-based tunnels for HTTP and TCP traffic. The server has six subsystems: SSH listener, HTTP edge router, TCP edge, tunnel manager, rate limiter, and admin API (with integrated dashboard). The client has three subsystems: SSH connection manager, local proxy, and CLI. SQLite provides persistent state. All subsystems communicate in-process — no message queues, no microservices.

# Fonzygrok System Architecture Blueprint

---

## 1. System Overview

```
┌───────────────────────────────────────────────────────────────────────┐
│                        TUNNEL SERVER                                  │
│                   (fonzygrok-server binary)                           │
│                                                                       │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────┐  │
│  │ SSH Listener │  │  HTTP Edge   │  │  TCP Edge    │  │Admin API │  │
│  │  :2222       │  │  Router :443 │  │  :40000+     │  │  :9090   │  │
│  │             │  │              │  │              │  │          │  │
│  │  Accepts    │  │  Routes by   │  │  Raw TCP     │  │ REST API │  │
│  │  client     │  │  Host header │  │  tunnel by   │  │ Dashboard│  │
│  │  connections│  │  to tunnels  │  │  port number │  │ (UI/HTML)│  │
│  └──────┬──────┘  └──────┬───────┘  └──────┬───────┘  └────┬─────┘  │
│         │                │                  │               │        │
│         │         ┌──────▼──────┐           │               │        │
│         │         │ Rate Limiter│           │               │        │
│         │         │ (token      │           │               │        │
│         │         │  bucket)    │           │               │        │
│         │         └──────┬──────┘           │               │        │
│         ▼                ▼                  ▼               │        │
│  ┌──────────────────────────────────────────────────────────┘        │
│  │              Tunnel Manager                                       │
│  │                                                                   │
│  │  - Registers HTTP and TCP tunnels                                 │
│  │  - Routes requests to correct SSH channel                         │
│  │  - Assigns TCP ports from pool                                    │
│  │  - Tracks tunnel state + metrics (bytes, requests)                │
│  │  - IP ACL per tunnel                                              │
│  └──────────────────────┬────────────────────────────────────────────┘│
│                         │                                             │
│                    ┌────▼────┐                                        │
│                    │ SQLite  │                                        │
│                    │ Store   │                                        │
│                    └─────────┘                                        │
└───────────────────────────────────────────────────────────────────────┘
                         │
                    SSH tunnel (encrypted, multiplexed)
                    Client initiates outbound
                         │
┌───────────────────────────────────────────────────────────────────┐
│                        TUNNEL CLIENT                              │
│                     (fonzygrok binary)                            │
│                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐            │
│  │ SSH Conn Mgr │  │ Local Proxy  │  │ CLI (cobra)  │            │
│  │              │  │              │  │              │            │
│  │ Connect,     │  │ Accept from  │  │ Parse args   │            │
│  │ auth, mux,   │  │ SSH channel, │  │ Read config  │            │
│  │ reconnect    │  │ dial local   │  │ Print status │            │
│  │              │  │ (HTTP + TCP) │  │ --protocol   │            │
│  └──────────────┘  └──────────────┘  └──────────────┘            │
└───────────────────────────────────────────────────────────────────┘
```

---

## 2. Component Descriptions

### 2.1 Server: SSH Listener

**Package:** `internal/server/ssh.go`

- Listens on `:2222` for incoming SSH connections
- Authenticates clients using token (via SSH password auth callback — the "password" is the API token)
- Creates a multiplexed SSH session per client
- Passes new channel requests to the Tunnel Manager
- Handles keepalive/heartbeat at the SSH transport level

### 2.2 Server: HTTP Edge Router

**Package:** `internal/server/edge.go`

- Listens on `:443` (TLS) or `:8080` (plain) for incoming public HTTP requests
- Extracts the `Host` header to determine which tunnel to route to
- For subdomain routing: `<tunnel-id>.tunnel.example.com` → lookup tunnel by ID
- **Rate limit** check: per-tunnel token bucket; returns **429 Too Many Requests** when exceeded
- **IP ACL** check: per-tunnel allow-list; returns **403 Forbidden** for blocked IPs
- Proxies the full HTTP request through the SSH channel to the client
- Receives the response and sends it back to the public requester
- Returns `502 Bad Gateway` if the tunnel is disconnected
- Returns `404 Not Found` if the subdomain doesn't match any tunnel
- **Fallback handler**: requests to the base/apex domain (no subdomain) are delegated to the admin API mux, which serves the dashboard UI

### 2.3 Server: TCP Edge

**Package:** `internal/server/tcpedge.go`

- Manages a pool of TCP ports (e.g., 40000–40100)
- When a TCP tunnel is registered, allocates a port and starts a TCP listener
- Each incoming TCP connection opens a `"tcp-proxy"` SSH channel to the client
- Bidirectional data copy between the TCP connection and the SSH channel
- Port is released back to the pool when the tunnel is deregistered
- Rate limiting applied per-connection if configured

### 2.4 Server: Rate Limiter

**Package:** `internal/server/ratelimit.go`

- Per-tunnel token bucket rate limiter
- Configured via `--rate-limit` (requests/sec) and `--rate-burst` (max burst)
- Wired to both HTTP edge router and TCP edge
- Uses `golang.org/x/time/rate` for token bucket implementation
- Returns false from `Allow()` when the bucket is exhausted

### 2.5 Server: Admin API + Dashboard

**Package:** `internal/server/admin.go`, `internal/server/dashboard.go`

- Listens on `:9090` (localhost-only in production)
- REST API for managing tokens, users, invite codes, and viewing tunnel state
- **Dashboard UI**: server-rendered HTML with Go templates + HTMX
  - Login, registration (invite code required), token management
  - Light/dark theme toggle (defaults to system preference)
  - Served on the apex domain via edge router fallback handler
- Endpoints defined in CON-002

### 2.6 Server: Tunnel Manager

**Package:** `internal/server/tunnel.go`

- Central registry of active tunnels (HTTP and TCP)
- Maps subdomain/ID → SSH channel
- Thread-safe (sync.RWMutex or sync.Map)
- Handles tunnel registration/deregistration
- Generates unique tunnel IDs (short random strings, e.g., `a3f8x2`)
- Assigns subdomains based on tunnel ID
- Allocates TCP ports from the pool when protocol is `"tcp"`
- Tracks per-tunnel metrics (bytes in/out, requests proxied, last request time)
- Per-tunnel IP ACLs (allow-list)  
- Persists tunnel metadata to SQLite (for resume, logging, metrics)

### 2.7 Server: SQLite Store

**Package:** `internal/store/`

- Schema managed via embedded SQL migrations
- Tables: `tokens`, `tunnels`, `connection_log`, `users`, `invite_codes`
- WAL mode for concurrent reads
- Connection pool via `database/sql`

### 2.8 Client: SSH Connection Manager

**Package:** `internal/client/connection.go`

- Dials the server at `<server>:2222`
- Authenticates with token (sent as SSH password)
- Establishes SSH session
- Requests tunnel registration via a control channel (JSON message)
- Receives tunnel assignment (subdomain, public URL, or assigned TCP port)
- Handles reconnection with exponential backoff (1s, 2s, 4s, 8s, max 30s)
- Maintains keepalive pings

### 2.9 Client: Local Proxy

**Package:** `internal/client/proxy.go`

- Accepts new channel requests from the SSH session
- **HTTP** (`"proxy"` channel): dials `localhost:<port>` and bidirectionally copies HTTP data
- **TCP** (`"tcp-proxy"` channel): dials `localhost:<port>` and bidirectionally copies raw TCP data
- Uses `io.Copy` with a buffer pool for efficiency
- Handles connection errors gracefully (return error response or close channel)

### 2.10 Client: CLI

**Package:** `cmd/client/main.go`

- Built with `cobra`
- Primary command: `fonzygrok --server <host> --token <token> --port <port> [--protocol tcp]`
- `--protocol tcp` flag for raw TCP tunnels
- `--allow-ip <cidr>` flag for IP access control
- Prints: assigned public URL (HTTP) or remote port (TCP), connection status, request log
- Signal handling: graceful shutdown on SIGINT/SIGTERM

---

## 3. Data Flow: HTTP Request Through Tunnel

```
1. Browser requests: GET http://a3f8x2.tunnel.example.com/api/users

2. DNS resolves *.tunnel.example.com → Server IP

3. Server HTTP Edge (:8080) receives request
   → Extracts Host: a3f8x2.tunnel.example.com
   → Looks up tunnel "a3f8x2" in Tunnel Manager
   → Found: tunnel is connected via SSH channel to Client

4. Server opens new SSH channel to Client
   → Writes serialized HTTP request (method, headers, body)

5. Client receives channel open request
   → Dials localhost:3000
   → Copies HTTP request to local service
   → Reads response from local service
   → Writes response back to SSH channel

6. Server reads response from SSH channel
   → Writes HTTP response to the original browser connection

7. Browser receives: 200 OK { "users": [...] }
```

---

## 4. Go Project Layout

```
fonzygrok/
├── cmd/
│   ├── server/
│   │   └── main.go              # Server entry point
│   └── client/
│       └── main.go              # Client entry point
├── internal/
│   ├── server/
│   │   ├── server.go            # Server orchestrator (wires subsystems)
│   │   ├── ssh.go               # SSH listener + auth
│   │   ├── edge.go              # HTTP edge router
│   │   ├── tunnel.go            # Tunnel manager (registry)
│   │   └── admin.go             # Admin REST API
│   ├── client/
│   │   ├── client.go            # Client orchestrator
│   │   ├── connection.go        # SSH connection + reconnect
│   │   └── proxy.go             # Local proxy (channel → localhost)
│   ├── proto/
│   │   ├── messages.go          # Control message types (JSON)
│   │   └── tunnel.go            # Tunnel metadata struct
│   ├── auth/
│   │   └── token.go             # Token generation + validation
│   └── store/
│       ├── store.go             # Database initialization + migrations
│       ├── tokens.go            # Token CRUD
│       └── tunnels.go           # Tunnel state persistence
├── migrations/
│   ├── 001_create_tokens.sql
│   ├── 002_create_tunnels.sql
│   └── 003_create_connection_log.sql
├── docker/
│   ├── Dockerfile               # Multi-stage build
│   └── docker-compose.yml       # Single-node deployment
├── Makefile
├── go.mod
├── go.sum
├── .gitignore
├── .env.example
└── README.md
```

---

## 5. Protocol Design

### 5.1 Control Channel

After SSH session establishment, the client opens an SSH channel of type `"control"`. This channel carries JSON-encoded control messages.

| Message Type | Direction | Purpose |
|:-------------|:----------|:--------|
| `TunnelRequest` | Client → Server | Request a new tunnel for a local port |
| `TunnelAssignment` | Server → Client | Assigned subdomain + public URL |
| `TunnelClose` | Either → Either | Close a specific tunnel |
| `Heartbeat` | Either → Either | Keepalive (SSH handles this natively, but app-level available) |
| `Error` | Server → Client | Error response for failed operations |

### 5.2 Data Channels

For each incoming HTTP request, the server opens an SSH channel of type `"proxy"`. The channel carries raw HTTP bytes (request → response, then channel closes).

For TCP tunnels, the server opens a `"tcp-proxy"` channel for each incoming TCP connection. Raw bytes are copied bidirectionally — no HTTP framing.

| Channel Type | Protocol | Direction | Lifecycle |
|:-------------|:---------|:----------|:----------|
| `"proxy"` | HTTP | Server → Client | One per HTTP request, closes on response |
| `"tcp-proxy"` | TCP | Server → Client | One per TCP connection, long-lived |

The `TunnelRequest` message includes a `protocol` field (`"http"` or `"tcp"`). For TCP tunnels, the `TunnelAssignment` includes an `assigned_port` field (the port the server listens on for inbound TCP).

---

## 6. Error Handling Strategy

Per GOV-004:

| Error Category | Example | Handling |
|:---------------|:--------|:---------|
| `NETWORK` | SSH connection lost | Client: reconnect with backoff. Server: deregister tunnel, log. |
| `AUTH` | Invalid token | Server: reject SSH auth, log attempt. Client: print error, exit. |
| `RESOURCE` | SQLite disk full | Server: return 503 on admin API, log critical error. |
| `VALIDATION` | Invalid tunnel request | Server: send Error message on control channel. |
| `EXTERNAL` | Local port unreachable | Client: send error response on proxy channel (502 to end user). |

---

## 7. Logging Strategy

Per GOV-006:

| Log Event | Level | Fields |
|:----------|:------|:-------|
| Client connected | INFO | `client_id`, `remote_addr`, `token_id` |
| Tunnel registered | INFO | `tunnel_id`, `subdomain`, `client_id`, `local_port` |
| HTTP request proxied | DEBUG | `tunnel_id`, `method`, `path`, `status`, `latency_ms` |
| Client disconnected | INFO | `client_id`, `reason`, `duration` |
| Auth failure | WARN | `remote_addr`, `reason` |
| Reconnect attempt | INFO | `attempt`, `backoff_ms`, `server_addr` |
| Fatal startup error | ERROR | `component`, `error`, `config` |

---

## 8. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-04-07 | 2.0.0 | v1.2: Added TCP edge, rate limiter, dashboard, IP ACL. Updated diagrams and protocol section. | Developer B |
| 2026-03-31 | 1.0.0 | Initial architecture blueprint | Architect |
