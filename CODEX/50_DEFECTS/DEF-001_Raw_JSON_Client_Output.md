---
id: DEF-001
title: "Client outputs raw JSON logs instead of human-friendly messages"
type: defect
status: RESOLVED
severity: HIGH
owner: architect
agents: [developer-b]
tags: [defect, ux, client, v1.1]
related: [SPR-014]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# DEF-001: Client Outputs Raw JSON Logs Instead of Human-Friendly Messages

## Summary

The fonzygrok client binary outputs raw JSON structured logs to stdout for all
events (connection, retry, tunnel establishment, errors). This is unacceptable
user experience for a CLI tool. End users see a wall of JSON instead of clear,
formatted status messages.

## Severity: HIGH

This is a user-facing UX defect that affects every single invocation of the
client. No user should need to parse JSON to understand what their tunnel tool
is doing.

---

## Reproduction

```powershell
fonzygrok-windows-amd64.exe --server fonzygrok.com --token fgk_... --port 3000
```

### Actual Output

```
{"time":"2026-04-01T10:59:56.096Z","level":"INFO","msg":"fonzygrok starting","version":"dev","server":"fonzygrok.com","local_port":3000}
{"time":"2026-04-01T10:59:56.097Z","level":"INFO","msg":"connecting to server","server_addr":"fonzygrok.com"}
{"time":"2026-04-01T10:59:56.097Z","level":"INFO","msg":"connection failed, will retry","attempt":1,"backoff_ms":1000,"error":"..."}
```

### Expected Output

```
fonzygrok v1.1.0

  Connecting to fonzygrok.com:2222...
  ✔ Connected!

  ✔ Tunnel established!
    ↳ Name:       my-api
    ↳ Public URL: https://my-api.fonzygrok.com
    ↳ Forwarding: https://my-api.fonzygrok.com → localhost:3000
    ↳ Inspector:  http://localhost:4040

  Press Ctrl+C to stop.
```

On errors:
```
  ✘ Connection failed: connection refused
  ↻ Retrying in 2s...
```

---

## Root Cause

The client uses `slog.NewJSONHandler(os.Stdout, ...)` at DEBUG level for all
output. There is no distinction between user-facing display messages and
internal structured logs. All events go through the same JSON logger.

## Affected Files

- `cmd/client/main.go` — logger creation, `onConnect` output
- `internal/client/connector.go` — retry loop logging

## Required Fix

1. Create a `Display` abstraction for human-friendly formatted output to stderr
2. Add `--verbose` flag: when off, suppress JSON logs; when on, show both
3. Wire all user-facing events through `Display` instead of raw `slog`
4. Support `NO_COLOR` env var for terminals that don't support ANSI
5. Detect Windows terminals and fall back to plain text when needed

## Related

- Sprint: SPR-014
