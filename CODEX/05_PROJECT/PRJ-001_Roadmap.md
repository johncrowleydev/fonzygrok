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

### Phase 1: MVP (v1.0)
**Goal:** A working tunnel server + client that routes HTTP traffic through an SSH tunnel.
**Exit criteria:**
- [ ] Server accepts SSH connections from authenticated clients
- [ ] Client connects, registers a tunnel, receives a public subdomain
- [ ] Public HTTP requests to the subdomain are proxied to the client's local port
- [ ] Auto-reconnect works on network interruption
- [ ] Docker Compose deploys the server on a fresh VPS
- [ ] All code has tests, structured logging, and structured error handling
- [ ] End-to-end test: `fonzygrok --port 3000` → public URL → local service responds

**Key deliverables:**
- `BLU-001` — System Architecture Blueprint
- `CON-001` — Client-Server Protocol Contract
- `CON-002` — HTTP Edge & Admin API Contract
- `SPR-001` through `SPR-007` — Development sprints

---

### Phase 2: Polish (v1.1)
**Goal:** Production-quality UX with custom subdomains, TLS, request inspection, and config files.
**Exit criteria:**
- [ ] Custom subdomain reservation (`fonzygrok --subdomain myapp`)
- [ ] Let's Encrypt auto-TLS on the public edge
- [ ] Local request inspection UI at `localhost:4040`
- [ ] YAML config file support for server and client
- [ ] Connection metrics (bytes, requests, latency)

**Key deliverables:**
- `SPR-008` through `SPR-012` — Polish sprints

---

### Phase 3: Production-Grade (v1.2)
**Goal:** Multi-user management, rate limiting, and a web dashboard.
**Exit criteria:**
- [ ] Web dashboard for managing tokens, viewing active tunnels
- [ ] Per-tunnel and per-user rate limiting
- [ ] Wildcard subdomain DNS routing
- [ ] TCP tunneling support (port-based)
- [ ] Access control (IP whitelisting)

**Key deliverables:**
- Sprint plan TBD based on Phase 1 and 2 learnings

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
| `CON-001` | Client-Server Protocol (SSH transport, control messages, tunnel lifecycle) | `DRAFT` |
| `CON-002` | HTTP Edge & Admin API (routing, subdomain assignment, admin endpoints) | `DRAFT` |

---

## 7. Success Criteria

This project is complete when:
- [ ] All phases completed and archived
- [ ] All contracts at `STABLE` status
- [ ] All `DEF-` defects resolved or explicitly deferred
- [ ] Human signs off on final verification report
- [ ] Docker deployment verified on a fresh VPS

---

## 8. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial roadmap | Human + Architect |
