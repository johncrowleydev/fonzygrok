---
id: SPR-005
title: "Merge — Integration, Docker & E2E Testing"
type: how-to
status: COMPLETE
owner: architect
agents: [coder]
tags: [project-management, sprint, workflow, integration, docker, testing]
related: [BCK-001, GOV-008, CON-001, CON-002]
created: 2026-03-31
updated: 2026-04-02
version: 1.0.0
---

> **BLUF:** Merge sprint. Both developer tracks converge. Merge server and client branches, resolve any conflicts, verify end-to-end tunnel operation, package into Docker, and run comprehensive integration tests. After this sprint, fonzygrok v1.0 is deployable. Either or both developers. 3 tasks.

# Sprint 005: Merge — Integration, Docker & E2E Testing

**Phase:** Phase 1 (MVP) — Merge Point
**Target:** Scope-bounded
**Agent(s):** Developer A and/or Developer B
**Dependencies:** SPR-004A (server complete) AND SPR-003B (client complete) — BOTH must be merged to main
**Contracts:** CON-001 §10, CON-002 §9 (verification checklists)

> **⚠️ Merge Sprint.** Both tracks must be merged to `main` before this sprint starts. Architect coordinates the merge.

---

## Pre-Sprint: Merge Coordination (Architect)

Before assigning this sprint, the Architect:
1. Audits SPR-004A output (server track complete)
2. Audits SPR-003B output (client track complete)
3. Merges both feature branches to `main` (resolve any conflicts)
4. Verifies `go build ./...` and `go test ./...` pass on merged main
5. Assigns SPR-005 to one or both developers

---

## Developer Tasks

### T-016: Integration Wiring

- **Dependencies:** All previous SPRs merged
- **Deliverable:**
  - Verify server + client work together (not just in isolation with test helpers):
    - Start `fonzygrok-server serve` in a goroutine
    - Create a token via `fonzygrok-server token create`
    - Start `fonzygrok --server localhost:2222 --token <token> --port <port>`
    - Verify tunnel registered, public URL assigned
  - Fix any interface mismatches between server and client implementations
  - Ensure proxy data flow works end-to-end (not just with mock channels)
  - `tests/integration_test.go` — in-process integration test
- **Acceptance criteria:**
  - Real server + real client connect and establish tunnel
  - HTTP request through tunnel returns correct response from local service
  - Client disconnect → server deregisters tunnel
  - Server shutdown → client reconnects
- **Status:** [ ] Not Started

### T-017: Docker Packaging

- **Dependencies:** T-016
- **Contracts:** GOV-008 §1, §4
- **Deliverable:**
  - `docker/Dockerfile` — multi-stage build:
    - Stage 1: `golang:1.22-alpine` → build both binaries with ldflags (version)
    - Stage 2: `gcr.io/distroless/static-debian12` → copy binaries
    - Run as non-root (UID 65534)
    - Expose 2222, 8080, 9090
    - Volume `/data`
    - Entrypoint: `fonzygrok-server serve`
  - `docker/docker-compose.yml`:
    - Port mapping per GOV-008 §3.1
    - Volume for `/data`
    - `.env` file for config
    - Health check: `wget` or custom binary health check against `/api/v1/health`
    - Restart: `unless-stopped`
  - Update `Makefile`: `docker-build`, `docker-up`, `docker-down`, `docker-logs`
- **Acceptance criteria:**
  - `make docker-build` succeeds
  - `make docker-up` starts server in container
  - Health check passes within 10s
  - SQLite persists across container restart
  - Container runs as non-root
- **Status:** [ ] Not Started

### T-018: End-to-End Test Suite

- **Dependencies:** T-016
- **Contracts:** CON-001 §10, CON-002 §9
- **Deliverable:**
  - `tests/e2e_test.go` (build tag `//go:build e2e`):
    1. Start fonzygrok server in-process
    2. Create token via admin API
    3. Start local HTTP test server on random port
    4. Start fonzygrok client → connect → get tunnel
    5. `GET` the tunnel URL → verify response matches local server
    6. `POST` with body → verify body proxied correctly
    7. Verify `X-Forwarded-For` header present
    8. Test 404 for non-existent subdomain
    9. Stop local server → request → verify 502
    10. Stop server → verify client logs reconnect attempts
    11. Restart server → verify client reconnects and re-registers
  - Run: `go test -v -race -tags=e2e ./tests/`
- **Acceptance criteria:**
  - All 11 test scenarios pass
  - Passes with `-race`
  - No external dependencies (fully in-process)
  - Test completes in < 60 seconds
- **Status:** [ ] Not Started

---

## Sprint Checklist

| Task | Agent | Status | Audited |
|:-----|:------|:-------|:--------|
| T-016 | Dev A/B | [ ] | [ ] |
| T-017 | Dev A/B | [ ] | [ ] |
| T-018 | Dev A/B | [ ] | [ ] |

---

## Sprint Completion Criteria

- [ ] Server + client work together end-to-end
- [ ] Docker image builds and runs
- [ ] E2E test suite passes all scenarios with `-race`
- [ ] All verification checklists from CON-001 §10 and CON-002 §9 covered
- [ ] `go test -race ./...` passes (all tests, including e2e)
- [ ] No open `DEF-` reports
- [ ] **Fonzygrok v1.0 is ready to deploy**

---

## Audit Notes (Architect)

**Verdict:** PENDING
**Deploy approved:** PENDING
