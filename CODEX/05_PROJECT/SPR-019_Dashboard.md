---
id: SPR-019
title: "Sprint 019: Web Dashboard"
type: how-to
status: ACTIVE
owner: architect
agents: [developer-b]
tags: [sprint, dashboard, frontend, ui, v1.2]
related: [SPR-017, SPR-018]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-019: Web Dashboard

## Goal

Build a server-rendered web dashboard for user self-service: login,
registration, token management, and admin views. Uses Go templates +
HTMX — no npm, no build step, no heavy JS framework.

**Depends on:** SPR-017 (merged), SPR-018 (API endpoints — in parallel)

**Coordination:** Dev B builds the frontend that calls the API endpoints
Dev A is building in SPR-018. Use the endpoint specs from SPR-018 as
your contract. If Dev A hasn't merged yet, mock the responses.

---

## Assigned: Dev B (Client-side / Frontend)

Branch: `feature/SPR-019-dashboard`

**Pull main first.**

---

## Tasks

### T-069: Dashboard Server + Template Engine

Create `internal/server/dashboard.go`:

- Go `html/template` with a base layout template
- Register routes on the admin router (`:9090`)
- Dashboard routes use path-based routing (not subdomain)
- Static file serving for CSS/JS assets

Route structure:
```
GET  /                    → redirect to /dashboard (if auth) or /login
GET  /login               → login page
POST /login               → submit login form (calls API)
GET  /register            → registration page
POST /register            → submit registration (calls API)
POST /logout              → clear session, redirect to /login
GET  /dashboard           → main dashboard (auth required)
GET  /admin/users         → user management (admin only)
GET  /admin/invite-codes  → invite code management (admin only)
```

---

### T-070: Base Layout + CSS

Create `internal/server/templates/` directory with embedded templates:

**Base layout** (`layout.html`):
- Clean, modern dark theme
- Responsive (works on mobile)
- Navigation: Dashboard | Admin (if admin) | Logout
- Flash messages for success/error feedback

**CSS** (`static/style.css`):
- Dark background (#0f172a or similar slate)
- Accent color for buttons and links (emerald or cyan)
- Card-based layout for content sections
- Form styling that matches the CLI aesthetics
- Table styling for token and user lists
- No external CSS frameworks — vanilla CSS

**Use `embed.FS`** to embed templates and static files in the binary.
No external file dependencies at runtime.

---

### T-071: Login Page

`/login` — clean login form:
- Username/email field
- Password field
- Submit button
- Link to registration page
- Error message display (flash)
- On success: redirect to `/dashboard`

---

### T-072: Registration Page

`/register` — registration form:
- Username field
- Email field
- Password field (with strength indicator text)
- Invite code field
- Submit button
- Link to login page
- Error message display (invalid code, username taken, etc.)
- On success: redirect to `/dashboard`

---

### T-073: Dashboard (Main Page)

`/dashboard` — user's home:

**My Tokens section:**
- Table: Name | Token ID | Created | Last Used | Status | Actions
- "Create New Token" button → shows form inline (HTMX)
- Token creation shows the raw token ONCE with copy button
- Revoke button per token (HTMX, with confirmation)

**Active Tunnels section:**
- Table: Name | Public URL | Local Port | Connected Since
- Real-time updates via HTMX polling (every 5s)
- Empty state: "No active tunnels. Connect with: fonzygrok --server ..."

---

### T-074: Admin Pages

`/admin/users` (admin only):
- User list: Username | Email | Role | Created | Last Login | Active
- No user CRUD for now — just viewing

`/admin/invite-codes` (admin only):
- Table: Code | Created By | Used By | Created At | Status
- "Generate New Code" button → HTMX inline creation
- Shows new code prominently (copy button)

---

### T-075: HTMX Integration

Use HTMX (single JS file, embedded) for:
- Token creation without full page reload
- Token revocation with confirmation
- Invite code generation inline
- Tunnel list polling
- Flash message auto-dismiss

Include HTMX via embed.FS — download the minified JS and embed it.
**Do NOT use a CDN** — the dashboard must work without internet access.

---

### T-076: Auth Flow in Dashboard

- Login/register forms POST to the API endpoints (SPR-018)
- API returns JWT in Set-Cookie header
- Subsequent page loads send cookie automatically
- Dashboard routes check for valid JWT via middleware
- Admin routes check for admin role
- Invalid/expired session → redirect to `/login`

---

## Design Guidelines

The dashboard should feel like a developer tool, not a corporate app:
- **Dark theme** — matches terminal aesthetics
- **Monospace fonts** for tokens, codes, URLs
- **Minimal** — no unnecessary decoration
- **Fast** — server-rendered, no SPA loading spinners
- **Copy buttons** on tokens, codes, and URLs
- **Responsive** — usable on mobile for quick checks

---

## Acceptance Criteria

- [ ] Login, register, dashboard pages render correctly
- [ ] Auth flow works: register → login → see tokens → create token
- [ ] Admin pages only accessible to admin role
- [ ] HTMX interactions work (token create/revoke, invite code create)
- [ ] All templates embedded in binary (no external files)
- [ ] Dark theme, responsive, clean design
- [ ] All 251+ tests pass with -race
- [ ] New tests: template rendering, auth redirects, admin role check
