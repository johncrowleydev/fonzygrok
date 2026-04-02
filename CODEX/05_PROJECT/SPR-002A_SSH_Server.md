---
id: SPR-002A
title: "Server Track — SSH Server & Control Handler"
type: how-to
status: COMPLETE
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, ssh, server]
related: [BCK-001, BLU-001, CON-001]
created: 2026-03-31
updated: 2026-04-02
version: 1.0.0
---

> **BLUF:** First parallel sprint for the Server Track. Builds the SSH server listener with token authentication and the server-side control channel handler. Dev A owns `internal/server/`. 3 tasks.

# Sprint 002A: Server Track — SSH Server & Control Handler

**Phase:** Phase 1 (MVP) — Parallel Track A (Server)
**Target:** Scope-bounded
**Agent(s):** Developer A (Server Track)
**Dependencies:** SPR-001 merged to main
**Contracts:** CON-001 §3 (SSH transport), CON-001 §4 (control channel)

> **⚠️ Parallel Sprint.** Dev B is working on SPR-002B (client track) simultaneously. You own `internal/server/` and `cmd/server/`. Do NOT modify `internal/client/` or `cmd/client/`.

---

## ⚠️ Mandatory Compliance — Every Task

| Governance Doc | Sprint Requirement |
|:---------------|:-------------------|
| **GOV-002** | Test files for all SSH + control code. Integration tests with test clients. |
| **GOV-003** | No global state. SSH config via dependency injection. |
| **GOV-004** | Auth failures logged as WARN. SSH errors wrapped with context. |
| **GOV-005** | Branch: `feature/SPR-002A-ssh-server`. |
| **GOV-008** | Host key at configurable path (default `/data/host_key`). Port `:2222`. |

---

## Developer Tasks

### T-005: SSH Server Listener

- **Dependencies:** T-004 (token validation from SPR-001)
- **Contracts:** CON-001 §3
- **Deliverable:**
  - `internal/server/ssh.go` — `SSHServer` struct:
    - `New(config SSHConfig, store *store.Store, logger *slog.Logger) *SSHServer`
    - `Start(ctx context.Context) error` — listen on configured address
    - `Stop() error` — graceful shutdown
    - `OnNewSession(fn func(Session))` — callback for new authenticated sessions
  - `SSHConfig` struct: `Addr string`, `HostKeyPath string`, `MaxConnections int`
  - ED25519 host key: generate on first start, load from file on subsequent starts
  - Auth callback: extract token from SSH password field → `store.ValidateToken()` → accept/reject
  - Keepalive: 30s interval, 3 missed → disconnect (per CON-001 §3.1)
  - `internal/server/ssh_test.go` — host key generation, auth accept/reject, keepalive config tests
- **Acceptance criteria:**
  - Server listens on configured address
  - Valid token → SSH session established
  - Invalid token → auth rejected, WARN logged
  - Host key persists across restarts
  - Multiple concurrent connections accepted
- **Status:** [ ] Not Started

### T-006A: Server-Side Control Channel

- **Dependencies:** T-005
- **Contracts:** CON-001 §4
- **Deliverable:**
  - Extend `internal/server/ssh.go` — accept `"control"` channel type per session
  - `internal/server/control.go` — `ControlHandler` struct:
    - Accept one control channel per SSH session
    - Reject additional control channel requests
    - Read `ControlMessage` from channel using `proto.Decoder`
    - Dispatch by message type: `tunnel_request`, `tunnel_close`
    - Write responses (`TunnelAssignment`, `Error`) using `proto.Encoder`
  - `internal/server/control_test.go` — dispatch tests, reject-duplicate tests, message round-trip tests
- **Acceptance criteria:**
  - One control channel per session, second rejected
  - Messages decode correctly per CON-001 §4.2
  - Dispatch routes to correct handler function
  - Unknown message types return `Error` with `INTERNAL_ERROR` code
- **Status:** [ ] Not Started

### T-007A: Tunnel Manager

- **Dependencies:** T-006A
- **Contracts:** CON-001 §4.3, §6
- **Deliverable:**
  - `internal/server/tunnel.go` — `TunnelManager` struct:
    - `New(domain string, store *store.Store, logger *slog.Logger) *TunnelManager`
    - `Register(session SSHSession, req *proto.TunnelRequest) (*proto.TunnelAssignment, error)`
    - `Lookup(tunnelID string) (*TunnelEntry, bool)`
    - `Deregister(tunnelID string)`
    - `DeregisterBySession(session SSHSession)` — cleanup when client disconnects
    - `ListActive() []TunnelEntry`
    - `OpenProxyChannel(tunnelID string) (ssh.Channel, error)` — open `"proxy"` channel to client
  - `TunnelEntry` struct: session reference, tunnel metadata, created timestamp
  - Tunnel ID: 6 random lowercase alphanumeric chars (unique, retry on collision)
  - Thread-safe: `sync.RWMutex`
  - Wire into `ControlHandler`: `tunnel_request` → `TunnelManager.Register()` → respond with assignment
  - `internal/server/tunnel_test.go` — register, lookup, deregister, concurrent access, session cleanup tests
- **Acceptance criteria:**
  - Register returns `TunnelAssignment` per CON-001 §4.3
  - Concurrent register/lookup safely handled (test with `-race`)
  - Deregister by session cleans up all tunnels for that client
  - `OpenProxyChannel` opens SSH channel type `"proxy"` with tunnel ID as extra data
  - Tunnel IDs are unique (no collisions in 10000 generation test)
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-005 | Dev A | [ ] | [ ] |
| T-006A | Dev A | [ ] | [ ] |
| T-007A | Dev A | [ ] | [ ] |

---

## File Ownership (Conflict Prevention)

| Path | Owner |
|:-----|:------|
| `internal/server/*` | ✅ Dev A (this sprint) |
| `internal/client/*` | ❌ Dev B only |
| `internal/proto/*` | ❌ Read-only (defined in SPR-001) |
| `internal/auth/*` | ❌ Read-only (defined in SPR-001) |
| `internal/store/*` | ✅ Dev A may extend with new methods |
| `cmd/server/*` | ✅ Dev A |
| `cmd/client/*` | ❌ Dev B only |

---

## Sprint Completion Criteria

- [ ] SSH server accepts/rejects connections based on token
- [ ] Control channel handles tunnel_request → tunnel_assignment flow
- [ ] Tunnel manager registers/deregisters with thread safety
- [ ] `go test -race ./internal/server/...` passes
- [ ] No modifications to `internal/client/` or `cmd/client/`
- [ ] No open `DEF-` reports

---

## Audit Notes (Architect)

**Verdict:** PENDING
**Deploy approved:** N/A
