package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/proto"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

// --- Test helpers ---

func newTestAdminAPI(t *testing.T) (*AdminAPI, *TunnelManager, *store.Store) {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}

	jwtMgr, _ := auth.NewJWTManager("", 1*time.Hour)
	tm := NewTunnelManager("tunnel.test.com", st, testLogger())
	admin := NewAdminAPI(AdminConfig{Addr: "127.0.0.1:0"}, st, jwtMgr, tm, nil, testLogger())
	return admin, tm, st
}

// addTestAuth creates an admin user and returns a Bearer token for requests.
func addTestAuth(t *testing.T, admin *AdminAPI, st *store.Store) string {
	t.Helper()
	hash, _ := auth.HashPassword("testpassword1")
	user, _ := st.CreateUser("testadmin", "admin@test.com", hash, "admin")
	token, _ := admin.jwt.CreateToken(auth.Claims{
		UserID: user.ID, Username: user.Username, Role: user.Role,
	})
	return token
}

// authReq adds Bearer auth header to a request.
func authReq(r *http.Request, token string) *http.Request {
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// --- T-013A: Health ---

func TestHealthEndpoint(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Status           string  `json:"status"`
		Version          string  `json:"version"`
		UptimeSeconds    float64 `json:"uptime_seconds"`
		TunnelsActive    int     `json:"tunnels_active"`
		ClientsConnected int     `json:"clients_connected"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("status: got %q, want %q", resp.Status, "healthy")
	}
	if resp.UptimeSeconds < 0 {
		t.Error("uptime should be non-negative")
	}
}

func TestHealthTunnelCount(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		TunnelsActive int `json:"tunnels_active"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TunnelsActive != 1 {
		t.Errorf("tunnels_active: got %d, want 1", resp.TunnelsActive)
	}
}

// --- T-013A: Tokens ---

func TestListTokensEmpty(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("GET", "/api/v1/tokens", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Tokens []interface{} `json:"tokens"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(resp.Tokens))
	}
}

func TestCreateToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	body := `{"name": "test-token"}`
	req := authReq(httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(body)), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Token     string `json:"token"`
		CreatedAt string `json:"created_at"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ID == "" {
		t.Error("expected non-empty id")
	}
	if resp.Name != "test-token" {
		t.Errorf("name: got %q, want %q", resp.Name, "test-token")
	}
	if resp.Token == "" {
		t.Error("expected non-empty token (raw token returned once)")
	}
	if !strings.HasPrefix(resp.Token, "fgk_") {
		t.Errorf("token should start with fgk_, got: %s", resp.Token)
	}
	if resp.CreatedAt == "" {
		t.Error("expected non-empty created_at")
	}
}

func TestCreateTokenValidation(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	tests := []struct {
		name string
		body string
	}{
		{"empty name", `{"name": ""}`},
		{"missing name", `{}`},
		{"invalid chars", `{"name": "bad name!"}`},
		{"too long", `{"name": "` + strings.Repeat("a", 65) + `"}`},
		{"invalid json", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authReq(httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(tt.body)), token)
			w := httptest.NewRecorder()
			admin.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d, body: %s", w.Code, w.Body.String())
			}

			var errResp map[string]string
			json.Unmarshal(w.Body.Bytes(), &errResp)
			if errResp["error"] != "validation_error" {
				t.Errorf("error: got %q, want %q", errResp["error"], "validation_error")
			}
		})
	}
}

func TestListTokensAfterCreate(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Create a token.
	body := `{"name": "my-token"}`
	req := authReq(httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(body)), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	// List tokens.
	req = authReq(httptest.NewRequest("GET", "/api/v1/tokens", nil), token)
	w = httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		Tokens []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"tokens"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(resp.Tokens))
	}
	if resp.Tokens[0].Name != "my-token" {
		t.Errorf("name: got %q, want %q", resp.Tokens[0].Name, "my-token")
	}
	if !resp.Tokens[0].IsActive {
		t.Error("expected token to be active")
	}
}

func TestDeleteToken(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Create a token.
	tok, _, _ := st.CreateToken("to-delete")

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tokens/"+tok.ID, nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestDeleteTokenNotFound(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tokens/tok_nonexistent", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["error"] != "not_found" {
		t.Errorf("error: got %q, want %q", errResp["error"], "not_found")
	}
}

