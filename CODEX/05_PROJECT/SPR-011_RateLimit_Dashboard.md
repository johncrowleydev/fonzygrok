---
id: SPR-011
title: "Sprint 011: Rate Limiting & Web Dashboard"
type: how-to
status: READY
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.2, ratelimit, dashboard]
related: [CON-002, BLU-001]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-011: Rate Limiting & Web Dashboard

## Goal

Per-tunnel rate limiting to prevent abuse. Web dashboard for managing tokens and viewing tunnels, served from the admin API using htmx + Alpine.js.

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-035A Rate Limiting | Server | Dev A | SPR-010 merged |
| T-036B Web Dashboard | Server | Dev B | SPR-010 merged |

Both tasks are fully parallel.

---

## Task Details

### T-035A: Rate Limiting

#### New file: `internal/server/ratelimit.go`

Token bucket rate limiter per tunnel:

```go
type RateLimiter struct {
    limiters sync.Map // tunnel_id → *rate.Limiter
    rate     float64  // default requests per second
    burst    int      // default burst size
}
```

- Use `golang.org/x/time/rate` (standard Go rate limiter)
- `Allow(tunnelID string) bool` — check if request is allowed
- `SetLimit(tunnelID string, rps float64, burst int)` — custom limit per tunnel
- `Remove(tunnelID string)` — clean up on tunnel disconnect
- Default: 100 req/s, burst of 200 (configurable)

#### Modify: `internal/server/edge.go`

- Before proxying HTTP request: `if !rateLimiter.Allow(tunnelID)` → return `429 Too Many Requests`
- Response body:
  ```json
  {
    "error": "rate_limit_exceeded",
    "message": "Too many requests to this tunnel",
    "retry_after_seconds": 1
  }
  ```
- Add `Retry-After` header

#### Modify: `internal/server/tcpedge.go`

- Rate limit TCP connection attempts (not bytes, just new connections)
- On exceed: close the connection immediately

#### Modify: `internal/server/admin.go`

- `PUT /api/v1/tunnels/:id/ratelimit` — set custom rate limit:
  ```json
  { "requests_per_second": 50, "burst": 100 }
  ```
- `GET /api/v1/tunnels` — include `rate_limit` in response

#### Modify: `cmd/server/main.go`

- Add `--rate-limit` flag (default `100`, requests per second)
- Add `--rate-burst` flag (default `200`)

#### Tests

- `ratelimit_test.go`:
  - Default limiter allows requests under limit
  - Limiter blocks requests over limit
  - Custom per-tunnel limits work
  - Cleanup on tunnel removal
- `edge_test.go`: 429 response format, Retry-After header
- `admin_test.go`: PUT ratelimit endpoint, verify in tunnel list

#### Acceptance Criteria

- Default 100 req/s per tunnel
- Exceeding rate → 429 with JSON body + `Retry-After` header
- Per-tunnel custom limits via admin API
- TCP connection rate limited separately
- Rate limiter cleaned up when tunnel disconnects

---

### T-036B: Web Dashboard

#### New directory: `internal/server/dashboard/`

Embedded HTML/CSS/JS admin dashboard using htmx + Alpine.js.

```
internal/server/dashboard/
├── assets/
│   ├── index.html        # main layout, navigation
│   ├── style.css         # dark theme, responsive
│   ├── htmx.min.js       # vendored (14KB gzipped)
│   └── alpine.min.js     # vendored (15KB gzipped)
├── embed.go              # //go:embed assets/*
└── dashboard.go          # HTTP handler, serves embedded files
```

#### `dashboard.go`

```go
//go:embed assets/*
var assets embed.FS

func NewDashboardHandler() http.Handler {
    // Serve embedded files at /dashboard/
    // index.html at /dashboard/
}
```

#### Modify: `internal/server/admin.go`

- Mount dashboard handler at `/dashboard/`
- Serve at root `/` with redirect to `/dashboard/`

#### Dashboard Pages

**1. Overview (`/dashboard/`)**

Using htmx to poll `/api/v1/health` every 5s:

```html
<div hx-get="/api/v1/health" hx-trigger="every 5s" hx-swap="innerHTML">
    <!-- auto-updating stats -->
</div>
```

Display:
- Server uptime, version
- Active tunnels count, connected clients
- Total traffic (from /api/v1/metrics)
- System status indicator

**2. Tunnels (`/dashboard/tunnels`)**

Table of active tunnels with:
- Name, subdomain, protocol, public URL
- Client IP, connected time
- Metrics: bytes in/out, requests proxied
- Actions: force disconnect (htmx DELETE to admin API)

```html
<tr x-data="{ confirm: false }">
    <td>my-api</td>
    <td>http</td>
    <td>1,234 reqs</td>
    <td>
        <button @click="confirm = true" x-show="!confirm">Disconnect</button>
        <span x-show="confirm">
            Sure?
            <button hx-delete="/api/v1/tunnels/abc123" hx-target="closest tr" hx-swap="outerHTML">Yes</button>
            <button @click="confirm = false">No</button>
        </span>
    </td>
</tr>
```

**3. Tokens (`/dashboard/tokens`)**

- Token list (id, name, created, last used)
- Create token form (htmx POST, shows generated token in modal)
- Revoke button with confirmation

#### Design

- Dark theme (dark gray background, light text)
- Monospace font for IDs, tokens, URLs
- Color-coded status indicators (green = active, red = down)
- Responsive: works on mobile
- No external CDN — all assets embedded in binary

#### Tests

- `dashboard_test.go`:
  - `GET /dashboard/` returns 200 with HTML
  - `GET /dashboard/htmx.min.js` returns JS with correct content-type
  - Embedded FS contains all expected files

#### Acceptance Criteria

- Dashboard served at `http://localhost:9090/dashboard/`
- All data comes from existing admin API endpoints (no new backend logic)
- Overview auto-updates every 5 seconds
- Tunnel disconnect works from dashboard
- Token create + revoke works from dashboard
- Dark theme, responsive, no external dependencies
- Page load < 100KB total (HTML + CSS + vendored JS)
