package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"github.com/fonzygrok/fonzygrok/internal/store"
	"golang.org/x/crypto/ssh"
)

// --- Test helpers ---

func newTestEdgeRouter(t *testing.T) (*EdgeRouter, *TunnelManager, *store.Store) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tm := NewTunnelManager("tunnel.example.com", st, logger)

	config := EdgeConfig{
		Addr:         "127.0.0.1:0",
		BaseDomain:   "tunnel.example.com",
		ProxyTimeout: 2 * time.Second, // short for tests
	}

	edge := NewEdgeRouter(config, tm, logger)
	return edge, tm, st
}

// --- T-009A: Host Header Routing ---

func TestExtractSubdomain(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	tests := []struct {
		host     string
		expected string
	}{
		{"abc123.tunnel.example.com", "abc123"},
		{"abc123.tunnel.example.com:8080", "abc123"},
		{"tunnel.example.com", ""},
		{"tunnel.example.com:8080", ""},
		{"other.domain.com", ""},
		{"ABC123.TUNNEL.EXAMPLE.COM", "abc123"},
		{"nested.sub.tunnel.example.com", ""},
		{"", ""},
		{"192.168.1.1:8080", ""},
		{"localhost", ""},
		{"x.tunnel.example.com", "x"},
	}

	for _, tt := range tests {
		got := edge.extractSubdomain(tt.host)
		if got != tt.expected {
			t.Errorf("extractSubdomain(%q) = %q, want %q", tt.host, got, tt.expected)
		}
	}
}

func TestRouteToTunnel(t *testing.T) {
	edge, tm, st := newTestEdgeRouter(t)
	defer st.Close()

	// Register a tunnel.
	session := &Session{TokenID: "tok_test123456", RemoteAddr: "127.0.0.1:9999"}
	assignment, err := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Request with matching subdomain — should attempt to proxy (will fail since no SSH).
	// But it proves routing works (we get 502 not 404).
	host := assignment.TunnelID + ".tunnel.example.com"
	req := httptest.NewRequest("GET", "http://"+host+"/api/test", nil)
	req.Host = host
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	// Should get 502 (tunnel found but can't open channel — no real SSH conn).
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for reachable tunnel with no SSH, got %d", w.Code)
	}
}

func TestRouteToServerInfo(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://tunnel.example.com/", nil)
	req.Host = "tunnel.example.com"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for base domain, got %d", w.Code)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("unmarshal server info: %v", err)
	}
	if info["service"] != "fonzygrok" {
		t.Errorf("service: got %v, want fonzygrok", info["service"])
	}
}

func TestRouteUnknownSubdomain404(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://nonexistent.tunnel.example.com/", nil)
	req.Host = "nonexistent.tunnel.example.com"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["error"] != "tunnel_not_found" {
		t.Errorf("error code: got %q, want %q", errResp["error"], "tunnel_not_found")
	}
}

func TestRouteIPBasedRequest(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://192.168.1.1:8080/", nil)
	req.Host = "192.168.1.1:8080"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	// IP-based requests should return server info.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for IP-based request, got %d", w.Code)
	}
}

// --- T-010A: Proxy Through Tunnel ---

// TestProxyRoundTrip tests end-to-end proxy through real SSH.
func TestProxyRoundTrip(t *testing.T) {
	// Start a real SSH server and edge router, register a tunnel,
	// and proxy a request through.
	srv, tm, st, sshAddr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := EdgeConfig{
		Addr:         "127.0.0.1:0",
		BaseDomain:   "tunnel.test.com",
		ProxyTimeout: 5 * time.Second,
	}
	edge := NewEdgeRouter(config, tm, logger)

	// Connect client via SSH.
	client := dialTestSSH(t, sshAddr, rawToken)
	defer client.Close()

	// Open control channel and request a tunnel.
	ctrlCh, _, err := client.OpenChannel("control", nil)
	if err != nil {
		t.Fatalf("open control channel: %v", err)
	}
	defer ctrlCh.Close()

	encoder := proto.NewEncoder(ctrlCh)
	decoder := proto.NewDecoder(ctrlCh)

	reqMsg, _ := proto.WrapPayload(proto.TypeTunnelRequest, proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})
	encoder.Encode(reqMsg)

	respMsg, err := decoder.Decode()
	if err != nil {
		t.Fatalf("decode assignment: %v", err)
	}
	var assignment proto.TunnelAssignment
	json.Unmarshal(respMsg.Payload, &assignment)

	// Start a goroutine to handle proxy channel requests from the server.
	// This simulates the client-side proxy behavior.
	go func() {
		for newCh := range client.HandleChannelOpen("proxy") {
			go func(nc ssh.NewChannel) {
				ch, _, err := nc.Accept()
				if err != nil {
					return
				}
				defer ch.Close()

				// Read the HTTP request from the channel.
				req, err := http.ReadRequest(bufio.NewReader(ch))
				if err != nil {
					return
				}
				req.Body.Close()

				// Write a mock HTTP response.
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					ProtoMajor: 1,
					ProtoMinor: 1,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader(`{"message":"hello from tunnel"}`)),
				}
				resp.Header.Set("Content-Type", "application/json")
				resp.Write(ch)
			}(newCh)
		}
	}()

	// Brief wait for channel handler to be ready.
	time.Sleep(100 * time.Millisecond)

	// Send HTTP request through the edge router.
	host := assignment.TunnelID + ".tunnel.test.com"
	httpReq := httptest.NewRequest("GET", "http://"+host+"/api/test", nil)
	httpReq.Host = host
	httpReq.RemoteAddr = "203.0.113.1:43210"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "hello from tunnel") {
		t.Errorf("expected body to contain 'hello from tunnel', got: %s", body)
	}
}

