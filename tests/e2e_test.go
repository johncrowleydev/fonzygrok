//go:build e2e

// Package tests contains end-to-end integration tests for fonzygrok.
// These tests start a real server and client in-process and verify the
// full tunnel round-trip: client connects → requests tunnel → HTTP request
// hits edge → proxied through SSH → reaches local service → response returns.
//
// Run: go test -v -race -tags=e2e -timeout 60s ./tests/
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/client"
	"github.com/fonzygrok/fonzygrok/internal/server"
)

// testEnv holds all the components for an integration test.
type testEnv struct {
	t         *testing.T
	tmpDir    string
	srv       *server.Server
	srvCancel context.CancelFunc

	sshAddr   string
	edgeAddr  string
	adminAddr string
	rawToken  string
	domain    string
}

// setupTestEnv creates a complete fonzygrok server and returns the test env.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	domain := "tunnel.test.local"
	config := server.ServerConfig{
		DataDir: tmpDir,
		Domain:  domain,
		SSH: server.SSHConfig{
			Addr:        "127.0.0.1:0",
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: server.EdgeConfig{
			Addr:       "127.0.0.1:0",
			BaseDomain: domain,
		},
		Admin: server.AdminConfig{
			Addr: "127.0.0.1:0",
		},
	}

	srv, err := server.NewServer(config, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Create a token before starting (since we have store access).
	_, _, err = srv.Store().CreateToken("e2e-test")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Start server in background.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Start(ctx); err != nil {
			// Server returns nil on clean shutdown.
		}
	}()

	// Wait for server to start and find its addresses.
	time.Sleep(300 * time.Millisecond)

	// Discover actual listen addresses by hitting the admin health endpoint.
	// Since we used :0 ports, we need to find them from the server.
	// For now, we'll use the admin API to confirm it's running.
	// We need to discover the ports — let's read the server log or use a helper.

	// Alternative approach: use the admin health endpoint after finding its addr.
	// Since Server doesn't expose Addr() methods publicly, let's use a different approach.
	// We'll start listeners ourselves, then pass the actual addresses.

	// Cancel and recreate with fixed ports.
	cancel()
	time.Sleep(200 * time.Millisecond)

	// Pick random available ports.
	sshAddr := getAvailableAddr(t)
	edgeAddr := getAvailableAddr(t)
	adminAddr := getAvailableAddr(t)

	config2 := server.ServerConfig{
		DataDir: tmpDir,
		Domain:  domain,
		SSH: server.SSHConfig{
			Addr:        sshAddr,
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: server.EdgeConfig{
			Addr:       edgeAddr,
			BaseDomain: domain,
		},
		Admin: server.AdminConfig{
			Addr: adminAddr,
		},
	}

	srv2, err := server.NewServer(config2, logger)
	if err != nil {
		t.Fatalf("NewServer (retry): %v", err)
	}

	// Create a token in the new server's store.
	tok2, rawToken2, err := srv2.Store().CreateToken("e2e-test-2")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	_ = tok2

	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() {
		srv2.Start(ctx2)
	}()
	time.Sleep(300 * time.Millisecond)

	env := &testEnv{
		t:         t,
		tmpDir:    tmpDir,
		srv:       srv2,
		srvCancel: cancel2,
		sshAddr:   sshAddr,
		edgeAddr:  edgeAddr,
		adminAddr: adminAddr,
		rawToken:  rawToken2,
		domain:    domain,
	}

	t.Cleanup(func() {
		cancel2()
		time.Sleep(200 * time.Millisecond)
	})

	return env
}

// getAvailableAddr returns a "127.0.0.1:<port>" string for a random available port.
func getAvailableAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get available addr: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// startLocalHTTPServer starts a local HTTP server that responds with a fixed body.
func startLocalHTTPServer(t *testing.T, handler http.Handler) (port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen local: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var p int
	fmt.Sscanf(portStr, "%d", &p)

	return p, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
}

// connectClient creates a fonzygrok client connector and connects.
func connectClient(t *testing.T, env *testEnv) *client.Connector {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := client.ClientConfig{
		ServerAddr: env.sshAddr,
		Token:      env.rawToken,
		Insecure:   true,
	}

	conn := client.NewConnector(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("client connect: %v", err)
	}

	return conn
}

// --- Test Cases ---

// TestE2E_1_ClientConnectsAndGetsTunnel verifies the client can connect,
// open a control channel, and receive a tunnel assignment.
func TestE2E_1_ClientConnectsAndGetsTunnel(t *testing.T) {
	env := setupTestEnv(t)

	conn := connectClient(t, env)
	defer conn.Close()

	if !conn.IsConnected() {
		t.Fatal("expected client to be connected")
	}

	ctrl, err := conn.OpenControl()
	if err != nil {
		t.Fatalf("open control: %v", err)
	}
	defer ctrl.Close()

	assignment, err := ctrl.RequestTunnel(3000, "http")
	if err != nil {
		t.Fatalf("request tunnel: %v", err)
	}

	if assignment.TunnelID == "" {
		t.Error("expected non-empty tunnel ID")
	}
	if assignment.PublicURL == "" {
		t.Error("expected non-empty public URL")
	}

	t.Logf("Got tunnel: ID=%s URL=%s", assignment.TunnelID, assignment.PublicURL)
}

// TestE2E_2_GETThroughTunnel verifies a full HTTP GET round-trip.
func TestE2E_2_GETThroughTunnel(t *testing.T) {
	env := setupTestEnv(t)

	// Start local HTTP server.
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "hello from local service",
			"path":    r.URL.Path,
		})
	})
	localPort, stopLocal := startLocalHTTPServer(t, localHandler)
	defer stopLocal()

	// Connect client and request tunnel.
	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, err := conn.OpenControl()
	if err != nil {
		t.Fatalf("open control: %v", err)
	}
	defer ctrl.Close()

	assignment, err := ctrl.RequestTunnel(localPort, "http")
	if err != nil {
		t.Fatalf("request tunnel: %v", err)
	}

	// Start proxy handler on client side.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(localPort, logger)
	sshClient := conn.SSHClient()
	proxyChans := sshClient.HandleChannelOpen("proxy")
	ctx, cancelProxy := context.WithCancel(context.Background())
	defer cancelProxy()
	go proxy.HandleChannels(ctx, proxyChans)

	time.Sleep(100 * time.Millisecond)

	// Make HTTP request through the edge.
	host := assignment.TunnelID + "." + env.domain
	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/api/test", nil)
	req.Host = host

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, string(body))
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["message"] != "hello from local service" {
		t.Errorf("message: got %q, want %q", body["message"], "hello from local service")
	}
	if body["path"] != "/api/test" {
		t.Errorf("path: got %q, want %q", body["path"], "/api/test")
	}
}

