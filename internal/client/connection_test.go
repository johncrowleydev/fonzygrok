package client

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

const testToken = "fgk_testtoken1234567890abcdef1234"

// TestConnectSuccess verifies a successful SSH connection with a valid token.
func TestConnectSuccess(t *testing.T) {
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

	if !c.IsConnected() {
		t.Error("IsConnected() = false after successful Connect()")
	}
}

// TestConnectInvalidToken verifies that auth fails with a bad token.
func TestConnectInvalidToken(t *testing.T) {
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	c := NewConnector(ClientConfig{
		ServerAddr: addr,
		Token:      "fgk_wrongtoken00000000000000000000",
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Connect(ctx)
	if err == nil {
		c.Close()
		t.Fatal("Connect() should fail with invalid token")
	}
	t.Logf("expected error: %v", err)
}

// TestConnectBadAddress verifies that dialing an unreachable address errors.
func TestConnectBadAddress(t *testing.T) {
	c := NewConnector(ClientConfig{
		ServerAddr: "127.0.0.1:1", // unlikely to have an SSH server
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Connect(ctx)
	if err == nil {
		c.Close()
		t.Fatal("Connect() should fail with unreachable address")
	}
	t.Logf("expected error: %v", err)
}

// TestCloseDisconnects verifies that Close terminates the session.
func TestCloseDisconnects(t *testing.T) {
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

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if c.IsConnected() {
		t.Error("IsConnected() = true after Close()")
	}
}

// TestCloseIdempotent verifies calling Close twice does not error.
func TestCloseIdempotent(t *testing.T) {
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

	if err := c.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close() should not error, got: %v", err)
	}
}

// TestConnectAlreadyConnected verifies that calling Connect twice returns an error.
func TestConnectAlreadyConnected(t *testing.T) {
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
		t.Fatalf("first Connect() error: %v", err)
	}
	defer c.Close()

	if err := c.Connect(ctx); err == nil {
		t.Fatal("second Connect() should return an error")
	}
}

// TestOpenControlNotConnected verifies OpenControl fails when not connected.
func TestOpenControlNotConnected(t *testing.T) {
	c := NewConnector(ClientConfig{
		ServerAddr: "127.0.0.1:0",
		Token:      testToken,
		Insecure:   true,
	}, slog.Default())

	_, err := c.OpenControl()
	if err == nil {
		t.Fatal("OpenControl() should fail when not connected")
	}
}
