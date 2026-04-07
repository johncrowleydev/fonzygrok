//go:build e2e

// Package tests — e2e_http_test.go contains HTTP tunnel E2E tests using
// the reusable test harness from e2e_helpers_test.go.
//
// These tests exercise the full HTTP tunnel lifecycle:
// client connects → tunnel established → HTTP requests proxied → cleanup.
//
// REF: SPR-013 E2E Test Scaffolding
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestE2E_HTTPTunnel_Basic verifies the full HTTP tunnel lifecycle:
// start server, create token, start client pointing to a local HTTP server,
// and verify requests through the tunnel URL reach the local service.
//
// Refs: SPR-010, CON-001 §5
func TestE2E_HTTPTunnel_Basic(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start a local HTTP service that returns a known response.
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": "local-backend",
			"path":    r.URL.Path,
			"method":  r.Method,
		})
	})
	localPort, stopLocal := startLocalHTTPService(t, localHandler)
	defer stopLocal()

	// Connect client and establish tunnel.
	sess := startTestClient(t, ts, token, localPort, "http")

	// Verify tunnel was assigned.
	if sess.assignment.TunnelID == "" {
		t.Fatal("expected non-empty tunnel ID")
	}
	if sess.assignment.PublicURL == "" {
		t.Fatal("expected non-empty public URL")
	}

	// Make HTTP request through the edge router.
	host := sess.Host(ts.domain)
	req, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/api/hello", nil)
	req.Host = host

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("HTTP GET through tunnel: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["service"] != "local-backend" {
		t.Errorf("service: got %q, want %q", body["service"], "local-backend")
	}
	if body["path"] != "/api/hello" {
		t.Errorf("path: got %q, want %q", body["path"], "/api/hello")
	}
	if body["method"] != "GET" {
		t.Errorf("method: got %q, want %q", body["method"], "GET")
	}

	t.Logf("HTTP tunnel basic: tunnel_id=%s url=%s", sess.assignment.TunnelID, sess.assignment.PublicURL)
}

// TestE2E_HTTPTunnel_Reconnect verifies that after a client disconnects and
// reconnects, the tunnel is re-established and requests flow again.
//
// Refs: SPR-010, CON-001 §3.1
func TestE2E_HTTPTunnel_Reconnect(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start a persistent local HTTP service.
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("reconnect-test-ok"))
	})
	localPort, stopLocal := startLocalHTTPService(t, localHandler)
	defer stopLocal()

	// First connection.
	sess1 := startTestClient(t, ts, token, localPort, "http")
	host1 := sess1.Host(ts.domain)

	// Verify first tunnel works.
	req1, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	req1.Host = host1
	resp1, err := httpClient().Do(req1)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	b1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d: %s", resp1.StatusCode, string(b1))
	}
	if string(b1) != "reconnect-test-ok" {
		t.Errorf("first request body: got %q, want %q", string(b1), "reconnect-test-ok")
	}

	// Disconnect the first client.
	sess1.Disconnect()
	time.Sleep(500 * time.Millisecond)

	// Verify first tunnel is gone.
	waitForTunnelGone(t, ts.edgeAddr, host1, 3*time.Second)

	// Reconnect with the same token.
	sess2 := startTestClient(t, ts, token, localPort, "http")
	host2 := sess2.Host(ts.domain)

	// Verify second tunnel works.
	req2, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	req2.Host = host2
	resp2, err := httpClient().Do(req2)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d: %s", resp2.StatusCode, string(b2))
	}
	if string(b2) != "reconnect-test-ok" {
		t.Errorf("second request body: got %q, want %q", string(b2), "reconnect-test-ok")
	}

	t.Logf("Reconnect: first=%s second=%s", sess1.assignment.TunnelID, sess2.assignment.TunnelID)
}

// TestE2E_HTTPTunnel_LocalDown verifies that when the tunnel is established
// but the local HTTP service is stopped, the edge returns 502 Bad Gateway.
//
// Refs: SPR-010, CON-001 §5.4
func TestE2E_HTTPTunnel_LocalDown(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start a local HTTP service, then stop it.
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	localPort, stopLocal := startLocalHTTPService(t, localHandler)

	// Establish tunnel while local service is running.
	sess := startTestClient(t, ts, token, localPort, "http")
	host := sess.Host(ts.domain)

	// Verify tunnel works initially.
	req1, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	req1.Host = host
	resp1, err := httpClient().Do(req1)
	if err != nil {
		t.Fatalf("initial request: %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("initial request: expected 200, got %d", resp1.StatusCode)
	}

	// Now stop the local service.
	stopLocal()
	time.Sleep(200 * time.Millisecond)

	// Request through tunnel should get 502.
	req2, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	req2.Host = host
	resp2, err := httpClient().Do(req2)
	if err != nil {
		t.Fatalf("request after local down: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadGateway {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("expected 502 Bad Gateway, got %d: %s", resp2.StatusCode, string(body))
	}

	t.Log("Local service down correctly returns 502 via tunnel")
}
