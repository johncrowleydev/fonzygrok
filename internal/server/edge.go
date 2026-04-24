package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Version is set at build time via -ldflags. Used by server info endpoint.
var Version = "dev"

// EdgeConfig holds configuration for the HTTP edge router.
type EdgeConfig struct {
	// Addr is the listen address (e.g., ":8080").
	Addr string
	// BaseDomain is the base domain for subdomain routing (e.g., "tunnel.example.com").
	BaseDomain string
	// ApexDomain is the apex domain (e.g., "fonzygrok.com") that should also
	// be routed to the dashboard when no subdomain is present.
	ApexDomain string
	// ProxyTimeout is the maximum time to wait for a proxied response.
	// Default: 30s.
	ProxyTimeout time.Duration
	// ReadHeaderTimeout limits how long the server spends reading request headers.
	// Default: 5s.
	ReadHeaderTimeout time.Duration
	// ReadTimeout limits how long the server spends reading the full request.
	// Default: 15s.
	ReadTimeout time.Duration
	// WriteTimeout limits response writes. Default: 10m for edge tunnel responses,
	// which may be long-lived streams and should not use a short API timeout.
	WriteTimeout time.Duration
	// IdleTimeout limits how long keep-alive connections remain idle.
	// Default: 120s.
	IdleTimeout time.Duration
}

const (
	defaultHTTPReadHeaderTimeout = 5 * time.Second
	defaultHTTPReadTimeout       = 15 * time.Second
	defaultHTTPWriteTimeout      = 30 * time.Second
	defaultEdgeWriteTimeout      = 10 * time.Minute
	defaultHTTPIdleTimeout       = 120 * time.Second
)

// EdgeRouter routes incoming public HTTP requests to the correct tunnel
// by extracting the subdomain from the Host header and proxying the
// request through an SSH channel. Per CON-002 §3.
type EdgeRouter struct {
	config            EdgeConfig
	tunnels           *TunnelManager
	logger            *slog.Logger
	server            *http.Server
	tlsMgr            *TLSManager
	redirectSrv       *http.Server // HTTP→HTTPS redirect server (when TLS enabled)
	baseDomainHandler http.Handler // fallback handler for apex/base domain (dashboard)
	rateLimiter       *RateLimiter // per-tunnel rate limiter
}

// NewEdgeRouter creates a new HTTP edge router.
func NewEdgeRouter(config EdgeConfig, tunnels *TunnelManager, logger *slog.Logger) *EdgeRouter {
	if config.ProxyTimeout == 0 {
		config.ProxyTimeout = 30 * time.Second
	}
	config = applyEdgeTimeoutDefaults(config)

	e := &EdgeRouter{
		config:  config,
		tunnels: tunnels,
		logger:  logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", e.handleRequest)

	e.server = &http.Server{
		Addr:              config.Addr,
		Handler:           mux,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
	}

	return e
}

func applyEdgeTimeoutDefaults(config EdgeConfig) EdgeConfig {
	if config.ReadHeaderTimeout == 0 {
		config.ReadHeaderTimeout = defaultHTTPReadHeaderTimeout
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = defaultHTTPReadTimeout
	}
	if config.WriteTimeout == 0 {
		// Use a deliberately long edge write timeout so slowloris-style writes are
		// bounded without cutting off normal tunnel streams as quickly as API calls.
		config.WriteTimeout = defaultEdgeWriteTimeout
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = defaultHTTPIdleTimeout
	}
	return config
}

func (e *EdgeRouter) newRedirectServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: e.config.ReadHeaderTimeout,
		ReadTimeout:       e.config.ReadTimeout,
		WriteTimeout:      e.config.WriteTimeout,
		IdleTimeout:       e.config.IdleTimeout,
	}
}

// Start begins listening for HTTP requests on the configured address.
// If a TLS manager is set, it starts TLS on :443 and a redirect on :80.
func (e *EdgeRouter) Start(ctx context.Context) error {
	if e.tlsMgr != nil {
		return e.startTLS(ctx)
	}
	return e.startPlain(ctx)
}

// SetTLSManager attaches a TLS manager to the edge router.
// Must be called before Start().
func (e *EdgeRouter) SetTLSManager(tm *TLSManager) {
	e.tlsMgr = tm
}

// SetBaseDomainHandler sets a fallback handler for requests to the base domain
// or apex domain (no subdomain). When set, these requests are delegated to the
// handler instead of returning the default server info JSON.
// Used to serve the dashboard UI on the public HTTPS port.
func (e *EdgeRouter) SetBaseDomainHandler(h http.Handler) {
	e.baseDomainHandler = h
}

// startPlain starts a plain HTTP listener (existing behavior).
func (e *EdgeRouter) startPlain(ctx context.Context) error {
	ln, err := net.Listen("tcp", e.config.Addr)
	if err != nil {
		return fmt.Errorf("edge: listen %s: %w", e.config.Addr, err)
	}

	e.logger.Info("edge router listening (HTTP)", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		e.Stop()
	}()

	if err := e.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("edge: serve: %w", err)
	}
	return nil
}

