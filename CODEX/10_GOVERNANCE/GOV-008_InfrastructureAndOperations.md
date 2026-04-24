---
id: GOV-008
title: "Infrastructure and Operations"
type: reference
status: APPROVED
owner: architect
agents: [all]
tags: [governance, infrastructure, operations, deployment, docker]
related: [PRJ-001, GOV-007, RES-001, RUN-001]
created: 2026-03-31
updated: 2026-04-24
version: 2.0.0
---

> **BLUF:** Fonzygrok production runs on a single Ubuntu EC2/VPS host using Docker Compose. The application container runs the Go server binary on Alpine, and PostgreSQL 16 runs as a companion Compose service with a persistent Docker volume. GitHub Actions builds release artifacts and deploys production on `v*` tag pushes after required human release approval. No Kubernetes, managed database, CDN, or multi-node orchestration is used.

# Infrastructure and Operations

---

## 1. Deployment Model

| Property | Decision |
|:---------|:---------|
| **Server deployment** | Docker Compose on one Ubuntu EC2/VPS host |
| **Application container** | `docker/Dockerfile.prod`, Alpine runtime, non-root `fonzygrok` user |
| **Database container** | `postgres:16-alpine` service in `docker/docker-compose.yml` |
| **Client deployment** | GitHub Release binary download / install scripts |
| **Orchestration** | Docker Compose only; no Kubernetes |
| **Reverse proxy** | Not required; `fonzygrok-server` is the HTTP/HTTPS edge |
| **TLS termination** | Built-in autocert / Let's Encrypt when `TLS_ENABLED=true` |
| **DNS** | Apex and wildcard records point at the host: `fonzygrok.com`, `*.fonzygrok.com` or deployment-specific equivalent |

---

## 2. Database

| Property | Decision |
|:---------|:---------|
| **Engine** | PostgreSQL 16 (`postgres:16-alpine`) |
| **Production location** | Docker named volume `fonzygrok-pgdata` mounted at `/var/lib/postgresql/data` |
| **Application connection** | `DATABASE_URL` constructed in Compose from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` |
| **Migration** | Embedded SQL migrations executed by the server on startup |
| **Backup** | `pg_dump` from the PostgreSQL container to a dated `.sql` file |
| **Restore** | Pipe backup into `psql` inside the PostgreSQL container |
| **Developer tests** | `TEST_DATABASE_URL` may be set by developers/CI for test databases; local PostgreSQL setup is outside this governance doc |

### 2.1 Backup / Restore Commands

```bash
# Load configured DB identity from the Compose env file when present.
cd ~/fonzygrok/docker
set -a
[ -f .env ] && . ./.env
set +a
DB_USER="${POSTGRES_USER:-fonzygrok}"
DB_NAME="${POSTGRES_DB:-fonzygrok}"

# Backup
docker exec fonzygrok-postgres pg_dump -U "$DB_USER" "$DB_NAME" \
  > "fonzygrok_backup_$(date +%Y%m%d).sql"

# Restore
cat fonzygrok_backup_YYYYMMDD.sql | \
  docker exec -i fonzygrok-postgres psql -U "$DB_USER" "$DB_NAME"
```

---

## 3. Networking

| Port | Protocol | Purpose |
|:-----|:---------|:--------|
| `2222` | TCP (SSH) | Client tunnel connections/control channel |
| `80` | HTTP | HTTP redirect and ACME HTTP-01 challenge handling |
| `443` | HTTPS | Public edge for tunnels, dashboard, and install scripts |
| `8080` | HTTP | Non-TLS HTTP edge fallback / internal mapping |
| `9090` | HTTP | Admin API and health checks; optional external mapping, prefer internal access only |
| `40000-40100` | TCP | TCP tunnel public port range |

### 3.1 Docker Compose Port Mapping

```yaml
ports:
  - "${SSH_PORT:-2222}:2222"
  - "80:80"
  - "8080:8080"
  - "${HTTPS_PORT:-443}:443"
  - "${TCP_PORT_RANGE:-40000-40100}:40000-40100"
  # Optional direct admin API exposure only:
  # - "${ADMIN_PORT:-9090}:9090"
