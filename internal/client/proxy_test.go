package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestProxyRoundTrip verifies end-to-end data flow through the proxy:
// server opens a proxy channel → proxy dials localhost → bidirectional copy.
func TestProxyRoundTrip(t *testing.T) {
	// Start a local HTTP server that will be proxied to.
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello from local")
	}))
	defer localSrv.Close()

	// Extract the port from the test server.
	_, portStr, _ := net.SplitHostPort(localSrv.Listener.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	// Start test SSH server.
	addr, cleanup := startTestSSHServer(t, testToken)
	defer cleanup()

	// Connect client.
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

	// Create a proxy for the local port.
	proxy := NewLocalProxy(localPort, slog.Default())

	// Simulate the server opening a proxy channel to the client.
	// We use the client's SSH connection to open a channel to itself
	// via the test server (which echoes, but for this test we need
	// to talk to the local proxy). Instead, we'll construct a pipe-based
	// test that exercises handleSingleChannel directly.

	// Create a pair of net.Conn to simulate SSH channel behavior.
	serverConn, clientConn := net.Pipe()

	// Wrap clientConn as a mock SSH NewChannel that the proxy will accept.
	mockCh := &mockNewChannel{
		chType: ChannelTypeProxy,
		conn:   clientConn,
	}

	// Run the proxy handler in a goroutine.
	go proxy.handleSingleChannel(mockCh)

	// From the "server" side, write an HTTP request.
	httpReq := "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"
	_, err := serverConn.Write([]byte(httpReq))
	if err != nil {
		t.Fatalf("write request: %v", err)
	}
	// Close the write side to signal end of request.
	if tc, ok := serverConn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	// For net.Pipe, we close after a short wait to let proxy read.
	// Read the response.
	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	io.Copy(&buf, serverConn)

	resp := buf.String()
	if !strings.Contains(resp, "hello from local") {
		t.Errorf("response should contain 'hello from local', got: %q", resp)
	}
}

// TestProxy502OnUnreachablePort verifies that a 502 is returned when
// the local service is unreachable.
func TestProxy502OnUnreachablePort(t *testing.T) {
	// Find a port that is definitely not listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	var deadPort int
	fmt.Sscanf(portStr, "%d", &deadPort)
	listener.Close() // close it so the port is unreachable

	proxy := NewLocalProxy(deadPort, slog.Default())

	serverConn, clientConn := net.Pipe()
	mockCh := &mockNewChannel{
		chType: ChannelTypeProxy,
		conn:   clientConn,
	}

	// Run the proxy in a goroutine.
	done := make(chan struct{})
	go func() {
		proxy.handleSingleChannel(mockCh)
		close(done)
	}()

	// Read the response — should be a 502.
	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	io.Copy(&buf, serverConn)

	<-done

	resp := buf.String()
	if !strings.Contains(resp, "502 Bad Gateway") {
		t.Errorf("expected 502 Bad Gateway, got: %q", resp)
	}
	if !strings.Contains(resp, "X-Fonzygrok-Error: true") {
		t.Errorf("expected X-Fonzygrok-Error header, got: %q", resp)
	}
}

// TestProxyConcurrentChannels verifies that multiple proxy channels can
// be handled concurrently.
func TestProxyConcurrentChannels(t *testing.T) {
	// Start a local HTTP server.
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "concurrent ok")
	}))
	defer localSrv.Close()

	_, portStr, _ := net.SplitHostPort(localSrv.Listener.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	proxy := NewLocalProxy(localPort, slog.Default())

	const numChannels = 5
	var wg sync.WaitGroup
	wg.Add(numChannels)

	for i := 0; i < numChannels; i++ {
		go func(idx int) {
			defer wg.Done()

			serverConn, clientConn := net.Pipe()
			mockCh := &mockNewChannel{
				chType: ChannelTypeProxy,
				conn:   clientConn,
			}

			go proxy.handleSingleChannel(mockCh)

			httpReq := fmt.Sprintf("GET /%d HTTP/1.1\r\nHost: localhost\r\n\r\n", idx)
			serverConn.Write([]byte(httpReq))

			var buf bytes.Buffer
			serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
			io.Copy(&buf, serverConn)

			resp := buf.String()
			if !strings.Contains(resp, "concurrent ok") {
				t.Errorf("channel %d: expected 'concurrent ok', got: %q", idx, resp)
			}
		}(i)
	}

	wg.Wait()
}

