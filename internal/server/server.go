package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/store"
	"golang.org/x/crypto/ssh"
)

// ServerConfig holds configuration for the complete fonzygrok server.
// It embeds the configs for each subsystem.
type ServerConfig struct {
	// DataDir is the directory for persistent data (database, host key).
	DataDir string
	// Domain is the base domain for tunnel routing.
	Domain string
	// ApexDomain is the apex domain for the dashboard (e.g., "fonzygrok.com").
	// If empty, derived by stripping the first label from Domain.
	ApexDomain string
	// SSH configuration.
	SSH SSHConfig
	// Edge (public HTTP) configuration.
	Edge EdgeConfig
	// Admin API configuration.
	Admin AdminConfig
	// TLS configuration for auto-TLS on the edge router.
	TLS TLSConfig
}

// Server is the top-level orchestrator that wires all subsystems together:
// store → SSH server → tunnel manager → edge router → admin API.
type Server struct {
	config  ServerConfig
	logger  *slog.Logger
	store   *store.Store
	ssh     *SSHServer
	tunnels *TunnelManager
	edge    *EdgeRouter
	admin   *AdminAPI
}

// deriveApexDomain strips the first label from a domain to derive the apex.
// e.g., "tunnel.fonzygrok.com" → "fonzygrok.com".
// Returns the domain unchanged if it has fewer than 3 labels.
func deriveApexDomain(domain string) string {
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) < 2 || !strings.Contains(parts[1], ".") {
		return domain
	}
	return parts[1]
}

// NewServer creates and wires all server subsystems. Does not start them.
func NewServer(config ServerConfig, logger *slog.Logger) (*Server, error) {
	// Derive apex domain from tunnel domain if not explicitly set.
	if config.ApexDomain == "" {
		config.ApexDomain = deriveApexDomain(config.Domain)
	}

	// Open the database.
	dbPath := filepath.Join(config.DataDir, "fonzygrok.db")
	st, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("server: open store: %w", err)
	}
	if err := st.Migrate(); err != nil {
		st.Close()
		return nil, fmt.Errorf("server: migrate store: %w", err)
	}

	// Set default host key path if not provided.
	if config.SSH.HostKeyPath == "" {
		config.SSH.HostKeyPath = filepath.Join(config.DataDir, "host_key")
	}

	// Create SSH server.
	sshSrv, err := New(config.SSH, st, logger)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("server: create SSH server: %w", err)
	}

	// Create tunnel manager.
	tm := NewTunnelManager(config.Domain, st, logger)

	// Wire SSH session callback → control handler → tunnel manager.
	sshSrv.OnNewSession(func(sess Session, chans <-chan ssh.NewChannel, reqs <-chan *ssh.Request) {
		handler := NewControlHandler(&sess, tm, logger)
		go handler.HandleGlobalRequests(reqs)
		go handler.HandleChannels(chans)
	})

	// Set edge base domain from config.
	if config.Edge.BaseDomain == "" {
		config.Edge.BaseDomain = config.Domain
	}
	// Propagate apex domain to edge config.
	if config.Edge.ApexDomain == "" {
		config.Edge.ApexDomain = config.ApexDomain
	}

	// Create edge router.
	edge := NewEdgeRouter(config.Edge, tm, logger)

	// Wire TLS if enabled.
	if config.TLS.Enabled {
		if config.TLS.CertDir == "" {
			config.TLS.CertDir = filepath.Join(config.DataDir, "certs")
		}
		if config.TLS.Domain == "" {
			config.TLS.Domain = config.Domain
		}
		// Propagate apex domain to TLS config for cert issuance.
		if config.TLS.ApexDomain == "" {
			config.TLS.ApexDomain = config.ApexDomain
		}
		tlsMgr := NewTLSManager(config.TLS)
		edge.SetTLSManager(tlsMgr)

		// Set URL scheme to https for tunnel URLs.
		tm.SetScheme("https")

		logger.Info("TLS enabled", "cert_dir", config.TLS.CertDir, "domain", config.TLS.Domain, "apex_domain", config.TLS.ApexDomain)
	}

	// Create JWT manager for dashboard sessions.
	jwtSecretPath := filepath.Join(config.DataDir, "jwt_secret")
	jwtMgr, err := auth.NewJWTManager(jwtSecretPath, auth.DefaultJWTExpiry)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("server: create JWT manager: %w", err)
	}

	// Create admin API.
	admin := NewAdminAPI(config.Admin, st, jwtMgr, tm, sshSrv, logger)

	// Register dashboard UI on the admin mux.
	dash := NewDashboard(st, jwtMgr, tm, logger)
	dash.SetTLSEnabled(config.TLS.Enabled)
	dash.RegisterRoutes(admin.Mux())

	// Wire admin mux as edge fallback handler — requests to the base domain
	// or apex domain (no subdomain) are served by the dashboard.
	edge.SetBaseDomainHandler(admin.Mux())

	return &Server{
		config:  config,
		logger:  logger,
		store:   st,
		ssh:     sshSrv,
		tunnels: tm,
		edge:    edge,
		admin:   admin,
	}, nil
}

// Start starts all subsystems concurrently and blocks until the context
// is canceled or a subsystem returns an error.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("server starting",
		"ssh_addr", s.config.SSH.Addr,
		"edge_addr", s.config.Edge.Addr,
		"admin_addr", s.config.Admin.Addr,
		"domain", s.config.Domain,
		"data_dir", s.config.DataDir,
	)

	errCh := make(chan error, 3)

	go func() { errCh <- s.ssh.Start(ctx) }()
	go func() { errCh <- s.edge.Start(ctx) }()
	go func() { errCh <- s.admin.Start(ctx) }()

	// Wait for context cancellation or first error.
	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errCh:
		if err != nil {
			s.Stop()
			return err
		}
		return nil
	}
}

// Stop gracefully shuts down all subsystems in order:
// 1. Stop accepting new connections (admin, edge)
// 2. Close SSH (disconnects all clients and tunnels)
// 3. Close store
func (s *Server) Stop() error {
	s.logger.Info("server shutting down")

	var errs []error

	// Stop admin API first (no new management requests).
	if err := s.admin.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop admin: %w", err))
	}

	// Stop edge router (no new public requests).
	if err := s.edge.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop edge: %w", err))
	}

	// Stop SSH server (disconnects all clients, cleans up tunnels).
	if err := s.ssh.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop ssh: %w", err))
	}

	// Close the database last.
	if err := s.store.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close store: %w", err))
	}

	s.logger.Info("server stopped")
	return errors.Join(errs...)
}

// Store returns the underlying store (for CLI token commands).
func (s *Server) Store() *store.Store {
	return s.store
}
