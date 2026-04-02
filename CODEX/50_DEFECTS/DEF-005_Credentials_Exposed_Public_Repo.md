---
id: DEF-005
title: "Security Incident: Private Keys and Production Database Committed to Public Repo"
type: defect
status: OPEN
severity: CRITICAL
priority: P0
owner: architect
agents: [all]
tags: [defect, security, incident, credentials, public-repo]
related: [GOV-003, GOV-005]
created: 2026-04-02
updated: 2026-04-02
version: 1.0.0
---

> **BLUF:** Private SSH keys and the production database (containing password hashes, API tokens, and invite codes) were committed to the `tmp/` directory and pushed to a public GitHub repository. Immediate remediation applied: files removed from git index, .gitignore hardened, pre-commit hook installed. Credential rotation and git history scrub still required.

# Security Incident: Credentials Exposed in Public Repository

> **"If your private keys are in a public repo, they are no longer private keys."**

---

## 1. Incident Timeline

| Time | Event |
|:-----|:------|
| Unknown (≤2026-03-31) | `tmp/` directory committed to git with SSH host key, production DB, and compiled binaries |
| Unknown | Changes pushed to public repo `johncrowleydev/fonzygrok` on GitHub |
| 2026-04-02 16:52 | Human reported the incident |
| 2026-04-02 16:55 | Architect began investigation |
| 2026-04-02 16:56 | Files removed from git index (`git rm --cached`) |
| 2026-04-02 16:56 | `.gitignore` hardened with comprehensive secret patterns |
| 2026-04-02 17:01 | Pre-commit hook created and activated |
| 2026-04-02 17:05 | Fix pushed to origin |
| 2026-04-02 ~17:00 | Human made repo private on GitHub |

---

## 2. Exposed Assets

| File | Type | Risk |
|:-----|:-----|:-----|
| `fonzygrok.pem` | RSA private key — EC2 deploy key | Anyone with this key can SSH into the production server |
| `tmp/data/host_key` | OpenSSH private key — SSH server host key | Attacker can impersonate the fonzygrok server (MITM) |
| `tmp/data/fonzygrok.db` | SQLite database — production | Contains bcrypt password hashes, API token hashes, invite codes |
| `tmp/data/fonzygrok.db-shm` | SQLite shared memory | May contain partial writes of sensitive data |
| `tmp/data/fonzygrok.db-wal` | SQLite write-ahead log | Contains all recent DB modifications |
| `tmp/fonzygrok-client` | Compiled binary | Low risk — but reveals exact build for reverse engineering |

---

## 3. Root Cause Analysis

### Immediate Cause
The `tmp/` directory was not in `.gitignore`. A `git add .` or `git add -A` swept it into the index, and the developer did not inspect staged files before committing.

### Contributing Factors

1. **No `tmp/` entry in `.gitignore`.** The original `.gitignore` covered binaries, `.env`, and `*.pem`, but had no entry for `tmp/`, `*.db`, `host_key`, or other runtime artifacts.

2. **No pre-commit hook.** There was no automated defense against committing secrets. The only protection was the developer's attention — which failed.

3. **`.gitignore` added `*.pem` reactively, not proactively.** The `*.pem` pattern was on line 43 (last line), suggesting it was added as an afterthought after the key was already on disk — but `fonzygrok.pem` was already in the repo root.

4. **No secret scanning enabled on GitHub.** GitHub offers free secret scanning and push protection for public repos. Neither was enabled.

5. **No governance requirement for secret management.** GOV-003 (Coding Standard) has no section on credential handling, `.gitignore` requirements, or secret management patterns.

6. **Public repo by default.** The repo was public despite containing infrastructure for a production system.

---

## 4. Remediation — Completed

| # | Action | Status |
|:--|:-------|:-------|
| 1 | Remove all tracked sensitive files from git index | ✅ Done |
| 2 | Harden `.gitignore` with comprehensive patterns (keys, certs, DBs, tmp/) | ✅ Done |
| 3 | Create pre-commit hook blocking secret commits | ✅ Done |
| 4 | Activate hook via `core.hooksPath` | ✅ Done |
| 5 | Push fix to origin | ✅ Done |
| 6 | Make repo private on GitHub | ✅ Done (Human) |

## 5. Remediation — Still Required

| # | Action | Owner | Status |
|:--|:-------|:------|:-------|
| 7 | **Generate new EC2 SSH key pair** — replace `fonzygrok.pem` | Human | ⏳ |
| 8 | **Update GitHub Secrets** — replace `DEPLOY_SSH_KEY` with new key | Human | ⏳ |
| 9 | **Regenerate SSH host key** on production server | Human | ⏳ |
| 10 | **Regenerate all API tokens** — old token hashes are exposed | Human | ⏳ |
| 11 | **Regenerate JWT secret** — invalidates all sessions | Human | ⏳ |
| 12 | **Invalidate all invite codes** | Human | ⏳ |
| 13 | **Scrub git history** — `git filter-repo` to remove all traces | Human + Architect | ⏳ |
| 14 | **Enable GitHub secret scanning** + push protection | Human | ⏳ |
| 15 | **Update GOV-003** — add Secret Management section | Architect | ⏳ |
| 16 | **Force password change** on all user accounts (if any beyond admin) | Human | ⏳ |

---

## 6. Prevention Measures Implemented

### 6.1 Hardened `.gitignore`
Added 40+ patterns covering all private key formats, database files, credential files, and the entire `tmp/` directory.

### 6.2 Pre-Commit Hook (`.githooks/pre-commit`)
- Blocks commits with dangerous filename patterns (`.pem`, `.key`, `.db`, `host_key`, etc.)
- Scans file contents for private key headers (`BEGIN RSA PRIVATE KEY`, etc.)
- Blocks any file in `tmp/` directory
- Installed via `git config core.hooksPath .githooks` (survives clone)
- Can be bypassed with `--no-verify` (intentional escape hatch with warning)

### 6.3 Proposed Governance Changes
- **GOV-003 §NEW**: Secret Management section (mandatory `.gitignore` patterns, credential handling rules)
- **GitHub**: Secret scanning and push protection enabled on repo

---

## 7. Change Log

| Date | Version | Change | Author |
|:-----|:--------|:-------|:-------|
| 2026-04-02 | 1.0.0 | Initial incident report | Architect |
