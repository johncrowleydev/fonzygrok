---
id: SPR-006
title: "Custom Subdomains & Readable Names"
type: sprint
status: COMPLETE
owner: architect
agents: [developer-a, developer-b]
tags: [sprint, feature, v1.1]
related: [CON-001, CON-002, BLU-001, SPR-005]
created: 2026-03-31
updated: 2026-04-02
version: 1.0.0
---

# SPR-006: Custom Subdomains & Readable Names

## Goal

Replace random 6-char tunnel IDs in public URLs with human-friendly names.

**Before:** `http://a7otkw.tunnel.fonzygrok.com/`
**After:** `http://my-api.tunnel.fonzygrok.com/` (user-specified)
**After:** `http://calm-tiger.tunnel.fonzygrok.com/` (auto-generated)

---

## Scope

1. Client `--name` flag for user-specified subdomain names
2. Auto-generated readable names (adjective-noun pairs) when `--name` is omitted
3. Server-side name validation, uniqueness enforcement, and reserved-name blacklist
4. Edge router routes by name instead of tunnel ID
5. Tunnel ID remains the internal identifier for admin API and logging

---

## Track Assignment

| Task | Track | Owner | Files |
|:-----|:------|:------|:------|
| T-019 Name Generator | Shared | Dev A | `internal/names/` (NEW) |
| T-020 Protocol Extension | Shared | Dev A | `internal/proto/messages.go` |
| T-021 Server Name Handling | Server | Dev A | `internal/server/tunnel.go`, `internal/server/control.go`, `internal/server/edge.go`, `internal/server/admin.go` |
| T-022 Client --name Flag | Client | Dev B | `cmd/client/main.go`, `internal/client/control.go` |

**Dev A** does T-019, T-020, T-021 on branch `feature/SPR-006A-custom-names`.
**Dev B** does T-022 on branch `feature/SPR-006B-client-name-flag` (depends on T-020 merge first).

---

## Task Details

### T-019: Name Generator (`internal/names/`)

**Create** a new package `internal/names/` with:

#### `words.go`
- `adjectives` — slice of ~80 short, friendly adjectives (e.g., `calm`, `swift`, `bold`, `warm`, `bright`, `cool`, `happy`, `lucky`, `noble`, `quiet`)
- `nouns` — slice of ~80 short, friendly nouns (e.g., `tiger`, `falcon`, `river`, `cloud`, `flame`, `frost`, `storm`, `panda`, `maple`, `coral`)
- Criteria: all lowercase, no hyphens, 3-7 chars each, family-friendly, easy to type

#### `generate.go`
- `Generate() string` — returns `"adjective-noun"` using `crypto/rand` for selection
- `GenerateUnique(exists func(string) bool) string` — retries until `exists()` returns false (max 100 attempts, then falls back to `adjective-noun-XXXX` with 4 random digits)

#### `validate.go`
- `Validate(name string) error` — validates a user-provided name:
  - 3–32 characters
  - Lowercase alphanumeric and hyphens only
  - Must start and end with alphanumeric (no leading/trailing hyphens)
  - Must not be in the reserved names list
- `IsReserved(name string) bool` — checks against reserved names
- Reserved names list: `www`, `api`, `admin`, `app`, `mail`, `ftp`, `ssh`, `dns`, `ns1`, `ns2`, `smtp`, `imap`, `pop`, `cdn`, `static`, `assets`, `docs`, `blog`, `status`, `health`, `tunnel`

#### Tests
- `generate_test.go` — uniqueness over 1000 generations, format validation
- `validate_test.go` — valid names, invalid names (too short, too long, bad chars, reserved)

**Acceptance criteria:**
- `Generate()` returns names matching `^[a-z]+-[a-z]+$` pattern
- `Validate()` rejects all reserved names
- `Validate()` rejects names outside the 3–32 char range
- 100% of generated names pass `Validate()`

---

### T-020: Protocol Extension (`internal/proto/messages.go`)

**Modify** the `TunnelRequest` message to include an optional `Name` field:

```go
type TunnelRequest struct {
    LocalPort int    `json:"local_port"`
    Protocol  string `json:"protocol"`
    Name      string `json:"name,omitempty"`  // NEW: optional user-specified subdomain
}
```

**Modify** the `TunnelAssignment` message to include the assigned name:

```go
type TunnelAssignment struct {
    TunnelID  string `json:"tunnel_id"`
    PublicURL string `json:"public_url"`
    Name      string `json:"name"`  // NEW: the subdomain name (user-specified or auto-generated)
}
```

**Do NOT break backward compatibility.** `Name` is `omitempty` on the request — old clients that don't send it still work (server auto-generates a name).

#### Tests
- Update existing round-trip tests to include `Name` field
- Add test for backward compat: request without `Name` should unmarshal cleanly

**Acceptance criteria:**
- Existing proto tests still pass
- New field round-trips correctly
- Empty `Name` in request unmarshals to `""`

---

### T-021: Server Name Handling

#### `internal/server/tunnel.go`

**Modify** `TunnelEntry` to add a `Name` field:

