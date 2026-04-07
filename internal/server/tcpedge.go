package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// TCPEdge manages TCP tunnel port listeners. For each TCP tunnel,
// it assigns a port from a configured range, starts a net.Listener,
// and proxies accepted connections through SSH "tcp-proxy" channels.
type TCPEdge struct {
	portMin     int
	portMax     int
	listeners   map[int]net.Listener // assigned port → listener
	tunnelMgr   *TunnelManager
	rateLimiter *RateLimiter
	mu          sync.Mutex
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewTCPEdge creates a TCP edge that allocates ports in [portMin, portMax].
func NewTCPEdge(portMin, portMax int, tm *TunnelManager, logger *slog.Logger) *TCPEdge {
	ctx, cancel := context.WithCancel(context.Background())
	return &TCPEdge{
		portMin:   portMin,
		portMax:   portMax,
		listeners: make(map[int]net.Listener),
		tunnelMgr: tm,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// AssignPort finds an unused port in the range, starts a listener, and
// returns the assigned port. Returns an error if the port range is exhausted.
func (t *TCPEdge) AssignPort() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for port := t.portMin; port <= t.portMax; port++ {
		if _, used := t.listeners[port]; used {
			continue
		}

		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			// Port might be in use externally; skip.
			continue
		}

		t.listeners[port] = ln
		go t.acceptLoop(port, ln)

		t.logger.Info("tcp-edge: port assigned",
			"port", port,
		)
		return port, nil
	}

	return 0, fmt.Errorf("tcp-edge: port range %d-%d exhausted", t.portMin, t.portMax)
}

// ReleasePort stops the listener for the given port and frees it for reuse.
func (t *TCPEdge) ReleasePort(port int) {
	t.mu.Lock()
	ln, ok := t.listeners[port]
	if ok {
		delete(t.listeners, port)
	}
	t.mu.Unlock()

	if ok {
		ln.Close()
		t.logger.Info("tcp-edge: port released", "port", port)
	}
}

// Shutdown stops all listeners and cancels the context.
func (t *TCPEdge) Shutdown() {
	t.cancel()

	t.mu.Lock()
	defer t.mu.Unlock()

	for port, ln := range t.listeners {
		ln.Close()
		t.logger.Info("tcp-edge: listener stopped", "port", port)
	}
	t.listeners = make(map[int]net.Listener)
}

// SetRateLimiter attaches a rate limiter for TCP connection attempts.
func (t *TCPEdge) SetRateLimiter(rl *RateLimiter) {
	t.rateLimiter = rl
}

// acceptLoop accepts TCP connections on the listener and proxies each
// through an SSH "tcp-proxy" channel to the client.
func (t *TCPEdge) acceptLoop(port int, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if we're shutting down.
			select {
			case <-t.ctx.Done():
				return
			default:
			}

			// Check if listener was closed (ReleasePort).
			t.mu.Lock()
			_, stillActive := t.listeners[port]
			t.mu.Unlock()
			if !stillActive {
				return
			}

			t.logger.Error("tcp-edge: accept",
				"port", port,
				"error", err,
			)
			continue
		}

		go t.handleConnection(port, conn)
	}
}

// handleConnection proxies a single TCP connection through the SSH tunnel.
// It looks up the tunnel by assigned port, opens a "tcp-proxy" SSH channel,
// and performs bidirectional copy.
func (t *TCPEdge) handleConnection(port int, conn net.Conn) {
	defer conn.Close()

	// Find the tunnel entry for this port.
	entry := t.lookupByPort(port)
	if entry == nil {
		t.logger.Warn("tcp-edge: no tunnel for port",
			"port", port,
			"remote_addr", conn.RemoteAddr().String(),
		)
		return
	}

	// Rate limit check (connection attempts, not bytes).
	if t.rateLimiter != nil && !t.rateLimiter.Allow(entry.TunnelID) {
		t.logger.Warn("tcp-edge: rate limited",
			"tunnel_id", entry.TunnelID,
			"port", port,
			"remote_addr", conn.RemoteAddr().String(),
		)
		return
	}

	// IP ACL check.
	if entry.ACL != nil {
		clientIP := extractIP(conn.RemoteAddr().String())
		if clientIP != nil && !entry.ACL.Check(clientIP) {
			t.logger.Warn("tcp-edge: IP blocked by ACL",
				"tunnel_id", entry.TunnelID,
				"port", port,
				"remote_addr", conn.RemoteAddr().String(),
			)
			return
		}
	}

	// Open SSH "tcp-proxy" channel to the client.
	ch, err := t.openTCPProxyChannel(entry)
	if err != nil {
		t.logger.Error("tcp-edge: open tcp-proxy channel",
			"tunnel_id", entry.TunnelID,
			"port", port,
			"error", err,
		)
		return
	}
	defer ch.Close()

	// Bidirectional copy with metrics tracking.
	t.proxy(conn, ch, entry)
}

// lookupByPort finds the TunnelEntry that has the given assigned port.
func (t *TCPEdge) lookupByPort(port int) *TunnelEntry {
	active := t.tunnelMgr.ListActive()
	for i := range active {
		if active[i].AssignedPort == port && active[i].Protocol == "tcp" {
			return &active[i]
		}
	}
	return nil
}

// openTCPProxyChannel opens an SSH channel of type "tcp-proxy" to the client.
// The tunnel ID is sent as extra data, matching the convention used by
// the HTTP "proxy" channel (CON-001 §5).
func (t *TCPEdge) openTCPProxyChannel(entry *TunnelEntry) (ssh.Channel, error) {
	if entry.Session == nil || entry.Session.Conn == nil {
		return nil, fmt.Errorf("tunnel %s: session has no SSH connection", entry.TunnelID)
	}

	ch, _, err := entry.Session.Conn.OpenChannel("tcp-proxy", []byte(entry.TunnelID))
	if err != nil {
		return nil, fmt.Errorf("open tcp-proxy channel for tunnel %s: %w", entry.TunnelID, err)
	}
	return ch, nil
}

// proxy performs bidirectional copy between a TCP connection and an SSH channel,
// tracking bytes transferred in the tunnel's metrics.
func (t *TCPEdge) proxy(conn net.Conn, ch ssh.Channel, entry *TunnelEntry) {
	var wg sync.WaitGroup
	wg.Add(2)

	// conn → ch (data going IN to tunnel, from external client).
	go func() {
		defer wg.Done()
		var written int64
		if entry.Metrics != nil {
			cw := NewCountingWriter(ch, &entry.Metrics.BytesIn)
			written, _ = io.Copy(cw, conn)
		} else {
			written, _ = io.Copy(ch, conn)
		}
		// Signal the SSH channel that no more data is coming from this side.
		ch.CloseWrite()
		t.logger.Debug("tcp-edge: conn→channel done",
			"tunnel_id", entry.TunnelID,
			"bytes", written,
		)
	}()

	// ch → conn (data going OUT from tunnel, to external client).
	go func() {
		defer wg.Done()
		var written int64
		if entry.Metrics != nil {
			cw := NewCountingWriter(conn, &entry.Metrics.BytesOut)
			written, _ = io.Copy(cw, ch)
		} else {
			written, _ = io.Copy(conn, ch)
		}
		t.logger.Debug("tcp-edge: channel→conn done",
			"tunnel_id", entry.TunnelID,
			"bytes", written,
		)
	}()

	wg.Wait()

	// Record a "request" for metrics (each TCP connection = 1 request).
	if entry.Metrics != nil {
		entry.Metrics.RecordRequest()
	}
}
