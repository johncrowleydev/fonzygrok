---
id: BCK-001
title: "Developer Backlog"
type: planning
status: ACTIVE
owner: architect
agents: [coder]
tags: [project-management, backlog, development]
related: [PRJ-001, BLU-001, CON-001, CON-002]
created: 2026-03-31
updated: 2026-03-31
version: 2.0.0
---

> **BLUF:** Prioritized developer backlog for fonzygrok v1.0, restructured for **parallel execution** with two developer tracks (Server + Client). Items map to sprint tasks, contracts, and file ownership boundaries.

# Developer Backlog — Fonzygrok v1.0 (Parallel Tracks)

---

## Execution Model

```
SPR-001 (Foundation — Dev A, serial)
         │
    ┌────┴────┐
    ▼         ▼
Track A    Track B
(Server)   (Client)
    │         │
SPR-002A   SPR-002B
SPR-003A   SPR-003B
SPR-004A      │
    │         │
    └────┬────┘
         ▼
   SPR-005 (Merge)
```

---

## Backlog Items

### Serial Phase (SPR-001)

| # | Item | Sprint | Contract | Owner |
|:--|:-----|:-------|:---------|:------|
| B-001 | Go module, scaffold, Makefile, .gitignore | SPR-001 | BLU-001 §4 | Dev A |
| B-002 | Shared protocol types + codec | SPR-001 | CON-001 §8 | Dev A |
| B-003 | SQLite store + migrations | SPR-001 | GOV-008 §2 | Dev A |
| B-004 | Token auth (generate, hash, validate) | SPR-001 | CON-001 §3.2, CON-002 §4.3 | Dev A |

### Server Track (Dev A)

| # | Item | Sprint | Contract | Owner |
|:--|:-----|:-------|:---------|:------|
| B-005 | SSH server listener + auth callback | SPR-002A | CON-001 §3 | Dev A |
| B-006A | Server-side control channel handler | SPR-002A | CON-001 §4 | Dev A |
| B-007A | Tunnel manager (register/lookup/deregister) | SPR-002A | CON-001 §4.3, §6 | Dev A |
| B-009A | HTTP edge — Host header routing | SPR-003A | CON-002 §3.1 | Dev A |
| B-010A | HTTP edge — proxy through tunnel | SPR-003A | CON-002 §3.2 | Dev A |
| B-011A | HTTP edge — error responses | SPR-003A | CON-002 §3.3 | Dev A |
| B-012A | Server info endpoint | SPR-003A | CON-002 §3.4 | Dev A |
| B-013A | Admin API (health, tokens, tunnels) | SPR-004A | CON-002 §4 | Dev A |
| B-014A | Server orchestrator (wire all subsystems) | SPR-004A | BLU-001 §1 | Dev A |
| B-015A | Server CLI + logging + graceful shutdown | SPR-004A | BLU-001 §7 | Dev A |

### Client Track (Dev B)

| # | Item | Sprint | Contract | Owner |
|:--|:-----|:-------|:---------|:------|
| B-006B | SSH client connector + auth | SPR-002B | CON-001 §3 | Dev B |
| B-007B | Client-side control channel | SPR-002B | CON-001 §4 | Dev B |
| B-008B | Client local proxy (buffer pool) | SPR-002B | CON-001 §5 | Dev B |
| B-009B | Client CLI (cobra) | SPR-003B | BLU-001 §2.8 | Dev B |
| B-010B | Auto-reconnect with exponential backoff | SPR-003B | CON-001 §3.1 | Dev B |
| B-011B | Structured logging + graceful shutdown | SPR-003B | BLU-001 §7, GOV-006 | Dev B |

### Merge Phase (Both)

| # | Item | Sprint | Contract | Owner |
|:--|:-----|:-------|:---------|:------|
| B-016 | Integration wiring (server + client) | SPR-005 | — | Both |
| B-017 | Docker packaging | SPR-005 | GOV-008 §1, §4 | Both |
| B-018 | E2E integration test suite | SPR-005 | CON-001 §10, CON-002 §9 | Both |

---

## File Ownership Boundaries

| Path | Dev A (Server) | Dev B (Client) |
|:-----|:--------------:|:--------------:|
| `internal/server/*` | ✅ Write | ❌ |
| `internal/client/*` | ❌ | ✅ Write |
| `internal/proto/*` | ✅ SPR-001 only | ❌ Read-only |
| `internal/auth/*` | ✅ SPR-001 only | ❌ Read-only |
| `internal/store/*` | ✅ Write | ❌ |
| `cmd/server/*` | ✅ Write | ❌ |
| `cmd/client/*` | ❌ | ✅ Write |
| `migrations/*` | ✅ SPR-001 only | ❌ |
| `docker/*` | ✅ SPR-005 | ✅ SPR-005 |
| `tests/*` | ✅ SPR-005 | ✅ SPR-005 |

---

## Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial sequential backlog | Architect |
| 2026-03-31 | 2.0.0 | Restructured for parallel execution (Server + Client tracks) | Architect |
