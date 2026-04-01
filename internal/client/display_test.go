package client

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestDisplayBanner verifies Banner output format.
func TestDisplayBanner(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Banner("v1.2.3")

	got := buf.String()
	if got != "fonzygrok v1.2.3\n\n" {
		t.Errorf("Banner = %q, want %q", got, "fonzygrok v1.2.3\n\n")
	}
}

// TestDisplayConnecting verifies Connecting output.
func TestDisplayConnecting(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Connecting("example.com:2222")

	got := buf.String()
	if !strings.Contains(got, "Connecting to example.com:2222...") {
		t.Errorf("Connecting = %q, should contain address", got)
	}
}

// TestDisplayConnected verifies Connected output.
func TestDisplayConnected(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Connected()

	got := buf.String()
	if !strings.Contains(got, "Connected!") {
		t.Errorf("Connected = %q, should contain 'Connected!'", got)
	}
}

// TestDisplayTunnelEstablished verifies the full tunnel info block.
func TestDisplayTunnelEstablished(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.TunnelEstablished("my-api", "https://my-api.example.com", 3000, "localhost:4040")

	got := buf.String()
	checks := []string{
		"Tunnel established!",
		"Name:       my-api",
		"Public URL: https://my-api.example.com",
		"Forwarding: https://my-api.example.com → localhost:3000",
		"Inspector:  http://localhost:4040",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("TunnelEstablished should contain %q, got:\n%s", want, got)
		}
	}
}

// TestDisplayTunnelEstablishedNoName verifies output when name is empty.
func TestDisplayTunnelEstablishedNoName(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.TunnelEstablished("", "https://auto.example.com", 8080, "")

	got := buf.String()
	if strings.Contains(got, "Name:") {
		t.Error("should not show Name when empty")
	}
	if strings.Contains(got, "Inspector:") {
		t.Error("should not show Inspector when empty")
	}
}

// TestDisplayConnectionFailed verifies error + retry output.
func TestDisplayConnectionFailed(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.ConnectionFailed(errors.New("connection refused"), 1, 2)

	got := buf.String()
	if !strings.Contains(got, "Connection failed: connection refused") {
		t.Errorf("should contain error message, got: %s", got)
	}
	if !strings.Contains(got, "Retrying in 2s...") {
		t.Errorf("should contain retry message, got: %s", got)
	}
}

// TestDisplayDisconnected verifies disconnection warning.
func TestDisplayDisconnected(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Disconnected()

	got := buf.String()
	if !strings.Contains(got, "Disconnected from server") {
		t.Errorf("Disconnected = %q, should contain warning", got)
	}
}

// TestDisplayShutdown verifies shutdown message.
func TestDisplayShutdown(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Shutdown()

	got := buf.String()
	if !strings.Contains(got, "fonzygrok stopped.") {
		t.Errorf("Shutdown = %q, should contain stop message", got)
	}
}

// TestDisplayError verifies error output.
func TestDisplayError(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Error("something broke")

	got := buf.String()
	if !strings.Contains(got, "something broke") {
		t.Errorf("Error = %q, should contain message", got)
	}
}

// TestDisplayReady verifies the Ctrl+C hint.
func TestDisplayReady(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Ready()

	got := buf.String()
	if !strings.Contains(got, "Press Ctrl+C to stop.") {
		t.Errorf("Ready = %q, should contain hint", got)
	}
}

// TestDisplayNoColorSuppressesANSI verifies no ANSI codes when noColor is true.
func TestDisplayNoColorSuppressesANSI(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayNoColor(&buf)
	d.Connected()
	d.Error("bad")
	d.ConnectionFailed(errors.New("fail"), 1, 5)

	got := buf.String()
	if strings.Contains(got, "\033[") {
		t.Errorf("noColor output should not contain ANSI escape codes, got:\n%s", got)
	}
}

// TestDisplayColorsWhenEnabled verifies ANSI codes are present when color is on.
func TestDisplayColorsWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	// Force color on by directly setting noColor=false.
	d := &Display{w: &buf, noColor: false}
	d.Connected()

	got := buf.String()
	if !strings.Contains(got, "\033[32m") {
		t.Errorf("color output should contain green ANSI code, got:\n%q", got)
	}
	if !strings.Contains(got, "\033[0m") {
		t.Errorf("color output should contain reset ANSI code, got:\n%q", got)
	}
}
