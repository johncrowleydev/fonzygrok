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
	// net.Pipe doesn't support half-close, so just return nil.
	return nil
}

func (mc *mockChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return false, nil
}

func (mc *mockChannel) Stderr() io.ReadWriter {
	return io.Discard.(io.ReadWriter)
}