// startTLS starts the edge router with TLS on :443 and an HTTP redirect on :80.
func (e *EdgeRouter) startTLS(ctx context.Context) error {
	// Determine TLS listen address. Use :443 if the configured addr is :8080
	// (default), otherwise use the configured addr for the TLS listener.
	tlsAddr := e.config.Addr
	if tlsAddr == ":8080" || tlsAddr == "0.0.0.0:8080" {
		tlsAddr = ":443"
	}

	// Create TLS listener.
	tlsLn, err := tls.Listen("tcp", tlsAddr, e.tlsMgr.TLSConfig())
	if err != nil {
		return fmt.Errorf("edge: tls listen %s: %w", tlsAddr, err)
	}

	e.logger.Info("edge router listening (HTTPS)", "addr", tlsLn.Addr().String())

	// Start HTTP redirect server on :80.
	redirectAddr := ":80"
	redirectHandler := e.tlsMgr.Manager().HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))

	e.redirectSrv = e.newRedirectServer(redirectAddr, redirectHandler)

	redirectLn, err := net.Listen("tcp", redirectAddr)
	if err != nil {
		// Port 80 may not be available — log and continue with just TLS.
		e.logger.Warn("edge: could not start HTTP redirect server", "addr", redirectAddr, "error", err)
	} else {
		e.logger.Info("edge redirect server listening (HTTP→HTTPS)", "addr", redirectLn.Addr().String())
		go func() {
			if err := e.redirectSrv.Serve(redirectLn); err != nil && err != http.ErrServerClosed {
				e.logger.Error("edge: redirect serve", "error", err)
			}
		}()
	}

	go func() {
		<-ctx.Done()
		e.Stop()
	}()

	if err := e.server.Serve(tlsLn); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("edge: tls serve: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the HTTP edge router and redirect server.
func (e *EdgeRouter) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop redirect server if running.
	if e.redirectSrv != nil {
		e.redirectSrv.Shutdown(ctx)
	}

	return e.server.Shutdown(ctx)
}

// Handler returns the http.Handler for testing purposes.
func (e *EdgeRouter) Handler() http.Handler {
	return e.server.Handler
}

// handleRequest is the main request handler. It extracts the subdomain,
// looks up the tunnel, and proxies the request.
func (e *EdgeRouter) handleRequest(w http.ResponseWriter, r *http.Request) {
	tunnelID := e.extractSubdomain(r.Host)

	// No subdomain → delegate to fallback handler (dashboard) or server info.
	if tunnelID == "" {
		if e.baseDomainHandler != nil {
			e.baseDomainHandler.ServeHTTP(w, r)
			return
		}
		e.handleServerInfo(w, r)
		return
	}

	// Lookup tunnel.
	entry, ok := e.tunnels.Lookup(tunnelID)
	if !ok {
		e.writeError(w, http.StatusNotFound, "tunnel_not_found", "No tunnel matches this hostname")
		return
	}

	// Rate limit check.
	if e.rateLimiter != nil && !e.rateLimiter.Allow(entry.TunnelID) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Fonzygrok-Error", "true")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":               "rate_limit_exceeded",
			"message":             "Too many requests to this tunnel",
			"retry_after_seconds": 1,
		})
		return
	}

	// IP ACL check (use source IP, NOT X-Forwarded-For to prevent spoofing).
	if entry.ACL != nil {
		clientIP := extractIP(r.RemoteAddr)
		if clientIP != nil && !entry.ACL.Check(clientIP) {
			e.writeError(w, http.StatusForbidden, "ip_blocked", "Your IP is not allowed to access this tunnel")
			return
		}
	}

	e.proxyRequest(w, r, entry)
}

// extractSubdomain extracts the tunnel ID from the Host header.
// For "abc123.tunnel.example.com" with BaseDomain "tunnel.example.com",
// returns "abc123". Returns "" for the base domain, apex domain, or unrecognized hosts.
func (e *EdgeRouter) extractSubdomain(host string) string {
	// Strip port if present.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	host = strings.ToLower(host)
	base := strings.ToLower(e.config.BaseDomain)

	// Exact match = base domain, no subdomain.
	if host == base {
		return ""
	}

	// Apex domain match (e.g., "fonzygrok.com" when BaseDomain is "tunnel.fonzygrok.com").
	if apex := strings.ToLower(e.config.ApexDomain); apex != "" && host == apex {
		return ""
	}

	// Check if host ends with ".<base_domain>".
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return ""
	}

	// Extract the subdomain prefix.
	subdomain := strings.TrimSuffix(host, suffix)

	// Subdomain should not contain additional dots (no nested subdomains).
	if strings.Contains(subdomain, ".") {
		return ""
	}

	return subdomain
}

