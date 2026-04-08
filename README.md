# Fonzygrok

A self-hosted [ngrok](https://ngrok.com) alternative. Expose local services to the internet through secure SSH tunnels.

## What is Fonzygrok?

Fonzygrok lets you share a local web server with the internet by creating a public URL that tunnels HTTP traffic to your machine. It's useful for webhook development, mobile testing, demoing local apps, and sharing work-in-progress without deploying.

Unlike managed services, you control the entire stack. Run the server on your own infrastructure, issue your own tokens, and keep your traffic on your network.

```
                         ┌─────────────────────────────────────┐
                         │          Your Server (VPS)          │
   Internet              │                                     │
   ────────────►  :443   │  HTTP Edge ──► Tunnel Manager       │
   browser hits          │       │              │              │
   my-app.               │       ▼              ▼              │
   fonzygrok.com         │  Route by     SSH Channel ◄──────── │ :2222
                         │  subdomain         │                │
                         └────────────────────│────────────────┘
                                              │  SSH tunnel
                                              ▼
                                     ┌────────────────┐
                                     │  Your Laptop   │
                                     │  fonzygrok     │
                                     │  client        │
                                     │       │        │
                                     │       ▼        │
                                     │  localhost:3000 │
                                     └────────────────┘
```

## Quick Start

```bash
# Install (Linux / macOS)
curl -sSfL https://fonzygrok.com/install.sh | sh

# Install (Windows PowerShell)
irm https://fonzygrok.com/install.ps1 | iex
```

Then connect:

```bash
fonzygrok --name my-app --port 3000 --token fgk_your_token
```

Output:

```
fonzygrok v1.3.0

  Connecting to fonzygrok.com:2222...
  ✔ Connected!

  ✔ Tunnel established!
    ↳ Name:       my-app
    ↳ Public URL: https://my-app.fonzygrok.com
    ↳ Forwarding: https://my-app.fonzygrok.com → localhost:3000
    ↳ Inspector:  http://localhost:4040

  Press Ctrl+C to stop.
```

Your local service on port 3000 is now accessible at the public URL. Works with both HTTP and HTTPS local services — auto-detected, no flags needed.

---

## Installation

### Install Script (Recommended)

**Linux / macOS:**

```bash
curl -sSfL https://fonzygrok.com/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://fonzygrok.com/install.ps1 | iex
```

Both scripts download the binary, install it to a directory on your PATH, and you're ready to go. Restart your terminal after installing.

### Download Binary

Download the latest release for your platform:

| Platform | Download |
|:---------|:---------|
| Linux (amd64) | [fonzygrok-linux-amd64](https://github.com/johncrowleydev/fonzygrok/releases/latest/download/fonzygrok-linux-amd64) |
| Linux (arm64) | [fonzygrok-linux-arm64](https://github.com/johncrowleydev/fonzygrok/releases/latest/download/fonzygrok-linux-arm64) |
| macOS (Intel) | [fonzygrok-darwin-amd64](https://github.com/johncrowleydev/fonzygrok/releases/latest/download/fonzygrok-darwin-amd64) |
| macOS (Apple Silicon) | [fonzygrok-darwin-arm64](https://github.com/johncrowleydev/fonzygrok/releases/latest/download/fonzygrok-darwin-arm64) |
| Windows (amd64) | [fonzygrok-windows-amd64.exe](https://github.com/johncrowleydev/fonzygrok/releases/latest/download/fonzygrok-windows-amd64.exe) |

```bash
# Linux / macOS
chmod +x fonzygrok-*
sudo mv fonzygrok-* /usr/local/bin/fonzygrok
```

### Build from Source

Requires Go 1.21+.

```bash
git clone https://github.com/johncrowleydev/fonzygrok.git
cd fonzygrok
go build -o fonzygrok ./cmd/client/
go build -o fonzygrok-server ./cmd/server/
```

---

## Client Usage

### Connecting

```bash
fonzygrok --port 3000 --token fgk_abc123
```

The server defaults to `fonzygrok.com`. Use `--server` to connect to a different instance:

```bash
fonzygrok --server self-hosted.dev --token fgk_abc123 --port 3000
```

If no port is specified in `--server`, `:2222` is appended automatically.

### Custom Subdomains

Use `--name` to request a specific subdomain:

```bash
fonzygrok --name my-api --port 8080
```

```
  ✔ Tunnel established!
    ↳ Name:       my-api
    ↳ Public URL: https://my-api.fonzygrok.com
    ↳ Forwarding: https://my-api.fonzygrok.com → localhost:8080
```

If the name is already taken, the server assigns a random name instead.

### HTTPS Local Services

The client auto-detects whether your local service speaks HTTP or HTTPS. If your app runs with a self-signed dev cert on `https://localhost:7091`, fonzygrok handles it transparently — no extra flags needed.

### Request Inspector

Every tunnel starts a local web UI at [http://localhost:4040](http://localhost:4040) where you can see all requests flowing through your tunnel in real time. The inspector shows:

- Method, path, status code, and duration for each request
- Request and response headers
- Body preview (first 1KB)
- Live streaming via SSE — new requests appear instantly

Disable it with `--no-inspect`, or change the address with `--inspect 127.0.0.1:5050`.

### Config File

Instead of passing flags every time, create `~/.fonzygrok.yaml`:

```yaml
server: fonzygrok.com
token: fgk_your_token_here
port: 3000
name: my-api
insecure: false
```

The client auto-detects `./fonzygrok.yaml` and `~/.fonzygrok.yaml`. Override with `--config /path/to/file.yaml`. CLI flags always take precedence over config file values.

### Verbose Mode

By default, the client shows human-friendly messages on stderr and suppresses JSON logs. To see structured JSON logs on stdout (useful for piping to log aggregators):

```bash
fonzygrok --port 3000 --verbose
```

Both streams are active simultaneously — Display output on stderr, JSON on stdout.

### Environment Variables

| Variable | Equivalent Flag |
|:---------|:----------------|
| `FONZYGROK_SERVER` | `--server` |
| `FONZYGROK_TOKEN` | `--token` |

```bash
export FONZYGROK_TOKEN=fgk_abc123
fonzygrok --port 3000
```

On Windows (PowerShell), set permanently:

```powershell
[Environment]::SetEnvironmentVariable("FONZYGROK_TOKEN", "fgk_abc123", "User")
```

### Complete Flag Reference

| Flag | Default | Description |
|:-----|:--------|:------------|
| `--server` | `fonzygrok.com` | Server address (domain or host:port) |
| `--token` | — | API token for authentication |
| `--port` | — | Local port to expose (**required**) |
| `--name` | (auto) | Custom subdomain name |
| `--protocol` | `http` | Tunnel protocol: `http` or `tcp` |
| `--config` | (auto-detect) | Path to YAML config file |
| `--inspect` | `localhost:4040` | Inspector web UI listen address |
| `--no-inspect` | `false` | Disable the request inspector |
| `--verbose` | `false` | Show JSON structured logs on stdout |
| `--insecure` | `false` | Skip SSH host key verification |
| `--version` | — | Print version and exit |

### TCP Tunnels

Expose a local TCP service (e.g., database, game server, SSH) via a raw TCP tunnel:

```bash
fonzygrok --port 5432 --protocol tcp
```

```
  ✔ TCP Tunnel established!
    ↳ Remote: fonzygrok.com:40003
    ↳ Forwarding: fonzygrok.com:40003 → localhost:5432
```

Anyone can connect to `fonzygrok.com:40003` and the traffic flows to your local port 5432. The server assigns a port from the configured range (default 40000–40100).

### Rate Limiting

The server enforces per-tunnel rate limiting (token bucket). When exceeded, HTTP requests receive **429 Too Many Requests** with a `Retry-After` header. Rate limits are configured server-side via `--rate-limit` and `--rate-burst` flags.

### IP Access Control

Restrict which IPs can access your tunnel:

```bash
fonzygrok --port 3000 --allow-ip 203.0.113.10 --allow-ip 10.0.0.0/8
```

Blocked IPs receive **403 Forbidden**. CIDR notation is supported. Without `--allow-ip`, all IPs are allowed.

### Web Dashboard

The server includes a web dashboard at the apex domain (e.g., `https://fonzygrok.com/`):

- Login with username/password
- Registration with invite codes
- Create and revoke tunnel tokens
- View active tunnels in real time
- Light/dark theme toggle (defaults to system preference)
- Admin panel for user and invite code management

---

## Self-Hosting

### Prerequisites

- **A domain** with wildcard DNS: `*.yourdomain.com` → your server IP
- **A server** (VPS, EC2, etc.) with ports open:
  - `2222` — SSH tunnel connections
  - `80` / `443` — Public HTTP/HTTPS traffic
  - `9090` — Admin API (bind to localhost or private network)
  - `40000-40100` — TCP tunnel port range (optional)
- **Docker** and **Docker Compose**

### Docker Deployment

```bash
git clone https://github.com/johncrowleydev/fonzygrok.git
cd fonzygrok/docker

cp .env.example .env
```

Edit `.env`:

```env
DOMAIN=yourdomain.com
TLS_ENABLED=true
SSH_PORT=2222
HTTP_PORT=80
HTTPS_PORT=443
ADMIN_PORT=9090
```

Start the server:

```bash
docker compose up -d
```

### DNS Configuration

Set up two DNS records pointing to your server's IP:

| Type | Name | Value |
|:-----|:-----|:------|
| A | `yourdomain.com` | Your server IP |
| A | `*.yourdomain.com` | Your server IP |

The wildcard record is required for tunnel subdomains (e.g., `my-app.yourdomain.com`).

### TLS / HTTPS Setup

Set `TLS_ENABLED=true` in `.env`. The server automatically provisions certificates from Let's Encrypt for your domain and all tunnel subdomains. Ensure ports 80 and 443 are open and reachable.

When TLS is enabled:
- HTTPS listens on `:443`
- HTTP on `:80` redirects to HTTPS
- Certificates are cached in the `fonzygrok-certs` Docker volume

### Token Management

Create tokens for your users via the admin API or CLI:

```bash
# Via CLI (inside the container)
docker exec fonzygrok-server fonzygrok-server token create --name "dev-laptop" --data-dir /data

# Via admin API
curl -s http://localhost:9090/api/v1/tokens -X POST \
  -H "Content-Type: application/json" \
  -d '{"name": "dev-laptop"}'
```

List and revoke tokens:

```bash
# List
docker exec fonzygrok-server fonzygrok-server token list --data-dir /data

# Revoke
docker exec fonzygrok-server fonzygrok-server token revoke --id tok_abc123 --data-dir /data
```

> **Note:** Tokens are shown in full only at creation time. Store them securely.

### Server Configuration Reference

#### Serve Command Flags

| Flag | Default | Description |
|:-----|:--------|:------------|
| `--data-dir` | `./data` | Directory for SSH host key |
| `--database-url` | (auto) | PostgreSQL connection string |
| `--domain` | `localhost` | Base domain for subdomain routing |
| `--ssh-addr` | `:2222` | SSH listener address |
| `--http-addr` | `:8080` | HTTP edge router address |
| `--admin-addr` | `127.0.0.1:9090` | Admin API listen address |
| `--tls` | `false` | Enable auto-TLS via Let's Encrypt |
| `--tls-cert-dir` | `<data-dir>/certs` | TLS certificate cache directory |
| `--tcp-port-range` | `40000-60000` | TCP tunnel port range (MIN-MAX) |
| `--rate-limit` | `100` | Default requests per second per tunnel |
| `--rate-burst` | `200` | Rate limit burst size |
| `--config` | — | Path to YAML config file |

#### Docker Environment Variables

| Variable | Default | Description |
|:---------|:--------|:------------|
| `DOMAIN` | `tunnel.localhost` | Base domain for tunnel routing |
| `TLS_ENABLED` | `false` | Enable HTTPS with Let's Encrypt |
| `SSH_PORT` | `2222` | Host port for SSH connections |
| `HTTP_PORT` | `80` | Host port for HTTP traffic |
| `HTTPS_PORT` | `443` | Host port for HTTPS traffic |
| `ADMIN_PORT` | `9090` | Host port for admin API |
| `DATABASE_URL` | (auto) | PostgreSQL connection string |
| `TCP_PORT_RANGE` | `40000-40100` | TCP tunnel port range |
| `RATE_LIMIT` | `100` | Requests per second per tunnel |
| `RATE_BURST` | `200` | Rate limit burst size |

### Architecture

```
                    ┌───────────────────────────────────────────────┐
                    │            fonzygrok-server                   │
                    │                                               │
  :2222 ───────────►│  SSH Listener                                 │
  (client connects) │    ├── Auth (token validation via PostgreSQL) │
                    │    ├── Control Channel (tunnel register)      │
                    │    └── Proxy Channels (HTTP + TCP relay)      │
                    │                                               │
  :443 ────────────►│  HTTP Edge Router                             │
  (public traffic)  │    ├── Subdomain extraction                   │
                    │    ├── Rate limit check                       │
                    │    ├── IP ACL check                           │
                    │    ├── Tunnel lookup                          │
                    │    └── Proxy via SSH channel                  │
                    │                                               │
  :40000-40100 ────►│  TCP Edge                                     │
  (TCP tunnels)     │    ├── Port pool allocation                   │
                    │    └── Raw TCP ↔ SSH channel relay            │
                    │                                               │
  :443 ────────────►│  Dashboard + Install Scripts                  │
  (apex domain)     │    ├── User auth (bcrypt + JWT sessions)      │
                    │    ├── Token management                       │
                    │    ├── Tunnel monitoring                      │
                    │    ├── Admin panel                            │
                    │    ├── /install.sh (Unix installer)           │
                    │    └── /install.ps1 (Windows installer)       │
                    │                                               │
                    │  PostgreSQL (via Docker service)               │
                    │    ├── Tokens, Users, Invite Codes            │
                    │    └── Connection metadata                    │
                    └───────────────────────────────────────────────┘
```

---

## Troubleshooting

### "missing port in address"

The `--server` flag auto-appends `:2222` if no port is specified. Make sure you're running the latest client version.

### "connection refused"

- Verify the server is running: `curl http://your-server:9090/api/v1/health`
- Check that port `2222` is open in your firewall / security group
- Try `--insecure` if you haven't set up host key verification

### "tunnel not found" (404 in browser)

- Verify wildcard DNS is configured: `*.yourdomain.com → your server IP`
- Check the tunnel is active: `curl http://your-server:9090/api/v1/tunnels`
- The subdomain in the URL must match the tunnel name exactly

### "upstream unreachable" / local service not responding

- Verify your local service is running on the port you specified
- The client auto-detects HTTP/HTTPS — both work transparently
- Check that nothing else is blocking localhost connections

### Inspector not loading

- Open [http://localhost:4040](http://localhost:4040) in your browser
- If port 4040 is already in use, set a different address: `--inspect 127.0.0.1:5050`
- Disable with `--no-inspect` if you don't need it

### Client shows raw JSON instead of pretty output

Update to the latest client version. Versions prior to v1.1.1 output raw JSON logs. The current version shows human-friendly formatted output by default.

---

## License

Proprietary — All rights reserved.
