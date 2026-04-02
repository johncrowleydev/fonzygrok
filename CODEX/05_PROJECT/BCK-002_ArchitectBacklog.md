---
id: BCK-002
title: "Architect Agent Backlog"
type: planning
status: ACTIVE
owner: architect
agents: [architect]
tags: [project-management, backlog, architect, audit, deployment]
related: [BCK-001, GOV-007, GOV-008]
created: 2026-03-31
updated: 2026-04-02
version: 2.0.0
---

> **BLUF:** The Architect Agent has its own work stream separate from developer sprints. This backlog tracks: CODEX bootstrapping, sprint audits, infrastructure prep, contract compliance testing, deployment verification, and document maintenance. v1.0/v1.1 items complete. v1.2 items active.

# Architect Agent Backlog — Fonzygrok

---

## Work Categories

| Category | Code | Description |
|:---------|:-----|:------------|
| **Infrastructure** | ARCH-INFRA | Docker setup, VPS prep, deployment scripts |
| **Audit** | ARCH-AUDIT | Sprint audit against contracts + GOV docs |
| **Integration** | ARCH-INTEG | E2E testing, contract compliance |
| **Deploy** | ARCH-DEPLOY | Production deployment execution |
| **CODEX** | ARCH-CODEX | Document maintenance, MANIFEST, sprint creation |
| **Monitor** | ARCH-MON | Developer progress monitoring, blocker resolution |
| **Governance** | ARCH-GOV | Governance doc updates, process improvements |

---

## Completed Tasks

| ID | Task | Category | Status |
|:---|:-----|:---------|:-------|
| A-001 | Bootstrap CODEX: PRJ-001, GOV-008, BLU-001, CON-001, CON-002 | ARCH-CODEX | [x] |
| A-002 | Create BCK-001 (developer backlog) | ARCH-CODEX | [x] |
| A-003 | Create all sprint docs (SPR-001 through SPR-013) | ARCH-CODEX | [x] |
| A-004 | Create developer boot docs (AGT-002-FG, -FG-A, -FG-B) | ARCH-CODEX | [x] |
| A-005 | Update MANIFEST.yaml with all new docs | ARCH-CODEX | [x] |
| A-006 | Audit SPR-001 output | ARCH-AUDIT | [x] |
| A-007 | Audit SPR-002A/B output | ARCH-AUDIT | [x] |
| A-008 | Audit SPR-003A/B output | ARCH-AUDIT | [x] |
| A-009 | Audit SPR-004A output | ARCH-AUDIT | [x] |
| A-010 | Audit SPR-005 output | ARCH-AUDIT | [x] |
| A-011 | Audit SPR-006–009 output (v1.1) | ARCH-AUDIT | [x] |
| A-012 | File DEF-001 through DEF-004 | ARCH-CODEX | [x] |
| A-013 | Create SPR-014 through SPR-019 | ARCH-CODEX | [x] |
| A-014 | Audit SPR-014–019 output | ARCH-AUDIT | [x] |
| A-015 | CI/CD auto-deploy pipeline (SPR-016 T-051) | ARCH-INFRA | [x] |
| A-016 | v1.1.0 production deployment | ARCH-DEPLOY | [x] |
| A-017 | v1.1.2 production deployment (bug fix release) | ARCH-DEPLOY | [x] |
| A-018 | DEF-002 Root Cause Analysis | ARCH-CODEX | [x] |
| A-019 | Full CODEX reconciliation (sprint/defect/contract statuses) | ARCH-CODEX | [x] |

---

## Active Tasks

| ID | Task | Category | Dependencies | Status |
|:---|:-----|:---------|:-------------|:-------|
| A-020 | Apply DEF-002 governance changes (GOV-002, GOV-005, GOV-007) | ARCH-GOV | A-018 | [/] |
| A-021 | Contract compliance test (CON-001) | ARCH-INTEG | All v1.2 sprints | [ ] |
| A-022 | Contract compliance test (CON-002 + auth extensions) | ARCH-INTEG | All v1.2 sprints | [ ] |
| A-023 | E2E integration test (full tunnel round-trip with auth) | ARCH-INTEG | SPR-013 | [ ] |
| A-024 | Update CON-002 for auth/dashboard routes | ARCH-CODEX | SPR-018/019 | [ ] |
| A-025 | Create deployment runbooks (RUN-FG-001, 002) | ARCH-INFRA | SPR-013 | [ ] |
| A-026 | Final CODEX reconciliation (post v1.2) | ARCH-CODEX | All v1.2 sprints | [ ] |
| A-027 | Tag release v1.2.0 | ARCH-DEPLOY | A-023, Human approval | [ ] |

---

## Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-03-31 | 1.0.0 | Initial architect backlog | Architect |
| 2026-04-02 | 2.0.0 | Full reconciliation: marked completed items, added v1.2 items | Architect |
