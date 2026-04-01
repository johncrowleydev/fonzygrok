---
id: SPR-014
title: "Sprint 014: Client UX — Pretty Output + Verbose Mode"
type: how-to
status: ACTIVE
owner: architect
agents: [developer-b]
tags: [sprint, defect-fix, ux, client, v1.1]
related: [DEF-001]
created: 2026-04-01
updated: 2026-04-01
version: 1.0.0
---

# SPR-014: Client UX — Pretty Output + Verbose Mode

## Goal

Fix DEF-001. Replace raw JSON log output with human-friendly formatted messages.
Add `--verbose` flag to opt-in to JSON structured logs.

---

## Track Assignment

**Dev B** — client-side only.

Branch: `feature/SPR-014-pretty-output`

---

## Tasks

### T-043: Display Abstraction

Create `internal/client/display.go`:

```go
type Display struct {
    w       io.Writer  // os.Stderr
    noColor bool       // true when NO_COLOR env is set or terminal doesn't support ANSI
}
```

Methods (all write to stderr):

| Method | Output |
|:-------|:-------|
| `Banner(version string)` | `fonzygrok v1.1.0` with blank line |
| `Connecting(addr string)` | `  Connecting to fonzygrok.com:2222...` |
| `Connected()` | `  ✔ Connected!` (green) |
| `TunnelEstablished(name, url string, port int, inspectAddr string)` | Full tunnel info block with aligned labels |
| `ConnectionFailed(err error, attempt int, backoffSec int)` | `  ✘ Connection failed: <reason>` (red) + `  ↻ Retrying in Ns...` (yellow) |
| `Disconnected()` | `  ⚠ Disconnected from server` (yellow) |
| `Shutdown()` | `  fonzygrok stopped.` |
| `Error(msg string)` | `  ✘ <msg>` (red) |
| `Ready()` | `  Press Ctrl+C to stop.` with blank line |

Color rules:
- Detect `NO_COLOR` env var → disable all ANSI
- Use `os.Getenv("TERM")` — if empty or `dumb`, disable ANSI
- On Windows, check for `WT_SESSION` (Windows Terminal) or `ANSICON` — if neither, disable ANSI
- Green: `\033[32m`, Red: `\033[31m`, Yellow: `\033[33m`, Reset: `\033[0m`

---

### T-044: Verbose Flag + Logger Refactor

Modify `cmd/client/main.go`:

- Add `--verbose` flag (bool, default `false`)
- When `--verbose` is **false**:
  - Create slog handler at `slog.LevelError` (suppresses INFO/DEBUG/WARN JSON)
  - All user-facing output goes through `Display`
- When `--verbose` is **true**:
  - Create slog handler at `slog.LevelDebug` (existing behavior, JSON to stdout)
  - `Display` messages still print to stderr (both streams active)

---

### T-045: Wire Display Into Client Lifecycle

Modify `cmd/client/main.go` and `internal/client/connector.go`:

- Replace all `fmt.Fprintf(os.Stderr, ...)` in `onConnect` with `display.TunnelEstablished(...)`
- Add event callbacks or `Display` field to `Connector` so retry loop uses `display.ConnectionFailed(...)` and `display.Connecting(...)` instead of raw slog
- Call `display.Banner(Version)` at startup
- Call `display.Ready()` after tunnel established
- Call `display.Shutdown()` on clean exit

---

### T-046: Tests

- `display_test.go`: each method outputs expected strings (capture with `bytes.Buffer`)
- `display_test.go`: `NO_COLOR=1` suppresses ANSI escape codes
- `main_test.go`: `--verbose` flag in help output, flag parsing
- All existing tests must pass with `-race`

---

## Acceptance Criteria

- [ ] Default output (no `--verbose`) is human-readable, no JSON visible
- [ ] `--verbose` shows both human messages (stderr) and JSON logs (stdout)
- [ ] ANSI colors work on Linux/macOS terminals
- [ ] `NO_COLOR=1` produces plain text output
- [ ] Windows without modern terminal gets plain text (no garbage ANSI codes)
- [ ] All 195+ existing tests pass
- [ ] New tests for Display methods and --verbose flag

## Governance Compliance

| GOV | Requirement | Deliverable |
|:----|:-----------|:------------|
| GOV-002 | Tests alongside code | `display_test.go`, updated `main_test.go` |
| GOV-003 | Code quality | JSDoc-style comments on Display methods |
| GOV-006 | Structured logging | JSON logging preserved behind `--verbose` |
