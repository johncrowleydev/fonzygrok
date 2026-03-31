# Fonzygrok

A self-hosted ngrok alternative built in Go. Expose local services to the internet through secure SSH tunnels.

## Overview

Fonzygrok consists of two binaries:

- **fonzygrok-server** — Accepts SSH connections from clients, manages tunnels, and routes public HTTP traffic to the correct client through SSH channels.
- **fonzygrok** (client) — Connects to a fonzygrok server, authenticates with a token, and proxies incoming requests to a local service.

## Architecture

```
Internet → HTTP Edge (:8080) → Tunnel Manager → SSH Channel → Client → localhost:PORT
```

- **SSH Listener** (:2222) — Accepts client connections, authenticates via token
- **HTTP Edge Router** (:8080) — Routes public HTTP requests by subdomain to the right tunnel
- **Admin API** (:9090) — REST API for token management and tunnel inspection
- **SQLite Store** — Persists tokens, tunnel metadata, and connection logs

## Prerequisites

- Go 1.21+
- SQLite3 (via `modernc.org/sqlite` — pure Go, no CGo)

## Build

```bash
# Build both binaries
make build

# Build only server
make build-server

# Build only client
make build-client

# Run tests
make test

# Run linter
make lint
```

## Quick Start

### 1. Start the Server

```bash
cp .env.example .env
# Edit .env with your domain and port settings

./fonzygrok-server
```

### 2. Create a Token

```bash
curl -X POST http://localhost:9090/api/v1/tokens -d '{"name":"dev-laptop"}'
# Save the returned token — it is only shown once
```

### 3. Connect a Client

```bash
./fonzygrok --server your-server.com --token fgk_... --port 3000
# Your local service on port 3000 is now accessible at:
# http://<tunnel-id>.tunnel.example.com
```

## Configuration

| Environment Variable | Default | Description |
|:---------------------|:--------|:------------|
| `FONZYGROK_DATA_DIR` | `/data` | Directory for SQLite database and host keys |
| `FONZYGROK_DOMAIN` | `tunnel.example.com` | Base domain for tunnel subdomains |
| `FONZYGROK_SSH_ADDR` | `:2222` | SSH listener address |
| `FONZYGROK_HTTP_ADDR` | `:8080` | HTTP edge router address |
| `FONZYGROK_ADMIN_ADDR` | `:9090` | Admin API address |

## License

Proprietary — All rights reserved.
