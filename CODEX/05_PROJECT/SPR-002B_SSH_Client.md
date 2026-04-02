---
id: SPR-002B
title: "Client Track — SSH Client & Control Sender"
type: how-to
status: COMPLETE
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, ssh, client]
related: [BCK-001, BLU-001, CON-001]
created: 2026-03-31
updated: 2026-04-02
version: 1.0.0
---

> **BLUF:** First parallel sprint for the Client Track. Builds the SSH client connector, client-side control channel, and local proxy. Dev B owns `internal/client/`. 3 tasks.

# Sprint 002B: Client Track — SSH Client & Control Sender

**Phase:** Phase 1 (MVP) — Parallel Track B (Client)
**Target:** Scope-bounded
**Agent(s):** Developer B (Client Track)
**Dependencies:** SPR-001 merged to main
**Contracts:** CON-001 §3 (SSH transport), CON-001 §4 (control channel), CON-001 §5 (data channel)

> **⚠️ Parallel Sprint.** Dev A is working on SPR-002A (server track) simultaneously. You own `internal/client/` and `cmd/client/`. Do NOT modify `internal/server/` or `cmd/server/`.

---

## ⚠️ Mandatory Compliance — Every Task

| Governance Doc | Sprint Requirement |
|:---------------|:-------------------|
| **GOV-002** | Test files for all client code. Use in-process test SSH server for integration tests. |
| **GOV-003** | No global state. Config via dependency injection. |
| **GOV-004** | Connection errors wrapped with context. Local dial failures → 502. |
| **GOV-005** | Branch: `feature/SPR-002B-ssh-client`. |

---

## Testing Strategy

Since the real SSH server (SPR-002A) is being built in parallel, use a **minimal test SSH server** in your tests. Create a `internal/client/testutil_test.go` with a helper that starts an in-process `crypto/ssh` server that:
- Accepts token auth (validates against a known test token)
- Accepts `"control"` channel type
- Accepts `"proxy"` channel type
- Echoes back whatever it receives on proxy channels

This lets you fully test the client without depending on Dev A's server implementation.

---

## Developer Tasks

### T-006B: SSH Client Connector

- **Dependencies:** T-002 (proto types from SPR-001)
- **Contracts:** CON-001 §3
- **Deliverable:**
  - `internal/client/connection.go` — `Connector` struct:
    - `New(config ClientConfig, logger *slog.Logger) *Connector`
    - `Connect(ctx context.Context) error` — dial server, authenticate
    - `Close() error` — clean disconnect
    - `Session() *ssh.Session` — access underlying SSH session
    - `IsConnected() bool`
  - `ClientConfig` struct: `ServerAddr string`, `Token string`, `Insecure bool`
  - Auth: SSH password auth, username `"fonzygrok"`, password = token (per CON-001 §3.2)
  - Host key: accept on first connect, store in `~/.fonzygrok/known_hosts`. Warn on mismatch. Skip verification if `Insecure: true`.
  - `internal/client/connection_test.go` — connect, auth success/failure, clean close tests (using test SSH server)
- **Acceptance criteria:**
  - Client dials and authenticates with valid token
  - Client gets rejected with invalid token (error returned)
  - `Close()` cleanly terminates the SSH session
  - `IsConnected()` reflects actual state
- **Status:** [ ] Not Started

### T-007B: Client-Side Control Channel

- **Dependencies:** T-006B
- **Contracts:** CON-001 §4
- **Deliverable:**
  - Extend `internal/client/connection.go`:
    - `OpenControl() (*ControlChannel, error)` — open `"control"` channel
  - `internal/client/control.go` — `ControlChannel` struct:
    - `RequestTunnel(localPort int, protocol string) (*proto.TunnelAssignment, error)` — send `TunnelRequest`, read response
    - `CloseTunnel(tunnelID string) error` — send `TunnelClose`
    - `Close() error`
  - Uses `proto.Encoder`/`proto.Decoder` for JSON messaging
  - Handle `Error` responses from server — convert to Go errors
  - `internal/client/control_test.go` — request/response tests, error handling tests
- **Acceptance criteria:**
  - `RequestTunnel()` sends properly formatted `TunnelRequest`, reads `TunnelAssignment`
  - Server error responses are converted to descriptive Go errors
  - Control channel uses newline-delimited JSON per CON-001 §4.2
- **Status:** [ ] Not Started

### T-008B: Client Local Proxy

- **Dependencies:** T-006B
- **Contracts:** CON-001 §5
- **Deliverable:**
  - `internal/client/proxy.go` — `LocalProxy` struct:
    - `New(localPort int, logger *slog.Logger) *LocalProxy`
    - `HandleChannels(ctx context.Context, channelChan <-chan ssh.NewChannel)` — accept `"proxy"` channels
  - For each incoming `"proxy"` channel:
    1. Accept the channel
    2. Dial `localhost:<localPort>`
    3. Bidirectional `io.Copy` with pooled buffers (`sync.Pool`)
    4. Close channel when copy completes
  - On local port unreachable: write HTTP 502 response per CON-001 §5.4, close channel
  - `internal/client/proxy_test.go` — test with local HTTP test server:
    - Successful round-trip (request → local → response)
    - Port unreachable → 502 response
    - Multiple concurrent channels
- **Acceptance criteria:**
  - Proxy channel data flows bidirectionally to localhost
  - Unreachable port → 502 response written back
  - Buffer pool used (no per-request allocation)
  - Multiple concurrent proxy channels work
  - `go test -race` passes
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-006B | Dev B | [ ] | [ ] |
| T-007B | Dev B | [ ] | [ ] |
| T-008B | Dev B | [ ] | [ ] |

---

## File Ownership (Conflict Prevention)

| Path | Owner |
|:-----|:------|
| `internal/client/*` | ✅ Dev B (this sprint) |
| `internal/server/*` | ❌ Dev A only |
| `internal/proto/*` | ❌ Read-only (defined in SPR-001) |
| `internal/auth/*` | ❌ Read-only (defined in SPR-001) |
| `internal/store/*` | ❌ Dev A only |
| `cmd/client/*` | ✅ Dev B |
| `cmd/server/*` | ❌ Dev A only |

---

## Sprint Completion Criteria

- [ ] Client connects/disconnects to test SSH server
- [ ] Control channel sends requests and reads responses
- [ ] Proxy handles bidirectional copy with buffer pool
- [ ] 502 returned for unreachable local ports
- [ ] `go test -race ./internal/client/...` passes
- [ ] No modifications to `internal/server/` or `cmd/server/`
- [ ] No open `DEF-` reports

---

## Audit Notes (Architect)

**Verdict:** PENDING
**Deploy approved:** N/A
