---
id: AGT-002-FG-B
title: "Developer B (Client Track) — Fonzygrok Boot Document"
type: reference
status: DRAFT
owner: architect
agents: [coder]
tags: [agent-instructions, agentic-development, project-specific, golang, client]
related: [AGT-002, AGT-002-FG, GOV-007, BLU-001, CON-001]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** You are Developer B — the **Client Track** developer for Fonzygrok. You own `internal/client/` and `cmd/client/`. You will NOT touch `internal/server/`, `internal/store/`, or `cmd/server/`. Dev A is building the server in parallel — stay in your lane.

# Developer B (Client Track) — Boot Document

---

## 1. Your Track

| Property | Value |
|:---------|:------|
| **Track** | Client (Track B) |
| **Parallel with** | Developer A (Server Track) |
| **You OWN** | `internal/client/`, `cmd/client/` |
| **You DO NOT TOUCH** | `internal/server/`, `internal/store/`, `cmd/server/` |
| **Shared (read-only)** | `internal/proto/`, `internal/auth/` (written in SPR-001) |

---

## 2. Your Sprint Sequence

| Sprint | Branch | What You Build |
|:-------|:-------|:---------------|
| **SPR-002B** | `feature/SPR-002B-ssh-client` | SSH client connector, control channel (client side), local proxy |
| **SPR-003B** | `feature/SPR-003B-cli-reconnect` | cobra CLI, auto-reconnect with backoff, structured logging |
| **SPR-005** | `feature/SPR-005-integration` | (MERGE) Integration, Docker, E2E (shared with Dev A) |

> **Note:** You start AFTER SPR-001 is merged to main by Dev A. Pull main, then begin SPR-002B.

---

## 3. Environment & Tech Stack

Same as [AGT-002-FG](file:///home/ubuntu/Fonzygrok/architect/CODEX/80_AGENTS/AGT-002-FG_Developer_Boot.md) §1–§2. Read that document for full details.

---

## 4. CODEX Reading Order

1. This document (AGT-002-FG-B)
2. `CODEX/05_PROJECT/SPR-002B_SSH_Client.md` — your first sprint
3. `CODEX/20_BLUEPRINTS/BLU-001_System_Architecture.md` — system design
4. `CODEX/20_BLUEPRINTS/CON-001_Client_Server_Protocol.md` — SSH protocol (you implement the client side)
5. `CODEX/10_GOVERNANCE/GOV-003_CodingStandard.md` through `GOV-006`

---

## 5. Key Client Components You Build

| Component | File | Contract |
|:----------|:-----|:---------|
| SSH connector + auth | `internal/client/connection.go` | CON-001 §3 |
| Control channel (client) | `internal/client/control.go` | CON-001 §4 |
| Local proxy | `internal/client/proxy.go` | CON-001 §5 |
| Auto-reconnect | `internal/client/connection.go` | CON-001 §3.1 |
| Client CLI | `cmd/client/main.go` | BLU-001 §2.8 |

---

## 6. Testing Strategy

Since the real server is built by Dev A in parallel, you MUST create a **test SSH server** helper:

```go
// internal/client/testutil_test.go
func startTestSSHServer(t *testing.T, validToken string) (addr string, cleanup func())
```

This test server should:
- Accept SSH connections with token auth
- Accept `"control"` channel → respond to `TunnelRequest` with `TunnelAssignment`
- Accept `"proxy"` channels → echo back data (or proxy to a local test HTTP server)

This lets you fully test without depending on Dev A's implementation.

---

## 7. Governance & Communication

Same rules as [AGT-002-FG](file:///home/ubuntu/Fonzygrok/architect/CODEX/80_AGENTS/AGT-002-FG_Developer_Boot.md) §7–§8.

**Critical addition:** Do NOT create, modify, or delete files in `internal/server/`, `internal/store/`, or `cmd/server/`. That's Dev A's territory. Merge conflicts in shared areas = `DEF-` report to Architect.
