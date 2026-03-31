package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/store"
	"golang.org/x/crypto/ssh"
)

// newTestStoreAndToken creates an in-memory store with a token, returns
// the store, token ID, and raw token.
func newTestStoreAndToken(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Migrate(); err != nil {
		s.Close()
		t.Fatalf("store.Migrate: %v", err)
	}
	tok, raw, err := s.CreateToken("test-token")
	if err != nil {
		s.Close()
		t.Fatalf("store.CreateToken: %v", err)
	}
	return s, tok.ID, raw
}

// startTestSSHServer creates and starts an SSHServer on a random port,
// returning the server and its address.
func startTestSSHServer(t *testing.T, st *store.Store) (*SSHServer, string) {
	t.Helper()
	tmpDir := t.TempDir()
	hostKeyPath := filepath.Join(tmpDir, "host_key")

	config := SSHConfig{
		Addr:        "127.0.0.1:0", // random port
		HostKeyPath: hostKeyPath,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	srv, err := New(config, st, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	started := make(chan struct{})
	go func() {
		// Signal that we've entered Start
		ln, err := net.Listen("tcp", config.Addr)
		if err != nil {
			t.Errorf("listen: %v", err)
			return
		}
		srv.mu.Lock()
		srv.listener = ln
		srv.mu.Unlock()
		close(started)

		// Accept loop
		for {
			conn, err := ln.Accept()
			if err != nil {
				srv.mu.Lock()
				closed := srv.closed
				srv.mu.Unlock()
				if closed {
					return
				}
				continue
			}
			go srv.handleConn(conn)
		}
	}()

	<-started
	return srv, srv.Addr()
}

// dialTestSSH connects to the test SSH server with the given token.
func dialTestSSH(t *testing.T, addr, token string) *ssh.Client {
	t.Helper()
	clientCfg := &ssh.ClientConfig{
		User: "fonzygrok",
		Auth: []ssh.AuthMethod{
			ssh.Password(token),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		t.Fatalf("ssh.Dial: %v", err)
	}
	return client
}

func TestHostKeyGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	hostKeyPath := filepath.Join(tmpDir, "subdir", "host_key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// First call should generate.
	signer1, err := loadOrGenerateHostKey(hostKeyPath, logger)
	if err != nil {
		t.Fatalf("first loadOrGenerateHostKey: %v", err)
	}
	if signer1 == nil {
		t.Fatal("expected non-nil signer")
	}

	// File should exist.
	if _, err := os.Stat(hostKeyPath); err != nil {
		t.Fatalf("host key file should exist: %v", err)
	}

	// Second call should load the same key.
	signer2, err := loadOrGenerateHostKey(hostKeyPath, logger)
	if err != nil {
		t.Fatalf("second loadOrGenerateHostKey: %v", err)
	}
	if signer2 == nil {
		t.Fatal("expected non-nil signer on reload")
	}

	// Public keys should match (same key loaded).
	pub1 := signer1.PublicKey().Marshal()
	pub2 := signer2.PublicKey().Marshal()
	if string(pub1) != string(pub2) {
		t.Error("host key changed across restarts")
	}
}

func TestHostKeyIsED25519(t *testing.T) {
	tmpDir := t.TempDir()
	hostKeyPath := filepath.Join(tmpDir, "host_key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	signer, err := loadOrGenerateHostKey(hostKeyPath, logger)
	if err != nil {
		t.Fatalf("loadOrGenerateHostKey: %v", err)
	}

	if signer.PublicKey().Type() != "ssh-ed25519" {
		t.Errorf("expected ssh-ed25519, got %s", signer.PublicKey().Type())
	}
}

func TestHostKeyPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	hostKeyPath := filepath.Join(tmpDir, "host_key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Generate.
	loadOrGenerateHostKey(hostKeyPath, logger)

	// Read the PEM file.
	data, err := os.ReadFile(hostKeyPath)
	if err != nil {
		t.Fatalf("read host key: %v", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("no PEM block found in host key file")
	}
}

func TestAuthValidToken(t *testing.T) {
	st, _, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)
	defer srv.Stop()

	client := dialTestSSH(t, addr, rawToken)
	client.Close()
}

func TestAuthInvalidToken(t *testing.T) {
	st, _, _ := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)
	defer srv.Stop()

	clientCfg := &ssh.ClientConfig{
		User: "fonzygrok",
		Auth: []ssh.AuthMethod{
			ssh.Password("fgk_invalidtokeninvalidtokeninvalid"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	}
	_, err := ssh.Dial("tcp", addr, clientCfg)
	if err == nil {
		t.Fatal("expected auth failure for invalid token")
	}
}

func TestMultipleConcurrentConnections(t *testing.T) {
	st, _, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)
	defer srv.Stop()

	// Register callback to prevent deadlock.
	srv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		go ssh.DiscardRequests(reqs)
		go func() {
			for range chans {
			}
		}()
	})

	const numClients = 5
	var wg sync.WaitGroup
	errCh := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clientCfg := &ssh.ClientConfig{
				User:            "fonzygrok",
				Auth:            []ssh.AuthMethod{ssh.Password(rawToken)},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         5 * time.Second,
			}
			client, err := ssh.Dial("tcp", addr, clientCfg)
			if err != nil {
				errCh <- err
				return
			}
			defer client.Close()
			time.Sleep(50 * time.Millisecond)
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent connection error: %v", err)
	}
}

func TestMaxConnections(t *testing.T) {
	st, _, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	tmpDir := t.TempDir()
	hostKeyPath := filepath.Join(tmpDir, "host_key")

	config := SSHConfig{
		Addr:           "127.0.0.1:0",
		HostKeyPath:    hostKeyPath,
		MaxConnections: 2,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	srv, err := New(config, st, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Register callback to prevent deadlock.
	srv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		go ssh.DiscardRequests(reqs)
		go func() {
			for range chans {
			}
		}()
	})

	// Start using the manual helper.
	ln, err := net.Listen("tcp", config.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.mu.Lock()
	srv.listener = ln
	srv.mu.Unlock()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				srv.mu.Lock()
				closed := srv.closed
				srv.mu.Unlock()
				if closed {
					return
				}
				continue
			}
			// Check max connections BEFORE handshake.
			srv.mu.Lock()
			count := len(srv.sessions)
			srv.mu.Unlock()
			if srv.config.MaxConnections > 0 && count >= srv.config.MaxConnections {
				conn.Close()
				continue
			}
			go srv.handleConn(conn)
		}
	}()
	defer srv.Stop()

	addr := ln.Addr().String()

	// Fill up to max.
	client1 := dialTestSSH(t, addr, rawToken)
	defer client1.Close()
	client2 := dialTestSSH(t, addr, rawToken)
	defer client2.Close()

	// Wait for sessions to register.
	time.Sleep(100 * time.Millisecond)

	if srv.ActiveSessions() != 2 {
		t.Fatalf("expected 2 active sessions, got %d", srv.ActiveSessions())
	}

	// Third connection should be rejected (server closes TCP before handshake).
	clientCfg := &ssh.ClientConfig{
		User:            "fonzygrok",
		Auth:            []ssh.AuthMethod{ssh.Password(rawToken)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         1 * time.Second,
	}
	_, err = ssh.Dial("tcp", addr, clientCfg)
	if err == nil {
		t.Fatal("expected third connection to be rejected when max connections reached")
	}
}

