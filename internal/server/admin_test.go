package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	tm := NewTunnelManager("tunnel.test.com", st, testLogger())
	admin := NewAdminAPI(AdminConfig{Addr: "127.0.0.1:0"}, st, tm, nil, testLogger())
	return admin, tm, st
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

	req := httptest.NewRequest("GET", "/api/v1/tokens", nil)
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

	body := `{"name": "test-token"}`
	req := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(body))
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
			req := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(tt.body))
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

	// Create a token.
	body := `{"name": "my-token"}`
	req := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(body))
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	// List tokens.
	req = httptest.NewRequest("GET", "/api/v1/tokens", nil)
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

	// Create a token.
	tok, _, _ := st.CreateToken("to-delete")

	req := httptest.NewRequest("DELETE", "/api/v1/tokens/"+tok.ID, nil)
	w := httptest.NewRecorder()
	admin.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestDeleteTokenNotFound(t *testing.T) {
	admin, _, st := newTestAdminAPI(t)
	defer st.Close()

	req := httptest.NewRequest("DELETE", "/api/v1/tokens/tok_nonexistent", nil)
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

	tok, _, _ := st.CreateToken("connected-token")

	// Register tunnels with this token.
	session := &Session{TokenID: tok.ID, RemoteAddr: "127.0.0.1:1234"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3001, Protocol: "http"})

	if len(tm.ListActive()) != 2 {
		t.Fatalf("expected 2 active tunnels before delete")
	}

	req := httptest.NewRequest("DELETE", "/api/v1/tokens/"+tok.ID, nil)
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

	session := &Session{TokenID: "tok_test", RemoteAddr: "127.0.0.1:9999"}
	tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := httptest.NewRequest("GET", "/api/v1/tunnels", nil)
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

func TestDeleteTunnel(t *testing.T) {
	admin, tm, st := newTestAdminAPI(t)
	defer st.Close()

	session := &Session{TokenID: "tok_test"}
	a, _ := tm.Register(session, &proto.TunnelRequest{LocalPort: 3000, Protocol: "http"})

	req := httptest.NewRequest("DELETE", "/api/v1/tunnels/"+a.TunnelID, nil)
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

	req := httptest.NewRequest("DELETE", "/api/v1/tunnels/nonexistent", nil)
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
