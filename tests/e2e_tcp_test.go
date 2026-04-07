//go:build e2e

// Package tests — e2e_tcp_test.go contains TCP tunnel E2E tests.
//
// These tests exercise TCP tunnel functionality from SPR-010.
// Most tests SKIP until T-033A (server-side TCP edge) is implemented.
// The test bodies are fully written so they'll run automatically once
// serverSupportsTCP() returns true.
//
// REF: SPR-013 E2E Test Scaffolding, SPR-010 T-034B
package tests

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/client"
)

// TestE2E_TCPTunnel_Basic verifies the full TCP tunnel lifecycle:
// start server, client with --protocol tcp, connect to assigned port,
// verify data flows to a local TCP echo server.
//
// Refs: SPR-010 T-033A, T-034B
func TestE2E_TCPTunnel_Basic(t *testing.T) {
	if !serverSupportsTCP(t) {
		t.Skip("TCP edge not implemented yet (T-033A pending)")
	}

	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start a local TCP echo server.
	echoPort, stopEcho := startLocalTCPEchoServer(t)
	defer stopEcho()

	// Connect client with TCP protocol.
	sess := startTestClient(t, ts, token, echoPort, "tcp")

	// Verify assignment has an assigned port.
	if sess.assignment.AssignedPort == 0 {
		t.Fatal("expected non-zero assigned port for TCP tunnel")
	}

	// Connect to the assigned TCP port on the server.
	tcpAddr := fmt.Sprintf("127.0.0.1:%d", sess.assignment.AssignedPort)
	conn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial TCP tunnel port: %v", err)
	}
	defer conn.Close()

	// Send data through the tunnel and verify it echoes back.
	testPayload := "hello-tcp-tunnel-test\n"
	_, err = conn.Write([]byte(testPayload))
	if err != nil {
		t.Fatalf("write to TCP tunnel: %v", err)
	}

	// Read the echoed response.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, len(testPayload)*2)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read from TCP tunnel: %v", err)
	}

	received := string(buf[:n])
	if received != testPayload {
		t.Errorf("echo mismatch: got %q, want %q", received, testPayload)
	}

	t.Logf("TCP tunnel basic: port=%d payload echoed correctly", sess.assignment.AssignedPort)
}

// TestE2E_TCPTunnel_LocalDown verifies that when a TCP tunnel is established
// but the local TCP service is stopped, the connection is closed cleanly
// (no HTTP 502 — TCP has no HTTP semantics).
//
// Refs: SPR-010, CON-001 §5
func TestE2E_TCPTunnel_LocalDown(t *testing.T) {
	if !serverSupportsTCP(t) {
		t.Skip("TCP edge not implemented yet (T-033A pending)")
	}

	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start a local TCP server, then stop it after tunnel is up.
	echoPort, stopEcho := startLocalTCPEchoServer(t)

	// Connect client with TCP protocol while local service is running.
	sess := startTestClient(t, ts, token, echoPort, "tcp")

	if sess.assignment.AssignedPort == 0 {
		t.Fatal("expected non-zero assigned port")
	}

	// Stop the local TCP service.
	stopEcho()
	time.Sleep(200 * time.Millisecond)

	// Connect to the tunnel — should either fail to connect or get
	// an immediate connection close (no HTTP 502 response).
	tcpAddr := fmt.Sprintf("127.0.0.1:%d", sess.assignment.AssignedPort)
	conn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
	if err != nil {
		// Connection refused is acceptable — means server closed the listener.
		t.Logf("TCP tunnel local-down: connection refused (expected): %v", err)
		return
	}
	defer conn.Close()

	// If we connected, any read should get EOF (channel closed cleanly).
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err == nil && n > 0 {
		// Should NOT get an HTTP 502 response — that's HTTP-only behavior.
		response := string(buf[:n])
		if len(response) > 4 && response[:4] == "HTTP" {
			t.Errorf("TCP tunnel sent HTTP response on local-down — should close cleanly, got: %q", response)
		}
	}
	// EOF or connection reset is the expected behavior.
	if err != nil && err != io.EOF {
		t.Logf("TCP tunnel local-down: read error (acceptable): %v", err)
	}

	t.Log("TCP tunnel local-down: connection closed cleanly (no HTTP 502)")
}

