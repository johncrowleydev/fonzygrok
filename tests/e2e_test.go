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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
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
	jwtToken  string // JWT for admin API auth
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

	// Create an admin user and JWT for authenticated admin API calls.
	hash, _ := auth.HashPassword("e2etestpassword1")
	adminUser, _ := srv2.Store().CreateUser("e2eadmin", "e2e@test.com", hash, "admin")
	jwtSecretPath := filepath.Join(tmpDir, "jwt_secret")
	jwtMgr, _ := auth.NewJWTManager(jwtSecretPath, 24*time.Hour)
	jwtToken, _ := jwtMgr.CreateToken(auth.Claims{
		UserID: adminUser.ID, Username: adminUser.Username, Role: adminUser.Role,
	})

	env := &testEnv{
		t:         t,
		tmpDir:    tmpDir,
		srv:       srv2,
		srvCancel: cancel2,
		sshAddr:   sshAddr,
		edgeAddr:  edgeAddr,
		adminAddr: adminAddr,
		rawToken:  rawToken2,
		jwtToken:  jwtToken,
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

	assignment, err := ctrl.RequestTunnel(3000, "http", "")
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

	assignment, err := ctrl.RequestTunnel(localPort, "http", "")
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

	assignment, _ := ctrl.RequestTunnel(localPort, "http", "")

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

	assignment, _ := ctrl.RequestTunnel(localPort, "http", "")

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

	assignment, _ := ctrl.RequestTunnel(unusedPort, "http", "")

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
	assignment, _ := ctrl.RequestTunnel(localPort, "http", "")
	ctrl.Close()

	// Verify tunnel exists via admin API.
	adminReq, _ := http.NewRequest("GET", "http://"+env.adminAddr+"/api/v1/tunnels", nil)
	adminReq.Header.Set("Authorization", "Bearer "+env.jwtToken)
	adminResp, err := http.DefaultClient.Do(adminReq)
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

// --- v1.1 Tests ---

// TestE2E_10_CustomNameTunnel verifies client --name → URL contains custom name.
func TestE2E_10_CustomNameTunnel(t *testing.T) {
	env := setupTestEnv(t)

	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": "custom-name"})
	})
	localPort, stopLocal := startLocalHTTPServer(t, localHandler)
	defer stopLocal()

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, err := conn.OpenControl()
	if err != nil {
		t.Fatalf("open control: %v", err)
	}
	defer ctrl.Close()

	assignment, err := ctrl.RequestTunnel(localPort, "http", "test-app")
	if err != nil {
		t.Fatalf("request tunnel: %v", err)
	}

	if assignment.Name != "test-app" {
		t.Errorf("name: got %q, want %q", assignment.Name, "test-app")
	}
	if !strings.Contains(assignment.PublicURL, "test-app") {
		t.Errorf("public URL should contain 'test-app': got %q", assignment.PublicURL)
	}

	// Verify proxying works via the custom name subdomain.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(localPort, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proxy.HandleChannels(ctx, conn.SSHClient().HandleChannelOpen("proxy"))
	time.Sleep(100 * time.Millisecond)

	req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/", nil)
	req.Host = "test-app." + env.domain
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("HTTP GET via custom name: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body: %s", resp.StatusCode, string(body))
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["hello"] != "custom-name" {
		t.Errorf("body: got %q, want %q", body["hello"], "custom-name")
	}

	t.Logf("Custom name tunnel: URL=%s", assignment.PublicURL)
}

// TestE2E_11_AutoGeneratedName verifies auto-generated names are adjective-noun format.
func TestE2E_11_AutoGeneratedName(t *testing.T) {
	env := setupTestEnv(t)

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	assignment, err := ctrl.RequestTunnel(3000, "http", "")
	if err != nil {
		t.Fatalf("request tunnel: %v", err)
	}

	namePattern := regexp.MustCompile(`^[a-z]+-[a-z]+(-[a-z0-9]+)?$`)
	if !namePattern.MatchString(assignment.Name) {
		t.Errorf("auto-generated name %q does not match adjective-noun pattern", assignment.Name)
	}

	if !strings.Contains(assignment.PublicURL, assignment.Name) {
		t.Errorf("public URL should contain name %q: got %q", assignment.Name, assignment.PublicURL)
	}

	t.Logf("Auto-generated name: %s URL=%s", assignment.Name, assignment.PublicURL)
}

// TestE2E_12_DuplicateNameRejected verifies second client with different token
// requesting the same name gets an error.
func TestE2E_12_DuplicateNameRejected(t *testing.T) {
	env := setupTestEnv(t)

	// First client claims the name.
	conn1 := connectClient(t, env)
	defer conn1.Close()

	ctrl1, _ := conn1.OpenControl()
	defer ctrl1.Close()

	_, err := ctrl1.RequestTunnel(3000, "http", "taken-name")
	if err != nil {
		t.Fatalf("first tunnel: %v", err)
	}

	// Create a DIFFERENT token for the second client.
	// Same-token reconnection is allowed (reconnect race handling),
	// so we need a different token to trigger the duplicate rejection.
	_, rawToken2, err := env.srv.Store().CreateToken("e2e-other-user")
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Connect second client with different token.
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg2 := client.ClientConfig{
		ServerAddr: env.sshAddr,
		Token:      rawToken2,
		Insecure:   true,
	}
	conn2 := client.NewConnector(cfg2, logger)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := conn2.Connect(ctx2); err != nil {
		t.Fatalf("second client connect: %v", err)
	}
	defer conn2.Close()

	ctrl2, _ := conn2.OpenControl()
	defer ctrl2.Close()

	_, err = ctrl2.RequestTunnel(3001, "http", "taken-name")
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}

	if !strings.Contains(err.Error(), "already in use") && !strings.Contains(err.Error(), "taken-name") {
		t.Logf("error message (acceptable): %v", err)
	}

	t.Logf("Duplicate name correctly rejected: %v", err)
}

