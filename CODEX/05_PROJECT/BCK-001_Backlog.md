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
updated: 2026-04-02
version: 4.0.0
---

> **BLUF:** Prioritized developer backlog for fonzygrok covering v1.0 through v1.2. **All items are complete.** v1.2.0 ready for tagging.

# Developer Backlog — Fonzygrok

---

## Execution History

```
SPR-001 (Foundation)     ✅ COMPLETE
         │
    ┌────┴────┐
    ▼         ▼
Track A    Track B
(Server)   (Client)
    │         │
SPR-002A ✅ SPR-002B ✅
SPR-003A ✅ SPR-003B ✅
SPR-004A ✅    │
    │         │
    └────┬────┘
         ▼
   SPR-005 ✅ (Merge → v1.0)
         │
   SPR-006–009 ✅ (v1.1 features → tagged v1.1.0)
         │
   SPR-014–016 ✅ (bug fixes, CI/CD, README → tagged v1.1.2)
         │
   SPR-017–019 ✅ (user auth, API auth, dashboard → merged to main)
         │
   SPR-020 ✅ (dashboard HTTPS edge, light/dark theme)
         │
   SPR-010 ✅ (TCP tunneling — server + client)
         │
   SPR-011–012 ✅ (rate limiting + IP ACL)
         │
   SPR-013 ✅ (integration, E2E, docs → v1.2.0)
```

---

## Completed Items

### v1.0 — Foundation + Integration (SPR-001 through SPR-005)

| # | Item | Sprint | Status |
|:--|:-----|:-------|:-------|
| B-001 | Go module, scaffold, Makefile, .gitignore | SPR-001 | ✅ |
| B-002 | Shared protocol types + codec | SPR-001 | ✅ |
| B-003 | SQLite store + migrations | SPR-001 | ✅ |
| B-004 | Token auth (generate, hash, validate) | SPR-001 | ✅ |
| B-005 | SSH server listener + auth callback | SPR-002A | ✅ |
| B-006A | Server-side control channel handler | SPR-002A | ✅ |
| B-007A | Tunnel manager (register/lookup/deregister) | SPR-002A | ✅ |
| B-006B | SSH client connector + auth | SPR-002B | ✅ |
| B-007B | Client-side control channel | SPR-002B | ✅ |
| B-008B | Client local proxy (buffer pool) | SPR-002B | ✅ |
| B-009A | HTTP edge — Host header routing | SPR-003A | ✅ |
| B-010A | HTTP edge — proxy through tunnel | SPR-003A | ✅ |
| B-011A | HTTP edge — error responses | SPR-003A | ✅ |
| B-012A | Server info endpoint | SPR-003A | ✅ |
| B-009B | Client CLI (cobra) | SPR-003B | ✅ |
| B-010B | Auto-reconnect with exponential backoff | SPR-003B | ✅ |
| B-011B | Structured logging + graceful shutdown | SPR-003B | ✅ |
| B-013A | Admin API (health, tokens, tunnels) | SPR-004A | ✅ |
| B-014A | Server orchestrator (wire all subsystems) | SPR-004A | ✅ |
| B-015A | Server CLI + logging + graceful shutdown | SPR-004A | ✅ |
| B-016 | Integration wiring (server + client) | SPR-005 | ✅ |
| B-017 | Docker packaging | SPR-005 | ✅ |
| B-018 | E2E integration test suite | SPR-005 | ✅ |

### v1.1 — Custom Subdomains, TLS, Metrics, Inspector (SPR-006 through SPR-009)

