---
id: RES-001
title: "Tunneling Architecture Research — ngrok-like Tool in Go"
type: reference
status: APPROVED
owner: architect
agents: [all]
tags: [research, architecture, networking, tunneling, golang]
related: []
created: 2026-03-31
updated: 2026-03-31
version: 1.0.0
---

> **BLUF:** Research into the architecture of ngrok and open-source alternatives (chisel, frp, sish, zrok). Captures core patterns, Go library choices, multiplexing trade-offs, and technology recommendations for building a self-hosted tunnel service in Go.

# Tunneling Architecture Research

---

## 1. How ngrok-style Tunneling Works

### 1.1 Core Concept

A tunnel service bridges a private network (behind NAT/firewall) to the public internet without requiring the client to configure port forwarding, static IPs, or DNS.

```
┌──────────────────┐        ┌───────────────────┐        ┌─────────────────┐
│   Public User    │──HTTP──│    Tunnel Server   │──mux──│   Tunnel Client  │
│  (browser, API)  │        │  (public VPS/cloud)│        │ (behind NAT)    │
└──────────────────┘        └───────────────────┘        └────────┬────────┘
                                                                  │
                                                           localhost:3000
```

### 1.2 Data Flow

1. **Client → Server:** Client initiates an outbound TLS/TCP connection to the server (always outbound — punches through NAT).
2. **Registration:** Client says "I want to expose port 3000." Server assigns a public endpoint (e.g., `abc123.tunnel.example.com`).
3. **Multiplexing:** Multiple logical streams share the single persistent connection.
4. **Proxying:** Public request arrives at server → server routes it through the tunnel → client forwards to `localhost:3000` → response travels back.

### 1.3 Key Architectural Layers

| Layer | Responsibility |
|:------|:---------------|
| **Transport** | The persistent connection between client and server (TLS/TCP, WebSocket, HTTP/2, SSH) |
| **Multiplexing** | Multiple logical streams over one connection (yamux, smux, HTTP/2 streams, SSH channels) |
| **Control plane** | Authentication, tunnel registration, subdomain assignment, heartbeats |
| **Data plane** | Actual proxied traffic (HTTP requests, raw TCP bytes) |
| **Edge routing** | Mapping incoming public requests to the correct tunnel (SNI, Host header, port) |

---

## 2. Existing Open-Source Landscape

| Project | Transport | Multiplexing | Auth | Storage | Stars |
|:--------|:----------|:-------------|:-----|:--------|:------|
| **chisel** | HTTP/WebSocket | SSH (`crypto/ssh`) | Fingerprint + user/pass | None (stateless) | ~13k |
| **frp** | Custom TCP protocol | Custom | Token-based | Config files | ~90k |
| **sish** | SSH | SSH channels | SSH public keys | None (SSH authorized_keys) | ~4k |
| **zrok** | OpenZiti overlay | OpenZiti | Identity/certificate | SQLite / PostgreSQL | ~3k |
| **bore** (Rust) | TCP | Custom | Shared secret | None | ~9k |
| **pgrok** | gRPC | HTTP/2 (via gRPC) | GitHub OAuth | PostgreSQL | ~1k |

### 2.1 Key Takeaways

- **chisel** is the cleanest reference architecture — lean, well-structured Go, SSH-based security.
- **frp** is the most feature-complete but uses a custom protocol that's harder to reason about.
- **sish** proves that SSH as the transport layer is viable and simplifies auth (reuse SSH keys).
- **zrok** shows that SQLite works fine as an embedded datastore for tunnel state.
- Most tools ship as a **single binary** with `server` and `client` subcommands.

---

## 3. Technology Decision Matrix

### 3.1 Transport Protocol

| Option | Pros | Cons | Recommendation |
|:-------|:-----|:-----|:---------------|
| **Raw TCP + TLS** | Maximum control, lowest overhead | Must implement framing, auth, keepalive yourself | ❌ Too much boilerplate |
| **WebSocket over HTTP** | Firewall-friendly, standard, good tooling | Slightly higher overhead than raw TCP | ⚠️ Good option |
| **HTTP/2** | Built-in multiplexing, TLS, flow control | More complex to use for raw TCP tunneling | ⚠️ Good for HTTP-only |
| **SSH (crypto/ssh)** | Built-in auth, encryption, channels, widely understood | Slightly heavier than minimal protocols | ✅ **Recommended** |
| **gRPC** | Strong typing, streaming, HTTP/2 underneath | Overkill for byte-stream proxying | ❌ Wrong tool |

**Recommendation: SSH (`crypto/ssh`).** It gives us encryption, authentication (key-based), multiplexed channels, and keepalive — all built into Go's standard ecosystem. This is what chisel uses successfully.

### 3.2 Multiplexing

