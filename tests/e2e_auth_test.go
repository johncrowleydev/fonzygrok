//go:build e2e

// Package tests — e2e_auth_test.go contains authentication E2E tests.
//
// These tests verify that authentication and token management work
// correctly end-to-end: invalid tokens are rejected, revoked tokens
// cause disconnection, etc.
//
// REF: SPR-013, SPR-017, SPR-018
package tests

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/client"
)

// TestE2E_InvalidToken verifies that a client with a bad token is rejected
// at the SSH authentication stage and cannot establish a tunnel.
//
// Refs: SPR-017, CON-001 §3
func TestE2E_InvalidToken(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := client.ClientConfig{
		ServerAddr: ts.sshAddr,
		Token:      "fgk_this_is_a_completely_bogus_token_value",
		Insecure:   true,
	}

	conn := client.NewConnector(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := conn.Connect(ctx)
	if err == nil {
		conn.Close()
		t.Fatal("expected connection to be rejected with invalid token, but it succeeded")
	}

	// The error should indicate auth failure (SSH handshake rejection).
	t.Logf("Invalid token correctly rejected: %v", err)

	// Verify the connector is not connected.
	if conn.IsConnected() {
		t.Error("connector should not be connected after auth failure")
	}
}

// TestE2E_RevokedToken verifies that when a token is revoked via the admin
// API, any active tunnel using that token is disconnected cleanly.
//
// Refs: SPR-018, CON-001 §4.3
func TestE2E_RevokedToken(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	// Create a token via the store so we know its ID for revocation.
	tok, rawToken, err := ts.srv.Store().CreateToken("revoke-test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Connect client with this token and establish a tunnel.
	localPort, stopLocal := startLocalHTTPService(t, echoHandler("revoke-test-ok"))
	defer stopLocal()

	sess := startTestClient(t, ts, rawToken, localPort, "http")
	host := sess.Host(ts.domain)

	// Verify tunnel works before revocation.
	waitForTunnel(t, ts.edgeAddr, host, 5*time.Second)

	// Verify the tunnel appears in the admin API.
	tunnels := listTunnels(t, ts)
	found := false
	for _, tun := range tunnels {
		if tun.TunnelID == sess.assignment.TunnelID {
			found = true
		}
	}
	if !found {
		t.Error("tunnel not found in admin API before revocation")
	}

	// Revoke the token via admin API.
	revokeTestToken(t, ts, tok.ID)

	// After revocation, the tunnel should be deregistered.
	// The server calls disconnectTunnelsByToken which deregisters tunnels.
	waitForTunnelGone(t, ts.edgeAddr, host, 5*time.Second)

	// Verify the tunnel is gone from admin API.
	tunnelsAfter := listTunnels(t, ts)
	for _, tun := range tunnelsAfter {
		if tun.TunnelID == sess.assignment.TunnelID {
			t.Error("tunnel still in admin API after token revocation")
		}
	}

	t.Logf("Token %s revoked, tunnel %s deregistered", tok.ID, sess.assignment.TunnelID)
}

// TestE2E_EmptyToken verifies that an empty token string is rejected.
//
// Refs: SPR-017
func TestE2E_EmptyToken(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := client.ClientConfig{
		ServerAddr: ts.sshAddr,
		Token:      "",
		Insecure:   true,
	}

	conn := client.NewConnector(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := conn.Connect(ctx)
	if err == nil {
		conn.Close()
		t.Fatal("expected connection to be rejected with empty token, but it succeeded")
	}

	t.Logf("Empty token correctly rejected: %v", err)

	if conn.IsConnected() {
		t.Error("connector should not be connected after empty token rejection")
	}
}

// echoHandler returns a simple HTTP handler that writes the given body.
func echoHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(body))
	})
}
