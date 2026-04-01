package client

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestRequestTunnelSuccess verifies the happy path: request a tunnel, get an assignment.
func TestRequestTunnelSuccess(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	cc, err := c.OpenControl()
	if err != nil {
		t.Fatalf("OpenControl() error: %v", err)
	}
	defer cc.Close()

	assignment, err := cc.RequestTunnel(3000, "http", "")
	if err != nil {
		t.Fatalf("RequestTunnel() error: %v", err)
	}

	if assignment.TunnelID != "test01" {
		t.Errorf("TunnelID = %q, want %q", assignment.TunnelID, "test01")
	}
	if assignment.Protocol != "http" {
		t.Errorf("Protocol = %q, want %q", assignment.Protocol, "http")
	}
	if assignment.PublicURL == "" {
		t.Error("PublicURL should not be empty")
	}
}

// TestRequestTunnelServerError verifies error response handling.
func TestRequestTunnelServerError(t *testing.T) {
	addr, cleanup := startTestSSHServerWithErrorResponse(
		t, testToken, "SUBDOMAIN_TAKEN", "subdomain already in use",
	)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	cc, err := c.OpenControl()
	if err != nil {
		t.Fatalf("OpenControl() error: %v", err)
	}
	defer cc.Close()

	_, err = cc.RequestTunnel(3000, "http", "")
	if err == nil {
		t.Fatal("RequestTunnel() should fail when server returns error")
	}

	if !strings.Contains(err.Error(), "SUBDOMAIN_TAKEN") {
		t.Errorf("error should contain error code, got: %v", err)
	}
	t.Logf("expected error: %v", err)
}

// TestCloseTunnel verifies sending a TunnelClose message.
func TestCloseTunnel(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	cc, err := c.OpenControl()
	if err != nil {
		t.Fatalf("OpenControl() error: %v", err)
	}
	defer cc.Close()

	// Request a tunnel first so there's something to close.
	_, err = cc.RequestTunnel(3000, "http", "")
	if err != nil {
		t.Fatalf("RequestTunnel() error: %v", err)
	}

	if err := cc.CloseTunnel("test01"); err != nil {
		t.Fatalf("CloseTunnel() error: %v", err)
	}
}

// TestRequestTunnelWithName verifies that a custom name is sent in the
// request and echoed back in the assignment.
func TestRequestTunnelWithName(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	cc, err := c.OpenControl()
	if err != nil {
		t.Fatalf("OpenControl() error: %v", err)
	}
	defer cc.Close()

	assignment, err := cc.RequestTunnel(3000, "http", "my-api")
	if err != nil {
		t.Fatalf("RequestTunnel() error: %v", err)
	}

	if assignment.Name != "my-api" {
		t.Errorf("Name = %q, want %q", assignment.Name, "my-api")
	}
	if !strings.Contains(assignment.PublicURL, "my-api") {
		t.Errorf("PublicURL should contain 'my-api', got %q", assignment.PublicURL)
	}
}

// TestRequestTunnelWithoutName verifies that omitting a name still works
// and the server auto-assigns one (test server defaults to "test01").
func TestRequestTunnelWithoutName(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	defer c.Close()

	cc, err := c.OpenControl()
	if err != nil {
		t.Fatalf("OpenControl() error: %v", err)
	}
	defer cc.Close()

	assignment, err := cc.RequestTunnel(3000, "http", "")
	if err != nil {
		t.Fatalf("RequestTunnel() error: %v", err)
	}

	// Test server defaults to "test01" when no name is given.
	if assignment.Name != "test01" {
		t.Errorf("Name = %q, want %q", assignment.Name, "test01")
	}
}
