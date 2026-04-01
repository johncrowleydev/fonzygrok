---
id: SPR-007
title: "Sprint 007: Auto-TLS & YAML Config Files"
type: how-to
status: READY
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.1, tls, config]
related: [CON-002, BLU-001, SPR-006]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-007: Auto-TLS & YAML Config Files

## Goal

HTTPS on public tunnel URLs via Let's Encrypt autocert. YAML config file support for both binaries.

---

## Track Assignment

| Task | Track | Owner | Depends On |
|:-----|:------|:------|:-----------|
| T-023A Auto-TLS | Server | Dev A | SPR-006 merged |
| T-024B Config Files | Both | Dev B | SPR-006 merged |

Both tasks are fully parallel.

---

## Task Details

### T-023A: Auto-TLS on Edge Router

#### New file: `internal/server/tls.go`

Create an `autocert.Manager` wrapper:

```go
type TLSConfig struct {
    Enabled    bool
    CertDir    string   // persistent cert cache directory
    Domain     string   // base domain for wildcard policy
}
```

- `NewTLSManager(cfg TLSConfig) *autocert.Manager` — configures autocert with:
  - `DirCache` pointing to `CertDir`
  - `HostPolicy` that accepts `*.tunnel.<domain>` and `tunnel.<domain>`
  - `Email` optional (can be empty for LE)

#### Modify: `internal/server/edge.go`

- If TLS enabled: listen on `:443` with `tls.NewListener` using autocert's `TLSConfig()`
- Also listen on `:80` with a redirect handler + ACME HTTP-01 challenge handler
- If TLS disabled: existing behavior unchanged (plain HTTP on configured port)

#### Modify: `internal/server/server.go`

- Add `TLS TLSConfig` to `ServerConfig`
- Wire TLS manager into edge router startup
- If TLS enabled, the public URL scheme changes from `http://` to `https://`

#### Modify: `cmd/server/main.go`

- Add flags: `--tls` (bool), `--tls-cert-dir` (string, default `<data-dir>/certs`)
- When `--tls` is set, listen on both :80 (redirect) and :443 (TLS)

#### Modify: `docker/Dockerfile`

- Add `EXPOSE 443`

#### Modify: `docker/docker-compose.yml`

- Add port mapping `"${HTTPS_PORT:-443}:443"`
- Add cert volume mount

#### Tests

- Test TLS config struct construction
- Test host policy accepts valid tunnel domains, rejects others
- Test redirect handler on :80
- Integration test with `httptest.NewTLSServer` (not real LE, mock cert)

#### Acceptance Criteria

- `fonzygrok-server serve --tls --domain tunnel.fonzygrok.com` → edge on :443 with valid cert
- Port 80 redirects to HTTPS (except `/.well-known/acme-challenge/`)
- Certs cached in `--tls-cert-dir`, survive container restart
- Without `--tls`, behavior identical to v1.0
- Tunnel public URLs use `https://` scheme when TLS enabled

---

### T-024B: YAML Config Files

#### New package: `internal/config/`

##### `config.go`

```go
type ServerConfig struct {
    DataDir string `yaml:"data_dir"`
    Domain  string `yaml:"domain"`
    SSH     struct {
        Addr string `yaml:"addr"`
    } `yaml:"ssh"`
    HTTP struct {
        Addr       string `yaml:"addr"`
        TLS        bool   `yaml:"tls"`
        TLSCertDir string `yaml:"tls_cert_dir"`
    } `yaml:"http"`
    Admin struct {
        Addr string `yaml:"addr"`
    } `yaml:"admin"`
}

type ClientConfig struct {
    Server   string `yaml:"server"`
    Token    string `yaml:"token"`
    Port     int    `yaml:"port"`
    Name     string `yaml:"name"`
    Insecure bool   `yaml:"insecure"`
    Inspect  string `yaml:"inspect"`
}
```

##### `load.go`

- `LoadServerConfig(path string) (*ServerConfig, error)` — read YAML, validate
- `LoadClientConfig(path string) (*ClientConfig, error)` — read YAML, validate
- `MergeServerConfig(file *ServerConfig, flags *ServerConfig) *ServerConfig` — flags override file
- Handle missing file gracefully (not an error, use defaults)

#### Modify: `cmd/server/main.go`

- Add `--config` flag (default: `""`, no config file)
- If set, load config, then overlay CLI flags on top
- Env vars still override everything

#### Modify: `cmd/client/main.go`

- Add `--config` flag
- Also auto-detect `~/.fonzygrok.yaml` and `./fonzygrok.yaml` if no explicit `--config`
- Load config, overlay CLI flags

#### Dependencies

- Add `gopkg.in/yaml.v3` to `go.mod`

#### Tests

- `config_test.go`: load valid YAML, load invalid YAML, load missing file, merge logic (flags override file values, empty flags don't override)
- `cmd/client/main_test.go`: test `--config` flag is accepted
- `cmd/server/`: would be nice but not blocking (matches v1.0 gap)

#### Acceptance Criteria

- `fonzygrok-server serve --config server.yaml` loads all settings from file
- `fonzygrok --config client.yaml --port 8080` → port 8080 overrides YAML value
- Missing config file → no error, defaults apply
- Invalid YAML → clear error: "config: parse error at line 5: ..."
- Auto-detect `~/.fonzygrok.yaml` for client