// TestProxyForwardedHeaders verifies X-Forwarded-* headers are added.
func TestProxyForwardedHeaders(t *testing.T) {
	srv, tm, st, sshAddr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := EdgeConfig{
		Addr:         "127.0.0.1:0",
		BaseDomain:   "tunnel.test.com",
		ProxyTimeout: 5 * time.Second,
	}
	edge := NewEdgeRouter(config, tm, logger)

	client := dialTestSSH(t, sshAddr, rawToken)
	defer client.Close()

	ctrlCh, _, _ := client.OpenChannel("control", nil)
	defer ctrlCh.Close()

	encoder := proto.NewEncoder(ctrlCh)
	decoder := proto.NewDecoder(ctrlCh)

	reqMsg, _ := proto.WrapPayload(proto.TypeTunnelRequest, proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})
	encoder.Encode(reqMsg)
	respMsg, _ := decoder.Decode()
	var assignment proto.TunnelAssignment
	json.Unmarshal(respMsg.Payload, &assignment)

	// Capture the forwarded headers.
	receivedHeaders := make(chan http.Header, 1)
	go func() {
		for newCh := range client.HandleChannelOpen("proxy") {
			go func(nc ssh.NewChannel) {
				ch, _, err := nc.Accept()
				if err != nil {
					return
				}
				defer ch.Close()

				req, err := http.ReadRequest(bufio.NewReader(ch))
				if err != nil {
					return
				}
				receivedHeaders <- req.Header
				req.Body.Close()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					ProtoMajor: 1,
					ProtoMinor: 1,
					Header:     http.Header{"Content-Type": {"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("ok")),
				}
				resp.Write(ch)
			}(newCh)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	host := assignment.TunnelID + ".tunnel.test.com"
	httpReq := httptest.NewRequest("GET", "http://"+host+"/path", nil)
	httpReq.Host = host
	httpReq.RemoteAddr = "203.0.113.42:12345"
	httpReq.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	select {
	case hdrs := <-receivedHeaders:
		if hdrs.Get("X-Forwarded-For") != "203.0.113.42" {
			t.Errorf("X-Forwarded-For: got %q, want %q", hdrs.Get("X-Forwarded-For"), "203.0.113.42")
		}
		if hdrs.Get("X-Forwarded-Host") != host {
			t.Errorf("X-Forwarded-Host: got %q, want %q", hdrs.Get("X-Forwarded-Host"), host)
		}
		if hdrs.Get("X-Forwarded-Proto") != "http" {
			t.Errorf("X-Forwarded-Proto: got %q, want %q", hdrs.Get("X-Forwarded-Proto"), "http")
		}
		if hdrs.Get("X-Fonzygrok-Tunnel-Id") != assignment.TunnelID {
			t.Errorf("X-Fonzygrok-Tunnel-Id: got %q, want %q", hdrs.Get("X-Fonzygrok-Tunnel-Id"), assignment.TunnelID)
		}
		// Original headers must be preserved.
		if hdrs.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization header not preserved: got %q", hdrs.Get("Authorization"))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for forwarded headers")
	}
}

// --- T-011A: Error Responses ---

func TestError404TunnelNotFound(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://missing.tunnel.example.com/", nil)
	req.Host = "missing.tunnel.example.com"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	assertErrorResponse(t, w, http.StatusNotFound, "tunnel_not_found", "No tunnel matches this hostname")
}

func TestError502TunnelOffline(t *testing.T) {
	edge, tm, st := newTestEdgeRouter(t)
	defer st.Close()

	// Register a tunnel with no real SSH connection — proxy channel will fail.
	session := &Session{TokenID: "tok_test123456", RemoteAddr: "127.0.0.1:9999"}
	assignment, _ := tm.Register(session, &proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})

	host := assignment.TunnelID + ".tunnel.example.com"
	req := httptest.NewRequest("GET", "http://"+host+"/", nil)
	req.Host = host
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	// Should be 502 since no SSH conn to open proxy channel.
	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
	assertHasErrorHeaders(t, w)
}

