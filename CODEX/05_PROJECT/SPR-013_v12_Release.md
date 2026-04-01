---
id: SPR-013
title: "Sprint 013: v1.2 Integration & Release"
type: how-to
status: READY
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, integration, v1.2, release]
related: [SPR-010, SPR-011, SPR-012]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-013: v1.2 Integration & Release

## Goal

Verify all v1.2 features work together end-to-end, update infrastructure for TCP tunnels, deploy to production, tag v1.2.0.

---

## Track Assignment

Both devs work together on a shared branch: `release/v1.2.0`

---

## Tasks

### T-039: v1.2 E2E Tests

Add to `tests/e2e_test.go`:

| Test | What it verifies |
|:-----|:----------------|
| `TestE2E_20_TCPTunnel` | Client `--protocol tcp --port 9999` → assigned port, TCP connection works |
| `TestE2E_21_TCPAndHTTPCoexist` | Same client has both HTTP and TCP tunnels active |
| `TestE2E_22_TCPMetrics` | TCP traffic increments bytes_in/bytes_out |
| `TestE2E_23_RateLimitHTTP` | Exceed rate limit → 429 response |
| `TestE2E_24_RateLimitCustom` | Set custom limit via admin API, verify enforced |
| `TestE2E_25_IPWhitelistAllow` | Request from allowed IP → 200 |
| `TestE2E_26_IPWhitelistDeny` | Request from non-allowed IP → 403 |
| `TestE2E_27_DashboardServed` | `GET /dashboard/` returns HTML with 200 |
| `TestE2E_28_AdminAuth` | With admin token set, 401 without auth, 200 with auth |

---

### T-040: Infrastructure Updates

#### Docker

Update `docker/docker-compose.yml`:
- Expose TCP port range: `"40000-40100:40000-40100"` (start small, expand as needed)
- Add `FONZYGROK_ADMIN_TOKEN` env var
- Add `--admin-token`, `--tcp-port-range`, `--rate-limit` to server command

Update `docker/.env.example`:
```env
FONZYGROK_ADMIN_TOKEN=changeme
TCP_PORT_RANGE=40000-40100
RATE_LIMIT=100
```

#### EC2 Security Group

Document in runbook: add inbound rule for TCP range 40000-40100.

---

### T-041: Update Documentation

Update `CODEX/30_RUNBOOKS/RUN-001_Production_Deployment.md`:
- TCP tunnel setup section
- Rate limiting configuration
- Dashboard access (admin token)
- IP whitelisting examples
- Security group updates for TCP ports

Update `CODEX/20_BLUEPRINTS/BLU-001_System_Architecture.md`:
- Add TCP edge to architecture diagram
- Update component descriptions
- Add dashboard to admin API section

---

### T-042: Production Deployment & Tag

1. Deploy to fonzygrok.com:
   ```bash
   cd ~/fonzygrok && git pull origin main
   cd docker && docker compose up -d --build
   ```

2. Verify:
   - HTTP tunnels still work (regression)
   - TCP tunnel: expose a local service, connect from outside
   - Rate limiting: hammer a tunnel, verify 429s
   - Dashboard: log in, view tunnels, disconnect one
   - IP whitelist: set via admin API, verify 403 for blocked IPs

3. Tag:
   ```bash
   git tag -a v1.2.0 -m "v1.2.0: TCP tunnels, rate limiting, web dashboard, IP whitelisting"
   git push origin v1.2.0
   ```

---

## Acceptance Criteria

- All v1.0 tests pass (120+ unit + 9 E2E)
- All v1.1 tests pass
- All v1.2 E2E tests pass (9 new)
- Production deployment verified
- Dashboard accessible with admin token
- TCP tunnels work from external network
- Rate limiting enforced
- IP whitelisting operational
- Project roadmap Phase 3 exit criteria met