// TestHandleChannelsContextCancel verifies that HandleChannels exits
// when the context is cancelled.
func TestHandleChannelsContextCancel(t *testing.T) {
	proxy := NewLocalProxy(9999, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	chChan := make(chan ssh.NewChannel)
	done := make(chan struct{})

	go func() {
		proxy.HandleChannels(ctx, chChan)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("HandleChannels did not exit on context cancel")
	}
}

// TestHandleChannelsClosedChan verifies HandleChannels exits when
// the channel source is closed.
func TestHandleChannelsClosedChan(t *testing.T) {
	proxy := NewLocalProxy(9999, slog.Default())
	ctx := context.Background()

	chChan := make(chan ssh.NewChannel)
	done := make(chan struct{})

	go func() {
		proxy.HandleChannels(ctx, chChan)
		close(done)
	}()

	close(chChan)

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("HandleChannels did not exit on closed channel")
	}
}

// mockNewChannel implements ssh.NewChannel for testing the proxy.
// It uses a net.Conn to simulate the SSH channel data stream.
type mockNewChannel struct {
	chType string
	conn   net.Conn
}

func (m *mockNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	reqs := make(chan *ssh.Request)
	close(reqs) // no requests expected
	return &mockChannel{conn: m.conn}, reqs, nil
}

func (m *mockNewChannel) Reject(reason ssh.RejectionReason, message string) error {
	return nil
}

func (m *mockNewChannel) ChannelType() string {
	return m.chType
}

func (m *mockNewChannel) ExtraData() []byte {
	return nil
}

// mockChannel implements ssh.Channel using a net.Conn.
type mockChannel struct {
	conn net.Conn
}

func (mc *mockChannel) Read(data []byte) (int, error) {
	return mc.conn.Read(data)
}

func (mc *mockChannel) Write(data []byte) (int, error) {
	return mc.conn.Write(data)
}

func (mc *mockChannel) Close() error {
	return mc.conn.Close()
}

func (mc *mockChannel) CloseWrite() error {
	// Support half-close for TCP connections (needed for slow service tests).
	if tc, ok := mc.conn.(*net.TCPConn); ok {
		return tc.CloseWrite()
	}
	// net.Pipe doesn't support half-close — return nil.
	return nil
}

func (mc *mockChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return false, nil
}

func (mc *mockChannel) Stderr() io.ReadWriter {
	return io.Discard.(io.ReadWriter)
}

// TestProxySlowLocalService verifies the proxy handles a deliberately slow
// local service (~500ms response delay) without timing-dependent failures.
// Per DEF-003 investigation: surfaces latency-dependent bugs in the
// bidirectional copy pipeline.
// Refs: DEF-003, SPR-015B T-049
func TestProxySlowLocalService(t *testing.T) {
	// Start a local HTTP server that sleeps before responding.
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("X-Slow", "true")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "slow response body")
	}))
	defer localSrv.Close()

	_, portStr, _ := net.SplitHostPort(localSrv.Listener.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	proxy := NewLocalProxy(localPort, slog.Default())

	// Use TCP pair for proper half-close support.
	serverConn, clientConn := tcpPipe(t)
	defer serverConn.Close()

	mockCh := &mockNewChannel{
		chType: ChannelTypeProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleSingleChannel(mockCh)
		close(done)
	}()

	// Write request then half-close write side (like server's CloseWrite).
	httpReq := "GET /slow HTTP/1.1\r\nHost: localhost\r\n\r\n"
	_, err := serverConn.Write([]byte(httpReq))
	if err != nil {
		t.Fatalf("write request: %v", err)
	}
	serverConn.(*net.TCPConn).CloseWrite()

	// Read the response — must arrive despite 500ms delay.
	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	io.Copy(&buf, serverConn)

	<-done

	resp := buf.String()
	if !strings.Contains(resp, "200 OK") {
		t.Errorf("expected 200 OK, got: %q", resp)
	}
	if !strings.Contains(resp, "slow response body") {
		t.Errorf("expected 'slow response body', got: %q", resp)
	}
	if !strings.Contains(resp, "X-Slow: true") {
		t.Errorf("expected X-Slow header, got: %q", resp)
	}
}