```go
type TunnelEntry struct {
    TunnelID  string    // internal ID (unchanged, still random 6-char)
    Name      string    // NEW: subdomain name (what appears in the URL)
    Subdomain string    // CHANGE: set to Name instead of TunnelID
    // ... rest unchanged
}
```

**Modify** `Register()` to accept an optional name parameter:
- If name is empty: auto-generate using `names.GenerateUnique()`
- If name is provided: validate with `names.Validate()`, check uniqueness
- Set `entry.Name = name` and `entry.Subdomain = name`
- Return error if name is taken or invalid

**Modify** `Lookup()`: no change needed — it already looks up by subdomain, which will now be the name.

**Add** `LookupByName(name string) (*TunnelEntry, bool)` — alias for clarity.

#### `internal/server/control.go`

**Modify** the tunnel request handler to:
1. Extract `Name` from the `TunnelRequest` message
2. Pass it to `Register()`
3. Include `Name` in the `TunnelAssignment` response

#### `internal/server/edge.go`

No changes needed — it already routes by subdomain, which will now be the name.

#### `internal/server/admin.go`

**Modify** the tunnel list response to include `name`:

```go
type tunnelResp struct {
    TunnelID    string `json:"tunnel_id"`
    Name        string `json:"name"`        // NEW
    Subdomain   string `json:"subdomain"`
    // ... rest unchanged
}
```

#### Tests
- `tunnel_test.go`: test Register with custom name, test duplicate name rejection, test reserved name rejection, test auto-generated name format
- `control_test.go`: test tunnel request with name, test tunnel request without name (auto-generate)
- `admin_test.go`: verify name appears in tunnel list response

**Acceptance criteria:**
- `Register("my-api", ...)` → subdomain is `my-api`
- `Register("", ...)` → subdomain is auto-generated readable name
- `Register("www", ...)` → error (reserved)
- `Register("my-api", ...)` twice → error (duplicate)
- Edge router continues to work (routes by subdomain)
- Admin API shows name in tunnel listing

---

### T-022: Client --name Flag

#### `cmd/client/main.go`

**Add** `--name` flag:

```go
cmd.Flags().StringVar(&name, "name", "", "Custom subdomain name for the tunnel URL")
```

Pass `name` through to `onConnect()`.

#### `internal/client/control.go`

**Modify** `RequestTunnel()` to accept and send the name:

```go
func (cc *ControlChannel) RequestTunnel(localPort int, protocol string, name string) (*proto.TunnelAssignment, error) {
```

Set `req.Name = name` in the `TunnelRequest`.

#### `cmd/client/main.go` output

Update the tunnel-established output to show the name:

```
  ✔ Tunnel established!
  ↳ Name: my-api
  ↳ Public URL: http://my-api.tunnel.fonzygrok.com
  ↳ Forwarding: http://my-api.tunnel.fonzygrok.com → localhost:3000
```

#### Tests
- Test CLI parses `--name` flag
- Test `RequestTunnel` sends name in request
- Test output includes name

**Acceptance criteria:**
- `fonzygrok --name my-api --port 3000` → URL is `http://my-api.tunnel.fonzygrok.com`
- `fonzygrok --port 3000` (no name) → URL has auto-generated readable name
- `fonzygrok --name WWW --port 3000` → error from server (reserved, also uppercase rejected)
- `fonzygrok --name existing-name --port 3000` → error from server (duplicate)
- Error messages are clear and actionable

---

## Execution Order

```
1. Dev A: T-019 (name generator) + T-020 (proto extension)
         ↓ merge to main
2. Dev A: T-021 (server handling) — parallel with:
   Dev B: T-022 (client flag) — pulls main first to get T-019/T-020
         ↓ both merge to main
3. Architect: audit, verify E2E, tag v1.1.0
```

---

## Verification

### Unit Tests
All existing tests pass + new tests for names package, proto extension, name handling.

### E2E Test
Add to `tests/e2e_test.go`:
- `TestE2E_10_CustomNameTunnel` — client requests name "test-app", verify URL contains "test-app"
- `TestE2E_11_AutoGeneratedName` — client omits name, verify URL matches `^[a-z]+-[a-z]+$`
- `TestE2E_12_DuplicateNameRejected` — two clients request same name, second gets error
- `TestE2E_13_ReservedNameRejected` — client requests "www", gets error

### Manual Verification
```bash
# Custom name
./bin/fonzygrok --server fonzygrok.com:2222 --token TOKEN --port 3000 --name my-api --insecure
# → http://my-api.tunnel.fonzygrok.com

# Auto-generated
./bin/fonzygrok --server fonzygrok.com:2222 --token TOKEN --port 3000 --insecure
# → http://calm-tiger.tunnel.fonzygrok.com
```

---

## Out of Scope

- Removing the `tunnel.` prefix (deferred — configurable via `--domain` already exists)
- Persistent name reservations (names are released when tunnel disconnects)
- Custom domains (e.g., bring your own CNAME) — v1.2 feature
- Traffic metrics — separate sprint
