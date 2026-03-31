// Package server implements the fonzygrok tunnel server subsystems:
// SSH listener, control channel handler, tunnel manager, HTTP edge,
// and admin API.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/fonzygrok/fonzygrok/internal/store"
	"golang.org/x/crypto/ssh"
)

// SSHConfig holds configuration for the SSH server.
type SSHConfig struct {
	// Addr is the listen address (e.g., ":2222").
	Addr string
	// HostKeyPath is the file path for the ED25519 host key.
	// If the file does not exist, a new key is generated and saved.
	HostKeyPath string
	// MaxConnections is the maximum number of concurrent SSH connections.
	// Zero means unlimited.
	MaxConnections int
}

// Session represents an authenticated SSH client session.
// It wraps the underlying SSH connection and carries auth metadata.
type Session struct {
	// Conn is the underlying SSH server connection.
	Conn *ssh.ServerConn
	// TokenID is the ID of the token used to authenticate.
	TokenID string
	// RemoteAddr is the remote address of the client.
	RemoteAddr string
}

// SSHServer listens for incoming SSH connections and authenticates
// clients using token-based password auth per CON-001 §3.
type SSHServer struct {
	config   SSHConfig
	store    *store.Store
	logger   *slog.Logger
	sshCfg   *ssh.ServerConfig
	listener net.Listener

	mu        sync.Mutex
	sessions  map[*ssh.ServerConn]*Session
	onSession func(Session, <-chan ssh.NewChannel, <-chan *ssh.Request)
	closed    bool
}

// New creates a new SSHServer with the given configuration.
// It loads or generates the host key and configures SSH auth callbacks.
func New(config SSHConfig, st *store.Store, logger *slog.Logger) (*SSHServer, error) {
	signer, err := loadOrGenerateHostKey(config.HostKeyPath, logger)
	if err != nil {
		return nil, fmt.Errorf("ssh: host key: %w", err)
	}

	s := &SSHServer{
		config:   config,
		store:    st,
		logger:   logger,
		sessions: make(map[*ssh.ServerConn]*Session),
	}

	sshCfg := &ssh.ServerConfig{
		PasswordCallback: s.passwordCallback,
		ServerVersion:    "SSH-2.0-fonzygrok",
	}
	sshCfg.AddHostKey(signer)
	s.sshCfg = sshCfg

	return s, nil
}

// OnNewSession registers a callback invoked for each new authenticated
// SSH session. The callback receives the Session, a channel for new channel
// requests, and a channel for global requests. This must be called before Start.
func (s *SSHServer) OnNewSession(fn func(Session, <-chan ssh.NewChannel, <-chan *ssh.Request)) {
	s.onSession = fn
}

// Start begins listening for SSH connections on the configured address.
// It blocks until the context is canceled or an error occurs.
func (s *SSHServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("ssh: listen %s: %w", s.config.Addr, err)
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info("ssh server listening", "addr", s.config.Addr)

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			s.logger.Error("ssh: accept connection", "error", err)
			continue
		}

		if s.config.MaxConnections > 0 {
			s.mu.Lock()
			count := len(s.sessions)
			s.mu.Unlock()
			if count >= s.config.MaxConnections {
				s.logger.Warn("ssh: max connections reached, rejecting", "remote_addr", conn.RemoteAddr())
				conn.Close()
				continue
			}
		}

		go s.handleConn(conn)
	}
}

// Stop gracefully shuts down the SSH server and closes all sessions.
func (s *SSHServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	var errs []error
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			errs = append(errs, fmt.Errorf("ssh: close listener: %w", err))
		}
	}
	for conn, sess := range s.sessions {
		s.logger.Info("ssh: closing session", "token_id", sess.TokenID, "remote_addr", sess.RemoteAddr)
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Addr returns the listener address, or empty string if not listening.
func (s *SSHServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// ActiveSessions returns the number of currently connected sessions.
func (s *SSHServer) ActiveSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// passwordCallback validates the SSH password (which carries the API token)
// against the store. Per CON-001 §3.2 the username is fixed "fonzygrok".
func (s *SSHServer) passwordCallback(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	rawToken := string(password)
	token, err := s.store.ValidateToken(rawToken)
	if err != nil {
		s.logger.Warn("ssh: auth failure",
			"remote_addr", conn.RemoteAddr(),
			"user", conn.User(),
			"reason", err.Error(),
		)
		return nil, fmt.Errorf("auth: invalid token")
	}

	// Update last used timestamp.
	if updateErr := s.store.UpdateLastUsed(token.ID); updateErr != nil {
		s.logger.Error("ssh: update last used", "token_id", token.ID, "error", updateErr)
	}

	return &ssh.Permissions{
		Extensions: map[string]string{
			"token_id": token.ID,
		},
	}, nil
}

// handleConn performs the SSH handshake and dispatches the session.
func (s *SSHServer) handleConn(netConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, s.sshCfg)
	if err != nil {
		s.logger.Warn("ssh: handshake failed",
			"remote_addr", netConn.RemoteAddr(),
			"error", err,
		)
		netConn.Close()
		return
	}

	tokenID := sshConn.Permissions.Extensions["token_id"]
	sess := Session{
		Conn:       sshConn,
		TokenID:    tokenID,
		RemoteAddr: sshConn.RemoteAddr().String(),
	}

	s.mu.Lock()
	s.sessions[sshConn] = &sess
	s.mu.Unlock()

	s.logger.Info("ssh: client connected",
		"token_id", tokenID,
		"remote_addr", sess.RemoteAddr,
	)

	if s.onSession != nil {
		s.onSession(sess, chans, reqs)
	}

	// Wait for connection to close.
	if err := sshConn.Wait(); err != nil && !errors.Is(err, net.ErrClosed) {
		s.logger.Info("ssh: client disconnected",
			"token_id", tokenID,
			"remote_addr", sess.RemoteAddr,
			"reason", err.Error(),
		)
	}

	s.mu.Lock()
	delete(s.sessions, sshConn)
	s.mu.Unlock()
}

// loadOrGenerateHostKey loads an ED25519 host key from path, or generates
// and saves a new one if the file does not exist. Per CON-001 §3.1.
func loadOrGenerateHostKey(path string, logger *slog.Logger) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse host key %s: %w", path, err)
		}
		logger.Info("ssh: loaded host key", "path", path)
		return signer, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read host key %s: %w", path, err)
	}

	// Generate new ED25519 key.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	// Marshal to PEM.
	pemBlock, err := ssh.MarshalPrivateKey(priv, "fonzygrok host key")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

	// Ensure parent directory exists.
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create host key dir: %w", err)
		}
	}

	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return nil, fmt.Errorf("write host key %s: %w", path, err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	logger.Info("ssh: generated new host key", "path", path)
	return signer, nil
}