// TestProxyWithInspectorSlowService verifies that the TeeReader wrapping
// for inspector capture does NOT interfere with the proxy pipeline when
// the local service is slow.
// Refs: DEF-003, SPR-015B T-049
func TestProxyWithInspectorSlowService(t *testing.T) {
	// Slow HTTP server (200ms).
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "inspector-proxied response")
	}))
	defer localSrv.Close()

	_, portStr, _ := net.SplitHostPort(localSrv.Listener.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	// Create proxy WITH inspector attached.
	inspector := NewInspector("127.0.0.1:0", slog.Default())
	proxy := NewLocalProxy(localPort, slog.Default())
	proxy.Inspector = inspector

	serverConn, clientConn := tcpPipe(t)
	defer serverConn.Close()

	mockCh := &mockNewChannel{
		chType: ChannelTypeProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleSingleChannel(mockCh)
		close(done)
	}()

	httpReq := "GET /test-inspect HTTP/1.1\r\nHost: localhost\r\nX-Test: inspector\r\n\r\n"
	serverConn.Write([]byte(httpReq))
	serverConn.(*net.TCPConn).CloseWrite()

	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	io.Copy(&buf, serverConn)

	<-done

	resp := buf.String()
	if !strings.Contains(resp, "inspector-proxied response") {
		t.Errorf("expected 'inspector-proxied response', got: %q", resp)
	}

	// Verify inspector captured the request.
	entries := inspector.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 inspector entry, got %d", len(entries))
	}
	if entries[0].Method != "GET" {
		t.Errorf("inspector entry Method = %q, want %q", entries[0].Method, "GET")
	}
	if entries[0].Path != "/test-inspect" {
		t.Errorf("inspector entry Path = %q, want %q", entries[0].Path, "/test-inspect")
	}
	if entries[0].StatusCode != 200 {
		t.Errorf("inspector entry StatusCode = %d, want 200", entries[0].StatusCode)
	}
}

// TestProxyLargeResponseSlowService verifies proxy with a large response body
// and artificial latency — catches buffering issues over real networks.
// Refs: DEF-003, SPR-015B T-049
func TestProxyLargeResponseSlowService(t *testing.T) {
	// 64KB response body with 100ms delay.
	largeBody := strings.Repeat("X", 64*1024)
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, largeBody)
	}))
	defer localSrv.Close()

	_, portStr, _ := net.SplitHostPort(localSrv.Listener.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	proxy := NewLocalProxy(localPort, slog.Default())

	serverConn, clientConn := tcpPipe(t)
	defer serverConn.Close()

	mockCh := &mockNewChannel{
		chType: ChannelTypeProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleSingleChannel(mockCh)
		close(done)
	}()

	httpReq := "GET /large HTTP/1.1\r\nHost: localhost\r\n\r\n"
	serverConn.Write([]byte(httpReq))
	serverConn.(*net.TCPConn).CloseWrite()

	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	io.Copy(&buf, serverConn)

	<-done

	resp := buf.String()
	if !strings.Contains(resp, "200 OK") {
		t.Errorf("expected 200 OK in response")
	}
	// The full 64KB body should be in the response.
	if !strings.Contains(resp, largeBody) {
		t.Errorf("response should contain full 64KB body, got %d bytes total", len(resp))
	}
}

// tcpPipe creates a pair of connected TCP connections that support proper
// half-close (CloseWrite). net.Pipe does NOT support half-close, which makes
// it unsuitable for testing bidirectional proxy copy with EOF signaling.
func tcpPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcpPipe: listen: %v", err)
	}
	defer ln.Close()

	var serverConn net.Conn
	accepted := make(chan struct{})
	go func() {
		var err error
		serverConn, err = ln.Accept()
		if err != nil {
			t.Errorf("tcpPipe: accept: %v", err)
		}
		close(accepted)
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("tcpPipe: dial: %v", err)
	}

	<-accepted
	return serverConn, clientConn
}

// TestTCPProxyRoundTrip verifies end-to-end TCP data flow through the tcp-proxy
// channel: server opens a tcp-proxy channel → proxy dials localhost → bidirectional copy.
func TestTCPProxyRoundTrip(t *testing.T) {
	// Start a local TCP server that echoes back data.
	localLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer localLn.Close()

	go func() {
		for {
			conn, err := localLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // echo
			}(conn)
		}
	}()

	_, portStr, _ := net.SplitHostPort(localLn.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	proxy := NewLocalProxy(localPort, slog.Default())

	serverConn, clientConn := net.Pipe()
	mockCh := &mockNewChannel{
		chType: ChannelTypeTCPProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleTCPChannel(mockCh)
		close(done)
	}()

	// Write some data through and read it back (echo).
	testData := "hello TCP tunnel"
	_, err = serverConn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(testData))
	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("read: %v (got %d bytes: %q)", err, n, buf[:n])
	}

	if string(buf) != testData {
		t.Errorf("echoed data = %q, want %q", buf, testData)
	}

	serverConn.Close()
	<-done
}

