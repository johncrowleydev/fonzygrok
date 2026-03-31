---
id: SPR-004A
title: "Server Track — Admin API & Server Orchestrator"
type: how-to
status: PLANNING
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, api, server, orchestrator]
related: [BCK-001, BLU-001, CON-002]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** Final server-track sprint. Builds the admin REST API, wires all server subsystems together into a single orchestrator, adds structured logging and graceful shutdown. After this sprint, `fonzygrok-server` is feature-complete. 3 tasks.

# Sprint 004A: Server Track — Admin API & Server Orchestrator

**Phase:** Phase 1 (MVP) — Parallel Track A (Server)
**Target:** Scope-bounded
**Agent(s):** Developer A (Server Track)
**Dependencies:** SPR-003A (HTTP edge router)
**Contracts:** CON-002 §4 (admin API)

---

## Developer Tasks

### T-013A: Admin API

- **Dependencies:** T-003 (store), T-007A (tunnel manager)
- **Contracts:** CON-002 §4
- **Deliverable:**
  - `internal/server/admin.go` — `AdminAPI` struct:
    - `New(config AdminConfig, store *store.Store, tunnels *TunnelManager, logger *slog.Logger) *AdminAPI`
    - `Start(ctx context.Context) error` — listen on admin address
    - `Stop() error`
  - Endpoints per CON-002 §4.3:
    - `GET /api/v1/health` — health check with uptime, tunnel count, client count
    - `GET /api/v1/tokens` — list all tokens
    - `POST /api/v1/tokens` — create token (returns raw token once)
    - `DELETE /api/v1/tokens/:id` — revoke token, disconnect active tunnels
    - `GET /api/v1/tunnels` — list active tunnels
    - `DELETE /api/v1/tunnels/:tunnel_id` — force-close tunnel
  - Error responses per CON-002 §5
  - `internal/server/admin_test.go` — `httptest` tests for all endpoints
- **Acceptance criteria:**
  - All responses match CON-002 §4.3 schemas exactly
  - Token creation returns raw token only once
  - Token deletion disconnects tunnels using that token
  - Invalid input → 400 with `validation_error`
  - Missing resource → 404 with `not_found`
- **Status:** [ ] Not Started

### T-014A: Server Orchestrator

- **Dependencies:** T-005, T-009A, T-013A
- **Deliverable:**
  - `internal/server/server.go` — `Server` struct:
    - `New(config ServerConfig, logger *slog.Logger) (*Server, error)`
    - `Start(ctx context.Context) error` — starts ALL subsystems: SSH listener, HTTP edge, Admin API
    - `Stop() error` — graceful shutdown in order: stop accepting new connections → drain active tunnels → close SSH → close HTTP → close admin → close store
  - `ServerConfig` struct: embeds `SSHConfig`, `EdgeConfig`, `AdminConfig`, plus `DataDir string`, `Domain string`
  - Wire subsystems: SSH server → tunnel manager → edge router → admin API → store
  - `internal/server/server_test.go` — start/stop lifecycle test
- **Acceptance criteria:**
  - `Server.Start()` brings up all subsystems
  - `Server.Stop()` shuts down cleanly (no goroutine leaks)
  - Logs startup with all listen addresses
- **Status:** [ ] Not Started

### T-015A: Server CLI & Logging

- **Dependencies:** T-014A
- **Deliverable:**
  - `cmd/server/main.go` — rewrite with cobra:
    - `serve` subcommand: starts the server (flags for all addresses, data dir, domain)
    - `token create --name <name>` — create token via store directly
    - `token list` — list tokens
    - `token revoke <id>` — revoke token
    - `--version` flag
  - `slog.NewJSONHandler(os.Stdout, nil)` for structured logging
  - Signal handling: SIGINT/SIGTERM → `Server.Stop()`
  - Log events per BLU-001 §7 (server-side events)
- **Acceptance criteria:**
  - `fonzygrok-server serve` starts all subsystems
  - `fonzygrok-server token create --name test` creates and prints token
  - `fonzygrok-server token list` lists tokens
  - SIGINT → graceful shutdown
  - All logs valid JSON
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-013A | Dev A | [ ] | [ ] |
| T-014A | Dev A | [ ] | [ ] |
| T-015A | Dev A | [ ] | [ ] |

---

## Sprint Completion Criteria

- [ ] Admin API all endpoints tested
- [ ] Server orchestrator starts/stops all subsystems
- [ ] Server CLI with token management commands
- [ ] All logs structured JSON
- [ ] `go test -race ./internal/server/... ./cmd/server/...` passes
- [ ] `fonzygrok-server` binary is feature-complete for v1.0

---

## Audit Notes (Architect)

**Verdict:** PENDING
