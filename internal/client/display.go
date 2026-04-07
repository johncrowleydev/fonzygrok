package client

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// ANSI color escape codes.
const (
	ansiGreen  = "\033[32m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiReset  = "\033[0m"
)

// Display writes human-friendly formatted output to stderr.
// It supports ANSI color escape codes with automatic fallback
// for terminals that don't support them (NO_COLOR, dumb TERM, old Windows).
type Display struct {
	w       io.Writer
	noColor bool
}

// NewDisplay creates a Display that writes to w.
// It auto-detects whether ANSI colors should be disabled based on:
//   - NO_COLOR env var (https://no-color.org)
//   - TERM=dumb or empty TERM
//   - Windows without WT_SESSION or ANSICON
func NewDisplay(w io.Writer) *Display {
	return &Display{
		w:       w,
		noColor: shouldDisableColor(),
	}
}

// NewDisplayNoColor creates a Display with colors explicitly disabled.
// Useful for testing.
func NewDisplayNoColor(w io.Writer) *Display {
	return &Display{
		w:       w,
		noColor: true,
	}
}

// Banner prints the startup banner with the version string.
func (d *Display) Banner(version string) {
	fmt.Fprintf(d.w, "fonzygrok %s\n\n", version)
}

// Connecting prints a connection-in-progress message.
func (d *Display) Connecting(addr string) {
	fmt.Fprintf(d.w, "  Connecting to %s...\n", addr)
}

// Connected prints a success message in green.
func (d *Display) Connected() {
	fmt.Fprintf(d.w, "  %s\n\n", d.green("✔ Connected!"))
}

// TunnelEstablished prints the full tunnel info block with aligned labels.
func (d *Display) TunnelEstablished(name, url string, port int, inspectAddr string) {
	fmt.Fprintf(d.w, "  %s\n", d.green("✔ Tunnel established!"))
	if name != "" {
		fmt.Fprintf(d.w, "    ↳ Name:       %s\n", name)
	}
	fmt.Fprintf(d.w, "    ↳ Public URL: %s\n", url)
	fmt.Fprintf(d.w, "    ↳ Forwarding: %s → localhost:%d\n", url, port)
	if inspectAddr != "" {
		fmt.Fprintf(d.w, "    ↳ Inspector:  http://%s\n", inspectAddr)
	}
}

// TunnelEstablishedTCP prints the TCP tunnel info block with tcp:// URL format.
func (d *Display) TunnelEstablishedTCP(name, host string, assignedPort, localPort int, inspectAddr string) {
	publicURL := fmt.Sprintf("tcp://%s:%d", host, assignedPort)
	fmt.Fprintf(d.w, "  %s\n", d.green("✔ Tunnel established!"))
	if name != "" {
		fmt.Fprintf(d.w, "    ↳ Name:       %s\n", name)
	}
	fmt.Fprintf(d.w, "    ↳ Public URL: %s\n", publicURL)
	fmt.Fprintf(d.w, "    ↳ Forwarding: %s → localhost:%d\n", publicURL, localPort)
	if inspectAddr != "" {
		fmt.Fprintf(d.w, "    ↳ Inspector:  http://%s\n", inspectAddr)
	}
}

// ConnectionFailed prints a red error message and yellow retry message.
func (d *Display) ConnectionFailed(err error, attempt int, backoffSec int) {
	fmt.Fprintf(d.w, "  %s %s\n", d.red("✘"), d.red(fmt.Sprintf("Connection failed: %s", err.Error())))
	fmt.Fprintf(d.w, "  %s\n", d.yellow(fmt.Sprintf("↻ Retrying in %ds...", backoffSec)))
}

// Disconnected prints a yellow warning about server disconnection.
func (d *Display) Disconnected() {
	fmt.Fprintf(d.w, "  %s\n", d.yellow("⚠ Disconnected from server"))
}

// Shutdown prints the shutdown message.
func (d *Display) Shutdown() {
	fmt.Fprintf(d.w, "  fonzygrok stopped.\n")
}

// Error prints a red error message.
func (d *Display) Error(msg string) {
	fmt.Fprintf(d.w, "  %s %s\n", d.red("✘"), d.red(msg))
}

// Ready prints the "press Ctrl+C" hint.
func (d *Display) Ready() {
	fmt.Fprintf(d.w, "\n  Press Ctrl+C to stop.\n\n")
}

// green wraps text in green ANSI codes if color is enabled.
func (d *Display) green(s string) string {
	if d.noColor {
		return s
	}
	return ansiGreen + s + ansiReset
}

// red wraps text in red ANSI codes if color is enabled.
func (d *Display) red(s string) string {
	if d.noColor {
		return s
	}
	return ansiRed + s + ansiReset
}

// yellow wraps text in yellow ANSI codes if color is enabled.
func (d *Display) yellow(s string) string {
	if d.noColor {
		return s
	}
	return ansiYellow + s + ansiReset
}

// shouldDisableColor returns true if ANSI colors should be suppressed.
func shouldDisableColor() bool {
	// NO_COLOR spec: https://no-color.org
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return true
	}

	// TERM=dumb or unset TERM on non-Windows.
	term := os.Getenv("TERM")
	if runtime.GOOS != "windows" && (term == "" || term == "dumb") {
		return true
	}

	// Windows: check for modern terminal indicators.
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") == "" && os.Getenv("ANSICON") == "" {
			return true
		}
	}

	return false
}