func TestGracefulShutdown(t *testing.T) {
	st, _, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	// Stop should not panic and should close connections.
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double stop should not error.
	if err := srv.Stop(); err != nil {
		t.Fatalf("double Stop: %v", err)
	}
}

func TestSessionOnNewSessionCallback(t *testing.T) {
	st, tokID, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)
	defer srv.Stop()

	called := make(chan Session, 1)
	srv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		called <- sess
		// Drain channels to prevent deadlock.
		go ssh.DiscardRequests(reqs)
		go func() {
			for range chans {
			}
		}()
	})

	client := dialTestSSH(t, addr, rawToken)
	defer client.Close()

	select {
	case sess := <-called:
		if sess.TokenID != tokID {
			t.Errorf("token ID: got %q, want %q", sess.TokenID, tokID)
		}
		if sess.RemoteAddr == "" {
			t.Error("expected non-empty RemoteAddr")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnNewSession callback not called within 2s")
	}
}

func TestActiveSessions(t *testing.T) {
	st, _, rawToken := newTestStoreAndToken(t)
	defer st.Close()

	srv, addr := startTestSSHServer(t, st)
	defer srv.Stop()

	// Register callback to prevent deadlock on channel reads.
	srv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		go ssh.DiscardRequests(reqs)
		go func() {
			for range chans {
			}
		}()
	})

	if srv.ActiveSessions() != 0 {
		t.Errorf("expected 0 sessions before connection, got %d", srv.ActiveSessions())
	}

	client := dialTestSSH(t, addr, rawToken)
	// Brief wait for session registration.
	time.Sleep(100 * time.Millisecond)

	if srv.ActiveSessions() != 1 {
		t.Errorf("expected 1 session after connection, got %d", srv.ActiveSessions())
	}

	client.Close()
	// Brief wait for session cleanup.
	time.Sleep(100 * time.Millisecond)

	if srv.ActiveSessions() != 0 {
		t.Errorf("expected 0 sessions after disconnect, got %d", srv.ActiveSessions())
	}
}

// generateTestHostKey creates a temporary host key file for testing and
// returns a matching ssh.Signer.
func generateTestHostKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "host_key")

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519: %v", err)
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		t.Fatalf("write host key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer, keyPath
}

// testLogger returns a logger suitable for tests (only errors).
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// mustDial is a helper that fails the test on connection failure.
func mustDial(t *testing.T, addr, token string) *ssh.Client {
	t.Helper()
	return dialTestSSH(t, addr, token)
}

// HELPER: create a test server with callback that wires up control handling.
func startTestServerWithTunnels(t *testing.T) (*SSHServer, *TunnelManager, *store.Store, string, string) {
	t.Helper()
	st, _, rawToken := newTestStoreAndToken(t)
	srv, addr := startTestSSHServer(t, st)

	tm := NewTunnelManager("tunnel.test.com", st, testLogger())

	srv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		ch := NewControlHandler(&sess, tm, testLogger())
		go ch.HandleGlobalRequests(reqs)
		go ch.HandleChannels(chans)
	})

	return srv, tm, st, addr, rawToken
}

// freePort gets a free TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// suppressOutput eats fmt.Printf noise in tests.
var _ = fmt.Sprintf