func TestDeleteTokenDisconnectsTunnels(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	jwtToken := addTestAuth(t, admin, st)

	tok, _, _ := st.CreateToken("connected-token")

	// Register tunnels with this token.
	session := &Session{TokenID: tok.ID, RemoteAddr: "127.0.0.1:1234"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http"})

	if len(tm.ListActive()) != 2 {
		t.Fatalf("expected 2 active tunnels before delete")
	}

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tokens/"+tok.ID, nil), jwtToken)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	if len(tm.ListActive()) != 0 {
		t.Errorf("expected 0 active tunnels after token delete, got %d", len(tm.ListActive()))
	}
}

// --- T-013A: Tunnels ---

func TestListTunnels(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := authReq(httptest.NewRequest("GET", "/api/v1/tunnels", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Tunnels []struct {
			TunnelID  string `json:"tunnel_id"`
			Subdomain string `json:"subdomain"`
			PublicURL string `json:"public_url"`
			Protocol  string `json:"protocol"`
			ClientIP  string `json:"client_ip"`
			TokenID   string `json:"token_id"`
			LocalPort int    `json:"local_port"`
		} `json:"tunnels"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(resp.Tunnels))
	}
	if resp.Tunnels[0].LocalPort != 3000 {
		t.Errorf("local_port: got %d, want 3000", resp.Tunnels[0].LocalPort)
	}
	if resp.Tunnels[0].TokenID != "tok_test" {
		t.Errorf("token_id: got %q, want %q", resp.Tunnels[0].TokenID, "tok_test")
	}
}

func TestListTunnelsTCP(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Set up TCP edge.
	tcpEdge := NewTCPEdge(50400, 50410, tm, testLogger())
	defer tcpEdge.Shutdown()
	tm.SetTCPEdge(tcpEdge)

	session := &Session{TokenID: "tok_tcp", RemoteAddr: "127.0.0.1:9999"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 5432, Protocol: "tcp"})

	req := authReq(httptest.NewRequest("GET", "/api/v1/tunnels", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Tunnels []struct {
			TunnelID     string `json:"tunnel_id"`
			Protocol     string `json:"protocol"`
			AssignedPort int    `json:"assigned_port"`
			LocalPort    int    `json:"local_port"`
		} `json:"tunnels"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(resp.Tunnels))
	}
	if resp.Tunnels[0].Protocol != "tcp" {
		t.Errorf("protocol: got %q, want %q", resp.Tunnels[0].Protocol, "tcp")
	}
	if resp.Tunnels[0].AssignedPort < 50400 || resp.Tunnels[0].AssignedPort > 50410 {
		t.Errorf("assigned_port out of range: got %d", resp.Tunnels[0].AssignedPort)
	}
	if resp.Tunnels[0].LocalPort != 5432 {
		t.Errorf("local_port: got %d, want 5432", resp.Tunnels[0].LocalPort)
	}
}

func TestDeleteTunnel(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tunnels/"+a.TunnelID, nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	if _, ok := tm.Lookup(a.TunnelID); ok {
		t.Error("tunnel should be deregistered after delete")
	}
}