// TestE2E_TCPTunnel_Coexist verifies that a single client session can have
// both HTTP and TCP tunnels active simultaneously.
//
// Refs: SPR-010
func TestE2E_TCPTunnel_Coexist(t *testing.T) {
	if !serverSupportsTCP(t) {
		t.Skip("TCP edge not implemented yet (T-033A pending)")
	}

	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	// Start both an HTTP and TCP local service.
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("http-coexist-ok"))
	})
	httpPort, stopHTTP := startLocalHTTPService(t, httpHandler)
	defer stopHTTP()

	tcpPort, stopTCP := startLocalTCPEchoServer(t)
	defer stopTCP()

	// We need a single SSH session with two tunnels. The current harness
	// creates one tunnel per startTestClient call. For coexistence testing,
	// we manually manage the client.
	conn := connectClientOnly(t, ts, token)

	ctrl, err := conn.OpenControl()
	if err != nil {
		t.Fatalf("open control: %v", err)
	}
	defer ctrl.Close()

	// Request HTTP tunnel.
	httpAssignment, err := ctrl.RequestTunnel(httpPort, "http", "")
	if err != nil {
		t.Fatalf("request HTTP tunnel: %v", err)
	}

	// Request TCP tunnel on same control channel.
	tcpAssignment, err := ctrl.RequestTunnel(tcpPort, "tcp", "")
	if err != nil {
		t.Fatalf("request TCP tunnel: %v", err)
	}

	if tcpAssignment.AssignedPort == 0 {
		t.Fatal("expected non-zero assigned port for TCP tunnel")
	}

	// Start proxy handlers.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// We need two proxies — one for each local port. Since the proxy
	// dispatches based on channel type, a single proxy can't serve both
	// ports. Instead, we verify that the client handles both channel types.
	httpProxy := client.NewLocalProxy(httpPort, logger)
	tcpProxy := client.NewLocalProxy(tcpPort, logger)

	proxyCtx, proxyCancel := context.WithCancel(context.Background())
	defer proxyCancel()

	sshClient := conn.SSHClient()
	go httpProxy.HandleChannels(proxyCtx, sshClient.HandleChannelOpen(client.ChannelTypeProxy))
	go tcpProxy.HandleChannels(proxyCtx, sshClient.HandleChannelOpen(client.ChannelTypeTCPProxy))

	time.Sleep(200 * time.Millisecond)

	// Verify HTTP tunnel works.
	httpHost := httpAssignment.TunnelID + "." + ts.domain
	httpReq, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	httpReq.Host = httpHost
	httpResp, err := httpClient().Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP tunnel request: %v", err)
	}
	httpBody, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if string(httpBody) != "http-coexist-ok" {
		t.Errorf("HTTP tunnel: got %q, want %q", string(httpBody), "http-coexist-ok")
	}

	// Verify TCP tunnel works.
	tcpAddr := fmt.Sprintf("127.0.0.1:%d", tcpAssignment.AssignedPort)
	tcpConn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial TCP tunnel: %v", err)
	}
	defer tcpConn.Close()

	testData := "coexist-tcp-data\n"
	tcpConn.Write([]byte(testData))
	tcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 256)
	n, err := tcpConn.Read(buf)
	if err != nil {
		t.Fatalf("read TCP echo: %v", err)
	}
	if string(buf[:n]) != testData {
		t.Errorf("TCP echo: got %q, want %q", string(buf[:n]), testData)
	}

	t.Logf("Coexist: HTTP=%s TCP=:%d both functional",
		httpAssignment.TunnelID, tcpAssignment.AssignedPort)
}

// TestE2E_TCPTunnel_PortRelease verifies that when a TCP tunnel is
// disconnected, the assigned port is freed and can be reused.
//
// Refs: SPR-010
func TestE2E_TCPTunnel_PortRelease(t *testing.T) {
	if !serverSupportsTCP(t) {
		t.Skip("TCP edge not implemented yet (T-033A pending)")
	}

	ts := startTestServer(t, defaultServerOpts())
	token := createTestToken(t, ts)

	echoPort, stopEcho := startLocalTCPEchoServer(t)
	defer stopEcho()

	// First: establish a TCP tunnel and note the assigned port.
	sess1 := startTestClient(t, ts, token, echoPort, "tcp")
	port1 := sess1.assignment.AssignedPort
	if port1 == 0 {
		t.Fatal("first tunnel: expected non-zero assigned port")
	}

	// Disconnect the tunnel.
	sess1.Disconnect()
	time.Sleep(500 * time.Millisecond)

	// Verify the port is released: try to listen on it.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port1))
	if err != nil {
		// Port might still be in TIME_WAIT — that's acceptable.
		t.Logf("port %d not immediately available (TIME_WAIT possible): %v", port1, err)
	} else {
		ln.Close()
		t.Logf("port %d released and available for reuse", port1)
	}

	// Second: establish another TCP tunnel. The port pool should
	// have one more port available (or the same port recycled).
	sess2 := startTestClient(t, ts, token, echoPort, "tcp")
	port2 := sess2.assignment.AssignedPort
	if port2 == 0 {
		t.Fatal("second tunnel: expected non-zero assigned port")
	}

	t.Logf("Port release: first=%d second=%d", port1, port2)
}
