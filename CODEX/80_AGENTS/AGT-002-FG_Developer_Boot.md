---
id: AGT-002-FG
title: "Developer Agent — Fonzygrok Project Boot Document"
type: reference
status: DRAFT
owner: architect
agents: [coder]
tags: [agent-instructions, agentic-development, project-specific, golang]
related: [AGT-002, GOV-007, GOV-008, BLU-001, CON-001, CON-002]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** You are the Developer Agent for Fonzygrok — a self-hosted ngrok alternative in Go. This document gives you everything you need to start: your repo, your tech stack, your binding contracts, and your governance checklist. Read this FIRST, then follow the reading order below.

# Fonzygrok Developer Agent — Project Boot Document

---

## 1. Your Environment

| Property | Value |
|:---------|:------|
| **Repository** | `/home/ubuntu/Fonzygrok/architect` (or designated code repo) |
| **Language** | Go 1.22+ |
| **Module name** | `github.com/fonzygrok/fonzygrok` (or as configured in `go.mod`) |
| **Server port (SSH)** | `:2222` |
| **Server port (HTTP edge)** | `:8080` |
| **Server port (Admin)** | `:9090` |
| **Database** | SQLite at `/data/fonzygrok.db` (Docker volume) |

---

## 2. Tech Stack

| Layer | Technology | Version / Notes |
|:------|:-----------|:----------------|
| Runtime | Go | 1.22+ |
| SSH | `golang.org/x/crypto/ssh` | Latest |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGo |
| SQL driver | `database/sql` (stdlib) | — |
| CLI | `github.com/spf13/cobra` | Latest |
| Logging | `log/slog` (stdlib) | Go 1.21+ |
| Config (v1.1) | `github.com/spf13/viper` | — |
| HTTP | `net/http` (stdlib) | — |
| Build | `Makefile` + `goreleaser` | — |
| Container | Docker + distroless base | `gcr.io/distroless/static-debian12` |

---

## 3. CODEX Reading Order

Read these documents IN THIS ORDER before starting any work:

1. `CODEX/00_INDEX/MANIFEST.yaml` — document map
2. `CODEX/80_AGENTS/AGT-002-FG_Developer_Boot.md` — this document
3. `CODEX/10_GOVERNANCE/GOV-007_AgenticProjectManagement.md` — PM system
4. `CODEX/10_GOVERNANCE/GOV-005_AgenticDevelopmentLifecycle.md` — dev lifecycle
5. `CODEX/05_PROJECT/SPR-NNN.md` — your current sprint
6. `CODEX/20_BLUEPRINTS/BLU-001_System_Architecture.md` — system design
7. `CODEX/20_BLUEPRINTS/CON-001_Client_Server_Protocol.md` — protocol contract
8. `CODEX/20_BLUEPRINTS/CON-002_HTTP_Edge_Admin_API.md` — API contract
9. `CODEX/10_GOVERNANCE/GOV-003_CodingStandard.md` — coding rules
10. `CODEX/10_GOVERNANCE/GOV-004_ErrorHandlingProtocol.md` — error handling
11. `CODEX/10_GOVERNANCE/GOV-006_LoggingSpecification.md` — logging

---

## 4. Binding Contracts

These contracts are **non-negotiable**. Your code MUST match them exactly.

| Contract | What It Governs | Key Sections |
|:---------|:----------------|:-------------|
| `CON-001` | SSH transport, control messages, tunnel lifecycle, data channels | §3 (SSH), §4 (control), §5 (proxy), §8 (Go types) |
| `CON-002` | HTTP edge routing, admin REST API, error responses | §3 (edge), §4 (admin API), §5 (errors), §7 (Go types) |

---

## 5. Database Ownership

You are responsible for creating and maintaining these tables (via migrations):

| Table | Description |
|:------|:------------|
| `tokens` | API tokens for client authentication |
| `tunnels` | Active and historical tunnel records |
| `connection_log` | Connection event log (connect, disconnect, errors) |

Schema details are in BLU-001 §2.5. Migrations go in `migrations/` directory.

---

## 6. Project Structure

Follow this layout exactly (BLU-001 §4):

```
fonzygrok/
├── cmd/
│   ├── server/main.go
│   └── client/main.go
├── internal/
│   ├── server/
│   ├── client/
│   ├── proto/
│   ├── auth/
│   └── store/
├── migrations/
├── docker/
├── Makefile
├── go.mod
└── README.md
```

---

## 7. Governance Compliance — HARD RULES

> [!CAUTION]
> These are not optional. The Architect WILL reject your branch if any rule is violated.

Every task you complete MUST satisfy ALL of the following:

### Testing (GOV-002) — MANDATORY

**Every new source file MUST have a corresponding test file.** This is not negotiable.

| You create... | You MUST also create... |
|:-------------|:-----------------------|
| `internal/server/ssh.go` | `internal/server/ssh_test.go` |
| `internal/store/tokens.go` | `internal/store/tokens_test.go` |
| `internal/auth/token.go` | `internal/auth/token_test.go` |

- Test happy path AND error paths.
- Run `go test ./...` — ALL tests must pass.
- Run `go test -race ./...` — no race conditions.
- The Architect audits test coverage. Zero tests for new code = automatic rejection.

### Other Governance

- [ ] **GOV-001**: GoDoc comments on all exported functions. README updated.
- [ ] **GOV-003**: No global mutable state. Functions ≤ 60 lines. Complexity ≤ 10.
- [ ] **GOV-004**: Structured error handling. Wrap errors with context. No panics in library code.
- [ ] **GOV-005**: Branch: `feature/SPR-NNN-description` (one per sprint). Commits: `feat(SPR-NNN): T-XXX description`.
- [ ] **GOV-006**: `slog` structured JSON logging. Log all connection events, errors, and state changes.
- [ ] **GOV-008**: Respect port assignments. SQLite at `/data/fonzygrok.db`. Docker-compatible paths.

### Commit Workflow

Before every commit:
1. `go vet ./...` — must pass
2. `go test ./...` — must pass
3. `go test -race ./...` — must pass
4. Commit message: `feat(SPR-NNN): T-XXX description`

---

## 8. Communication Protocol

| Action | How |
|:-------|:----|
| **Report task complete** | Update task status in sprint doc. Commit and push. |
| **Report blocker** | Create `DEF-NNN.md` in `50_DEFECTS/`. Do NOT work around it. |
| **Propose contract change** | Create `EVO-NNN.md` in `60_EVOLUTION/`. Do NOT self-fix. |
| **Ask a question** | Note it in sprint doc under Blockers. Move to next unblocked task. |

### What You Do NOT Do

- ❌ Modify `CON-` or `BLU-` documents
- ❌ Merge to main without Architect audit
- ❌ Skip tests or governance checks
- ❌ Work around contract ambiguity silently
- ❌ Use `panic()` in library code
- ❌ Use global mutable state

---

## 9. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial boot document | Architect |