| Option | Pros | Cons |
|:-------|:-----|:-----|
| **yamux** (HashiCorp) | Battle-tested, robust flow control, bidirectional | Slightly heavier memory |
| **smux** (xtaci) | Low memory, high performance | Had stability issues in v1 |
| **SSH channels** | Comes free with crypto/ssh transport | Good enough for most use cases |
| **HTTP/2 streams** | Standard, good tooling | Awkward for raw TCP |

**Recommendation: SSH channels.** If we pick SSH as transport, we get multiplexing for free via SSH channels. No need for a separate library.

### 3.3 Storage / Database

| Option | Pros | Cons |
|:-------|:-----|:-----|
| **None (stateless)** | Simplest, no dependencies | Can't track users, tunnels, or history |
| **SQLite** | Embedded, zero-ops, single file | Single-writer, not great for distributed setups |
| **PostgreSQL** | Scalable, concurrent writes | External dependency, operational overhead |
| **BoltDB / bbolt** | Embedded key-value, Go-native | Less query flexibility than SQL |

**Recommendation: SQLite for v1.** Embedded, zero-config, single binary deployment. Can migrate to PostgreSQL later if multi-server is needed.

### 3.4 Authentication Model

| Option | Pros | Cons |
|:-------|:-----|:-----|
| **SSH public keys** | Industry standard, no password management | Key distribution needed |
| **Token/API key** | Simple to implement, easy to integrate | Must store and manage tokens |
| **OAuth** | Standard, delegate to GitHub/Google | Complex, requires web UI |

**Recommendation: Token-based auth for v1, with SSH key support as a stretch goal.** Tokens are simple to generate, store in SQLite, and validate. SSH keys can come later.

---

## 4. Architecture Recommendation

### 4.1 System Components

```
┌─────────────────────────────────────────────────────────────┐
│                     TUNNEL SERVER                            │
│                                                             │
│  ┌──────────┐  ┌───────────┐  ┌───────────┐  ┌──────────┐  │
│  │  Edge    │  │  Control  │  │   Tunnel   │  │  Admin   │  │
│  │  Router  │  │  Plane    │  │   Manager  │  │  API     │  │
│  │ (HTTP +  │  │ (auth,    │  │ (track     │  │ (REST,   │  │
│  │  TCP     │  │  register,│  │  active    │  │  manage  │  │
│  │  listener│  │  heartbeat│  │  tunnels)  │  │  tunnels)│  │
│  └──────────┘  └───────────┘  └───────────┘  └──────────┘  │
│                       │                                     │
│                  ┌────▼────┐                                │
│                  │ SQLite  │                                │
│                  └─────────┘                                │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                     TUNNEL CLIENT                            │
│                                                             │
│  ┌──────────┐  ┌───────────┐  ┌───────────┐               │
│  │  SSH     │  │  Local    │  │  Config   │               │
│  │  Conn    │  │  Proxy    │  │  Manager  │               │
│  │  Manager │  │  (forward │  │  (CLI +   │               │
│  │          │  │  to local │  │   YAML)   │               │
│  │          │  │  port)    │  │           │               │
│  └──────────┘  └───────────┘  └───────────┘               │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Go Project Layout

```
cmd/
├── server/main.go       # Server binary entry point
└── client/main.go       # Client binary entry point
internal/
├── server/              # Server-only code
│   ├── edge.go          # HTTP/TCP listener + routing
│   ├── control.go       # Auth, registration, heartbeat
│   ├── tunnel.go        # Tunnel lifecycle management
│   └── admin.go         # Admin REST API
├── client/              # Client-only code
│   ├── connection.go    # SSH connection management
│   └── proxy.go         # Local port forwarding
├── proto/               # Shared protocol definitions
│   ├── messages.go      # Control message types
│   └── tunnel.go        # Tunnel metadata types
├── auth/                # Authentication (token validation)
└── store/               # SQLite storage layer
    ├── store.go
    ├── users.go
    └── tunnels.go
```

---

## 5. Competitive Differentiation Opportunities

What could make this tool stand out vs. chisel/frp/sish:

1. **Local inspection UI** — Like ngrok's localhost:4040 request inspector
2. **Wildcard subdomains** — `*.tunnel.example.com` routing
3. **TCP + HTTP awareness** — Parse HTTP headers for smarter routing, pass TCP through raw
4. **Built-in Let's Encrypt** — Automatic TLS for the server's public endpoint
5. **Dashboard** — Web UI showing active tunnels, traffic stats, connected clients
6. **Team/multi-user** — API keys, usage limits, access controls

---

## 6. References

- chisel: https://github.com/jpillora/chisel
- frp: https://github.com/fatedier/frp
- sish: https://github.com/antoniomika/sish
- zrok: https://github.com/openziti/zrok
- yamux: https://github.com/hashicorp/yamux
- Go crypto/ssh: https://pkg.go.dev/golang.org/x/crypto/ssh
