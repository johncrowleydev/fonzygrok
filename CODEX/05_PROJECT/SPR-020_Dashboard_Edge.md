---
id: SPR-020
title: "Sprint 020: Dashboard HTTPS Edge + Light/Dark Theme + TCP Proto"
type: how-to
status: READY
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, dashboard, tls, ui, tcp, v1.2]
related: [CON-001, CON-002, BLU-001, SPR-019, SPR-010]
created: 2026-04-07
updated: 2026-04-07
version: 2.0.0
---

# SPR-020: Dashboard HTTPS Edge + Light/Dark Theme + TCP Proto

## Goal

Two objectives in one sprint, structured for **parallel execution** by Dev A and Dev B:

1. **Dev A:** Serve the dashboard at `https://fonzygrok.com/` on the edge router, add light/dark theme toggle
2. **Dev B:** Implement TCP client proxy and CLI support (SPR-010 T-034B), unblocked by T-068 proto changes

T-068 (proto change) is the serial prerequisite — Dev A does it first, commits, then both agents run concurrently.

**Before:**
- Dashboard at `http://127.0.0.1:9090/` (localhost only, no TLS)
- Tunnel traffic at `https://*.tunnel.fonzygrok.com/` via edge router
- `https://fonzygrok.com/` returns a JSON server info blob
- No TCP tunnel support

