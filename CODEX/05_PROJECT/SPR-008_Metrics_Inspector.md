---
id: SPR-008
title: "Sprint 008: Connection Metrics & Request Inspection UI"
type: how-to
status: COMPLETE
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.1, metrics, ui]
related: [CON-002, BLU-001, SPR-007]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-008: Connection Metrics & Request Inspection UI

## Goal

Server-side traffic metrics per tunnel exposed via admin API. Client-side request inspection web UI at `localhost:4040`.

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-025A Metrics | Server | Dev A | SPR-007 merged |
| T-026B Inspector | Client | Dev B | SPR-007 merged |

Both tasks are fully parallel.

---

## Task Details

### T-025A: Connection Metrics

#### New file: `internal/server/metrics.go`

```go
// TunnelMetrics holds per-tunnel traffic counters.
// All fields are accessed atomically.
type TunnelMetrics struct {
    BytesIn         atomic.Int64
    BytesOut        atomic.Int64
    RequestsProxied atomic.Int64
    LastRequestAt   atomic.Value // time.Time
}

// CountingReader wraps an io.Reader and increments a counter.
type CountingReader struct { ... }

// CountingWriter wraps an io.Writer and increments a counter.
type CountingWriter struct { ... }
```

#### Modify: `internal/server/tunnel.go`

- Add `Metrics *TunnelMetrics` field to `TunnelEntry`
- Initialize on registration, reset on deregistration

#### Modify: `internal/server/edge.go`

- Wrap the proxy request/response copy with `CountingReader`/`CountingWriter`
- Increment `RequestsProxied` after each successful proxy
- Set `LastRequestAt` to `time.Now()`

#### Modify: `internal/server/admin.go`

- Extend tunnel list response with metrics fields:
  - `bytes_in`, `bytes_out`, `requests_proxied`, `last_request_at`
- New endpoint `GET /api/v1/metrics`:
  ```json
  {
    "total_bytes_in": 1452300,
    "total_bytes_out": 8923410,
    "total_requests_proxied": 470,
    "active_tunnels": 3,
    "active_clients": 2,
    "uptime_seconds": 3600
  }
  ```

#### Tests

- `metrics_test.go`: CountingReader/Writer correctly count bytes
- `edge_test.go`: after proxying a request, tunnel metrics are incremented
- `admin_test.go`: /api/v1/metrics returns correct aggregates

#### Acceptance Criteria

- `bytes_in`/`bytes_out` track actual bytes within 1% accuracy
- `requests_proxied` increments by exactly 1 per request
- Metrics are atomic (safe under concurrent proxy traffic)
- Metrics reset when tunnel disconnects and re-registers
- `GET /api/v1/metrics` aggregates across all active tunnels

---

### T-026B: Request Inspection UI

#### New file: `internal/client/inspector.go`

```go
// Inspector captures request/response metadata and broadcasts to SSE clients.
type Inspector struct {
    entries  []RequestEntry  // ring buffer, capacity 100
    mu       sync.RWMutex
    subs     []chan RequestEntry  // SSE subscribers
    logger   *slog.Logger
}

type RequestEntry struct {
    ID            string    `json:"id"`
    Timestamp     time.Time `json:"timestamp"`
    Method        string    `json:"method"`
    Path          string    `json:"path"`
    StatusCode    int       `json:"status_code"`
    DurationMs    float64   `json:"duration_ms"`
    RequestSize   int64     `json:"request_size"`
    ResponseSize  int64     `json:"response_size"`
    RequestHeaders  map[string]string `json:"request_headers"`
    ResponseHeaders map[string]string `json:"response_headers"`
    BodyPreview   string    `json:"body_preview,omitempty"` // first 1KB
}
```

- `NewInspector(addr string, logger *slog.Logger) *Inspector`
- `Start(ctx context.Context) error` — start HTTP server
- `Record(entry RequestEntry)` — add to ring buffer, broadcast to SSE subscribers
- HTTP handlers:
  - `GET /` — serve embedded HTML UI
  - `GET /api/requests` — JSON array of recent requests
  - `GET /api/requests/stream` — SSE stream of new requests
  - `DELETE /api/requests` — clear buffer

#### New file: `internal/client/inspector_ui.go`

Embedded HTML/CSS/JS single-page app using `//go:embed`:

```go
//go:embed inspector_assets/*
var inspectorAssets embed.FS
```

**UI layout:**
- Header: "Fonzygrok Inspector" + tunnel name + connection status
- Request table: timestamp, method, path, status (color-coded), duration, size
- Click row to expand: full headers, body preview
- Filter bar: text search on path, status code filter
- Clear button
- Auto-scroll for new requests

**JS approach:** Vanilla JS + SSE (`EventSource`). No framework needed for the inspector — it's a single list view.

#### New directory: `internal/client/inspector_assets/`

- `index.html` — main page
- `style.css` — dark theme matching terminal aesthetic
- `app.js` — SSE listener, DOM updates, filtering

#### Modify: `internal/client/proxy.go`

- Add `Inspector *Inspector` field to `LocalProxy`
- After each proxy round-trip, call `inspector.Record()` with captured metadata
- Capture: method, path, status from parsing the HTTP request/response line
- Capture: first 1KB of response body for preview
- Record timing (start → response received)

#### Modify: `cmd/client/main.go`

- Add `--inspect` flag (string, default `"localhost:4040"`)
- Add `--no-inspect` flag (bool, disables inspector)
- Start inspector in `onConnect` callback
- Print `Inspector: http://localhost:4040` alongside tunnel URL

#### Tests

- `inspector_test.go`:
  - Record entries, verify ring buffer capacity (max 100)
  - Record entry, verify SSE broadcast
  - Test HTTP endpoints (GET /, GET /api/requests, DELETE /api/requests)
  - Test overflow: 101st entry evicts first

#### Acceptance Criteria

- `fonzygrok --port 3000` → "Inspector: http://localhost:4040" in output
- Open browser to `localhost:4040` → see live request feed
- Requests appear within 100ms of being proxied (SSE latency)
- Ring buffer holds 100 entries max
- `--no-inspect` prevents inspector from starting (no goroutine, no listener)
- UI works in Chrome, Firefox, Safari
- Body preview limited to first 1KB
