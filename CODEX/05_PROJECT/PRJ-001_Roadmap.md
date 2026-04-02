---
id: PRJ-001
title: "Fonzygrok — Roadmap"
type: explanation
status: APPROVED
owner: human
agents: [all]
tags: [project-management, roadmap, governance, agentic-development]
related: [BCK-001, RES-001]
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** Fonzygrok is a self-hosted, open-source ngrok alternative written in Go. It lets users establish secure tunnels from their local machines to a public server, exposing local HTTP services to the internet via auto-assigned subdomains. The goal is a single-binary, Docker-deployable tunnel server + client that is simple, fast, and self-hostable.

# Fonzygrok — Project Roadmap

> **This document is authored by the Human. The Architect Agent maintains it.**

---

## 1. Project Vision

Fonzygrok solves the problem of exposing local development services to the public internet without port forwarding, static IPs, or third-party SaaS dependencies. Developers and small teams need to share local work, receive webhooks, or demo features — without trusting their traffic to someone else's infrastructure.

The tool ships as two binaries (`fonzygrok-server` and `fonzygrok`) compiled from a single Go codebase. The server runs on a VPS inside Docker. The client runs on the developer's machine. One command connects them via an encrypted SSH tunnel.

Success means: a developer can run `fonzygrok --port 3000` and immediately get a public URL that routes traffic to their local service, with zero configuration beyond an auth token.

---

## 2. Guiding Principles

- **Single binary, zero deps:** Both server and client are static Go binaries. No runtime dependencies.
- **Self-hostable first:** Every feature works on a single VPS with Docker. No cloud vendor lock-in.
- **SSH as transport:** Proven, encrypted, multiplexed. No custom protocol unless SSH proves insufficient.
- **TCP-ready architecture:** v1 is HTTP-only, but all abstractions must accommodate raw TCP tunneling later.
- **Governance from day one:** Structured logging, error handling, and tests ship with every sprint — not bolted on later.

---

## 3. Scope

### 3.1 In Scope
- Encrypted tunnel server accepting client connections over SSH
- HTTP reverse proxy routing public requests to tunneled clients via Host header
- Token-based authentication for clients
- SQLite storage for tokens, tunnel state, connection logs
- CLI interface for both server and client
- Auto-reconnect with exponential backoff
- Docker-based deployment for the server
- Structured JSON logging

### 3.2 Out of Scope
- TCP/UDP tunneling (deferred to v1.2+, but architecture must support it)
- Multi-server / distributed deployment
- OAuth / SSO authentication
- Paid tier / billing integration
- Mobile clients
- Windows service mode (cross-compile yes, service management no)

---

## 4. Delivery Phases

Phases are **scope-bounded**, not time-bounded. Each phase is complete when its exit criteria are met.

### Phase 1: MVP (v1.0) ✅ COMPLETE
**Goal:** A working tunnel server + client that routes HTTP traffic through an SSH tunnel.
**Exit criteria:**
- [x] Server accepts SSH connections from authenticated clients
- [x] Client connects, registers a tunnel, receives a public subdomain
- [x] Public HTTP requests to the subdomain are proxied to the client's local port
- [x] Auto-reconnect works on network interruption
- [x] Docker Compose deploys the server on a fresh VPS
- [x] All code has tests, structured logging, and structured error handling
- [x] End-to-end test: `fonzygrok --port 3000` → public URL → local service responds

**Key deliverables:**
- `BLU-001` — System Architecture Blueprint (APPROVED)
- `CON-001` — Client-Server Protocol Contract (STABLE)
- `CON-002` — HTTP Edge & Admin API Contract (STABLE)
- `SPR-001` through `SPR-005` — Foundation + integration sprints (COMPLETE)

---

### Phase 2: Polish (v1.1) ✅ COMPLETE — tagged v1.1.0, patched to v1.1.2
**Goal:** Production-quality UX with custom subdomains, TLS, request inspection, and config files.
**Exit criteria:**
- [x] Custom subdomain reservation (`fonzygrok --name myapp`)
- [x] Let's Encrypt auto-TLS on the public edge
- [x] Local request inspection UI at `localhost:4040`
- [x] YAML config file support for server and client
- [x] Connection metrics (bytes, requests, latency)

**Additional work (post-v1.1.0):**
- `SPR-014` — Pretty client output (DEF-001 fix)
- `SPR-015` — Critical proxy EOF + TLS fixes → tagged v1.1.2
- `SPR-016` — CI/CD auto-deploy + README overhaul

**Key deliverables:**
- `SPR-006` through `SPR-009` — v1.1 feature sprints (COMPLETE)
- `SPR-014` through `SPR-016` — Post-release fixes + infra (COMPLETE)

---

### Phase 3: Production-Grade (v1.2) 🔄 IN PROGRESS
**Goal:** Multi-user management, rate limiting, and a web dashboard.
**Exit criteria:**
- [x] Web dashboard for managing tokens, viewing active tunnels
- [x] User authentication (invite codes, bcrypt, JWT sessions)
- [x] Self-service registration and token management
- [ ] Per-tunnel and per-user rate limiting
- [ ] TCP tunneling support (port-based)
- [ ] Access control (IP whitelisting)

**Completed deliverables:**
- `SPR-017` — User model + auth backend (COMPLETE)
- `SPR-018` — API endpoints + auth middleware (COMPLETE)
- `SPR-019` — Web dashboard (COMPLETE)

**Remaining deliverables:**
- `SPR-010` — TCP tunneling (READY)
- `SPR-011` — Rate limiting (READY)
- `SPR-012` — IP whitelisting / ACL (READY)
- `SPR-013` — v1.2 integration + release (READY)

---

## 5. Agent Team

| Role | Agent Type | Primary CODEX Docs |
|:-----|:-----------|:------------------|
| Project manager | Architect Agent | `AGT-001`, all `SPR-`, all `CON-` |
| Implementation | Developer Agent | `AGT-002-FG`, assigned `SPR-` |
| Quality | Tester Agent | `AGT-003`, `VER-`, `40_VERIFICATION/` |

---

## 6. Key Contracts

| Contract | Description | Status |
|:---------|:------------|:-------|
| `CON-001` | Client-Server Protocol (SSH transport, control messages, tunnel lifecycle) | `STABLE` |
| `CON-002` | HTTP Edge & Admin API (routing, subdomain assignment, admin endpoints, auth) | `STABLE` |

---

## 7. Success Criteria

This project is complete when:
- [ ] All phases completed and archived
- [x] All contracts at `STABLE` status
- [ ] All `DEF-` defects resolved or explicitly deferred
- [ ] Human signs off on final verification report
- [x] Docker deployment verified on a fresh VPS

---

## 8. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial roadmap | Human + Architect |
| 2026-04-02 | 1.1.0 | Reconciliation: Phase 1+2 complete, Phase 3 partially complete, contract statuses updated | Architect |
