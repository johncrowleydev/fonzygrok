package server

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/names"
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
	// Name is the human-friendly subdomain name (user-specified or auto-generated).
	Name string
	// Subdomain is the assigned subdomain for HTTP routing (set to Name).
	Subdomain string
	// PublicURL is the full public URL for this tunnel.
	PublicURL string
	// Protocol is the tunnel protocol ("http" or "tcp").
	Protocol string
	// LocalPort is the port on the client's machine.
	LocalPort int
	// AssignedPort is the server-side port for TCP tunnels (0 for HTTP).
	AssignedPort int
	// Session is the SSH session that owns this tunnel.
	Session *Session
	// CreatedAt is when the tunnel was registered.
	CreatedAt time.Time
	// Metrics holds per-tunnel traffic counters.
	Metrics *TunnelMetrics
	// ACL is the optional IP access control list for this tunnel.
	ACL *ACL
}

// TunnelManager is the central registry of active tunnels.
// It maps tunnel IDs to SSH sessions and manages the tunnel lifecycle.
// Thread-safe via sync.RWMutex. Implements TunnelRegistrar.
type TunnelManager struct {
	domain      string
	scheme      string // "http" or "https"
	store       *store.Store
	logger      *slog.Logger
	tcpEdge     *TCPEdge      // optional: TCP port allocator
	rateLimiter *RateLimiter  // optional: per-tunnel rate limiter

	mu        sync.RWMutex
	tunnels   map[string]*TunnelEntry         // tunnelID → entry
	byName    map[string]*TunnelEntry         // name/subdomain → entry
	bySession map[*Session]map[string]bool    // session → set of tunnelIDs
	byPort    map[int]*TunnelEntry            // assignedPort → entry (TCP only)
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(domain string, st *store.Store, logger *slog.Logger) *TunnelManager {
	return &TunnelManager{
		domain:    domain,
		scheme:    "http",
		store:     st,
		logger:    logger,
		tunnels:   make(map[string]*TunnelEntry),
		byName:    make(map[string]*TunnelEntry),
		bySession: make(map[*Session]map[string]bool),
		byPort:    make(map[int]*TunnelEntry),
	}
}

// SetScheme sets the URL scheme ("http" or "https") for public tunnel URLs.
// Must be called before any tunnels are registered.
func (tm *TunnelManager) SetScheme(scheme string) {
	tm.scheme = scheme
}

// SetTCPEdge sets the TCP edge used for port allocation in TCP tunnels.
func (tm *TunnelManager) SetTCPEdge(tcpEdge *TCPEdge) {
	tm.tcpEdge = tcpEdge
}

// SetRateLimiter sets the rate limiter for per-tunnel rate limiting.
func (tm *TunnelManager) SetRateLimiter(rl *RateLimiter) {
	tm.rateLimiter = rl
}

// RateLimiter returns the configured rate limiter (may be nil).
func (tm *TunnelManager) RateLimiter() *RateLimiter {
	return tm.rateLimiter
}

// Register creates a new tunnel for the given session and request.
// If req.Name is provided, it is validated and checked for uniqueness.
// If req.Name is empty, a human-friendly name is auto-generated.
// Returns a TunnelAssignment per CON-001 §4.3.
func (tm *TunnelManager) Register(session *Session, req *proto.TunnelRequest) (*proto.TunnelAssignment, error) {
	if req.Protocol != "http" && req.Protocol != "tcp" {
		return nil, fmt.Errorf("unsupported protocol: %s", req.Protocol)
	}
	if req.LocalPort < 1 || req.LocalPort > 65535 {
		return nil, fmt.Errorf("invalid local port: %d", req.LocalPort)
	}

	tunnelID, err := tm.generateUniqueID()
	if err != nil {
		return nil, err
	}

	// Resolve subdomain name.
	var name string
	if req.Name != "" {
		// User-specified name: validate and check uniqueness.
		if err := names.Validate(req.Name); err != nil {
			return nil, fmt.Errorf("invalid name: %w", err)
		}
		tm.mu.RLock()
		existing, taken := tm.byName[req.Name]
		tm.mu.RUnlock()
		if taken {
			// Allow re-registration by the same token (handles reconnect race).
			if existing.Session.TokenID != session.TokenID {
				return nil, fmt.Errorf("name %q is already in use", req.Name)
			}
			// Same token reconnecting — deregister the stale entry first.
			tm.Deregister(existing.TunnelID)
		}
		name = req.Name
	} else {
		// Auto-generate a unique readable name.
		name = names.GenerateUnique(func(n string) bool {
			tm.mu.RLock()
			_, exists := tm.byName[n]
			tm.mu.RUnlock()
			return exists
		})
	}

	subdomain := name
	publicURL := fmt.Sprintf("%s://%s.%s", tm.scheme, subdomain, tm.domain)

	// For TCP tunnels, assign a port from the TCP edge.
	var assignedPort int
	if req.Protocol == "tcp" {
		if tm.tcpEdge == nil {
			return nil, fmt.Errorf("TCP tunnels not supported (no TCP edge configured)")
		}
		assignedPort, err = tm.tcpEdge.AssignPort()
		if err != nil {
			return nil, fmt.Errorf("assign TCP port: %w", err)
		}
		// Override public URL for TCP tunnels.
		publicURL = fmt.Sprintf("tcp://%s:%d", tm.domain, assignedPort)
	}

	entry := &TunnelEntry{
		TunnelID:     tunnelID,
		Name:         name,
		Subdomain:    subdomain,
		PublicURL:    publicURL,
		Protocol:     req.Protocol,
		LocalPort:    req.LocalPort,
		AssignedPort: assignedPort,
		Session:      session,
		CreatedAt:    time.Now().UTC(),
		Metrics:      NewTunnelMetrics(),
	}

	tm.mu.Lock()
	tm.tunnels[tunnelID] = entry
	tm.byName[name] = entry
	if assignedPort > 0 {
		tm.byPort[assignedPort] = entry
	}
	if tm.bySession[session] == nil {
		tm.bySession[session] = make(map[string]bool)
	}
	tm.bySession[session][tunnelID] = true
	tm.mu.Unlock()

	tm.logger.Info("tunnel: registered",
		"tunnel_id", tunnelID,
		"name", name,
		"subdomain", subdomain,
		"protocol", req.Protocol,
		"local_port", req.LocalPort,
		"assigned_port", assignedPort,
		"token_id", session.TokenID,
	)

	return &proto.TunnelAssignment{
		TunnelID:          tunnelID,
		AssignedSubdomain: subdomain,
		PublicURL:         publicURL,
		Protocol:          req.Protocol,
		Name:              name,
		AssignedPort:      assignedPort,
	}, nil
}

// Lookup returns the TunnelEntry for the given tunnel ID or subdomain name.
// It first checks the tunnelID index, then falls back to the name index.
func (tm *TunnelManager) Lookup(key string) (*TunnelEntry, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if entry, ok := tm.tunnels[key]; ok {
		return entry, ok
	}
	entry, ok := tm.byName[key]
	return entry, ok
}

// LookupByName returns the TunnelEntry for the given subdomain name.
func (tm *TunnelManager) LookupByName(name string) (*TunnelEntry, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, ok := tm.byName[name]
	return entry, ok
}

// Deregister removes a tunnel by ID.
// If the tunnel is a TCP tunnel, its assigned port is released.
func (tm *TunnelManager) Deregister(tunnelID string) {
	tm.mu.Lock()

	entry, ok := tm.tunnels[tunnelID]
	if !ok {
		tm.mu.Unlock()
		return
	}

	delete(tm.tunnels, tunnelID)
	// Remove from byName index.
	if entry.Name != "" {
		delete(tm.byName, entry.Name)
	}
	// Remove from byPort index.
	if entry.AssignedPort > 0 {
		delete(tm.byPort, entry.AssignedPort)
	}
	if sessionTunnels, ok := tm.bySession[entry.Session]; ok {
		delete(sessionTunnels, tunnelID)
		if len(sessionTunnels) == 0 {
			delete(tm.bySession, entry.Session)
		}
	}

	tm.mu.Unlock()

	// Release TCP port outside the lock to avoid deadlock with TCPEdge.
	if entry.Protocol == "tcp" && entry.AssignedPort > 0 && tm.tcpEdge != nil {
		tm.tcpEdge.ReleasePort(entry.AssignedPort)
	}

	// Clean up rate limiter state.
	if tm.rateLimiter != nil {
		tm.rateLimiter.Remove(tunnelID)
	}

	tm.logger.Info("tunnel: deregistered", "tunnel_id", tunnelID, "name", entry.Name)
}

// DeregisterBySession removes all tunnels owned by the given session.
// Called on client disconnect to clean up resources.
func (tm *TunnelManager) DeregisterBySession(session *Session) {
	tm.mu.Lock()

	tunnelIDs, ok := tm.bySession[session]
	if !ok {
		tm.mu.Unlock()
		return
	}

	// Collect entries that need TCP port release.
	var tcpPorts []int
	for id := range tunnelIDs {
		if entry, ok := tm.tunnels[id]; ok {
			if entry.Name != "" {
				delete(tm.byName, entry.Name)
			}
			if entry.AssignedPort > 0 {
				delete(tm.byPort, entry.AssignedPort)
				if entry.Protocol == "tcp" {
					tcpPorts = append(tcpPorts, entry.AssignedPort)
				}
			}
			delete(tm.tunnels, id)
		}
		tm.logger.Info("tunnel: deregistered (session cleanup)", "tunnel_id", id)
	}
	delete(tm.bySession, session)

	tm.mu.Unlock()

	// Release TCP ports outside the lock.
	if tm.tcpEdge != nil {
		for _, port := range tcpPorts {
			tm.tcpEdge.ReleasePort(port)
		}
	}
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
