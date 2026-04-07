---
id: SPR-010
title: "Sprint 010: TCP Tunneling"
type: how-to
status: COMPLETE
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.2, tcp]
related: [CON-001, BLU-001]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-010: TCP Tunneling

## Goal

Support raw TCP tunnels alongside HTTP tunnels. A client can expose a local Postgres, Redis, or any TCP service via an assigned public port.

**Example:**
```bash
fonzygrok --port 5432 --protocol tcp --name my-db
# → tcp://fonzygrok.com:45123
# → psql -h fonzygrok.com -p 45123 connects to localhost:5432
```

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-033A TCP Server | Server | Dev A | v1.1 released |
| T-034B TCP Client | Client | Dev B | T-033A proto changes merged |

Dev A starts first (proto + server), Dev B follows once proto is on main.

---

## Task Details

### T-033A: TCP Tunnel Server

#### Modify: `internal/proto/messages.go`

Add `AssignedPort` to `TunnelAssignment`:

```go
type TunnelAssignment struct {
    TunnelID    string `json:"tunnel_id"`
    PublicURL   string `json:"public_url"`
    Name        string `json:"name"`
    AssignedPort int   `json:"assigned_port,omitempty"` // NEW: for TCP tunnels
}
```

#### New file: `internal/server/tcpedge.go`

```go
type TCPEdge struct {
    portRange  [2]int         // default 40000-60000
    listeners  map[int]net.Listener
    tunnelMgr  *TunnelManager
    mu         sync.Mutex
    logger     *slog.Logger
}
```

- `AssignPort() (int, error)` — find unused port in range, start listener
- `ReleasePort(port int)` — stop listener, free port
- For each accepted TCP connection:
  1. Look up tunnel by assigned port
  2. Open SSH channel type `"tcp-proxy"` to client
  3. Bidirectional copy (same logic as HTTP proxy, but no HTTP framing)
  4. When either side closes, close both

#### Modify: `internal/server/tunnel.go`

- Add `Protocol string` and `AssignedPort int` to `TunnelEntry`
- `Register()` accepts protocol parameter
- If `protocol == "tcp"`: call `tcpEdge.AssignPort()`, store in entry

#### Modify: `internal/server/control.go`

- Handle `protocol: "tcp"` in tunnel request
- Return `AssignedPort` in assignment

#### Modify: `internal/server/server.go`

- Create `TCPEdge` in orchestrator
- Wire to tunnel manager
- Clean up TCP listeners on shutdown

#### Modify: `internal/server/admin.go`

- Show `protocol` and `assigned_port` in tunnel list

#### Modify: `cmd/server/main.go`

- Add `--tcp-port-range` flag (default `"40000-60000"`)

#### Tests

- `tcpedge_test.go`: assign port, release port, accept connection, proxy to mock channel
- `tunnel_test.go`: register TCP tunnel, verify protocol and port fields
- `admin_test.go`: TCP tunnel appears in list with port

#### Acceptance Criteria

- TCP tunnel assigns a port from the configured range
- External TCP connection on assigned port → proxied through SSH to client
- TCP and HTTP tunnels coexist on the same client session
- Port is released when tunnel disconnects
- Metrics (bytes_in/out) work for TCP tunnels
- `assigned_port` appears in admin API tunnel list

---

### T-034B: TCP Tunnel Client

#### Modify: `internal/client/proxy.go`

- Handle SSH channel type `"tcp-proxy"`:
  - Same bidirectional copy as HTTP proxy
  - No 502 error page (TCP has no HTTP semantics)
  - On local dial failure: close SSH channel immediately

#### Modify: `internal/client/control.go`

- Parse `AssignedPort` from `TunnelAssignment`
- Format TCP public URL: `tcp://server:port`

#### Modify: `cmd/client/main.go`

- Add `--protocol` flag (default `"http"`, choices: `"http"`, `"tcp"`)
- Display TCP URL format:
  ```
  ✔ Tunnel established!
  ↳ Name: my-db
  ↳ Public URL: tcp://fonzygrok.com:45123
  ↳ Forwarding: tcp://fonzygrok.com:45123 → localhost:5432
  ```

#### Modify: `internal/client/inspector.go`

- Record TCP connections (peer IP, bytes transferred, duration)
- Different display in inspector: "TCP connection" instead of "GET /api/..."

#### Tests

- `proxy_test.go`: test tcp-proxy channel handling, bidirectional copy, local dial failure
- `main_test.go`: test `--protocol tcp` flag, `--protocol invalid` rejected

#### Acceptance Criteria

- `fonzygrok --port 5432 --protocol tcp` → assigned TCP port
- TCP connections through assigned port reach local service
- Inspector shows TCP connections
- Reconnect works: TCP tunnel re-registered on reconnect
- Invalid protocol value → clear error

---

## Security Considerations

- TCP tunnels expose raw ports. The port range should be documented.
- EC2 security group needs the port range opened (40000-60000) — add to runbook.
- Rate limiting (SPR-011) will apply to TCP connection rate, not request rate.
