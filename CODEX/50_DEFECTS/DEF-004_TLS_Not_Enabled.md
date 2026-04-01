---
id: DEF-004
title: "TLS not enabled in production Docker configuration"
type: defect
status: OPEN
severity: HIGH
owner: architect
agents: [developer-a]
tags: [defect, tls, docker, deployment, v1.1]
related: [DEF-002, SPR-015]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# DEF-004: TLS Not Enabled in Production Docker Configuration

## Summary

Auto-TLS (SPR-007A) was implemented, tested in isolation, and merged. But the
Docker compose command in production never includes `--tls` or
`--tls-cert-dir`. The feature is dead code in production.

## Root Cause

The docker-compose.yml `command:` section doesn't include TLS flags:

```yaml
command:
  - serve
  - --data-dir=/data
  - --ssh-addr=:2222
  - --http-addr=:8080
  - --admin-addr=0.0.0.0:9090
  - --domain=${DOMAIN:-tunnel.localhost}
  # MISSING: --tls --tls-cert-dir=/data/certs
```

The sprint (SPR-007A) added the code and unit tests but the acceptance criteria
didn't require verifying TLS in the actual Docker deployment config.

## Required Fix

1. Add `--tls` and `--tls-cert-dir=/data/certs` to docker-compose.yml command
2. Add `TLS_ENABLED` env var toggle in `.env` (default: true in production)
3. Verify Let's Encrypt cert issuance works for `*.tunnel.fonzygrok.com`
4. Smoke test HTTPS from a remote client after deployment

## Prerequisites

- Port 443 is already exposed in docker-compose.yml
- DNS for `*.tunnel.fonzygrok.com` already points to EC2
- Cert volume `fonzygrok-certs` already exists