// TestE2E_13_ReservedNameRejected verifies reserved names are rejected.
func TestE2E_13_ReservedNameRejected(t *testing.T) {
	env := setupTestEnv(t)

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	_, err := ctrl.RequestTunnel(3000, "http", "www")
	if err == nil {
		t.Fatal("expected error for reserved name 'www', got nil")
	}

	t.Logf("Reserved name correctly rejected: %v", err)
}

// TestE2E_15_MetricsIncrement verifies proxy requests update tunnel metrics.
func TestE2E_15_MetricsIncrement(t *testing.T) {
	env := setupTestEnv(t)

	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("metric-test-response-body"))
	})
	localPort, stopLocal := startLocalHTTPServer(t, localHandler)
	defer stopLocal()

	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	assignment, _ := ctrl.RequestTunnel(localPort, "http", "metrics-test")

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	proxy := client.NewLocalProxy(localPort, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proxy.HandleChannels(ctx, conn.SSHClient().HandleChannelOpen("proxy"))
	time.Sleep(100 * time.Millisecond)

	// Send 5 requests.
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "http://"+env.edgeAddr+"/test", nil)
		req.Host = "metrics-test." + env.domain
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}

	// Check tunnel metrics via admin API.
	time.Sleep(100 * time.Millisecond)
	adminReq, _ := http.NewRequest("GET", "http://"+env.adminAddr+"/api/v1/tunnels", nil)
	adminReq.Header.Set("Authorization", "Bearer "+env.jwtToken)
	adminResp, err := http.DefaultClient.Do(adminReq)
	if err != nil {
		t.Fatalf("admin tunnels: %v", err)
	}
	defer adminResp.Body.Close()

	var tunnelList struct {
		Tunnels []struct {
			TunnelID        string `json:"tunnel_id"`
			Name            string `json:"name"`
			BytesOut        int64  `json:"bytes_out"`
			RequestsProxied int64  `json:"requests_proxied"`
		} `json:"tunnels"`
	}
	json.NewDecoder(adminResp.Body).Decode(&tunnelList)

	var found bool
	for _, tun := range tunnelList.Tunnels {
		if tun.TunnelID == assignment.TunnelID {
			found = true
			if tun.RequestsProxied != 5 {
				t.Errorf("requests_proxied: got %d, want 5", tun.RequestsProxied)
			}
			if tun.BytesOut <= 0 {
				t.Error("bytes_out should be > 0 after proxying")
			}
			t.Logf("Metrics: requests=%d bytes_out=%d", tun.RequestsProxied, tun.BytesOut)
		}
	}
	if !found {
		t.Error("tunnel not found in admin API")
	}
}

// TestE2E_16_MetricsEndpoint verifies GET /api/v1/metrics returns aggregate data.
func TestE2E_16_MetricsEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	// First register a tunnel so there's at least 1 active.
	conn := connectClient(t, env)
	defer conn.Close()

	ctrl, _ := conn.OpenControl()
	defer ctrl.Close()

	_, err := ctrl.RequestTunnel(3000, "http", "")
	if err != nil {
		t.Fatalf("request tunnel: %v", err)
	}

	// Query metrics endpoint.
	metricsReq, _ := http.NewRequest("GET", "http://"+env.adminAddr+"/api/v1/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer "+env.jwtToken)
	resp, err := http.DefaultClient.Do(metricsReq)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var metrics struct {
		TotalBytesIn         int64   `json:"total_bytes_in"`
		TotalBytesOut        int64   `json:"total_bytes_out"`
		TotalRequestsProxied int64   `json:"total_requests_proxied"`
		ActiveTunnels        int     `json:"active_tunnels"`
		ActiveClients        int     `json:"active_clients"`
		UptimeSeconds        float64 `json:"uptime_seconds"`
	}
	json.NewDecoder(resp.Body).Decode(&metrics)

	if metrics.ActiveTunnels < 1 {
		t.Errorf("active_tunnels: got %d, want >= 1", metrics.ActiveTunnels)
	}
	if metrics.UptimeSeconds <= 0 {
		t.Errorf("uptime_seconds should be > 0, got %f", metrics.UptimeSeconds)
	}

	t.Logf("Metrics: tunnels=%d clients=%d uptime=%.1fs total_requests=%d",
		metrics.ActiveTunnels, metrics.ActiveClients, metrics.UptimeSeconds, metrics.TotalRequestsProxied)
}