// proxyRequest proxies an HTTP request through the SSH tunnel to the client.
// Per CON-002 §3.2 and CON-001 §5.
func (e *EdgeRouter) proxyRequest(w http.ResponseWriter, r *http.Request, entry *TunnelEntry) {
	// Open proxy channel to the client via SSH.
	ch, err := e.tunnels.OpenProxyChannel(entry.TunnelID)
	if err != nil {
		e.logger.Error("edge: open proxy channel",
			"tunnel_id", entry.TunnelID,
			"error", err,
		)
		e.writeError(w, http.StatusBadGateway, "tunnel_offline", "The tunnel is currently offline")
		return
	}
	defer ch.Close()

	// Add forwarding headers per CON-002 §3.2.
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	r.Header.Set("X-Forwarded-For", clientIP)
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Proto", e.requestProto(r))
	r.Header.Set("X-Fonzygrok-Tunnel-Id", entry.TunnelID)

	// Write the full HTTP request to the SSH channel.
	// Count request bytes in (ContentLength if known, or wrap body).
	if entry.Metrics != nil && r.ContentLength > 0 {
		entry.Metrics.BytesIn.Add(r.ContentLength)
	}
	if err := r.Write(ch); err != nil {
		e.logger.Error("edge: write request to tunnel",
			"tunnel_id", entry.TunnelID,
			"error", err,
		)
		e.writeError(w, http.StatusBadGateway, "tunnel_offline", "The tunnel is currently offline")
		return
	}

	// NOTE: We intentionally do NOT call ch.CloseWrite() here.
	// The HTTP request has well-defined message boundaries (Content-Length
	// or chunked encoding), so the client can detect end-of-request from
	// the HTTP protocol. Calling CloseWrite() sends an SSH EOF that races
	// with the client reading the request over real networks — the client
	// sees EOF before it finishes reading, causing the response to never
	// be written back, resulting in "unexpected EOF" on the server side.
	// See DEF-003 for the full root cause analysis.

	// Read the response from the channel with timeout.
	type responseResult struct {
		resp *http.Response
		err  error
	}
	resultCh := make(chan responseResult, 1)
	go func() {
		resp, err := http.ReadResponse(bufio.NewReader(ch), r)
		resultCh <- responseResult{resp, err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			e.logger.Error("edge: read response from tunnel",
				"tunnel_id", entry.TunnelID,
				"error", result.err,
			)
			e.writeError(w, http.StatusBadGateway, "upstream_unreachable", "The local service did not respond")
			return
		}
		defer result.resp.Body.Close()

		// Copy response headers.
		for key, vals := range result.resp.Header {
			for _, val := range vals {
				w.Header().Add(key, val)
			}
		}
		w.WriteHeader(result.resp.StatusCode)

		// Copy response body, counting bytes out.
		if entry.Metrics != nil {
			cw := NewCountingWriter(w, &entry.Metrics.BytesOut)
			if _, err := io.Copy(cw, result.resp.Body); err != nil {
				e.logger.Error("edge: copy response body",
					"tunnel_id", entry.TunnelID,
					"error", err,
				)
			}
			// Record the proxy request.
			entry.Metrics.RecordRequest()
		} else {
			if _, err := io.Copy(w, result.resp.Body); err != nil {
				e.logger.Error("edge: copy response body",
					"tunnel_id", entry.TunnelID,
					"error", err,
				)
			}
		}

		e.logger.Debug("edge: request proxied",
			"tunnel_id", entry.TunnelID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", result.resp.StatusCode,
		)

	case <-time.After(e.config.ProxyTimeout):
		e.logger.Warn("edge: proxy timeout",
			"tunnel_id", entry.TunnelID,
			"timeout", e.config.ProxyTimeout,
		)
		e.writeError(w, http.StatusGatewayTimeout, "proxy_timeout", "The local service did not respond in time")
		return
	}
}

// handleServerInfo returns the server info JSON per CON-002 §3.4.
func (e *EdgeRouter) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	info := struct {
		Service       string `json:"service"`
		Version       string `json:"version"`
		Status        string `json:"status"`
		TunnelsActive int    `json:"tunnels_active"`
	}{
		Service:       "fonzygrok",
		Version:       Version,
		Status:        "running",
		TunnelsActive: len(e.tunnels.ListActive()),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(info)
}

// writeError writes a JSON error response per CON-002 §3.3.
// All error responses include Content-Type: application/json and
// X-Fonzygrok-Error: true.
func (e *EdgeRouter) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Fonzygrok-Error", "true")
	w.WriteHeader(status)

	resp := struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{
		Error:   code,
		Message: message,
	}
	json.NewEncoder(w).Encode(resp)
}

// requestProto determines the protocol scheme of the request.
func (e *EdgeRouter) requestProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if fp := r.Header.Get("X-Forwarded-Proto"); fp != "" {
		return fp
	}
	return "http"
}

// SetRateLimiter attaches a rate limiter to the edge router.
// Must be called before Start().
func (e *EdgeRouter) SetRateLimiter(rl *RateLimiter) {
	e.rateLimiter = rl
}

// extractIP parses a net.IP from an addr string like "1.2.3.4:5678".
func extractIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.ParseIP(host)
}
