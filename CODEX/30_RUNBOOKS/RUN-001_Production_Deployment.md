---
id: RUN-001
title: "Production Deployment — fonzygrok on Ubuntu EC2 + Docker"
type: how-to
status: ACTIVE
owner: architect
agents: [architect]
tags: [deployment, docker, production, runbook]
related: [GOV-008, BLU-001]
created: 2026-03-31
updated: 2026-04-07
version: 4.0.0
---

# Production Deployment: fonzygrok on Ubuntu EC2 + Docker

> **Domain:** `fonzygrok.com`
> **Tunnel URLs:** `*.tunnel.fonzygrok.com`
> **Server binary runs inside Docker on a single Ubuntu EC2 instance.**

---

## Prerequisites

Before you start, you need:

- An AWS account
- A domain (`fonzygrok.com`) registered on Namecheap
- An SSH key pair for your EC2 instance
- A local machine with the `fonzygrok` client binary (or Go installed to build it)

---

## Phase 1: EC2 Instance Setup

### 1.1 Launch the Instance

In the AWS Console → EC2 → Launch Instance:

| Setting | Value |
|:--------|:------|
| **Name** | `fonzygrok-server` |
| **AMI** | Ubuntu 24.04 LTS (or 22.04) |
| **Instance type** | `t3.micro` (free tier) or `t3.small` |
| **Key pair** | Select or create one (you'll need this to SSH in) |
| **Security group** | Create new — see below |
| **Storage** | 8 GB gp3 (default is fine) |

### 1.2 Security Group Rules

You need **5 inbound rules**. This is critical — if you miss one, something won't work.

| Type | Port | Source | Why |
|:-----|:-----|:-------|:----|
| SSH | 22 | Your IP (or `0.0.0.0/0`) | So you can SSH into the server |
| Custom TCP | 2222 | `0.0.0.0/0` | fonzygrok SSH tunnel port (clients connect here) |
| HTTP | 80 | `0.0.0.0/0` | HTTP redirect + ACME challenge (TLS) or edge traffic |
| HTTPS | 443 | `0.0.0.0/0` | Public tunnel traffic + dashboard (TLS edge router) |
| Custom TCP | 40000–40100 | `0.0.0.0/0` | TCP tunnel ports (dynamically assigned per tunnel) |

> ⚠️ **Port 9090 is no longer required for external access.** The dashboard is now served on `:443` via the edge router. The admin API still listens on `127.0.0.1:9090` internally for health checks and direct API access.

> 💡 **TCP tunnel ports** are assigned from the `--tcp-port-range` range. Start with 40000–40100 (101 ports). Expand if you need more concurrent TCP tunnels.

### 1.3 Allocate an Elastic IP

EC2 → Elastic IPs → Allocate → Associate with your instance.

**Write down this IP.** You'll need it for DNS. Example: `54.123.45.67`

> Without an Elastic IP, your public IP changes every time you stop/start the instance.

---

## Phase 2: DNS Configuration (Namecheap)

Log into Namecheap → Domain List → `fonzygrok.com` → Manage → Advanced DNS.

**Delete any default records** (parking page, etc.), then add these:

| Type | Host | Value | TTL |
|:-----|:-----|:------|:----|
| **A Record** | `@` | `54.123.45.67` | Automatic |
| **A Record** | `tunnel` | `54.123.45.67` | Automatic |
| **A Record** | `*.tunnel` | `54.123.45.67` | Automatic |

Replace `54.123.45.67` with your actual Elastic IP.

**What these do:**
- `@` → `fonzygrok.com` points to your server (optional, for a landing page later)
- `tunnel` → `tunnel.fonzygrok.com` points to your server (the edge router base domain)
- `*.tunnel` → `anything.tunnel.fonzygrok.com` points to your server (wildcard for tunnel subdomains)

> ⚠️ DNS propagation takes 5–30 minutes. You can check with: `dig +short abc.tunnel.fonzygrok.com`

---

## Phase 3: Server Setup

### 3.1 SSH into your instance

```bash
ssh -i ~/path/to/your-key.pem ubuntu@54.123.45.67
```

### 3.2 Install Docker

Run these commands one at a time:

```bash
# Update packages
sudo apt update && sudo apt upgrade -y

# Install Docker
sudo apt install -y docker.io docker-compose-v2

# Add your user to the docker group (so you don't need sudo)
sudo usermod -aG docker ubuntu

# Log out and back in for group change to take effect
exit
```

Then SSH back in:

```bash
ssh -i ~/path/to/your-key.pem ubuntu@54.123.45.67
```

Verify Docker works:

```bash
docker --version
docker compose version
```

### 3.3 Clone the repo

```bash
cd ~
git clone https://github.com/johncrowleydev/fonzygrok.git
cd fonzygrok
```

### 3.4 Configure environment

```bash
cd ~/fonzygrok/docker
cp .env.example .env
nano .env
```

Set these values in the `.env` file:

```env
DOMAIN=tunnel.fonzygrok.com
APEX_DOMAIN=fonzygrok.com
SSH_PORT=2222
HTTPS_PORT=443
TLS_ENABLED=true
TCP_PORT_RANGE=40000-40100
RATE_LIMIT=100
RATE_BURST=200
POSTGRES_USER=fonzygrok
POSTGRES_PASSWORD=<generate-a-secure-password>
POSTGRES_DB=fonzygrok
```

> **Key settings:**
> - `POSTGRES_PASSWORD` — change from default. Used internally between containers.
> - `DATABASE_URL` is auto-constructed from POSTGRES_* vars in docker-compose.yml.
> - Port 9090 is no longer exposed externally — the dashboard is accessed via HTTPS.
> - `TCP_PORT_RANGE=40000-40100` — port range for dynamically assigned TCP tunnels.

Save and exit nano: `Ctrl+O`, `Enter`, `Ctrl+X`.

### 3.5 Build and start

```bash
cd ~/fonzygrok/docker
docker compose up -d --build
```

This will:
1. Pull `postgres:16-alpine` (first run only)
2. Start PostgreSQL with a persistent named volume (`fonzygrok-pgdata`)
3. Wait for PG health check to pass
4. Build the fonzygrok server image from pre-built binaries (Dockerfile.prod)
5. Run migrations automatically on startup

Wait for it to finish. Then verify:

```bash
# Check both containers are running and healthy
docker compose ps

# Check logs
docker compose logs -f
```

You should see:
```
fonzygrok-postgres  Up (healthy)
fonzygrok-server    Up (healthy)
```

Press `Ctrl+C` to stop following logs (the server keeps running).

### 3.6 Verify from the server

```bash
# Health check
curl http://localhost:9090/api/v1/health

# Server info (through the edge)
curl http://localhost:80/
```

Both should return JSON.

---

## Phase 4: Create a Token

You need a token for the client to authenticate. Run this on the EC2 server:

```bash
docker compose -f ~/fonzygrok/docker/docker-compose.yml exec fonzygrok-server \
  fonzygrok-server token create --name my-laptop --data-dir /data
```

You'll see output like:

```
Token created successfully.

  ID:    tok_abc123def456
  Name:  my-laptop
  Token: fgk_xxxxxxxxxxxxxxxxxxxxxxxxxxxx

⚠️  Save this token now — it cannot be retrieved again.
```

**Copy the `fgk_...` token and save it somewhere safe.** You cannot retrieve it again — only the hash is stored.

---

## Phase 5: Connect from Your Local Machine

### 5.1 Build the client (on your local machine)

Back on YOUR machine (not the EC2):

```bash
cd ~/Fonzygrok/architect
make build
```

The client binary is at `./bin/fonzygrok`.

### 5.2 Start a local service to tunnel

Open a terminal and start anything on a port. Example:

```bash
python3 -m http.server 3000
```

### 5.3 Connect the tunnel

Open another terminal:

```bash
./bin/fonzygrok \
  --server fonzygrok.com:2222 \
  --token fgk_xxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --port 3000 \
  --insecure
```

> `--insecure` skips host key verification. Fine for testing. For production, remove it after first connection.

You should see:

```
  ✔ Tunnel established!
  ↳ Public URL: http://abc123.tunnel.fonzygrok.com
  ↳ Forwarding: http://abc123.tunnel.fonzygrok.com → localhost:3000
```

### 5.4 Test it!

Open a browser or use curl from anywhere:

```bash
curl http://abc123.tunnel.fonzygrok.com/
```

You should see the response from your local Python server. 🎉

---

## Phase 6: TLS Configuration (v1.1+)

### Enable auto-TLS

Update `docker-compose.yml` command to include TLS flags:

```yaml
command:
  - serve
  - --data-dir=/data
  - --tls
  - --tls-cert-dir=/data/certs
  - --domain=tunnel.fonzygrok.com
  - --ssh-addr=:2222
  - --http-addr=:8080
  - --admin-addr=0.0.0.0:9090
```

### How it works

- **Port 443**: HTTPS with auto-provisioned Let's Encrypt certificates
- **Port 80**: HTTP redirect to HTTPS + ACME HTTP-01 challenge handler
- **Cert cache**: `/data/certs` (persisted via Docker volume, survives restarts)
- **Host policy**: accepts `tunnel.fonzygrok.com`, `*.tunnel.fonzygrok.com`, and `fonzygrok.com` (apex)
- **Dashboard**: accessible at `https://fonzygrok.com/` (login, registration, token management)

### Prerequisites

> DNS must be configured before enabling TLS. autocert requires the domain to resolve to the server for HTTP-01 challenge validation.

### Verify TLS

```bash
# Valid HTTPS cert
curl -v https://tunnel.fonzygrok.com/

# Dashboard loads on apex domain
curl -v https://fonzygrok.com/
# Should show the login page HTML

# HTTP redirects to HTTPS
curl -v http://tunnel.fonzygrok.com/
# Should show Location: https://...
```

---

## Phase 7: Custom Subdomain Names (v1.1+)

### Client usage

```bash
# Custom name
./bin/fonzygrok --server fonzygrok.com:2222 --token fgk_XXX --port 3000 --name my-api
# → https://my-api.tunnel.fonzygrok.com

# Auto-generated name (adjective-noun format)
./bin/fonzygrok --server fonzygrok.com:2222 --token fgk_XXX --port 3000
# → https://calm-tiger.tunnel.fonzygrok.com
```

### Reserved names (blocked)

`www`, `api`, `admin`, `app`, `mail`, `ftp`, `ssh`, `dns`, `ns1`, `ns2`, `smtp`, `imap`, `pop`, `cdn`, `static`, `assets`, `docs`, `blog`, `status`, `health`, `tunnel`

### Name rules

- 3–32 characters, lowercase alphanumeric + hyphens
- No leading/trailing hyphens
- Must be unique (first-come, first-served)
- Released when tunnel disconnects

---

## Phase 8a: TCP Tunnel Setup (v1.2+)

### Server configuration

Add TCP port range flags to the server command:

```yaml
command:
  - serve
  - --data-dir=/data
  - --tls
  - --tls-cert-dir=/data/certs
  - --domain=tunnel.fonzygrok.com
  - --ssh-addr=:2222
  - --http-addr=:8080
  - --admin-addr=0.0.0.0:9090
  - --tcp-port-range=40000-40100
```

### Client usage

```bash
./bin/fonzygrok \
  --server fonzygrok.com:2222 \
  --token fgk_XXX \
  --port 5432 \
  --protocol tcp
```

Output:
```
  ✔ TCP Tunnel established!
    ↳ Remote: fonzygrok.com:40003
    ↳ Forwarding: fonzygrok.com:40003 → localhost:5432
```

### Security group rules

Ensure your EC2 security group allows inbound TCP on the port range:

```
Type: Custom TCP
Port range: 40000-40100
Source: 0.0.0.0/0
```

### Docker compose

Expose the TCP port range in `docker-compose.yml`:

```yaml
ports:
  - "${SSH_PORT:-2222}:2222"
  - "${HTTP_PORT:-8080}:8080"
  - "${HTTPS_PORT:-443}:443"
  - "40000-40100:40000-40100"     # TCP tunnel ports
```

---

## Phase 8b: Rate Limiting (v1.2+)

### Configuration

Rate limiting uses a per-tunnel token bucket. Configure via server flags:

| Flag | Default | Description |
|:-----|:--------|:------------|
| `--rate-limit` | `0` (disabled) | Requests per second per tunnel |
| `--rate-burst` | `0` (disabled) | Maximum burst size |

```yaml
command:
  - serve
  - --rate-limit=100
  - --rate-burst=20
```

When a client exceeds the rate limit, requests receive **HTTP 429 Too Many Requests** with a `Retry-After: 1` header.

### Custom per-tunnel limits

Use the admin API to set custom limits for specific tunnels:

```bash
curl -X PATCH http://localhost:9090/api/v1/tunnels/<tunnel_id>/rate-limit \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"rate": 50, "burst": 10}'
```

---

## Phase 8c: IP Whitelisting (v1.2+)

### Client-side `--allow-ip`

Clients can restrict which source IPs can access their tunnel:

```bash
./bin/fonzygrok --server fonzygrok.com:2222 --token fgk_XXX --port 3000 \
  --allow-ip 203.0.113.10 \
  --allow-ip 10.0.0.0/8
```

Blocked IPs receive **HTTP 403 Forbidden** with:
```json
{"error": "ip_blocked", "message": "Your IP is not allowed to access this tunnel"}
```

### How it works

- ACL is set per-tunnel during registration
- Uses the source IP from the TCP connection (NOT `X-Forwarded-For`) to prevent spoofing
- CIDR notation supported (e.g., `10.0.0.0/8`)
- No ACL = all IPs allowed (default)

---

## Phase 8d: Dashboard (v1.2+)

### Access

The dashboard is served on the apex domain via the edge router:
- **Production**: `https://fonzygrok.com/` (no port 9090 needed externally)
- Login with username/password, registration requires invite code

### Features

- Token management (create, revoke)
- Live tunnel list with connection status
- Admin panel (users, invite codes) for admin role

### Theme

The dashboard supports light and dark themes:
- **Default**: follows system preference via `prefers-color-scheme` media query
- **Toggle**: click the ◑ button in the nav bar to cycle: system → dark → light → system
- **Persistence**: theme choice saved to `localStorage` as `fonzygrok-theme`

---

## Phase 8: Monitoring & Metrics (v1.1+)

### Health check

```bash
curl http://localhost:9090/api/v1/health
# {"status":"healthy","version":"v1.1.0","uptime_seconds":3600}
```

### Aggregate metrics

```bash
curl http://localhost:9090/api/v1/metrics
# {
#   "total_bytes_in": 1452300,
#   "total_bytes_out": 8923410,
#   "total_requests_proxied": 470,
#   "active_tunnels": 3,
#   "active_clients": 2,
#   "uptime_seconds": 3600
# }
```

### Per-tunnel metrics

```bash
curl http://localhost:9090/api/v1/tunnels
# Each tunnel includes: bytes_in, bytes_out, requests_proxied, last_request_at
```

### Key metrics to monitor

| Metric | Alert Threshold | Action |
|:-------|:---------------|:-------|
| `active_tunnels` | > 100 | Scale or rate-limit |
| `uptime_seconds` | resets unexpectedly | Check crash logs |
| `total_bytes_out` | > 10GB/hour | Check for abuse |

---

## Phase 9: Verification Checklist

Run through these to confirm everything works:

| Test | Command | Expected |
|:-----|:--------|:---------|
| DNS resolves | `dig +short tunnel.fonzygrok.com` | Your Elastic IP |
| Wildcard DNS resolves | `dig +short abc.tunnel.fonzygrok.com` | Your Elastic IP |
| Apex DNS resolves | `dig +short fonzygrok.com` | Your Elastic IP |
| Dashboard (HTTPS) | `curl https://fonzygrok.com/login` | HTML login page |
| Server info | `curl https://tunnel.fonzygrok.com/` | Redirects to dashboard (login) |
| Tunnel works | `curl https://SUBDOMAIN.tunnel.fonzygrok.com/` | Response from your local service |
| 404 for bad subdomain | `curl https://nonexistent.tunnel.fonzygrok.com/` | JSON with `"error":"tunnel_not_found"` |
| Health check | `docker compose exec fonzygrok-server wget -qO- http://localhost:9090/api/v1/health` | JSON with `"status":"healthy"` |

---

## Troubleshooting

### "Connection refused" on port 2222
- Check EC2 security group has port 2222 open to `0.0.0.0/0`
- Check Docker is running: `docker compose ps`

### "Connection refused" on port 80
- Check EC2 security group has port 80 open to `0.0.0.0/0`
- Check the edge router is mapped to port 80: look for `HTTP_PORT=80` in `.env`

### DNS not resolving
- Wait 5–30 minutes after adding records
- Verify in Namecheap that `*.tunnel` record exists (not just `tunnel`)
- Test: `dig +short xyz.tunnel.fonzygrok.com` — should show your IP

### "tunnel_not_found" when curling
- Make sure the Host header matches: `SUBDOMAIN.tunnel.fonzygrok.com`
- The subdomain is the random ID printed when the client connects
- Check the client is still connected (not disconnected/reconnecting)

### Client says "auth: invalid token"
- Token was typed wrong — copy it exactly, including the `fgk_` prefix
- Token was revoked — create a new one

### Logs
```bash
# Server logs
cd ~/fonzygrok/docker && docker compose logs -f

# Client logs are printed to stdout (JSON)
```

---

## Operations

### Restart the server
```bash
cd ~/fonzygrok/docker && docker compose restart
```

### Stop the server
```bash
cd ~/fonzygrok/docker && docker compose down
```

### Update to a new version
```bash
cd ~/fonzygrok
git pull origin main
cd docker
docker compose up -d --build
```

### List tokens
```bash
docker compose -f ~/fonzygrok/docker/docker-compose.yml exec fonzygrok-server \
  fonzygrok-server token list --data-dir /data
```

### Revoke a token
```bash
docker compose -f ~/fonzygrok/docker/docker-compose.yml exec fonzygrok-server \
  fonzygrok-server token revoke --id tok_XXXXXX --data-dir /data
```

### View active tunnels
```bash
curl http://localhost:9090/api/v1/tunnels
```

### Access PostgreSQL directly
```bash
docker exec fonzygrok-postgres psql -U fonzygrok -c "SELECT id, username, role FROM users;"
```

### Backup PostgreSQL
```bash
docker exec fonzygrok-postgres pg_dump -U fonzygrok fonzygrok > fonzygrok_backup_$(date +%Y%m%d).sql
```

### Restore from backup
```bash
cat fonzygrok_backup_YYYYMMDD.sql | docker exec -i fonzygrok-postgres psql -U fonzygrok fonzygrok
```

---

## Post-Deployment Verification Checklist

> **This checklist is MANDATORY after every deployment (per DEF-002 §6).**
> Deployment is not considered complete until all items pass.

| # | Check | Command | Expected |
|:--|:------|:--------|:---------|
| 1 | PG is healthy | `docker ps --format '{{.Names}} {{.Status}}'` | `fonzygrok-postgres Up (healthy)` |
| 2 | Server is healthy | `docker ps --format '{{.Names}} {{.Status}}'` | `fonzygrok-server Up (healthy)` |
| 3 | Health endpoint | `docker exec fonzygrok-server wget -qO- http://localhost:9090/api/v1/health` | `{"status":"healthy",...}` |
| 4 | HTTPS serves | `curl -s -o /dev/null -w '%{http_code}' https://fonzygrok.com/` | `200` |
| 5 | HTTP redirects | `curl -s -o /dev/null -w '%{http_code}' http://fonzygrok.com/` | `301` |
| 6 | Login page renders | `curl -s https://fonzygrok.com/login \| grep -c '<form'` | `1` |
| 7 | Admin can log in | Manual: log in at `https://fonzygrok.com/login` | Dashboard loads |
| 8 | DB has users | `docker exec fonzygrok-postgres psql -U fonzygrok -c 'SELECT count(*) FROM users;'` | ≥1 |

