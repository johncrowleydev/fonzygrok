package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

// AdminConfig holds configuration for the admin API server.
type AdminConfig struct {
	// Addr is the listen address (e.g., "127.0.0.1:9090").
	Addr string
}

// AdminAPI serves the admin REST API per CON-002 §4.
// It provides health, auth, token management, and tunnel management endpoints.
type AdminAPI struct {
	config    AdminConfig
	store     *store.Store
	jwt       *auth.JWTManager
	tunnels   *TunnelManager
	sshServer *SSHServer
	logger    *slog.Logger
	server    *http.Server
	mux       *http.ServeMux
	startTime time.Time
}

// namePattern validates token names: 1-64 chars, alphanumeric + hyphens.
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,63}$`)

// NewAdminAPI creates a new admin API server.
func NewAdminAPI(config AdminConfig, st *store.Store, jwtMgr *auth.JWTManager, tunnels *TunnelManager, sshSrv *SSHServer, logger *slog.Logger) *AdminAPI {
	a := &AdminAPI{
		config:    config,
		store:     st,
		jwt:       jwtMgr,
		tunnels:   tunnels,
		sshServer: sshSrv,
		logger:    logger,
		startTime: time.Now().UTC(),
	}

	a.mux = http.NewServeMux()

	// Public endpoints (no auth).
	a.mux.HandleFunc("/api/v1/health", a.methodRoute(map[string]http.HandlerFunc{
		"GET": a.handleHealth,
	}))
	a.mux.HandleFunc("/api/v1/register", a.methodRoute(map[string]http.HandlerFunc{
		"POST": a.handleRegister,
	}))
	a.mux.HandleFunc("/api/v1/login", a.methodRoute(map[string]http.HandlerFunc{
		"POST": a.handleLogin,
	}))
	a.mux.HandleFunc("/api/v1/logout", a.methodRoute(map[string]http.HandlerFunc{
		"POST": a.handleLogout,
	}))

	// Authenticated endpoints.
	a.mux.Handle("/api/v1/me", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{"GET": a.handleMe}),
	)))
	a.mux.Handle("/api/v1/tokens", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"GET":  a.handleListTokensAuth,
			"POST": a.handleCreateTokenAuth,
		}),
	)))
	a.mux.Handle("/api/v1/tokens/", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"DELETE": a.handleDeleteTokenAuth,
		}),
	)))

	// Admin-only endpoints.
	a.mux.Handle("/api/v1/invite-codes", a.AuthMiddleware(a.RequireRole("admin", http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"GET":  a.handleListInviteCodes,
			"POST": a.handleCreateInviteCode,
		}),
	))))
	a.mux.Handle("/api/v1/users", a.AuthMiddleware(a.RequireRole("admin", http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"GET": a.handleListUsers,
		}),
	))))

	// Existing admin-only endpoints (now behind auth).
	a.mux.Handle("/api/v1/tunnels", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"GET": a.handleListTunnels,
		}),
	)))
	a.mux.Handle("/api/v1/tunnels/", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"DELETE": a.handleDeleteTunnel,
		}),
	)))
	a.mux.Handle("/api/v1/metrics", a.AuthMiddleware(http.HandlerFunc(
		a.methodRouteHandler(map[string]http.HandlerFunc{
			"GET": a.handleMetrics,
		}),
	)))

	a.server = &http.Server{
		Addr:    config.Addr,
		Handler: corsMiddleware(a.mux),
	}

	return a
}

// Start begins serving the admin API.
func (a *AdminAPI) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return fmt.Errorf("admin: listen %s: %w", a.config.Addr, err)
	}

	a.logger.Info("admin API listening", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		a.Stop()
	}()

	if err := a.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("admin: serve: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the admin API.
func (a *AdminAPI) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.server.Shutdown(ctx)
}

// Handler returns the http.Handler for testing.
func (a *AdminAPI) Handler() http.Handler {
	return a.server.Handler
}

// Mux returns the underlying ServeMux for registering additional routes
// (e.g., the dashboard UI).
func (a *AdminAPI) Mux() *http.ServeMux {
	return a.mux
}

// --- Route helpers ---

// methodRoute dispatches requests by HTTP method.
func (a *AdminAPI) methodRoute(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := handlers[r.Method]; ok {
			handler(w, r)
			return
		}
		a.writeAdminError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			fmt.Sprintf("Method %s not allowed", r.Method))
	}
}

// methodRouteHandler is like methodRoute but returns a regular HandlerFunc
// (for use with middleware wrappers that expect http.Handler).
func (a *AdminAPI) methodRouteHandler(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return a.methodRoute(handlers)
}

// corsMiddleware adds CORS headers for web dashboard access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Endpoint handlers ---

// handleHealth returns system health per CON-002 §4.3.
func (a *AdminAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(a.startTime).Seconds()
	tunnelCount := len(a.tunnels.ListActive())
	clientCount := 0
	if a.sshServer != nil {
		clientCount = a.sshServer.ActiveSessions()
	}

	resp := struct {
		Status           string  `json:"status"`
		Version          string  `json:"version"`
		UptimeSeconds    float64 `json:"uptime_seconds"`
		TunnelsActive    int     `json:"tunnels_active"`
		ClientsConnected int     `json:"clients_connected"`
	}{
		Status:           "healthy",
		Version:          Version,
		UptimeSeconds:    uptime,
		TunnelsActive:    tunnelCount,
		ClientsConnected: clientCount,
	}

	a.writeJSON(w, http.StatusOK, resp)
}

// handleListTokens returns all tokens per CON-002 §4.3.
func (a *AdminAPI) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := a.store.ListTokens()
	if err != nil {
		a.logger.Error("admin: list tokens", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to list tokens")
		return
	}

	type tokenResp struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		CreatedAt  string  `json:"created_at"`
		LastUsedAt *string `json:"last_used_at"`
		IsActive   bool    `json:"is_active"`
	}

	items := make([]tokenResp, 0, len(tokens))
	for _, t := range tokens {
		item := tokenResp{
			ID:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
			IsActive:  t.IsActive,
		}
		if t.LastUsedAt != nil {
			s := t.LastUsedAt.Format(time.RFC3339)
			item.LastUsedAt = &s
		}
		items = append(items, item)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": items})
}

// handleCreateToken creates a new token per CON-002 §4.3.
func (a *AdminAPI) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Invalid JSON body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Field 'name' is required")
		return
	}
	if !namePattern.MatchString(req.Name) {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error",
			"Field 'name' must be 1-64 chars, alphanumeric and hyphens only")
		return
	}

	tok, rawToken, err := a.store.CreateToken(req.Name)
	if err != nil {
		a.logger.Error("admin: create token", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to create token")
		return
	}

	a.logger.Info("admin: token created", "token_id", tok.ID, "name", req.Name)

	resp := struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Token     string `json:"token"`
		CreatedAt string `json:"created_at"`
	}{
		ID:        tok.ID,
		Name:      tok.Name,
		Token:     rawToken,
		CreatedAt: tok.CreatedAt.Format(time.RFC3339),
	}

	a.writeJSON(w, http.StatusCreated, resp)
}

// handleDeleteToken revokes a token per CON-002 §4.3.
func (a *AdminAPI) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	tokenID := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	if tokenID == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Token ID is required")
		return
	}

	if err := a.store.DeleteToken(tokenID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			a.writeAdminError(w, http.StatusNotFound, "not_found", "Token not found")
			return
		}
		a.logger.Error("admin: delete token", "error", err)
		a.writeAdminError(w, http.StatusInternalServerError, "internal_error", "Failed to delete token")
		return
	}

	// Disconnect any tunnels using this token.
	a.disconnectTunnelsByToken(tokenID)

	a.logger.Info("admin: token revoked", "token_id", tokenID)
	w.WriteHeader(http.StatusNoContent)
}

// handleListTunnels returns all active tunnels per CON-002 §4.3.
func (a *AdminAPI) handleListTunnels(w http.ResponseWriter, r *http.Request) {
	active := a.tunnels.ListActive()

	type tunnelResp struct {
		TunnelID        string `json:"tunnel_id"`
		Name            string `json:"name"`
		Subdomain       string `json:"subdomain"`
		PublicURL       string `json:"public_url"`
		Protocol        string `json:"protocol"`
		ClientIP        string `json:"client_ip"`
		TokenID         string `json:"token_id"`
		LocalPort       int    `json:"local_port"`
		AssignedPort    int    `json:"assigned_port,omitempty"`
		ConnectedAt     string `json:"connected_at"`
		BytesIn         int64  `json:"bytes_in"`
		BytesOut        int64  `json:"bytes_out"`
		RequestsProxied int64  `json:"requests_proxied"`
		LastRequestAt   string `json:"last_request_at,omitempty"`
	}

	items := make([]tunnelResp, 0, len(active))
	for _, entry := range active {
		item := tunnelResp{
			TunnelID:     entry.TunnelID,
			Name:         entry.Name,
			Subdomain:    entry.Subdomain,
			PublicURL:    entry.PublicURL,
			Protocol:     entry.Protocol,
			LocalPort:    entry.LocalPort,
			AssignedPort: entry.AssignedPort,
			ConnectedAt:  entry.CreatedAt.Format(time.RFC3339),
		}
		if entry.Session != nil {
			item.ClientIP = entry.Session.RemoteAddr
			item.TokenID = entry.Session.TokenID
		}
		if entry.Metrics != nil {
			snap := entry.Metrics.Snapshot()
			item.BytesIn = snap.BytesIn
			item.BytesOut = snap.BytesOut
			item.RequestsProxied = snap.RequestsProxied
			if !snap.LastRequestAt.IsZero() {
				item.LastRequestAt = snap.LastRequestAt.Format(time.RFC3339)
			}
		}
		items = append(items, item)
	}

	a.writeJSON(w, http.StatusOK, map[string]interface{}{"tunnels": items})
}

// handleDeleteTunnel force-closes a tunnel per CON-002 §4.3.
func (a *AdminAPI) handleDeleteTunnel(w http.ResponseWriter, r *http.Request) {
	tunnelID := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/")
	if tunnelID == "" {
		a.writeAdminError(w, http.StatusBadRequest, "validation_error", "Tunnel ID is required")
		return
	}

	_, ok := a.tunnels.Lookup(tunnelID)
	if !ok {
		a.writeAdminError(w, http.StatusNotFound, "not_found", "Tunnel not found")
		return
	}

	a.tunnels.Deregister(tunnelID)
	a.logger.Info("admin: tunnel force-closed", "tunnel_id", tunnelID)
	w.WriteHeader(http.StatusNoContent)
}

// handleMetrics returns aggregated metrics across all active tunnels.
func (a *AdminAPI) handleMetrics(w http.ResponseWriter, r *http.Request) {
	active := a.tunnels.ListActive()

	var totalBytesIn, totalBytesOut, totalRequests int64

	// Count unique sessions for active clients.
	sessions := make(map[*Session]bool)

	for _, entry := range active {
		if entry.Metrics != nil {
			snap := entry.Metrics.Snapshot()
			totalBytesIn += snap.BytesIn
			totalBytesOut += snap.BytesOut
			totalRequests += snap.RequestsProxied
		}
		if entry.Session != nil {
			sessions[entry.Session] = true
		}
	}

	uptime := time.Since(a.startTime).Seconds()

	resp := struct {
		TotalBytesIn       int64   `json:"total_bytes_in"`
		TotalBytesOut      int64   `json:"total_bytes_out"`
		TotalRequestsProxied int64 `json:"total_requests_proxied"`
		ActiveTunnels      int     `json:"active_tunnels"`
		ActiveClients      int     `json:"active_clients"`
		UptimeSeconds      float64 `json:"uptime_seconds"`
	}{
		TotalBytesIn:         totalBytesIn,
		TotalBytesOut:        totalBytesOut,
		TotalRequestsProxied: totalRequests,
		ActiveTunnels:        len(active),
		ActiveClients:        len(sessions),
		UptimeSeconds:        uptime,
	}

	a.writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

// disconnectTunnelsByToken removes all tunnels belonging to the given token.
func (a *AdminAPI) disconnectTunnelsByToken(tokenID string) {
	active := a.tunnels.ListActive()
	for _, entry := range active {
		if entry.Session != nil && entry.Session.TokenID == tokenID {
			a.tunnels.Deregister(entry.TunnelID)
			a.logger.Info("admin: tunnel disconnected (token revoked)",
				"tunnel_id", entry.TunnelID,
				"token_id", tokenID,
			)
		}
	}
}

// writeJSON writes a JSON response with the given status code.
func (a *AdminAPI) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeAdminError writes a JSON error response per CON-002 §5.
func (a *AdminAPI) writeAdminError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
