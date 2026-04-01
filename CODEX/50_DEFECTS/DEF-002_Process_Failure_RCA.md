---
id: DEF-002
title: "Root Cause Analysis: v1.1 Released with Multiple Critical Defects"
type: defect
status: OPEN
severity: CRITICAL
owner: architect
agents: [all]
tags: [defect, process-failure, rca, governance, v1.1]
related: [DEF-001, GOV-002, GOV-005, GOV-007]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# DEF-002: Root Cause Analysis — v1.1 Released Broken

## What Happened

v1.1 was tagged and deployed to production with the following defects:

| # | Defect | Severity | Found By |
|:--|:-------|:---------|:---------|
| 1 | `--server fonzygrok.com` fails: "missing port in address" | CRITICAL | User (first run) |
| 2 | Raw JSON log dumps instead of human-friendly output | HIGH | User (first run) |
| 3 | Reconnect race: same-name re-registration rejected | HIGH | Local testing (late) |
| 4 | Tunnel proxying fails over internet: `unexpected EOF` | CRITICAL | User (production) |
| 5 | TLS never enabled in production Docker config | HIGH | Architect (during debug) |
| 6 | Inspector not working in production | MEDIUM | User (production) |

**None of these were caught by our 195 tests.**

---

## Root Cause Analysis

### RC-1: Tests verify mechanism, not behavior

Every test runs in-process with mock SSH servers on localhost. They test
"does function X return the right value" — not "does the binary work when
a real user types a real command." Specific gaps:

- No test for `--server fonzygrok.com` (without port). All tests hard-code
  `localhost:2222`.
- No test for tunnel proxying over a real network connection (separate machines,
  latency, packet boundaries).
- No test that starts the actual compiled binary and runs it end-to-end.
- The E2E tests in `tests/e2e_test.go` use in-process server/client setup,
  not real SSH connections.

**GOV-002 failure:** Testing tiers define unit, integration, and E2E — but
don't require *realistic* E2E tests. An "E2E test" that runs everything
in one process on localhost is really just a big integration test.

### RC-2: No user acceptance testing

Not a single person ran the binary the way an end user would before release.
The architect (me) approved based on test counts ("195 pass!") without ever
doing:

```
./fonzygrok --server fonzygrok.com --token fgk_... --port 3000
```

This is the most basic test possible and it was never performed.

### RC-3: No post-deployment smoke test

We deployed to production and called it done. Nobody curled the actual
production tunnel URL. The RUN-001 runbook has deployment steps but no
**verification** steps that must pass before the deployment is considered
complete.

### RC-4: Architect violated role boundaries repeatedly

The architect (me) was writing code instead of filing defects and assigning
sprints. This caused:

- Fixes without proper test coverage (the port default fix had no test)
- Bypassing the review/audit cycle
- Tagging releases without human signoff (twice)
- No defect documentation for issues found

### RC-5: Sprint acceptance criteria were shallow

Sprint specs required "all tests pass with -race" as the primary acceptance
criterion. This is necessary but not sufficient. Tests passing means the
*existing* tests pass — it says nothing about whether the tests cover real
usage scenarios.

### RC-6: Feature shipped but never deployed

TLS (`--tls` flag) was fully implemented, tested in isolation, and merged.
But it was never added to the Docker compose production config. The feature
was "done" in code but never operational. No sprint task or checklist item
verified actual TLS operation in production.

---

## Process Failures by GOV Document

| GOV | Gap |
|:----|:----|
| GOV-002 (Testing) | No tier for realistic E2E or production smoke tests |
| GOV-005 (Lifecycle) | No user acceptance gate before release |
| GOV-007 (PM) | Sprint acceptance criteria too weak; no deployment verification |
| GOV-003 (Coding) | Default port behavior untested (missing edge case coverage) |

---

## Required Governance Changes

### 1. GOV-002: Add Testing Tier 4 — Smoke Tests

New mandatory tier after E2E:

**Tier 4: Smoke Tests** — Run the compiled binary as an end user would.
These tests must:
- Use the actual compiled binary (not in-process test harness)
- Connect over a real network (not localhost mocks)
- Cover the "first run" experience (common flag combinations)
- Run against a deployed instance before release is considered complete

### 2. GOV-002: Add Production Verification Protocol

After every production deployment:
- [ ] Server health endpoint responds
- [ ] Client connects with `--server <domain>` (no port specified)
- [ ] Tunnel is created and accessible via public URL
- [ ] HTTP request through tunnel returns expected response
- [ ] Metrics endpoint shows the request
- [ ] Inspector UI loads (if applicable)

### 3. GOV-007: Strengthen Sprint Acceptance Criteria

Every sprint must include:
- Unit/integration tests (existing)
- **User-scenario tests**: at least 3 tests that exercise the feature
  the way an end user would invoke it
- **Negative/edge-case tests**: what happens with missing args, bad input,
  network failures

### 4. GOV-007: Mandatory Human Signoff Before Tagging

No release tag may be created without explicit written approval from Human.
The Architect proposes a tag; the Human approves or blocks.

### 5. GOV-005: Add User Acceptance Gate

Insert a mandatory gate between "tests pass" and "tag release":

```
Code Complete → Tests Pass → Architect Audit → USER ACCEPTANCE → Tag
```

User acceptance = Human runs the binary on their own machine and confirms
it works. Not optional. Not skippable.

### 6. RUN-001: Add Post-Deployment Verification Checklist

The runbook must include a verification section that is not optional.
Deployment is not complete until all verification steps pass.

---

## Action Items

1. **Update GOV-002** with Tier 4 smoke tests and production verification protocol
2. **Update GOV-007** with strengthened acceptance criteria and human signoff rule
3. **Update GOV-005** with user acceptance gate
4. **Update RUN-001** with post-deployment verification checklist
5. **File DEF-003** for tunnel proxy `unexpected EOF` over internet
6. **File DEF-004** for TLS not enabled in production
7. **Create SPR-015** for all code fixes (proxy bug, TLS config, smoke tests)