**After:**
- Dashboard at `https://fonzygrok.com/` (public, TLS, auth-protected)
- Tunnel traffic at `https://*.tunnel.fonzygrok.com/` (unchanged)
- Port 9090 kept for backward compat / internal healthchecks (unchanged)
- Dashboard supports light and dark mode with a toggle (defaults to system preference)
- Proto layer ready for TCP tunnels (Dev B's client work runs in parallel)

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-068 TCP Proto Changes | Shared (proto) | Dev A | SPR-019 complete |
| T-069 Dashboard on Edge | Server | Dev A | T-068 merged |
| T-070 Light/Dark Theme | Server | Dev A | T-069 |
| T-071 Docker + Config Updates | Server | Dev A | T-069 |
| T-034B TCP Client | Client | Dev B | T-068 merged |

### Execution Flow

```
      T-068 (proto — Dev A, serial)
       /            \
   T-069, T-070,    T-034B
   T-071             (Dev B — parallel)
   (Dev A — parallel)
```

Dev A commits T-068 proto changes first. Then Dev A continues with T-069–T-071 while Dev B starts T-034B concurrently.

---

## Task Details

### T-068: TCP Proto Changes (SERIAL — Dev A first)

This is the prerequisite that unblocks Dev B. Small, targeted change.

#### Modify: `internal/proto/messages.go`

Add `AssignedPort` to `TunnelAssignment`:

```go
type TunnelAssignment struct {
    TunnelID     string `json:"tunnel_id"`
    PublicURL    string `json:"public_url"`
    Name         string `json:"name"`
    AssignedPort int    `json:"assigned_port,omitempty"` // NEW: for TCP tunnels
}
```

Add error code constant:

```go
const ErrPortExhausted = "PORT_EXHAUSTED"
```

#### Tests

- `messages_test.go`: verify `AssignedPort` serializes with `omitempty` (zero value omitted, non-zero included)

#### Acceptance Criteria

- `TunnelAssignment` includes `AssignedPort` field
- Existing HTTP tunnel assignments (AssignedPort=0) produce identical JSON to before (omitempty)
- `go build ./...` passes, `go test ./...` passes
- **Commit immediately after completion so Dev B can pull**

---

### T-069: Mount Dashboard on Edge Router

#### Architecture

The edge router currently dispatches requests based on subdomain presence:
- Subdomain present → tunnel proxy
- No subdomain → `handleServerInfo()` (returns JSON)

**Change:** When the request hits the base domain (`fonzygrok.com`), route it to the dashboard/admin mux instead of returning server info JSON.

The edge router needs a "fallback handler" — a handler that receives requests when no tunnel subdomain is detected. The server orchestrator sets this to the admin mux (which already has dashboard routes registered on it via `Dashboard.RegisterRoutes()`).

#### Modify: `internal/server/edge.go`

- Add `baseDomainHandler http.Handler` field to `EdgeRouter`
- Add `SetBaseDomainHandler(h http.Handler)` method
- Modify `handleRequest()`:
  - If subdomain is empty AND `baseDomainHandler` is set → delegate to `baseDomainHandler`
  - If subdomain is empty AND `baseDomainHandler` is NOT set → existing `handleServerInfo()` (backward compat)
  - If subdomain is present → tunnel proxy (unchanged)
- Also accept requests where the Host is the apex domain (`fonzygrok.com`), not just `tunnel.fonzygrok.com`
  - The `extractSubdomain()` method returns "" for the base domain — but it currently only recognizes `tunnel.fonzygrok.com` as the base. It must also recognize `fonzygrok.com` (the apex) as a "no subdomain" case that should route to the dashboard.
  - Add `ApexDomain string` field to `EdgeConfig`. When the request Host matches `ApexDomain`, treat it as a base domain request (route to dashboard handler).

#### Modify: `internal/server/tls.go`

- Extend `tunnelHostPolicy()` to also accept the apex domain
  - Currently accepts: `tunnel.fonzygrok.com` and `*.tunnel.fonzygrok.com`
  - Must also accept: `fonzygrok.com`
- Add `ApexDomain string` field to `TLSConfig`
- Extract the apex domain from the tunnel domain (strip first label) OR accept it as a separate config field

#### Modify: `internal/server/server.go`

- After creating the admin API and dashboard, wire the admin mux as the edge router's base-domain handler:
  ```
  edge.SetBaseDomainHandler(corsMiddleware(admin.Mux()))
  ```
- Pass apex domain config to EdgeConfig and TLSConfig
- Derive apex domain: if Domain is `tunnel.fonzygrok.com`, apex is `fonzygrok.com`

#### Modify: `internal/server/dashboard.go`

- Set `Secure: true` on session cookies when TLS is enabled
  - The dashboard needs to know if TLS is active. Pass a `TLSEnabled bool` to `NewDashboard()` or to `setSessionCookie()`.
  - Currently hardcoded: `Secure: false` with comment "Set true in production with TLS"

#### Tests

- `edge_test.go`: request to apex domain routes to fallback handler
- `edge_test.go`: request with subdomain still routes to tunnel proxy
- `edge_test.go`: request to base domain without fallback handler returns server info (backward compat)
- `tls_test.go`: host policy accepts apex domain

#### Acceptance Criteria

- `https://fonzygrok.com/` serves the dashboard login page
- `https://fonzygrok.com/dashboard` works (auth-protected)
- `https://fonzygrok.com/api/v1/health` returns health JSON
- `https://*.tunnel.fonzygrok.com/` still proxies tunnel traffic (no regression)
- `http://localhost:9090/` still works for local healthchecks (no regression)
- Session cookies are Secure when TLS is enabled
- Let's Encrypt issues a cert for `fonzygrok.com` (in addition to tunnel domain certs)

---

### T-070: Light/Dark Theme Toggle

#### Modify: `internal/server/dashboard_assets/static/style.css`

Current state: dark theme only, hardcoded CSS variables in `:root`.

**Change:**

1. Move current dark theme variables into a `[data-theme="dark"]` selector
2. Create matching light theme variables under `[data-theme="light"]`
3. Set `:root` defaults to respect `prefers-color-scheme` media query:
   ```css
   /* Default: follow system preference */
   :root {
       /* Light mode variables (default) */
   }
   @media (prefers-color-scheme: dark) {
       :root {
           /* Dark mode variables */
       }
   }
   /* Explicit overrides when user toggles */
   [data-theme="dark"] {
       /* Dark mode variables */
   }
   [data-theme="light"] {
       /* Light mode variables */
   }
   ```

Light theme palette suggestion (developer may refine):
- `--bg-primary: #f8fafc` (slate-50)
- `--bg-secondary: #ffffff`
- `--bg-card: #ffffff`
- `--bg-input: #f1f5f9` (slate-100)
- `--bg-hover: #e2e8f0` (slate-200)
- `--border: #e2e8f0`
- `--text-primary: #0f172a` (slate-900)
- `--text-secondary: #475569` (slate-600)
- `--text-muted: #94a3b8` (slate-400)
- Keep accents the same (`--accent: #10b981`)

4. Add theme toggle button styling (icon-based: ☀ / ◑ / 🌙)

#### Modify: `internal/server/dashboard_assets/templates/layout.html`

- Add theme toggle button in the nav bar (right side, before logout)
- Add inline `<script>` at top of `<head>` (before CSS loads, to prevent FOUC):
  ```javascript
  // Apply saved theme or system default before first paint
  (function() {
      var saved = localStorage.getItem('fonzygrok-theme');
      if (saved) {
          document.documentElement.setAttribute('data-theme', saved);
      }
      // If no saved preference, let CSS media query handle it (no data-theme attribute)
  })();
  ```
- Add theme toggle script (after htmx.min.js):
  ```javascript
  function toggleTheme() {
      var html = document.documentElement;
      var current = html.getAttribute('data-theme');
      // Cycle: system → dark → light → system
      // Or simpler: just toggle dark/light, defaulting to opposite of current
      var next;
      if (!current) {
          // Currently following system — switch to explicit opposite
          next = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'light' : 'dark';
      } else if (current === 'dark') {
          next = 'light';
      } else {
          // Remove attribute to return to system default
          html.removeAttribute('data-theme');
          localStorage.removeItem('fonzygrok-theme');
          return;
      }
      html.setAttribute('data-theme', next);
      localStorage.setItem('fonzygrok-theme', next);
  }
  ```

#### Tests

- `dashboard_test.go`: verify layout includes theme toggle button
- Manual: light mode renders correctly for all pages (login, register, dashboard, admin)
- Manual: toggle persists across page loads (localStorage)
- Manual: default follows OS preference (test with Chrome DevTools emulation)

#### Acceptance Criteria

- Theme toggle button visible in nav bar on all pages
- Click cycles: system default → opposite theme → back to system
- Preference persists in localStorage (survives browser close)
- No flash of wrong theme on page load (script in `<head>` prevents FOUC)
- Light theme is readable and polished (not just inverted colors)
- All existing UI components (cards, tables, forms, badges, flash messages) look correct in both themes

---

### T-071: Docker + Config Updates

#### Modify: `docker/entrypoint.sh`

- No changes required to entrypoint for edge routing (it's a code change, not a config change)
- BUT: if we add an `--apex-domain` flag, add it here conditioned on `APEX_DOMAIN` env var

#### Modify: `docker/docker-compose.yml`

- Add `APEX_DOMAIN` env var (default: derived from `DOMAIN` by stripping first label)
- Document that port 9090 is now optional for external access (dashboard is on :443)
- Consider removing the 9090 port mapping from default config (keep it as commented-out option)

#### Modify: `docker/.env.example`

- Add `APEX_DOMAIN=fonzygrok.com`

#### Modify: `cmd/server/main.go`

- Add `--apex-domain` flag (default: derived from `--domain` by stripping the first label)
  - e.g., `--domain tunnel.fonzygrok.com` → apex = `fonzygrok.com`
- Pass to `ServerConfig`

#### Modify: `internal/config/config.go`

- Add `ApexDomain string` to `HTTPSection` (YAML: `apex_domain`)

#### Update: `CODEX/30_RUNBOOKS/RUN-001_Production_Deployment.md`

- Update security group table: note that port 9090 is no longer required for external access
- Add note that dashboard is now at `https://fonzygrok.com/`
- Update verification checklist

#### Acceptance Criteria

- `docker compose up` with default config serves dashboard on :443
- Entrypoint derives apex domain correctly
- Deployment runbook is accurate

---

### T-034B: TCP Tunnel Client (Dev B — parallel with T-069–T-071)

> **Note:** This is the client-side TCP work originally scoped in SPR-010. It runs here in parallel with Dev A's dashboard work, unblocked by T-068.

#### Modify: `internal/client/proxy.go`

- Add constant: `ChannelTypeTCPProxy = "tcp-proxy"`
- Modify `HandleChannels()`: accept both `"proxy"` (HTTP) and `"tcp-proxy"` (TCP) channel types
- Add `handleTCPChannel(newCh ssh.NewChannel)`:
  - Accept channel
  - Dial `localhost:<localPort>`
  - On dial failure: close SSH channel immediately (no HTTP 502 — TCP has no HTTP semantics)
  - Bidirectional copy using pooled buffers (same as HTTP, minus HTTP parsing)
  - No inspector recording for TCP (or basic: peer IP, bytes, duration)

#### Modify: `internal/client/connection.go`

- Add `ChannelTypeTCPProxy = "tcp-proxy"` constant (alongside existing `ChannelTypeProxy`)

#### Modify: `internal/client/control.go`

- `RequestTunnel` already sends `protocol` field — no changes needed
- Parse `AssignedPort` from response — already populated via `TunnelAssignment` struct after T-068

#### Modify: `internal/client/display.go`

- Add `TunnelEstablishedTCP(name, host string, assignedPort, localPort int, inspectAddr string)`:
  ```
  ✔ Tunnel established!
    ↳ Name:       my-db
    ↳ Public URL: tcp://fonzygrok.com:45123
    ↳ Forwarding: tcp://fonzygrok.com:45123 → localhost:5432
    ↳ Inspector:  http://localhost:4040
  ```

#### Modify: `internal/client/inspector.go`

- Add `Protocol string` field to `RequestEntry` (default `"http"`)
- TCP entries: record peer IP, bytes transferred, duration (no Method/Path/StatusCode)

#### Modify: `cmd/client/main.go`

- Add `--protocol` flag (default `"http"`, choices: `"http"`, `"tcp"`)
- Validate: reject unknown protocol values with clear error
- Pass protocol to `cc.RequestTunnel(port, protocol, name)`
- Switch display output based on assignment: if `AssignedPort > 0`, use TCP format
- Register handler for both `"proxy"` and `"tcp-proxy"` channel types:
  ```go
  go proxy.HandleChannels(ctx, sshClient.HandleChannelOpen(client.ChannelTypeProxy))
  go proxy.HandleChannels(ctx, sshClient.HandleChannelOpen(client.ChannelTypeTCPProxy))
  ```

#### Modify: `internal/config/config.go`

- Add `Protocol string` to `ClientConfig` (YAML: `protocol`, default `"http"`)

#### Modify: `internal/config/load.go`

- Merge `Protocol` field in client config merge

#### Tests

- `proxy_test.go`: test `tcp-proxy` channel handling, bidirectional copy, local dial failure
- `main_test.go`: test `--protocol tcp` flag, `--protocol invalid` rejected
- `display_test.go`: TCP display format output

#### Acceptance Criteria

- `fonzygrok --port 5432 --protocol tcp` → parses and sends protocol in tunnel request
- `tcp-proxy` SSH channels accepted and proxied to local port
- On local dial failure: SSH channel closed cleanly (no HTTP 502 written)
- Inspector shows TCP connections with protocol field
- Reconnect works: TCP tunnel re-registered on reconnect
- Invalid `--protocol` value → clear error message
- `go build ./...` and `go test ./...` pass

---

## Security Considerations

- Dashboard is now publicly accessible — auth middleware (JWT sessions from SPR-017-019) is the only gatekeeper. Verify all dashboard routes require auth (except `/login`, `/register`).
- Session cookies MUST have `Secure: true` when TLS is active.
- The `handleServerInfo` JSON endpoint was unauthenticated and leaked `tunnels_active` count. Moving to dashboard means this info is now behind auth. If a public info endpoint is still desired, it can be a separate route.
- TCP tunnels expose raw ports. The port range (SPR-010 T-033A server work, future sprint) must be documented.

---

## Dependencies

- SPR-019 (dashboard) must be complete ✅
- DNS: `fonzygrok.com` A record must point to the server ✅ (per RUN-001 §Phase 2, already configured)
- No SSH access to server required — all changes are in Go code + Docker config
- T-068 proto change must be committed before Dev B starts T-034B
