---
id: SPR-015
title: "Sprint 015: Critical Bug Fixes — Proxy EOF + TLS + Smoke Tests"
type: how-to
status: ACTIVE
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, defect-fix, critical, proxy, tls, v1.1]
related: [DEF-002, DEF-003, DEF-004]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-015: Critical Bug Fixes — Proxy EOF + TLS + Smoke Tests

## Priority: CRITICAL — Production is broken

v1.1 shipped with tunnel proxying non-functional over real networks. This
sprint fixes that and the TLS enablement gap.

---

## Track A — Dev A (Server-side)

Branch: `feature/SPR-015A-proxy-fix`

### T-047: Fix Proxy EOF Over Internet (DEF-003)

The `unexpected EOF` in `edge.go` when reading the tunnel response over a
real network connection. Root cause investigation required.

**Start by reading:**
- `CODEX/50_DEFECTS/DEF-003_Proxy_EOF_Over_Internet.md`
- `internal/server/edge.go` — `proxyRequest()` function (lines 229-337)
- `internal/client/proxy.go` — `handleSingleChannel()` function (lines 86-156)

**Investigation areas:**
1. `CloseWrite()` on edge.go:267-269 — is this prematurely signaling EOF
   to the client before the response is ready?
2. The client's bidirectional copy (proxy.go:128-149) — does the SSH→local
   copy goroutine's `CloseWrite` on the TCP conn trigger the response before
   the local service has finished processing?
3. Buffering over real TCP — does `http.ReadResponse` fail when the response
   arrives in multiple TCP segments?
4. TeeReader wrapping for inspector — is the wrapping interfering with
   proper EOF detection?

**The fix MUST be verified by connecting a client on a DIFFERENT machine from
the server.** Localhost-only testing is not acceptable for this fix.

**Test requirements (per GOV-007 §4.2):**
- Regression test that reproduces the EOF (if possible in automated test)
- Test with real SSH connection (not in-process mock)
- Test with 100ms+ artificial latency if possible (tc netem or similar)

### T-048: Enable TLS in Docker Config (DEF-004)

Modify `docker/docker-compose.yml`:

```yaml
command:
  - serve
  - --data-dir=/data
  - --ssh-addr=:2222
  - --http-addr=:8080
  - --admin-addr=0.0.0.0:9090
  - --domain=${DOMAIN:-tunnel.localhost}
  - --tls
  - --tls-cert-dir=/data/certs
```

Add to `docker/.env.example`:
```
TLS_ENABLED=true
```

Make TLS conditional via env var if needed for local dev (where certs
aren't available).

---

## Track B — Dev B (Client-side)

Branch: `feature/SPR-015B-client-fixes`

### T-049: Client-Side Proxy Investigation

Work with Dev A on DEF-003. The client proxy in `internal/client/proxy.go`
may be part of the problem. Specifically:

1. Does the bidirectional copy handle partial writes correctly?
2. Does `CloseWrite` on the TCP connection trigger before the local service
   has finished writing its response?
3. Is the TeeReader capture interfering with the response?

Test the client proxy against a deliberately slow local service (one that
takes 500ms to respond) to surface timing-dependent bugs.

### T-050: Smoke Test Script

Create `scripts/smoke_test.sh` — a production smoke test per GOV-002 §18A:

```bash
#!/bin/bash
# Production smoke test — run from a DIFFERENT machine than the server
# Usage: ./smoke_test.sh <server_domain> <token>

DOMAIN=$1
TOKEN=$2

# 1. Health check
# 2. Connect client, create tunnel to local echo server
# 3. Curl the tunnel URL
# 4. Verify response
# 5. Check metrics endpoint
# 6. Clean up
```

This script is run by the Architect after every deployment. It is NOT
optional.

---

## Acceptance Criteria

Per GOV-007 §4.2 (strengthened):

- [ ] Tunnel proxying works over the internet (not just localhost)
- [ ] TLS enabled in Docker compose, certs issued by Let's Encrypt
- [ ] Smoke test script exists and passes against production
- [ ] All 209+ existing tests pass with -race
- [ ] New regression tests for the EOF bug
- [ ] User-scenario tests with realistic inputs (no localhost shortcuts)

## Governance Compliance

| GOV | Requirement | Deliverable |
|:----|:-----------|:------------|
| GOV-002 §18A | Production smoke test | `scripts/smoke_test.sh` |
| GOV-002 §24 | Regression test for bug fix | EOF regression test |
| GOV-007 §4.2 | User-scenario tests | Tests with `--server domain` (no port) |
| GOV-007 §4.3 | Human signoff before tag | No tag until Human approves |
