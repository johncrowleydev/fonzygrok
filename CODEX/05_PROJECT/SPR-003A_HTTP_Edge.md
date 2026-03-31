---
id: SPR-003A
title: "Server Track — HTTP Edge Router"
type: how-to
status: PLANNING
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, http, routing, server]
related: [BCK-001, BLU-001, CON-001, CON-002]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** Second parallel sprint for Server Track. Builds the public-facing HTTP edge router that maps incoming requests by Host header to tunnels and proxies traffic through SSH channels. 4 tasks.

# Sprint 003A: Server Track — HTTP Edge Router

**Phase:** Phase 1 (MVP) — Parallel Track A (Server)
**Target:** Scope-bounded
**Agent(s):** Developer A (Server Track)
**Dependencies:** SPR-002A (SSH server + tunnel manager)
**Contracts:** CON-002 §3 (edge routing), CON-001 §5 (data channel)

> **⚠️ Parallel Sprint.** Dev B is working on SPR-003B simultaneously. Same file ownership rules apply.

---

## Developer Tasks

### T-009A: HTTP Edge — Host Header Routing

- **Dependencies:** T-007A (tunnel manager)
- **Contracts:** CON-002 §3.1
- **Deliverable:**
  - `internal/server/edge.go` — `EdgeRouter` struct:
    - `New(config EdgeConfig, tunnels *TunnelManager, logger *slog.Logger) *EdgeRouter`
    - `Start(ctx context.Context) error` — listen on configured HTTP address
    - `Stop() error`
  - Extract subdomain from `Host` header: `<tunnel_id>.<base_domain>` → lookup `tunnel_id`
  - Route matched requests to tunnel proxy handler
  - `EdgeConfig` struct: `Addr string`, `BaseDomain string`, `ProxyTimeout time.Duration`
  - `internal/server/edge_test.go` — routing tests with mock tunnel manager
- **Acceptance criteria:**
  - `abc123.tunnel.example.com` → routes to tunnel `abc123`
  - `tunnel.example.com` → server info response
  - Unknown subdomain → 404
  - IP-based requests (no Host) → server info response
- **Status:** [ ] Not Started

### T-010A: HTTP Edge — Proxy Through Tunnel

- **Dependencies:** T-007A, T-009A
- **Contracts:** CON-002 §3.2, CON-001 §5
- **Deliverable:**
  - Wire edge router to `TunnelManager.OpenProxyChannel()` for matched tunnels
  - Write full HTTP request (method, path, headers, body) to SSH channel
  - Read full HTTP response from SSH channel
  - Add headers per CON-002 §3.2: `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Proto`, `X-Fonzygrok-Tunnel-Id`
  - Timeout: 30s default (configurable via `EdgeConfig.ProxyTimeout`)
  - `internal/server/edge_test.go` (extend) — proxy round-trip with mock channel
- **Acceptance criteria:**
  - Complete HTTP request/response round-trip through SSH channel
  - All `X-Forwarded-*` headers added to proxied request
  - Original headers preserved (not modified or dropped)
  - 30s timeout enforced
- **Status:** [ ] Not Started

### T-011A: HTTP Edge — Error Responses

- **Dependencies:** T-009A
- **Contracts:** CON-002 §3.3
- **Deliverable:**
  - Error responses per CON-002 §3.3:
    - `404` — tunnel not found
    - `502` — tunnel offline or upstream unreachable
    - `504` — proxy timeout
    - `500` — internal server error
  - All errors: `Content-Type: application/json`, `X-Fonzygrok-Error: true`
  - JSON body: `{"error": "<code>", "message": "<description>"}`
  - `internal/server/edge_test.go` (extend) — test each error scenario
- **Acceptance criteria:**
  - Each scenario returns correct status code, JSON body, headers per CON-002 §3.3
- **Status:** [ ] Not Started

### T-012A: Server Info Endpoint

- **Dependencies:** T-009A
- **Contracts:** CON-002 §3.4
- **Deliverable:**
  - Root domain `GET /` returns `ServerInfo` JSON per CON-002 §3.4
  - Version from build-time `ldflags`
  - Active tunnel count from `TunnelManager.ListActive()`
- **Acceptance criteria:**
  - Correct JSON schema per CON-002 §3.4
  - `tunnels_active` reflects real count
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-009A | Dev A | [ ] | [ ] |
| T-010A | Dev A | [ ] | [ ] |
| T-011A | Dev A | [ ] | [ ] |
| T-012A | Dev A | [ ] | [ ] |

---

## Sprint Completion Criteria

- [ ] Edge routes by Host header correctly
- [ ] Proxy round-trip works through SSH channel
- [ ] All error responses match CON-002 §3.3
- [ ] `go test -race ./internal/server/...` passes
- [ ] No modifications to client-owned files

---

## Audit Notes (Architect)

**Verdict:** PENDING