```

---

## 4. Build, Release, and Deploy

| Property | Decision |
|:---------|:---------|
| **Build system** | GitHub Actions release workflow plus Go toolchain |
| **Release trigger** | Push a SemVer-style `v*` tag after human approval per GOV-007 |
| **Artifacts** | Server and client binaries for Linux, macOS, and Windows plus checksums |
| **Docker image** | Runtime-only Alpine image that copies pre-built Linux binaries from `bin/` |
| **Deploy mechanism** | GitHub Actions SSHes to production, checks out the release tag, copies binaries, and runs Docker Compose |
| **Production deploy command** | Workflow executes `docker compose down || true && docker compose up -d --build` on the host |

> **Release control:** Production deployment remains tag-triggered. Branch pushes and PR merges must not deploy production unless the release workflow is explicitly changed by an approved infrastructure track.

---

## 5. Adaptation Table

> Per GOV-007 §9.7: when architecture documents assume capabilities that differ from actual deployment, document the adaptation here.

| Prior Assumption | Actual Deployment | Adaptation |
|:-----------------|:------------------|:-----------|
| Prior embedded database assumption | PostgreSQL container | Persist `fonzygrok-pgdata`; use `pg_dump`/`psql` for backup and restore |
| Managed database service | Self-managed PostgreSQL in Docker Compose | Secure credentials in `.env` / GitHub secrets; monitor container health |
| Prior multi-stage/minimal app image assumption | Runtime-only Alpine image | GitHub Actions pre-builds binaries; Dockerfile copies them into Alpine |
| Manual release artifacts | GitHub Actions release workflow | `v*` tags build binaries, checksums, and GitHub Release assets |
| Manual deploy only | Tag-triggered GitHub Actions deploy | Workflow SSHes to host and restarts Compose after release build |
| Load balancer / CDN | No load balancer | Server binds directly to host ports 80/443/2222 and TCP range |
| DNS management API | Manual DNS setup | Human configures apex and wildcard DNS records |
| Secret management service | GitHub Actions secrets + host `.env` | Never commit secrets; restrict `.env` file permissions |

---

## 6. Security

| Concern | Mitigation |
|:--------|:-----------|
| SSH brute force | Token auth only; rate-limit connection attempts |
| Admin API exposure | Keep port `9090` internal by default; access from host or trusted SSH session |
| Container privileges | Run application container as non-root user |
| PostgreSQL credential exposure | Use non-default `POSTGRES_PASSWORD`; store in `.env`/secrets only |
| PostgreSQL persistence | Restrict host and Docker volume access to trusted operators |
| TLS private material | Persist cert cache in `fonzygrok-certs`; do not copy cert material into git |
| CI/CD deploy key | Store deploy SSH key only in GitHub Actions secrets |

---

## 7. Monitoring and Operations

| Tool | Purpose | Priority |
|:-----|:--------|:---------|
| GitHub Actions logs | Build, release, and deploy audit trail | v1.2+ |
| Docker health checks | PostgreSQL and server health in Compose | v1.2+ |
| Admin health endpoint | `GET /api/v1/health` on `localhost:9090` from host/container | v1.0+ |
| Structured logs (`slog`) | Application logs to stdout captured by Docker | v1.0+ |
| `docker compose logs -f` | Live operational log tailing | v1.0+ |
| PostgreSQL backup files | Point-in-time logical backups | v1.2+ |

---

## 8. Operational Runbooks

| Runbook | Topics |
|:--------|:-------|
| `RUN-001` | Production deployment, tag-triggered release/deploy, verification, PostgreSQL backup/restore |

---

## 9. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-04-24 | 2.0.0 | Updated infrastructure standard for PostgreSQL, Alpine Docker runtime, GitHub Actions tag-triggered release/deploy, and PostgreSQL backup/restore | Hermes Agent |
| 2026-03-31 | 1.0.0 | Initial infrastructure decisions | Architect + Human |
