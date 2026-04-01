---
id: SPR-016
title: "Sprint 016: CI/CD Auto-Deploy + README Overhaul"
type: how-to
status: ACTIVE
owner: architect
agents: [architect, developer-b]
tags: [sprint, ci-cd, documentation, infrastructure]
related: []
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-016: CI/CD Auto-Deploy + README Overhaul

## Goal

1. Extend release workflow to auto-deploy server to production on tagged releases
2. Rewrite README with comprehensive end-user documentation

---

## Track A — Architect (CI/CD)

### T-051: Release Workflow Auto-Deploy

Extend `.github/workflows/release.yml` with a `deploy` job that:

1. Runs after the `release` job succeeds
2. SSHs into production EC2 using GitHub Secrets
3. Pulls the tagged commit
4. Rebuilds and restarts Docker container
5. Runs a health check

**Required GitHub Secrets:**
- `DEPLOY_SSH_KEY` — PEM private key
- `DEPLOY_HOST` — EC2 IP (3.139.160.15)
- `DEPLOY_USER` — ubuntu

**Auto-deploy on tag push — no manual gate.**

---

## Track B — Dev B (README)

### T-052: README Overhaul

Rewrite `README.md` with comprehensive documentation. **Client usage comes
first, server/self-hosting at the end.**

**Structure (in this exact order):**

```
# Fonzygrok
  Tagline, one-sentence description

## What is Fonzygrok?
  2-paragraph explanation. Include ASCII diagram showing the flow.

## Quick Start
  3-step: download binary → get token → connect
  Use fonzygrok.com as the example server

## Installation
  ### Download Binary (recommended)
    Table of platform download links (use GitHub release URL pattern)
  ### Build from Source
    git clone + go build

## Client Usage
  ### Connecting
    Basic --server --token --port with example output (the pretty output)
  ### Custom Subdomains
    --name flag, URL pattern
  ### Request Inspector
    localhost:4040, screenshot description, --no-inspect to disable
  ### Config File
    ~/.fonzygrok.yaml example with all fields
  ### Verbose Mode
    --verbose for JSON structured logs
  ### Environment Variables
    FONZYGROK_SERVER, FONZYGROK_TOKEN
  ### Complete Flag Reference
    Table of all client flags

## Self-Hosting
  ### Prerequisites
    - Domain with wildcard DNS (*.tunnel.yourdomain.com → your server IP)
    - VPS/EC2 with ports 80, 443, 2222 open
    - Docker + Docker Compose
  ### Docker Deployment
    docker-compose.yml example, .env configuration
  ### TLS/HTTPS Setup
    Set TLS_ENABLED=true, Let's Encrypt auto-provisioning
  ### Token Management
    fonzygrok-server token create/list/revoke examples
  ### Server Configuration Reference
    Full flag reference table + env var table
  ### Architecture
    Component diagram, port descriptions

## Troubleshooting
  Common errors:
  - "missing port in address" → use domain only, port auto-added
  - "connection refused" → check firewall, port 2222
  - "tunnel not found" → check DNS wildcard setup
  - Inspector not loading → check localhost:4040, try --no-inspect

## License
```

**Rules:**
- Use the actual CLI help text and real examples throughout
- Show the pretty output format (with checkmarks) in examples
- Link download URLs to `github.com/johncrowleydev/fonzygrok/releases/latest`
- Do NOT use placeholder screenshots — describe the inspector UI in text
- Keep it under 500 lines — concise, scannable, no padding

---

## Acceptance Criteria

- [ ] Release workflow deploys to production on tag push
- [ ] Health check passes after deploy
- [ ] README covers all client flags and server setup
- [ ] README renders correctly on GitHub
- [ ] No broken links in README
