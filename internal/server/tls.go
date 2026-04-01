package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

// TLSConfig holds configuration for Let's Encrypt auto-TLS on the edge router.
type TLSConfig struct {
	// Enabled controls whether TLS is active.
	Enabled bool
	// CertDir is the directory for cached certificates (autocert.DirCache).
	CertDir string
	// Domain is the base domain for the host policy (e.g., "tunnel.fonzygrok.com").
	Domain string
}

// TLSManager wraps autocert.Manager to provide auto-TLS for the edge router.
type TLSManager struct {
	manager *autocert.Manager
	config  TLSConfig
}

// NewTLSManager creates an autocert.Manager configured for fonzygrok's edge router.
// The host policy accepts:
//   - The base domain itself (e.g., "tunnel.fonzygrok.com")
//   - Any subdomain of the base domain (e.g., "*.tunnel.fonzygrok.com")
func NewTLSManager(cfg TLSConfig) *TLSManager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cfg.CertDir),
		HostPolicy: tunnelHostPolicy(cfg.Domain),
	}

	return &TLSManager{
		manager: m,
		config:  cfg,
	}
}

// TLSConfig returns a *tls.Config suitable for use with tls.NewListener.
// It delegates certificate management to the autocert.Manager.
func (tm *TLSManager) TLSConfig() *tls.Config {
	return tm.manager.TLSConfig()
}

// Manager returns the underlying autocert.Manager (for HTTPHandler).
func (tm *TLSManager) Manager() *autocert.Manager {
	return tm.manager
}

// tunnelHostPolicy returns an autocert.HostPolicy that accepts:
//   - The exact base domain
//   - Any single-level subdomain of the base domain
func tunnelHostPolicy(baseDomain string) autocert.HostPolicy {
	return func(_ context.Context, host string) error {
		// Accept the base domain itself.
		if host == baseDomain {
			return nil
		}

		// Accept <subdomain>.<baseDomain>.
		suffix := "." + baseDomain
		if strings.HasSuffix(host, suffix) {
			// Reject multi-level subdomains (e.g., "a.b.tunnel.example.com").
			prefix := strings.TrimSuffix(host, suffix)
			if !strings.Contains(prefix, ".") && len(prefix) > 0 {
				return nil
			}
		}

		return fmt.Errorf("tls: host %q not allowed by policy", host)
	}
}