// TestTCPProxyNoHTTP502OnDialFailure verifies that when the local TCP service
// is unreachable, the tcp-proxy channel is simply closed — no HTTP 502 is written.
func TestTCPProxyNoHTTP502OnDialFailure(t *testing.T) {
	// Find a port that's not listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	var deadPort int
	fmt.Sscanf(portStr, "%d", &deadPort)
	listener.Close()

	proxy := NewLocalProxy(deadPort, slog.Default())

	serverConn, clientConn := net.Pipe()
	mockCh := &mockNewChannel{
		chType: ChannelTypeTCPProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleTCPChannel(mockCh)
		close(done)
	}()

	// Read whatever comes back — should be nothing (channel closed, no HTTP 502).
	var buf bytes.Buffer
	serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.Copy(&buf, serverConn)

	<-done

	resp := buf.String()
	if strings.Contains(resp, "502") {
		t.Errorf("TCP proxy should NOT write HTTP 502, got: %q", resp)
	}
	if strings.Contains(resp, "HTTP/") {
		t.Errorf("TCP proxy should NOT write any HTTP response, got: %q", resp)
	}
	if resp != "" {
		t.Errorf("TCP proxy on dial failure should write nothing, got: %q", resp)
	}
}

// TestTCPProxyInspectorRecording verifies that TCP proxy records entries
// to the inspector with Protocol="tcp" and byte counts.
func TestTCPProxyInspectorRecording(t *testing.T) {
	// Start a local TCP echo server.
	localLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer localLn.Close()

	go func() {
		conn, err := localLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	_, portStr, _ := net.SplitHostPort(localLn.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	inspector := NewInspector("127.0.0.1:0", slog.Default())
	proxy := NewLocalProxy(localPort, slog.Default())
	proxy.Inspector = inspector

	serverConn, clientConn := net.Pipe()
	mockCh := &mockNewChannel{
		chType: ChannelTypeTCPProxy,
		conn:   clientConn,
	}

	done := make(chan struct{})
	go func() {
		proxy.handleTCPChannel(mockCh)
		close(done)
	}()

	testData := "inspector test data"
	serverConn.Write([]byte(testData))

	// Read echo back.
	buf := make([]byte, len(testData))
	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	io.ReadFull(serverConn, buf)

	// Close to let handler complete.
	serverConn.Close()
	<-done

	entries := inspector.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 inspector entry, got %d", len(entries))
	}
	if entries[0].Protocol != "tcp" {
		t.Errorf("entry Protocol = %q, want %q", entries[0].Protocol, "tcp")
	}
	if entries[0].Method != "" {
		t.Errorf("TCP entry should have empty Method, got %q", entries[0].Method)
	}
	if entries[0].RequestSize <= 0 {
		t.Errorf("TCP entry RequestSize should be > 0, got %d", entries[0].RequestSize)
	}
}

// TestHandleChannelsDispatchesTCPProxy verifies that HandleChannels correctly
// dispatches tcp-proxy channels to handleTCPChannel (not the HTTP handler).
func TestHandleChannelsDispatchesTCPProxy(t *testing.T) {
	// Start a local TCP echo server.
	localLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer localLn.Close()

	go func() {
		conn, err := localLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	_, portStr, _ := net.SplitHostPort(localLn.Addr().String())
	var localPort int
	fmt.Sscanf(portStr, "%d", &localPort)

	proxy := NewLocalProxy(localPort, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chChan := make(chan ssh.NewChannel, 1)

	go proxy.HandleChannels(ctx, chChan)

	// Send a tcp-proxy channel.
	serverConn, clientConn := net.Pipe()
	chChan <- &mockNewChannel{
		chType: ChannelTypeTCPProxy,
		conn:   clientConn,
	}

	// Write data and read echo.
	testData := "dispatch test"
	serverConn.Write([]byte(testData))

	buf := make([]byte, len(testData))
	serverConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("read: %v (got %d bytes)", err, n)
	}

	if string(buf) != testData {
		t.Errorf("echoed = %q, want %q", buf, testData)
	}

	serverConn.Close()
	cancel()
}