func TestError504ProxyTimeout(t *testing.T) {
	srv, tm, st, sshAddr, rawToken := startTestServerWithTunnels(t)
	defer srv.Stop()
	defer st.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := EdgeConfig{
		Addr:         "127.0.0.1:0",
		BaseDomain:   "tunnel.test.com",
		ProxyTimeout: 200 * time.Millisecond, // very short for test
	}
	edge := NewEdgeRouter(config, tm, logger)

	client := dialTestSSH(t, sshAddr, rawToken)
	defer client.Close()

	ctrlCh, _, _ := client.OpenChannel("control", nil)
	defer ctrlCh.Close()

	encoder := proto.NewEncoder(ctrlCh)
	decoder := proto.NewDecoder(ctrlCh)

	reqMsg, _ := proto.WrapPayload(proto.TypeTunnelRequest, proto.TunnelRequest{
		LocalPort: 3000,
		Protocol:  "http",
	})
	encoder.Encode(reqMsg)
	respMsg, _ := decoder.Decode()
	var assignment proto.TunnelAssignment
	json.Unmarshal(respMsg.Payload, &assignment)

	// Accept proxy channels but never respond (causes timeout).
	go func() {
		for newCh := range client.HandleChannelOpen("proxy") {
			go func(nc ssh.NewChannel) {
				ch, _, err := nc.Accept()
				if err != nil {
					return
				}
				// Read request but don't respond — let it timeout.
				io.ReadAll(ch)
				// Hold the channel open, never write a response.
				time.Sleep(5 * time.Second)
				ch.Close()
			}(newCh)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	host := assignment.TunnelID + ".tunnel.test.com"
	httpReq := httptest.NewRequest("GET", "http://"+host+"/slow", nil)
	httpReq.Host = host
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, httpReq)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got %d, body: %s", w.Code, w.Body.String())
	}
	assertHasErrorHeaders(t, w)
}

func TestErrorResponseFormat(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://test.tunnel.example.com/", nil)
	req.Host = "test.tunnel.example.com"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	// Verify Content-Type and X-Fonzygrok-Error headers.
	assertHasErrorHeaders(t, w)

	// Verify JSON body structure.
	var errResp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if _, ok := errResp["error"]; !ok {
		t.Error("error response missing 'error' field")
	}
	if _, ok := errResp["message"]; !ok {
		t.Error("error response missing 'message' field")
	}
}

// --- T-012A: Server Info Endpoint ---

func TestServerInfo(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "http://tunnel.example.com/", nil)
	req.Host = "tunnel.example.com"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}

	var info struct {
		Service       string `json:"service"`
		Version       string `json:"version"`
		Status        string `json:"status"`
		TunnelsActive int    `json:"tunnels_active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Service != "fonzygrok" {
		t.Errorf("service: got %q, want %q", info.Service, "fonzygrok")
	}
	if info.Status != "running" {
		t.Errorf("status: got %q, want %q", info.Status, "running")
	}
	if info.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestServerInfoTunnelCount(t *testing.T) {
	edge, tm, st := newTestEdgeRouter(t)
	defer st.Close()

	// Initially 0 tunnels.
	req := httptest.NewRequest("GET", "http://tunnel.example.com/", nil)
	req.Host = "tunnel.example.com"
	w := httptest.NewRecorder()
	edge.Handler().ServeHTTP(w, req)

	var info1 struct {
		TunnelsActive int `json:"tunnels_active"`
	}
	json.Unmarshal(w.Body.Bytes(), &info1)
	if info1.TunnelsActive != 0 {
		t.Errorf("expected 0 tunnels, got %d", info1.TunnelsActive)
	}

	// Add tunnels.
	session := &Session{TokenID: "tok_test123456"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http"})

	w2 := httptest.NewRecorder()
	edge.Handler().ServeHTTP(w2, req)

	var info2 struct {
		TunnelsActive int `json:"tunnels_active"`
	}
	json.Unmarshal(w2.Body.Bytes(), &info2)
	if info2.TunnelsActive != 2 {
		t.Errorf("expected 2 tunnels, got %d", info2.TunnelsActive)
	}
}

func TestServerInfoWithPort(t *testing.T) {
	edge, _, st := newTestEdgeRouter(t)
	defer st.Close()

	// Base domain with port should still return server info.
	req := httptest.NewRequest("GET", "http://tunnel.example.com:8080/", nil)
	req.Host = "tunnel.example.com:8080"
	w := httptest.NewRecorder()

	edge.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for base domain with port, got %d", w.Code)
	}
}

// --- Assertion helpers ---

func assertErrorResponse(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedCode, expectedMessage string) {
	t.Helper()

	if w.Code != expectedStatus {
		t.Errorf("status: got %d, want %d", w.Code, expectedStatus)
	}

	assertHasErrorHeaders(t, w)

	var errResp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp["error"] != expectedCode {
		t.Errorf("error code: got %q, want %q", errResp["error"], expectedCode)
	}
	if errResp["message"] != expectedMessage {
		t.Errorf("message: got %q, want %q", errResp["message"], expectedMessage)
	}
}

func assertHasErrorHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
	if fe := w.Header().Get("X-Fonzygrok-Error"); fe != "true" {
		t.Errorf("X-Fonzygrok-Error: got %q, want %q", fe, "true")
	}
}

// Suppress unused import warnings for packages used in test helpers.
var (
	_ = net.SplitHostPort
	_ = fmt.Sprintf
)
