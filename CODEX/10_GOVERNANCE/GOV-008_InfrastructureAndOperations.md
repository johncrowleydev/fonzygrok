---
id: GOV-008
title: "Infrastructure and Operations"
type: reference
status: APPROVED
owner: architect
agents: [all]
tags: [governance, infrastructure, operations, deployment, docker]
related: [PRJ-001, GOV-007, RES-001]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** Fonzygrok deploys as a Docker container on a single Linux VPS. SQLite is the database (embedded, single-file). Both server and client are static Go binaries cross-compiled via `goreleaser`. No managed cloud services. No Kubernetes. No multi-node. This doc overrides any BLU- assumptions that conflict.

# Infrastructure and Operations

---

## 1. Deployment Model

| Property | Decision |
|:---------|:---------|
| **Server deployment** | Docker Compose on a single Linux VPS |
| **Client deployment** | Direct binary download (user's machine) |
| **Container base** | `gcr.io/distroless/static-debian12` (minimal, no shell) |
| **Orchestration** | Docker Compose (single-node, no Kubernetes) |
| **Reverse proxy** | Not needed — fonzygrok server IS the edge |
| **TLS termination** | Built-in (autocert / Let's Encrypt) in v1.1. Self-signed or no TLS in v1.0. |
| **DNS** | Wildcard A record `*.tunnel.example.com` → VPS IP (v1.1+). Direct IP in v1.0. |

---

## 2. Database

| Property | Decision |
|:---------|:---------|
| **Engine** | SQLite via `modernc.org/sqlite` (pure Go, CGo-free) |
| **Location** | `/data/fonzygrok.db` inside container, mounted as Docker volume |
| **Backup** | File-level copy of the `.db` file. VACUUM periodically. |
| **Migration** | Embedded SQL migrations executed on startup |
| **Concurrency** | WAL mode. Single-writer is acceptable for v1 scale. |

---

## 3. Networking

| Port | Protocol | Purpose |
|:-----|:---------|:--------|
| `2222` | TCP (SSH) | Client tunnel connections (control + data plane) |
| `8080` | HTTP | Public-facing edge (proxied HTTP requests) |
| `8443` | HTTPS | Public-facing edge with TLS (v1.1+) |
| `9090` | HTTP | Admin API (internal, not exposed publicly) |

### 3.1 Docker Compose Port Mapping

```yaml
ports:
  - "2222:2222"   # SSH tunnel endpoint
  - "80:8080"     # Public HTTP edge
  - "443:8443"    # Public HTTPS edge (v1.1)
  - "9090:9090"   # Admin API (bind to localhost in production)
```

---

## 4. Build & Release

| Property | Decision |
|:---------|:---------|
| **Build system** | `Makefile` + `goreleaser` |
| **Targets** | `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` |
| **Artifacts** | Two binaries: `fonzygrok-server`, `fonzygrok` (client) |
| **Docker image** | Built from multi-stage Dockerfile (build → distroless) |
| **Versioning** | Semantic Versioning. Git tags trigger releases. |

---

## 5. Adaptation Table

> Per GOV-007 §9.7: When architecture documents assume capabilities that differ from actual deployment, document the adaptation here.

| BLU- Assumption | Actual Deployment | Adaptation |
|:----------------|:------------------|:-----------|
| Managed database service | Self-managed SQLite in container | Mount `/data` as Docker volume for persistence |
| Load balancer / CDN | No load balancer | Server binds directly to ports 80/443 |
| Auto-scaling | Single instance | Vertical scaling only (bigger VPS) |
| DNS management API | Manual DNS setup | Human configures wildcard DNS record |
| Secret management service | Environment variables | `.env` file mounted into container, gitignored |
| CI/CD pipeline | Manual build + deploy | `make docker-push` + `docker compose pull && docker compose up -d` |

---

## 6. Security

| Concern | Mitigation |
|:--------|:-----------|
| SSH brute force | Token auth only (no password). Rate-limit connection attempts. |
| Admin API exposure | Bind `:9090` to `127.0.0.1` only. Access via SSH tunnel to VPS. |
| Container privileges | Run as non-root user inside distroless container |
| SQLite file access | Volume permissions restricted to container user |
| Secrets in env | `.env` file with `600` permissions. Never committed to git. |

---

## 7. Monitoring

| Tool | Purpose | Priority |
|:-----|:--------|:---------|
| Structured logs (`slog`) | Application logging to stdout (Docker captures) | v1.0 |
| Health check endpoint | `GET /healthz` on admin API | v1.0 |
| `docker compose logs -f` | Log tailing | v1.0 |
| Prometheus metrics | Connection counts, latency histograms | v1.2 |

---

## 8. Operational Runbooks (TODO)

| Runbook | Topics | Sprint |
|:--------|:-------|:-------|
| `RUN-FG-001` | Fresh VPS setup, Docker install, domain/DNS, first deploy | SPR-007 |
| `RUN-FG-002` | Upgrade deployment (pull new image, restart) | SPR-007 |
| `RUN-FG-003` | Backup and restore SQLite database | SPR-007 |

---

## 9. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial infrastructure decisions | Architect + Human |
