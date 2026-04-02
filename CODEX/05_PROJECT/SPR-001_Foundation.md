---
id: SPR-001
title: "Foundation — Scaffold, Storage & Auth"
type: how-to
status: COMPLETE
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, scaffolding, storage, auth]
related: [BCK-001, BLU-001, CON-001, CON-002]
created: 2026-03-31
updated: 2026-04-02
version: 2.0.0
---

> **BLUF:** Sprint 001 creates the project foundation that BOTH developer tracks depend on: Go module, directory structure, Makefile, shared protocol types, SQLite storage, and token authentication. This is the SERIAL phase — one developer executes, the other waits. After SPR-001 merges to main, both tracks run in parallel. 4 tasks.

# Sprint 001: Foundation — Scaffold, Storage & Auth

**Phase:** Phase 1 (MVP) — Serial Foundation
**Target:** Scope-bounded
**Agent(s):** Developer A (Server Track) — first sprint only
**Dependencies:** None (first sprint)
**Contracts:** CON-001 §8 (Go types), CON-002 §4.3 (token format)

---

## ⚠️ Mandatory Compliance — Every Task

| Governance Doc | Sprint Requirement |
|:---------------|:-------------------|
| **GOV-001** | GoDoc on all exported types. README with project overview. |
| **GOV-002** | Test files for all packages. `go test ./...` passes. |
| **GOV-003** | Go project layout per BLU-001 §4. No global state. |
| **GOV-004** | All DB errors wrapped with context. No silent failures. |
| **GOV-005** | Branch: `feature/SPR-001-foundation`. Commits: `feat(SPR-001): T-XXX description`. |
| **GOV-008** | `.env.example` created. SQLite at configurable path. WAL mode. |

---

## Developer Tasks

### T-001: Go Project Scaffold

- **Dependencies:** None
- **Contracts:** BLU-001 §4
- **Deliverable:**
  - `go.mod` with module name `github.com/fonzygrok/fonzygrok`
  - Directory structure per BLU-001 §4: `cmd/server/`, `cmd/client/`, `internal/server/`, `internal/client/`, `internal/proto/`, `internal/auth/`, `internal/store/`, `migrations/`, `docker/`
  - `Makefile` with targets: `build-server`, `build-client`, `build` (both), `test`, `lint`, `clean`
  - `.gitignore` (Go defaults + `/data/`, `.env`, `dist/`, `fonzygrok-server`, `fonzygrok`)
  - `.env.example` with: `FONZYGROK_DATA_DIR=/data`, `FONZYGROK_DOMAIN=tunnel.example.com`, `FONZYGROK_SSH_ADDR=:2222`, `FONZYGROK_HTTP_ADDR=:8080`, `FONZYGROK_ADMIN_ADDR=:9090`
  - `README.md` with project overview, build instructions, usage example
  - Minimal `cmd/server/main.go` and `cmd/client/main.go` (prints version, exits)
- **Acceptance criteria:**
  - `go build ./cmd/server/` and `go build ./cmd/client/` succeed
  - `make test` runs without error
  - `make lint` runs `go vet ./...` without error
  - All directories exist per BLU-001 §4
- **Status:** [ ] Not Started

### T-002: Shared Protocol Types

- **Dependencies:** T-001
- **Contracts:** CON-001 §8
- **Deliverable:**
  - `internal/proto/messages.go` — `ControlMessage`, `TunnelRequest`, `TunnelAssignment`, `TunnelClose`, `ErrorMessage` structs matching CON-001 §8 exactly
  - `internal/proto/tunnel.go` — `TunnelState` enum constants (`Requested`, `Assigned`, `Active`, `Closed`)
  - `internal/proto/codec.go` — `Encoder` and `Decoder` for newline-delimited JSON over `io.ReadWriter`
  - `internal/proto/messages_test.go` — JSON round-trip tests for all message types
  - `internal/proto/codec_test.go` — multi-message encode/decode tests
- **Acceptance criteria:**
  - All Go types match CON-001 §8 exactly (field names, JSON tags, types)
  - Round-trip JSON marshal/unmarshal passes for every message type
  - Codec handles multiple messages in sequence
  - `go vet ./internal/proto/` passes
- **Status:** [ ] Not Started

### T-003: SQLite Store Foundation

- **Dependencies:** T-001
- **Contracts:** BLU-001 §2.5, GOV-008 §2
- **Deliverable:**
  - `internal/store/store.go` — `Store` struct with `New(dbPath string) (*Store, error)`, `Close() error`, `Migrate() error`
  - `migrations/001_create_tokens.sql` — tokens table: `id TEXT PK`, `name TEXT`, `token_hash TEXT UNIQUE`, `created_at TEXT`, `last_used_at TEXT`, `is_active INTEGER`
  - `migrations/002_create_tunnels.sql` — tunnels table: `tunnel_id TEXT PK`, `subdomain TEXT`, `protocol TEXT`, `token_id TEXT`, `client_ip TEXT`, `local_port INTEGER`, `connected_at TEXT`, `disconnected_at TEXT`, `bytes_in INTEGER`, `bytes_out INTEGER`, `requests_proxied INTEGER`
  - `migrations/003_create_connection_log.sql` — connection_log table: `id INTEGER PK AUTOINCREMENT`, `token_id TEXT`, `client_ip TEXT`, `event TEXT`, `details TEXT`, `created_at TEXT`
  - Migrations embedded via `embed.FS`
  - `internal/store/store_test.go` — init, migrate, close tests
- **Acceptance criteria:**
  - `New()` creates SQLite database with WAL mode
  - `Migrate()` runs all embedded migrations idempotently
  - Tests use `:memory:` database
  - All tables exist after migration
- **Status:** [ ] Not Started

### T-004: Token Authentication

- **Dependencies:** T-003
- **Contracts:** CON-001 §3.2, CON-002 §4.3
- **Deliverable:**
  - `internal/auth/token.go` — `GenerateToken() (id, rawToken string)`, `HashToken(raw string) string`
  - `internal/store/tokens.go` — `CreateToken(name string) (*Token, string, error)`, `ValidateToken(rawToken string) (*Token, error)`, `ListTokens() ([]Token, error)`, `DeleteToken(id string) error`, `UpdateLastUsed(id string) error`
  - Token ID format: `tok_` + 12 random hex chars
  - Token format: `fgk_` + 32 random lowercase alphanumeric chars
  - Storage: SHA-256 hash of raw token
  - `internal/auth/token_test.go` — format, uniqueness, hash determinism tests
  - `internal/store/tokens_test.go` — full CRUD tests with happy + error paths
- **Acceptance criteria:**
  - Token format matches CON-002 §4.3 exactly
  - `ValidateToken(raw)` hashes and matches against stored hash
  - `CreateToken` returns raw token exactly once (not stored in plaintext)
  - `DeleteToken` for non-existent ID returns descriptive error
  - All tests pass with `-race`
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-001 | Dev A | [ ] | [ ] |
| T-002 | Dev A | [ ] | [ ] |
| T-003 | Dev A | [ ] | [ ] |
| T-004 | Dev A | [ ] | [ ] |

---

## Post-Sprint Actions

After this sprint merges to `main`:
1. **Dev B pulls main** and begins SPR-002B (client track)
2. **Dev A** begins SPR-002A (server track)
3. Both tracks run in parallel from this point

---

## Sprint Completion Criteria

- [ ] All tasks pass acceptance criteria
- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` passes
- [ ] Merged to `main`
- [ ] Architect audit complete
- [ ] Both Dev A and Dev B can pull and build

---

## Audit Notes (Architect)

**Verdict:** PENDING
**Deploy approved:** N/A
