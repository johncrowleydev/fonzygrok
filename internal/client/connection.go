// Package client implements the fonzygrok tunnel client. It manages SSH
// connections to the server, negotiates tunnels via a control channel,
// and proxies traffic between remote SSH channels and local services.
package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// SSH channel type constants per CON-001 §4.1 and §5.1.
const (
	ChannelTypeControl = "control"
	ChannelTypeProxy   = "proxy"
	SSHUsername         = "fonzygrok"
)

// ClientConfig holds the settings required to connect to a fonzygrok server.
type ClientConfig struct {
	// ServerAddr is the host:port of the SSH server (e.g. "example.com:2222").
	ServerAddr string
	// Token is the API token used for authentication (sent as SSH password).
	Token string
	// Insecure skips host key verification when true.
	Insecure bool
}

// Connector manages an SSH connection to a fonzygrok server.
// It is safe for concurrent use after Connect returns.
type Connector struct {
	cfg    ClientConfig
	logger *slog.Logger

	mu        sync.RWMutex
	client    *ssh.Client
	connected bool
}

// NewConnector creates a new Connector with the given config and logger.
// It does not initiate a connection — call Connect for that.
func NewConnector(cfg ClientConfig, logger *slog.Logger) *Connector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Connector{
		cfg:    cfg,
		logger: logger,
	}
}

// Connect dials the server and authenticates via SSH password auth.
// The ctx is used for the TCP dial timeout only; once the SSH handshake
// completes the connection is independent of ctx.
func (c *Connector) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return errors.New("client: already connected")
	}

	sshCfg := &ssh.ClientConfig{
		User: SSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.cfg.Token),
		},
		HostKeyCallback: c.hostKeyCallback(),
	}

	c.logger.Info("connecting to server",
		slog.String("server_addr", c.cfg.ServerAddr),
	)

	// Use a context-aware dialer so callers can set deadlines.
	var d net.Dialer
	tcpConn, err := d.DialContext(ctx, "tcp", c.cfg.ServerAddr)
	if err != nil {
		return fmt.Errorf("client: dial %s: %w", c.cfg.ServerAddr, err)
	}

	// Perform the SSH handshake over the raw TCP connection.
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, c.cfg.ServerAddr, sshCfg)
	if err != nil {
		tcpConn.Close()
		return fmt.Errorf("client: ssh handshake: %w", err)
	}

	c.client = ssh.NewClient(sshConn, chans, reqs)
	c.connected = true

	c.logger.Info("connected",
		slog.String("server_addr", c.cfg.ServerAddr),
	)
	return nil
}

// Close cleanly terminates the SSH connection.
func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.client == nil {
		return nil
	}

	c.logger.Info("disconnecting from server")
	err := c.client.Close()
	c.connected = false
	c.client = nil
	if err != nil {
		return fmt.Errorf("client: close: %w", err)
	}
	return nil
}

// IsConnected reports whether the connector currently has an active SSH session.
func (c *Connector) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// OpenControl opens a control channel on the SSH connection.
// Per CON-001 §4.1, the server accepts exactly one control channel per session.
func (c *Connector) OpenControl() (*ControlChannel, error) {
	c.mu.RLock()
	cl := c.client
	conn := c.connected
	c.mu.RUnlock()

	if !conn || cl == nil {
		return nil, errors.New("client: not connected")
	}

	ch, reqs, err := cl.OpenChannel(ChannelTypeControl, nil)
	if err != nil {
		return nil, fmt.Errorf("client: open control channel: %w", err)
	}

	// Discard out-of-band requests on the control channel.
	go ssh.DiscardRequests(reqs)

	return newControlChannel(ch, c.logger), nil
}

// SSHClient returns the underlying *ssh.Client for advanced usage such as
// accepting incoming channels. Returns nil if not connected.
func (c *Connector) SSHClient() *ssh.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// hostKeyCallback returns the appropriate ssh.HostKeyCallback based on config.
func (c *Connector) hostKeyCallback() ssh.HostKeyCallback {
	if c.cfg.Insecure {
		return ssh.InsecureIgnoreHostKey()
	}
	// For v1.0 we accept any host key with a warning log.
	// Full known_hosts support is a v1.1 feature.
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		c.logger.Warn("accepting unverified host key",
			slog.String("hostname", hostname),
			slog.String("remote", remote.String()),
			slog.String("key_type", key.Type()),
		)
		return nil
	}
}
