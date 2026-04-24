//go:build e2e

// Package tests — e2e_helpers_test.go provides reusable helpers for E2E tests.
//
// These helpers start real fonzygrok server and client processes in-process,
// manage tunnel lifecycle, and provide assertion utilities. All resources
// are cleaned up via t.Cleanup.
//
// REF: SPR-013 E2E Test Scaffolding
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

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/client"
	"github.com/fonzygrok/fonzygrok/internal/proto"
	"github.com/fonzygrok/fonzygrok/internal/server"
)

// --- Server Helpers ---

func e2eDatabaseURL() string {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	return databaseURL
}

// serverOpts configures the test server.
type serverOpts struct {
	// Domain overrides the base domain (default: "tunnel.test.local").
	Domain string
}

// defaultServerOpts returns sane defaults for E2E server configuration.
func defaultServerOpts() serverOpts {
	return serverOpts{
		Domain: "tunnel.test.local",
	}
}

// testServer holds a running fonzygrok server and its metadata.
type testServer struct {
	t         *testing.T
	srv       *server.Server
	cancel    context.CancelFunc
	sshAddr   string
	edgeAddr  string
	adminAddr string
	domain    string
	dataDir   string
	jwtToken  string // Pre-created admin JWT for API auth.
}

// startTestServer spins up a real fonzygrok server with a temp DB,
// random ports, and an admin user+JWT pre-created. Returns the server
// metadata and auto-registers cleanup.
func startTestServer(t *testing.T, opts serverOpts) *testServer {
	t.Helper()

	tmpDir := t.TempDir()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	sshAddr := getAvailPort(t)
	edgeAddr := getAvailPort(t)
	adminAddr := getAvailPort(t)

	databaseURL := e2eDatabaseURL()

	config := server.ServerConfig{
		DataDir:     tmpDir,
		Domain:      opts.Domain,
		DatabaseURL: databaseURL,
		TCPPortMin:  40000,
		TCPPortMax:  41000,
		RateLimit:   100,
		RateBurst:   200,
		SSH: server.SSHConfig{
			Addr:        sshAddr,
			HostKeyPath: filepath.Join(tmpDir, "host_key"),
		},
		Edge: server.EdgeConfig{
			Addr:       edgeAddr,
			BaseDomain: opts.Domain,
		},
		Admin: server.AdminConfig{
			Addr: adminAddr,
		},
	}

	srv, err := server.NewServer(config, logger)
	if err != nil {
		t.Fatalf("startTestServer: NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		srv.Start(ctx)
	}()

	// Allow server to start listening.
	waitForAddr(t, sshAddr, 3*time.Second)
	waitForAddr(t, edgeAddr, 3*time.Second)
	waitForAddr(t, adminAddr, 3*time.Second)

	// Create admin user + JWT for authenticated admin API calls.
	hash, err := auth.HashPassword("e2etestpassword1")
	if err != nil {
		cancel()
		t.Fatalf("startTestServer: hash password: %v", err)
	}
	adminName := "e2eadmin-" + sanitizeName(t.Name())
	adminUser, err := srv.Store().CreateUser(adminName, adminName+"@test.local", hash, "admin")
	if err != nil {
		cancel()
		t.Fatalf("startTestServer: create admin user: %v", err)
	}

	jwtSecretPath := filepath.Join(tmpDir, "jwt_secret")
	jwtMgr, err := auth.NewJWTManager(jwtSecretPath, 24*time.Hour)
	if err != nil {
		cancel()
		t.Fatalf("startTestServer: create JWT manager: %v", err)
	}
	jwtToken, err := jwtMgr.CreateToken(auth.Claims{
		UserID:   adminUser.ID,
		Username: adminUser.Username,
		Role:     adminUser.Role,
	})
	if err != nil {
		cancel()
		t.Fatalf("startTestServer: create JWT: %v", err)
	}

	ts := &testServer{
		t:         t,
		srv:       srv,
		cancel:    cancel,
		sshAddr:   sshAddr,
		edgeAddr:  edgeAddr,
		adminAddr: adminAddr,
		domain:    opts.Domain,
		dataDir:   tmpDir,
		jwtToken:  jwtToken,
	}

	t.Cleanup(func() {
		cancel()
		// Allow subsystems to drain.
		time.Sleep(200 * time.Millisecond)
	})

	return ts
}

// --- Token Helpers ---

// createTestToken creates a tunnel token via the store (direct DB access)
// and returns the raw token string (fgk_...).
func createTestToken(t *testing.T, ts *testServer) string {
	t.Helper()
	_, rawToken, err := ts.srv.Store().CreateToken("e2e-token-" + sanitizeName(t.Name()))
	if err != nil {
		t.Fatalf("createTestToken: %v", err)
	}
	return rawToken
}

// createTestTokenViaAPI creates a tunnel token through the admin REST API
// (POST /api/v1/tokens). This exercises the full auth+API stack.
func createTestTokenViaAPI(t *testing.T, ts *testServer, name string) string {
	t.Helper()

	body := fmt.Sprintf(`{"name":"%s"}`, name)
	req, err := http.NewRequest("POST", "http://"+ts.adminAddr+"/api/v1/tokens",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("createTestTokenViaAPI: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("createTestTokenViaAPI: request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("createTestTokenViaAPI: expected 201, got %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Token == "" {
		t.Fatal("createTestTokenViaAPI: empty token in response")
	}
	return result.Token
}

// revokeTestToken revokes a token via admin API (DELETE /api/v1/tokens/:id).
func revokeTestToken(t *testing.T, ts *testServer, tokenID string) {
	t.Helper()

	req, err := http.NewRequest("DELETE", "http://"+ts.adminAddr+"/api/v1/tokens/"+tokenID, nil)
	if err != nil {
		t.Fatalf("revokeTestToken: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("revokeTestToken: request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("revokeTestToken: expected 204, got %d: %s", resp.StatusCode, string(b))
	}
}

// --- Client Helpers ---

// tunnelSession wraps a single client tunnel session with its resources.
type tunnelSession struct {
	t          *testing.T
	connector  *client.Connector
	ctrl       *client.ControlChannel
	proxy      *client.LocalProxy
	assignment *proto.TunnelAssignment
	cancelFn   context.CancelFunc
}

// Host returns the Host header value for edge requests to this tunnel.
func (s *tunnelSession) Host(domain string) string {
	return s.assignment.TunnelID + "." + domain
}

// NameHost returns the name-based Host header for edge requests.
func (s *tunnelSession) NameHost(domain string) string {
	return s.assignment.Name + "." + domain
}

// Disconnect tears down the tunnel explicitly (for reconnect tests).
func (s *tunnelSession) Disconnect() {
	s.cancelFn()
	s.ctrl.Close()
	s.connector.Close()
}

// startTestClient connects a real fonzygrok client to the test server,
// requests a tunnel, and starts the proxy handler. Returns the tunnel
// session and auto-registers cleanup.
func startTestClient(t *testing.T, ts *testServer, token string, port int, protocol string) *tunnelSession {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := client.ClientConfig{
		ServerAddr: ts.sshAddr,
		Token:      token,
		Insecure:   true,
	}

	conn := client.NewConnector(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := conn.Connect(ctx); err != nil {
		cancel()
		t.Fatalf("startTestClient: connect: %v", err)
	}
	cancel() // connect timeout no longer needed

	ctrl, err := conn.OpenControl()
	if err != nil {
		conn.Close()
		t.Fatalf("startTestClient: open control: %v", err)
	}

	assignment, err := ctrl.RequestTunnel(port, protocol, "")
	if err != nil {
		ctrl.Close()
		conn.Close()
		t.Fatalf("startTestClient: request tunnel: %v", err)
	}

	// Start proxy handler(s).
	proxy := client.NewLocalProxy(port, logger)
	proxyCtx, proxyCancel := context.WithCancel(context.Background())

	sshClient := conn.SSHClient()
	if sshClient == nil {
		proxyCancel()
		ctrl.Close()
		conn.Close()
		t.Fatal("startTestClient: SSH client is nil")
	}

	go proxy.HandleChannels(proxyCtx, sshClient.HandleChannelOpen(client.ChannelTypeProxy))
	go proxy.HandleChannels(proxyCtx, sshClient.HandleChannelOpen(client.ChannelTypeTCPProxy))

	// Allow proxy handlers to register.
	time.Sleep(100 * time.Millisecond)

	sess := &tunnelSession{
		t:          t,
		connector:  conn,
		ctrl:       ctrl,
		proxy:      proxy,
		assignment: assignment,
		cancelFn:   proxyCancel,
	}

	t.Cleanup(func() {
		proxyCancel()
		ctrl.Close()
		conn.Close()
	})

	return sess
}

// connectClientOnly connects a client to the server without requesting
// a tunnel. Returns the connector for manual control flow.
func connectClientOnly(t *testing.T, ts *testServer, token string) *client.Connector {
	t.Helper()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := client.ClientConfig{
		ServerAddr: ts.sshAddr,
		Token:      token,
		Insecure:   true,
	}

	conn := client.NewConnector(cfg, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Connect(ctx); err != nil {
		t.Fatalf("connectClientOnly: connect: %v", err)
	}

	t.Cleanup(func() {
		conn.Close()
	})

	return conn
}

// --- Local Service Helpers ---

// startLocalHTTPService starts a local HTTP server with the given handler
// and returns its port plus a stop function.
func startLocalHTTPService(t *testing.T, handler http.Handler) (port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startLocalHTTPService: listen: %v", err)
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

// startLocalTCPEchoServer starts a local TCP server that echoes all received
// data back to the sender. Useful for validating TCP tunnel data flow.
func startLocalTCPEchoServer(t *testing.T) (port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startLocalTCPEchoServer: listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // echo
			}(conn)
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var p int
	fmt.Sscanf(portStr, "%d", &p)

	return p, func() {
		cancel()
		ln.Close()
	}
}

// --- Tunnel Status Helpers ---

// waitForTunnel polls the edge router until the tunnel responds (HTTP 200)
// or the timeout expires.
func waitForTunnel(t *testing.T, edgeAddr, host string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", "http://"+edgeAddr+"/", nil)
		req.Host = host
		resp, err := httpClient().Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waitForTunnel: tunnel %s did not respond within %v", host, timeout)
}

// waitForTunnelGone polls the edge router until the tunnel returns 404
// or the timeout expires.
func waitForTunnelGone(t *testing.T, edgeAddr, host string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", "http://"+edgeAddr+"/", nil)
		req.Host = host
		resp, err := httpClient().Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waitForTunnelGone: tunnel %s still responding after %v", host, timeout)
}

// serverSupportsTCP probes whether the server supports TCP tunnels (T-033A).
// Returns true now that the server-side TCP edge is implemented.
func serverSupportsTCP(t *testing.T) bool {
	t.Helper()
	return true
}

// --- Admin API Helpers ---

// listTunnels queries the admin API for active tunnels.
func listTunnels(t *testing.T, ts *testServer) []tunnelInfo {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://"+ts.adminAddr+"/api/v1/tunnels", nil)
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("listTunnels: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("listTunnels: expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Tunnels []tunnelInfo `json:"tunnels"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Tunnels
}

// tunnelInfo represents a tunnel from the admin API.
type tunnelInfo struct {
	TunnelID        string `json:"tunnel_id"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	TokenID         string `json:"token_id"`
	AssignedPort    int    `json:"assigned_port,omitempty"`
	RequestsProxied int64  `json:"requests_proxied"`
	BytesIn         int64  `json:"bytes_in"`
	BytesOut        int64  `json:"bytes_out"`
}

// listTokensAPI queries the admin API for tokens.
func listTokensAPI(t *testing.T, ts *testServer) []tokenInfo {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://"+ts.adminAddr+"/api/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("listTokensAPI: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Tokens []tokenInfo `json:"tokens"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Tokens
}

// tokenInfo represents a token from the admin API.
type tokenInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// --- Utility Functions ---

// getAvailPort returns a "127.0.0.1:<port>" string for a random available port.
func getAvailPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("getAvailPort: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// waitForAddr polls a TCP address until it accepts a connection or the
// timeout expires. Used to wait for servers to start listening.
func waitForAddr(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForAddr: %s did not start listening within %v", addr, timeout)
}

// httpClient returns a reusable HTTP client with a 10s timeout.
func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// sanitizeName converts a test name to a safe token name (alphanumeric + hyphen only).
func sanitizeName(name string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	// Trim leading/trailing hyphens and collapse.
	result = strings.Trim(result, "-")
	if len(result) > 60 {
		result = result[:60]
	}
	if result == "" {
		result = "e2e-token"
	}
	return result
}
