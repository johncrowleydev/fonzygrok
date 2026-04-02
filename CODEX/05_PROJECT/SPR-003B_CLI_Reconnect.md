---
id: SPR-003B
title: "Client Track — CLI, Auto-Reconnect & Logging"
type: how-to
status: COMPLETE
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, cli, reconnect, client]
related: [BCK-001, BLU-001, CON-001]
created: 2026-03-31
updated: 2026-04-02
version: 1.0.0
---

> **BLUF:** Second parallel sprint for Client Track. Builds the cobra CLI, auto-reconnect with exponential backoff, structured logging, and graceful shutdown. After this sprint, the client binary is feature-complete. 3 tasks.

# Sprint 003B: Client Track — CLI, Auto-Reconnect & Logging

**Phase:** Phase 1 (MVP) — Parallel Track B (Client)
**Target:** Scope-bounded
**Agent(s):** Developer B (Client Track)
**Dependencies:** SPR-002B (SSH client + proxy)
**Contracts:** CON-001 §3.1 (keepalive/reconnect), BLU-001 §7 (logging)

> **⚠️ Parallel Sprint.** Dev A is working on SPR-003A simultaneously. Same file ownership rules apply.

---

## Developer Tasks

### T-009B: Client CLI (cobra)

- **Dependencies:** T-006B, T-007B, T-008B
- **Contracts:** BLU-001 §2.8
- **Deliverable:**
  - `cmd/client/main.go` — rewrite with cobra:
    - Root command: `fonzygrok` — connects to server and starts tunnel
    - Flags: `--server` (required, `FONZYGROK_SERVER`), `--token` (required, `FONZYGROK_TOKEN`), `--port` (required), `--insecure` (skip host key verification)
    - On connect: print assigned public URL and status
    - On disconnect: print reconnecting status
    - On SIGINT/SIGTERM: graceful shutdown
  - `--version` flag (set via ldflags at build time)
  - `--help` with usage examples
- **Acceptance criteria:**
  - `fonzygrok --server localhost:2222 --token fgk_xxx --port 3000` connects (using test server)
  - Missing required flags → clear error message
  - `--version` prints version
  - `--help` shows usage with examples
  - SIGINT triggers clean shutdown
- **Status:** [ ] Not Started

### T-010B: Auto-Reconnect

- **Dependencies:** T-006B
- **Contracts:** CON-001 §3.1
- **Deliverable:**
  - Extend `internal/client/connection.go`:
    - `ConnectWithRetry(ctx context.Context) error` — retry loop
    - Exponential backoff: 1s, 2s, 4s, 8s, 16s, cap at 30s
    - Reset backoff on successful connection
    - Detect disconnect: SSH session close, keepalive failure, io.EOF
    - After reconnect: re-open control channel, re-request tunnels
    - Log each attempt: `slog.Info("reconnecting", "attempt", n, "backoff_ms", ms)`
  - `internal/client/connection_test.go` (extend):
    - Test backoff progression (1, 2, 4, 8, 16, 30, 30...)
    - Test reconnect after server restart (stop test server → restart → client reconnects)
    - Test context cancellation stops retry loop
- **Acceptance criteria:**
  - Backoff follows 1s, 2s, 4s, 8s, 16s, 30s, 30s...
  - Successful connect resets backoff to 1s
  - Context cancellation cleanly stops retries
  - Tunnels re-registered after reconnect
  - Each attempt logged with structured fields
- **Status:** [ ] Not Started

### T-011B: Structured Logging & Graceful Shutdown

- **Dependencies:** T-009B
- **Contracts:** BLU-001 §7, GOV-006
- **Deliverable:**
  - Initialize `slog.Logger` with `slog.NewJSONHandler(os.Stdout, nil)` in `cmd/client/main.go`
  - Pass logger via dependency injection to `Connector`, `ControlChannel`, `LocalProxy`
  - Log events per BLU-001 §7:
    - `INFO` "connected" — `server_addr`, `tunnel_id`, `public_url`
    - `INFO` "disconnected" — `reason`, `duration`
    - `INFO` "reconnecting" — `attempt`, `backoff_ms`
    - `DEBUG` "request_proxied" — `tunnel_id`, `local_port`, `bytes`
    - `WARN` "local_port_unreachable" — `port`, `error`
  - Graceful shutdown:
    - Catch SIGINT/SIGTERM via `signal.NotifyContext`
    - Close SSH session → close proxy channels → exit
  - `internal/client/` — ensure all exported constructors accept `*slog.Logger`
- **Acceptance criteria:**
  - All log output is valid JSON lines
  - Log events match BLU-001 §7 schema
  - SIGINT → clean shutdown, no panic
  - Logger is never nil (validated in constructors)
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-009B | Dev B | [ ] | [ ] |
| T-010B | Dev B | [ ] | [ ] |
| T-011B | Dev B | [ ] | [ ] |

---

## Sprint Completion Criteria

- [ ] CLI works with all flags and environment variables
- [ ] Auto-reconnect with correct backoff sequence
- [ ] All logs are structured JSON
- [ ] Graceful shutdown on SIGINT
- [ ] `go test -race ./internal/client/... ./cmd/client/...` passes
- [ ] No modifications to server-owned files

---

## Audit Notes (Architect)

**Verdict:** PENDING