| # | Item | Sprint | Status |
|:--|:-----|:-------|:-------|
| B-019 | Name generator package | SPR-006 | ✅ |
| B-020 | Protocol extension (Name field) | SPR-006 | ✅ |
| B-021 | Server name handling (uniqueness, reserved) | SPR-006 | ✅ |
| B-022 | Client --name flag + auto-generated fallback | SPR-006 | ✅ |
| B-023 | Auto-TLS on edge router (Let's Encrypt) | SPR-007 | ✅ |
| B-024 | YAML config file support (server + client) | SPR-007 | ✅ |
| B-025 | Per-tunnel traffic metrics via admin API | SPR-008 | ✅ |
| B-026 | Request inspection web UI (localhost:4040) | SPR-008 | ✅ |
| B-027 | v1.1 E2E tests | SPR-009 | ✅ |
| B-028 | Docker TLS updates | SPR-009 | ✅ |

### Post-v1.1 — Bug Fixes + Infrastructure (SPR-014 through SPR-016)

| # | Item | Sprint | Status |
|:--|:-----|:-------|:-------|
| B-029 | Pretty client output (DEF-001 fix) | SPR-014 | ✅ |
| B-030 | --verbose flag for JSON logs | SPR-014 | ✅ |
| B-031 | Proxy EOF fix (DEF-003) | SPR-015 | ✅ |
| B-032 | TLS enablement in Docker (DEF-004) | SPR-015 | ✅ |
| B-033 | Production smoke test script | SPR-015 | ✅ |
| B-034 | CI/CD auto-deploy on tag push | SPR-016 | ✅ |
| B-035 | README overhaul | SPR-016 | ✅ |

### v1.2 — User Auth + Dashboard (SPR-017 through SPR-019)

| # | Item | Sprint | Status |
|:--|:-----|:-------|:-------|
| B-036 | Users table migration | SPR-017 | ✅ |
| B-037 | bcrypt password hashing | SPR-017 | ✅ |
| B-038 | JWT session management | SPR-017 | ✅ |
| B-039 | Invite code system | SPR-017 | ✅ |
| B-040 | Admin bootstrap command | SPR-017 | ✅ |
| B-041 | Auth middleware (JWT + cookie) | SPR-018 | ✅ |
| B-042 | Registration endpoint | SPR-018 | ✅ |
| B-043 | Login/logout endpoints | SPR-018 | ✅ |
| B-044 | Token management API (user-scoped) | SPR-018 | ✅ |
| B-045 | Invite code API (admin) | SPR-018 | ✅ |
| B-046 | Web dashboard (Go templates + HTMX) | SPR-019 | ✅ |
| B-047 | Login/register pages | SPR-019 | ✅ |
| B-048 | Admin pages (users, invite codes) | SPR-019 | ✅ |

---

## Pending Items

### v1.2 — Dashboard HTTPS + Theme (SPR-020)

| # | Item | Sprint | Contract | Owner | Status |
|:--|:-----|:-------|:---------|:------|:-------|
| B-058 | Mount dashboard on edge router (HTTPS) | SPR-020 | CON-002 (ext) | Dev A | ✅ |
| B-059 | Extend TLS host policy for apex domain | SPR-020 | — | Dev A | ✅ |
| B-060 | Light/dark theme toggle (system default) | SPR-020 | — | Dev A | ✅ |
| B-061 | Docker + config updates for dashboard edge | SPR-020 | GOV-008 | Dev A | ✅ |

### v1.2 — TCP, Rate Limiting, ACL (SPR-010 through SPR-013)

| # | Item | Sprint | Contract | Owner | Status |
|:--|:-----|:-------|:---------|:------|:-------|
| B-049 | Raw TCP tunnel support | SPR-010 | CON-001 (ext) | Dev A | ✅ |
| B-050 | TCP port assignment + listener | SPR-010 | CON-002 (ext) | Dev A | ✅ |
| B-051 | Client TCP tunnel mode | SPR-010 | — | Dev B | ✅ |
| B-052 | Per-tunnel token bucket rate limiter | SPR-011 | — | Dev A | ✅ |
| B-053 | 429 response on rate limit | SPR-011 | CON-002 (ext) | Dev A | ✅ |
| B-054 | Per-tunnel IP allow/deny (CIDR) | SPR-012 | — | Dev A | ✅ |
| B-055 | v1.2 E2E test suite | SPR-013 | — | Both | ✅ |
| B-056 | v1.2 Docker + production deployment | SPR-013 | GOV-008 | Both | ✅ |
| B-057 | Tag v1.2.0 release | SPR-013 | — | Architect | ⏳ |

---

## File Ownership Boundaries

| Path | Dev A (Server) | Dev B (Client) |
|:-----|:--------------:|:--------------:|
| `internal/server/*` | ✅ Write | ❌ |
| `internal/client/*` | ❌ | ✅ Write |
| `internal/proto/*` | ✅ SPR-001, SPR-006 | ❌ Read-only |
| `internal/names/*` | ✅ SPR-006 | ❌ Read-only |
| `internal/auth/*` | ✅ Write | ❌ Read-only |
| `internal/store/*` | ✅ Write | ❌ |
| `internal/config/*` | ✅ Write | ✅ Write |
| `cmd/server/*` | ✅ Write | ❌ |
| `cmd/client/*` | ❌ | ✅ Write |
| `migrations/*` | ✅ Write | ❌ |
| `docker/*` | ✅ Write | ✅ SPR-005 |
| `tests/*` | ✅ Write | ✅ Write |

---

## Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial sequential backlog | Architect |
| 2026-03-31 | 2.0.0 | Restructured for parallel execution (Server + Client tracks) | Architect |
| 2026-03-31 | 3.0.0 | Added v1.1 backlog items (SPR-006 custom subdomains) | Architect |
| 2026-04-02 | 4.0.0 | Full reconciliation: marked all completed items, added v1.1/v1.2 items, updated pending | Architect |
| 2026-04-07 | 5.0.0 | Added SPR-020 dashboard edge + theme items, sequenced before SPR-010 | Architect |
| 2026-04-07 | 6.0.0 | All v1.2 items complete — SPR-010/011/012/013/020 done. Ready for v1.2.0 tag. | Architect |