func TestDeleteTunnelNotFound(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tunnels/nonexistent", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Method routing ---

func TestMethodNotAllowed(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("PUT", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}

	var errResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["error"] != "method_not_allowed" {
		t.Errorf("error: got %q, want %q", errResp["error"], "method_not_allowed")
	}
}

func TestContentTypeJSON(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// --- T-035A: Rate Limit Admin API ---

func TestSetRateLimit(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	// Configure a rate limiter.
	rl := NewRateLimiter(100, 200)
	tm.SetRateLimiter(rl)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	// PUT custom rate limit.
	body := `{"requests_per_second": 50, "burst": 100}`
	req := authReq(httptest.NewRequest("PUT", "/api/v1/tunnels/"+a.TunnelID+"/ratelimit", strings.NewReader(body)), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify the custom limit was set.
	rps, burst := rl.GetLimit(a.TunnelID)
	if rps != 50 || burst != 100 {
		t.Errorf("rate limit: got rps=%v burst=%d, want 50/100", rps, burst)
	}
}

func TestSetRateLimitNotFound(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	rl := NewRateLimiter(100, 200)
	tm.SetRateLimiter(rl)

	body := `{"requests_per_second": 50, "burst": 100}`
	req := authReq(httptest.NewRequest("PUT", "/api/v1/tunnels/nonexistent/ratelimit", strings.NewReader(body)), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestListTunnelsIncludesRateLimit(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	rl := NewRateLimiter(100, 200)
	tm.SetRateLimiter(rl)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := authReq(httptest.NewRequest("GET", "/api/v1/tunnels", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		Tunnels []struct {
			RateLimit struct {
				RequestsPerSecond float64 `json:"requests_per_second"`
				Burst             int     `json:"burst"`
			} `json:"rate_limit"`
		} `json:"tunnels"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(resp.Tunnels))
	}
	if resp.Tunnels[0].RateLimit.RequestsPerSecond != 100 {
		t.Errorf("rate_limit.rps: got %v, want 100", resp.Tunnels[0].RateLimit.RequestsPerSecond)
	}
	if resp.Tunnels[0].RateLimit.Burst != 200 {
		t.Errorf("rate_limit.burst: got %d, want 200", resp.Tunnels[0].RateLimit.Burst)
	}
}

// --- T-037A: ACL Admin API ---

func TestSetACL(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	body := `{"mode": "allow", "cidrs": ["10.0.0.0/8", "192.168.1.0/24"]}`
	req := authReq(httptest.NewRequest("PUT", "/api/v1/tunnels/"+a.TunnelID+"/acl", strings.NewReader(body)), token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify ACL was set.
	entry, _ := tm.Lookup(a.TunnelID)
	if entry.ACL == nil {
		t.Fatal("expected ACL to be set")
	}
	if entry.ACL.Mode != "allow" {
		t.Errorf("ACL mode: got %q, want %q", entry.ACL.Mode, "allow")
	}
	if len(entry.ACL.AllowCIDRs) != 2 {
		t.Errorf("ACL CIDRs: got %d, want 2", len(entry.ACL.AllowCIDRs))
	}
}

func TestDeleteACL(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	// Set ACL first.
	entry, _ := tm.Lookup(a.TunnelID)
	acl, _ := ParseACL("allow", []string{"10.0.0.0/8"})
	entry.ACL = acl

	// Delete it.
	req := authReq(httptest.NewRequest("DELETE", "/api/v1/tunnels/"+a.TunnelID+"/acl", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Verify ACL was removed.
	entry2, _ := tm.Lookup(a.TunnelID)
	if entry2.ACL != nil {
		t.Error("expected ACL to be nil after delete")
	}
}

func TestSetACLInvalidCIDR(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	body := `{"mode": "allow", "cidrs": ["not-a-cidr"]}`
	req := authReq(httptest.NewRequest("PUT", "/api/v1/tunnels/"+a.TunnelID+"/acl", strings.NewReader(body)), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListTunnelsIncludesACL(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()
	token := addTestAuth(t, admin, st)

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	// Set ACL.
	entry, _ := tm.Lookup(a.TunnelID)
	acl, _ := ParseACL("deny", []string{"192.168.0.0/16"})
	entry.ACL = acl

	req := authReq(httptest.NewRequest("GET", "/api/v1/tunnels", nil), token)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	var resp struct {
		Tunnels []struct {
			ACL *struct {
				Mode  string   `json:"mode"`
				CIDRs []string `json:"cidrs"`
			} `json:"acl"`
		} `json:"tunnels"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(resp.Tunnels))
	}
	if resp.Tunnels[0].ACL == nil {
		t.Fatal("expected ACL in tunnel list")
	}
	if resp.Tunnels[0].ACL.Mode != "deny" {
		t.Errorf("ACL mode: got %q, want %q", resp.Tunnels[0].ACL.Mode, "deny")
	}
	if len(resp.Tunnels[0].ACL.CIDRs) != 1 {
		t.Errorf("ACL CIDRs: got %d, want 1", len(resp.Tunnels[0].ACL.CIDRs))
	}
}
