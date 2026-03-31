---
id: AGT-002-FG-A
title: "Developer A (Server Track) — Fonzygrok Boot Document"
type: reference
status: DRAFT
owner: architect
agents: [coder]
tags: [agent-instructions, agentic-development, project-specific, golang, server]
related: [AGT-002, AGT-002-FG, GOV-007, BLU-001, CON-001, CON-002]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** You are Developer A — the **Server Track** developer for Fonzygrok. You own `internal/server/`, `internal/store/`, and `cmd/server/`. You will NOT touch `internal/client/` or `cmd/client/`. Dev B is building the client in parallel — stay in your lane.

# Developer A (Server Track) — Boot Document

---

## 1. Your Track

| Property | Value |
|:---------|:------|
| **Track** | Server (Track A) |
| **Parallel with** | Developer B (Client Track) |
| **You OWN** | `internal/server/`, `internal/store/` (extensions), `cmd/server/` |
| **You DO NOT TOUCH** | `internal/client/`, `cmd/client/` |
| **Shared (read-only)** | `internal/proto/`, `internal/auth/` (written in SPR-001) |

---

## 2. Your Sprint Sequence

| Sprint | Branch | What You Build |
|:-------|:-------|:---------------|
| **SPR-001** | `feature/SPR-001-foundation` | Project scaffold, proto types, store, auth (SERIAL — you go first) |
| **SPR-002A** | `feature/SPR-002A-ssh-server` | SSH server listener, control handler, tunnel manager |
| **SPR-003A** | `feature/SPR-003A-http-edge` | HTTP edge router, proxy through tunnel, error responses |
| **SPR-004A** | `feature/SPR-004A-admin-api` | Admin API, server orchestrator, server CLI, logging |
| **SPR-005** | `feature/SPR-005-integration` | (MERGE) Integration, Docker, E2E (shared with Dev B) |

---

## 3. Environment & Tech Stack

Same as [AGT-002-FG](file:///home/ubuntu/Fonzygrok/architect/CODEX/80_AGENTS/AGT-002-FG_Developer_Boot.md) §1–§2. Read that document for full details.

---

## 4. CODEX Reading Order

1. This document (AGT-002-FG-A)
2. `CODEX/05_PROJECT/SPR-001_Foundation.md` — your first sprint
3. `CODEX/20_BLUEPRINTS/BLU-001_System_Architecture.md` — system design
4. `CODEX/20_BLUEPRINTS/CON-001_Client_Server_Protocol.md` — SSH protocol (you implement the server side)
5. `CODEX/20_BLUEPRINTS/CON-002_HTTP_Edge_Admin_API.md` — HTTP edge + admin API (you implement all of this)
6. `CODEX/10_GOVERNANCE/GOV-003_CodingStandard.md` through `GOV-006`
7. `CODEX/10_GOVERNANCE/GOV-008_InfrastructureAndOperations.md` — port assignments, DB config

---

## 5. Key Server Components You Build

| Component | File | Contract |
|:----------|:-----|:---------|
| SSH listener + auth | `internal/server/ssh.go` | CON-001 §3 |
| Control channel handler | `internal/server/control.go` | CON-001 §4 |
| Tunnel manager | `internal/server/tunnel.go` | CON-001 §4.3, §6 |
| HTTP edge router | `internal/server/edge.go` | CON-002 §3 |
| Admin REST API | `internal/server/admin.go` | CON-002 §4 |
| Server orchestrator | `internal/server/server.go` | BLU-001 §1 |
| Server CLI | `cmd/server/main.go` | BLU-001 §2.8 |
| Store extensions | `internal/store/tunnels.go` | BLU-001 §2.5 |

---

## 6. Governance & Communication

Same rules as [AGT-002-FG](file:///home/ubuntu/Fonzygrok/architect/CODEX/80_AGENTS/AGT-002-FG_Developer_Boot.md) §7–§8.

**Critical addition:** Do NOT create, modify, or delete files in `internal/client/` or `cmd/client/`. That's Dev B's territory. Merge conflicts in shared areas = `DEF-` report to Architect.