// TestE2E_3_POSTWithBody verifies body proxying.
func TestE2E_3_POSTWithBody(t *testing.T) {
	env := setupTestEnv(t)

	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"method":   r.Method,
			"received": string(bodyBytes),
		})
	})
	localPort, stopLocal := startLocalHTTPServer(t, localHandler)
	defer stopLocal()

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	assignment, _ := ctrl.RequestTunnel(localPort, "http")

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(localPort, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proxy.HandleChannels(ctx, conn.SSHClient().HandleChannelOpen("proxy"))

	time.Sleep(100 * time.Millisecond)

	host := assignment.TunnelID + "." + env.domain
	reqBody := `{"name":"fonzygrok","version":"1.0"}`
	req, _ := http.NewRequest("POST", "http://"+env.edgeAddr+"/data", strings.NewReader(reqBody))
	req.Host = host
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("HTTP POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["method"] != "POST" {
		t.Errorf("method: got %q, want %q", body["method"], "POST")
	}
	if body["received"] != reqBody {
		t.Errorf("body not proxied correctly: got %q, want %q", body["received"], reqBody)
	}
}

// TestE2E_4_ForwardedHeaders verifies X-Forwarded-* headers.
func TestE2E_4_ForwardedHeaders(t *testing.T) {
	env := setupTestEnv(t)

	receivedHeaders := make(chan http.Header, 1)
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders <- r.Header
		w.WriteHeader(http.StatusOK)
	})
	localPort, stopLocal := startLocalHTTPServer(t, localHandler)
	defer stopLocal()

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	assignment, _ := ctrl.RequestTunnel(localPort, "http")

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(localPort, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proxy.HandleChannels(ctx, conn.SSHClient().HandleChannelOpen("proxy"))

	time.Sleep(100 * time.Millisecond)

	host := assignment.TunnelID + "." + env.domain
	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = host
	(&http.Client{Timeout: 10 * time.Second}).Do(req)

	select {
	case hdrs := <-receivedHeaders:
		if hdrs.Get("X-Forwarded-For") == "" {
			t.Error("missing X-Forwarded-For header")
		}
		if hdrs.Get("X-Fonzygrok-Tunnel-Id") != assignment.TunnelID {
			t.Errorf("X-Fonzygrok-Tunnel-Id: got %q, want %q",
				hdrs.Get("X-Fonzygrok-Tunnel-Id"), assignment.TunnelID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for headers")
	}
}

// TestE2E_5_404NonexistentSubdomain verifies 404 for unknown tunnels.
func TestE2E_5_404NonexistentSubdomain(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = "nonexistent." + env.domain

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestE2E_6_502LocalServiceDown verifies 502 when local service is unreachable.
func TestE2E_6_502LocalServiceDown(t *testing.T) {
	env := setupTestEnv(t)

	// Use a port that nothing is listening on.
	unusedPort := 59999

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	assignment, _ := ctrl.RequestTunnel(unusedPort, "http")

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(unusedPort, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proxy.HandleChannels(ctx, conn.SSHClient().HandleChannelOpen("proxy"))

	time.Sleep(100 * time.Millisecond)

	host := assignment.TunnelID + "." + env.domain
	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = host

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer resp.Body.Close()

	// Client proxy returns a canned 502 response to the edge,
	// which the edge forwards as-is (502 status).
	if resp.StatusCode != http.StatusBadGateway {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 502, got %d, body: %s", resp.StatusCode, string(body))
	}
}

// TestE2E_7_ClientDisconnectDeregistersTunnel verifies cleanup on disconnect.
func TestE2E_7_ClientDisconnectDeregistersTunnel(t *testing.T) {
	env := setupTestEnv(t)

	localPort, stopLocal := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer stopLocal()

	conn := connectClient(t, env)

	ctrl, _ := conn.OpenControl()
	assignment, _ := ctrl.RequestTunnel(localPort, "http")
	ctrl.Close()

	// Verify tunnel exists via admin API.
	adminResp, err := http.Get("http://" + env.adminAddr + "/api/v1/tunnels")
	if err != nil {
		t.Fatalf("admin GET tunnels: %v", err)
	}
	var tunnelList struct {
		Tunnels []struct {
			TunnelID string `json:"tunnel_id"`
		} `json:"tunnels"`
	}
	json.NewDecoder(adminResp.Body).Decode(&tunnelList)
	adminResp.Body.Close()

	found := false
	for _, tun := range tunnelList.Tunnels {
		if tun.TunnelID == assignment.TunnelID {
			found = true
		}
	}
	if !found {
		t.Error("tunnel not found in admin API before disconnect")
	}

	// Disconnect client.
	conn.Close()
	time.Sleep(500 * time.Millisecond)

	// Tunnel should be deregistered.
	host := assignment.TunnelID + "." + env.domain
	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = host

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("HTTP after disconnect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after disconnect, got %d", resp.StatusCode)
	}
}

// TestE2E_8_AdminHealthEndpoint verifies the health endpoint in the integration context.
func TestE2E_8_AdminHealthEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	resp, err := http.Get("http://" + env.adminAddr + "/api/v1/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&health)
	if health.Status != "healthy" {
		t.Errorf("status: got %q, want %q", health.Status, "healthy")
	}
}

// TestE2E_9_ServerInfo verifies the edge root returns server info.
func TestE2E_9_ServerInfo(t *testing.T) {
	env := setupTestEnv(t)

	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = env.domain

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("server info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var info struct {
		Service string `json:"service"`
	}
	json.NewDecoder(resp.Body).Decode(&info)
	if info.Service != "fonzygrok" {
		t.Errorf("service: got %q, want %q", info.Service, "fonzygrok")
	}
}
