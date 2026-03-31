package server

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/proto"
	"github.com/fonzygrok/fonzygrok/internal/store"
	"golang.org/x/crypto/ssh"
)

const (
	// tunnelIDLen is the length of generated tunnel IDs.
	tunnelIDLen = 6
	// tunnelIDAlphabet is the character set for tunnel IDs (lowercase alphanumeric).
	tunnelIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	// maxIDAttempts is the maximum number of attempts to generate a unique tunnel ID.
	maxIDAttempts = 100
)

// TunnelEntry represents an active tunnel in the tunnel manager.
type TunnelEntry struct {
	// TunnelID is the unique identifier for this tunnel.
	TunnelID string
	// Subdomain is the assigned subdomain for HTTP routing.
	Subdomain string
	// PublicURL is the full public URL for this tunnel.
	PublicURL string
	// Protocol is the tunnel protocol (e.g., "http").
	Protocol string
	// LocalPort is the port on the client's machine.
	LocalPort int
	// Session is the SSH session that owns this tunnel.
	Session *Session
	// CreatedAt is when the tunnel was registered.
	CreatedAt time.Time
}

// TunnelManager is the central registry of active tunnels.
// It maps tunnel IDs to SSH sessions and manages the tunnel lifecycle.
// Thread-safe via sync.RWMutex. Implements TunnelRegistrar.
type TunnelManager struct {
	domain  string
	store   *store.Store
	logger  *slog.Logger

	mu      sync.RWMutex
	tunnels map[string]*TunnelEntry         // tunnelID → entry
	bySession map[*Session]map[string]bool  // session → set of tunnelIDs
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(domain string, st *store.Store, logger *slog.Logger) *TunnelManager {
	return &TunnelManager{
		domain:    domain,
		store:     st,
		logger:    logger,
		tunnels:   make(map[string]*TunnelEntry),
		bySession: make(map[*Session]map[string]bool),
	}
}

// Register creates a new tunnel for the given session and request.
// Returns a TunnelAssignment per CON-001 §4.3.
func (tm *TunnelManager) Register(session *Session, req *proto.TunnelRequest) (*proto.TunnelAssignment, error) {
	if req.Protocol != "http" {
		return nil, fmt.Errorf("unsupported protocol: %s", req.Protocol)
	}
	if req.LocalPort < 1 || req.LocalPort > 65535 {
		return nil, fmt.Errorf("invalid local port: %d", req.LocalPort)
	}

	tunnelID, err := tm.generateUniqueID()
	if err != nil {
		return nil, err
	}

	subdomain := tunnelID
	publicURL := fmt.Sprintf("http://%s.%s", subdomain, tm.domain)

	entry := &TunnelEntry{
		TunnelID:  tunnelID,
		Subdomain: subdomain,
		PublicURL: publicURL,
		Protocol:  req.Protocol,
		LocalPort: req.LocalPort,
		Session:   session,
		CreatedAt: time.Now().UTC(),
	}

	tm.mu.Lock()
	tm.tunnels[tunnelID] = entry
	if tm.bySession[session] == nil {
		tm.bySession[session] = make(map[string]bool)
	}
	tm.bySession[session][tunnelID] = true
	tm.mu.Unlock()

	tm.logger.Info("tunnel: registered",
		"tunnel_id", tunnelID,
		"subdomain", subdomain,
		"local_port", req.LocalPort,
		"token_id", session.TokenID,
	)

	return &proto.TunnelAssignment{
		TunnelID:          tunnelID,
		AssignedSubdomain: subdomain,
		PublicURL:         publicURL,
		Protocol:          req.Protocol,
	}, nil
}

// Lookup returns the TunnelEntry for the given tunnel ID.
func (tm *TunnelManager) Lookup(tunnelID string) (*TunnelEntry, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, ok := tm.tunnels[tunnelID]
	return entry, ok
}

// Deregister removes a tunnel by ID.
func (tm *TunnelManager) Deregister(tunnelID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	entry, ok := tm.tunnels[tunnelID]
	if !ok {
		return
	}

	delete(tm.tunnels, tunnelID)
	if sessionTunnels, ok := tm.bySession[entry.Session]; ok {
		delete(sessionTunnels, tunnelID)
		if len(sessionTunnels) == 0 {
			delete(tm.bySession, entry.Session)
		}
	}

	tm.logger.Info("tunnel: deregistered", "tunnel_id", tunnelID)
}

// DeregisterBySession removes all tunnels owned by the given session.
// Called on client disconnect to clean up resources.
func (tm *TunnelManager) DeregisterBySession(session *Session) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tunnelIDs, ok := tm.bySession[session]
	if !ok {
		return
	}

	for id := range tunnelIDs {
		delete(tm.tunnels, id)
		tm.logger.Info("tunnel: deregistered (session cleanup)", "tunnel_id", id)
	}
	delete(tm.bySession, session)
}

// ListActive returns all currently active tunnel entries.
func (tm *TunnelManager) ListActive() []TunnelEntry {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]TunnelEntry, 0, len(tm.tunnels))
	for _, entry := range tm.tunnels {
		result = append(result, *entry)
	}
	return result
}

// OpenProxyChannel opens an SSH channel of type "proxy" to the client
// for proxying an HTTP request through the tunnel. Per CON-001 §5.1,
// the tunnel ID is sent as extra data.
func (tm *TunnelManager) OpenProxyChannel(tunnelID string) (ssh.Channel, error) {
	tm.mu.RLock()
	entry, ok := tm.tunnels[tunnelID]
	tm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tunnel %s not found", tunnelID)
	}

	if entry.Session.Conn == nil {
		return nil, fmt.Errorf("tunnel %s: session has no SSH connection", tunnelID)
	}

	ch, _, err := entry.Session.Conn.OpenChannel("proxy", []byte(tunnelID))
	if err != nil {
		return nil, fmt.Errorf("open proxy channel for tunnel %s: %w", tunnelID, err)
	}
	return ch, nil
}

// generateUniqueID generates a random 6-character lowercase alphanumeric
// tunnel ID, retrying on collisions.
func (tm *TunnelManager) generateUniqueID() (string, error) {
	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		id := randomTunnelID(tunnelIDLen)
		tm.mu.RLock()
		_, exists := tm.tunnels[id]
		tm.mu.RUnlock()
		if !exists {
			return id, nil
		}
	}
	return "", fmt.Errorf("tunnel: failed to generate unique ID after %d attempts", maxIDAttempts)
}

// randomTunnelID generates a random string of given length from tunnelIDAlphabet.
func randomTunnelID(n int) string {
	b := make([]byte, n)
	randomBytes := make([]byte, n)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Sprintf("tunnel: crypto/rand.Read failed: %v", err))
	}
	for i := 0; i < n; i++ {
		b[i] = tunnelIDAlphabet[int(randomBytes[i])%len(tunnelIDAlphabet)]
	}
	return string(b)
}
