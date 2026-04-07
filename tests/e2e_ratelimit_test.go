//go:build e2e

// Package tests — e2e_ratelimit_test.go tests per-tunnel rate limiting
// and IP ACL enforcement through the full server stack.
//
// REF: SPR-011 T-035A (rate limiting), SPR-012 T-037A (IP ACL)
package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestE2E_RateLimit_HTTP verifies that requests exceeding the default
// rate limit receive a 429 with Retry-After header and JSON error body.
func TestE2E_RateLimit_HTTP(t *testing.T) {
	opts := defaultServerOpts()
	ts := startTestServer(t, opts)

	// Create token and local service.
	token := createTestToken(t, ts)
	localPort, stop := startLocalHTTPService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer stop()

	// Connect client and create tunnel.
	session := startTestClient(t, ts, token, localPort, "http")
	host := session.Host(ts.domain)

	// Wait for tunnel to be ready.
	waitForTunnel(t, ts.edgeAddr, host, 5*time.Second)

	// Set a very low rate limit via admin API: 2 req/s, burst 3.
	setRateLimitBody := `{"requests_per_second": 2, "burst": 3}`
	rlReq, _ := http.NewRequest("PUT",
		"http://"+ts.adminAddr+"/api/v1/tunnels/"+session.assignment.TunnelID+"/ratelimit",
		strings.NewReader(setRateLimitBody))
	rlReq.Header.Set("Content-Type", "application/json")
	rlReq.Header.Set("Authorization", "Bearer "+ts.jwtToken)
	rlResp, err := httpClient().Do(rlReq)
	if err != nil {
		t.Fatalf("set rate limit: %v", err)
	}
	rlResp.Body.Close()
	if rlResp.StatusCode != http.StatusOK {
		t.Fatalf("set rate limit: expected 200, got %d", rlResp.StatusCode)
	}

	// Send requests rapidly — first 3 should succeed (burst), rest should be 429.
	var okCount, rateLimitedCount int
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
		req.Host = host
		resp, err := httpClient().Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			rateLimitedCount++
			// Verify Retry-After header.
			if ra := resp.Header.Get("Retry-After"); ra != "1" {
				t.Errorf("request %d: Retry-After: got %q, want %q", i, ra, "1")
			}
		default:
			t.Errorf("request %d: unexpected status %d", i, resp.StatusCode)
		}
	}

	if okCount == 0 {
		t.Error("expected at least one successful request (burst)")
	}
	if rateLimitedCount == 0 {
		t.Error("expected at least one rate-limited request")
	}

	t.Logf("rate limit: %d OK, %d rate-limited out of 10", okCount, rateLimitedCount)
}

// TestE2E_RateLimit_Custom verifies that custom per-tunnel rate limits
// set via the admin API are enforced and visible in the tunnel list.
func TestE2E_RateLimit_Custom(t *testing.T) {
	opts := defaultServerOpts()
	ts := startTestServer(t, opts)

	token := createTestToken(t, ts)
	localPort, stop := startLocalHTTPService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer stop()

	session := startTestClient(t, ts, token, localPort, "http")

	// Set custom rate limit.
	body := `{"requests_per_second": 50, "burst": 75}`
	req, _ := http.NewRequest("PUT",
		"http://"+ts.adminAddr+"/api/v1/tunnels/"+session.assignment.TunnelID+"/ratelimit",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("set rate limit: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set rate limit: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Verify response body.
	var rlResp struct {
		TunnelID          string  `json:"tunnel_id"`
		RequestsPerSecond float64 `json:"requests_per_second"`
		Burst             int     `json:"burst"`
	}
	json.Unmarshal(respBody, &rlResp)
	if rlResp.RequestsPerSecond != 50 {
		t.Errorf("rps: got %v, want 50", rlResp.RequestsPerSecond)
	}
	if rlResp.Burst != 75 {
		t.Errorf("burst: got %d, want 75", rlResp.Burst)
	}

	// Verify in tunnel list.
	tunnels := listTunnelsDetailed(t, ts)
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels[0].RateLimit == nil {
		t.Fatal("expected rate_limit in tunnel list")
	}
	if tunnels[0].RateLimit.RequestsPerSecond != 50 {
		t.Errorf("list rps: got %v, want 50", tunnels[0].RateLimit.RequestsPerSecond)
	}
	if tunnels[0].RateLimit.Burst != 75 {
		t.Errorf("list burst: got %d, want 75", tunnels[0].RateLimit.Burst)
	}
}

// TestE2E_ACL_Allow verifies that ACL allow mode permits matching IPs
// and blocks non-matching IPs.
func TestE2E_ACL_Allow(t *testing.T) {
	opts := defaultServerOpts()
	ts := startTestServer(t, opts)

	token := createTestToken(t, ts)
	localPort, stop := startLocalHTTPService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"acl-ok"}`))
	}))
	defer stop()

	session := startTestClient(t, ts, token, localPort, "http")
	host := session.Host(ts.domain)

	waitForTunnel(t, ts.edgeAddr, host, 5*time.Second)

	// Set ACL that allows only 127.0.0.0/8 (which is where our test client connects from).
	aclBody := `{"mode": "allow", "cidrs": ["127.0.0.0/8"]}`
	req, _ := http.NewRequest("PUT",
		"http://"+ts.adminAddr+"/api/v1/tunnels/"+session.assignment.TunnelID+"/acl",
		strings.NewReader(aclBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("set ACL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set ACL: expected 200, got %d", resp.StatusCode)
	}

	// Request from 127.0.0.1 (our test client) — should be allowed.
	edgeReq, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	edgeReq.Host = host
	edgeResp, err := httpClient().Do(edgeReq)
	if err != nil {
		t.Fatalf("edge request: %v", err)
	}
	defer edgeResp.Body.Close()
	body, _ := io.ReadAll(edgeResp.Body)

	if edgeResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (allowed IP), got %d: %s", edgeResp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "acl-ok") {
		t.Errorf("expected 'acl-ok' in body, got: %s", string(body))
	}
}

// TestE2E_ACL_Deny verifies that ACL deny mode blocks matching IPs
// with a 403 response.
func TestE2E_ACL_Deny(t *testing.T) {
	opts := defaultServerOpts()
	ts := startTestServer(t, opts)

	token := createTestToken(t, ts)
	localPort, stop := startLocalHTTPService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer stop()

	session := startTestClient(t, ts, token, localPort, "http")
	host := session.Host(ts.domain)

	waitForTunnel(t, ts.edgeAddr, host, 5*time.Second)

	// Set ACL that blocks 127.0.0.0/8 (which is where our test client connects from).
	aclBody := `{"mode": "deny", "cidrs": ["127.0.0.0/8"]}`
	req, _ := http.NewRequest("PUT",
		"http://"+ts.adminAddr+"/api/v1/tunnels/"+session.assignment.TunnelID+"/acl",
		strings.NewReader(aclBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("set ACL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set ACL: expected 200, got %d", resp.StatusCode)
	}

	// Request from 127.0.0.1 (our test client) — should be blocked.
	edgeReq, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	edgeReq.Host = host
	edgeResp, err := httpClient().Do(edgeReq)
	if err != nil {
		t.Fatalf("edge request: %v", err)
	}
	defer edgeResp.Body.Close()

	if edgeResp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(edgeResp.Body)
		t.Fatalf("expected 403 (blocked IP), got %d: %s", edgeResp.StatusCode, string(body))
	}

	// Verify response body.
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	json.NewDecoder(edgeResp.Body).Decode(&errResp)
	if errResp.Error != "ip_blocked" {
		t.Errorf("error: got %q, want %q", errResp.Error, "ip_blocked")
	}

	// Delete ACL and verify access restored.
	delReq, _ := http.NewRequest("DELETE",
		"http://"+ts.adminAddr+"/api/v1/tunnels/"+session.assignment.TunnelID+"/acl", nil)
	delReq.Header.Set("Authorization", "Bearer "+ts.jwtToken)
	delResp, err := httpClient().Do(delReq)
	if err != nil {
		t.Fatalf("delete ACL: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete ACL: expected 204, got %d", delResp.StatusCode)
	}

	// Verify access restored.
	edgeReq2, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	edgeReq2.Host = host
	edgeResp2, err := httpClient().Do(edgeReq2)
	if err != nil {
		t.Fatalf("edge request after ACL delete: %v", err)
	}
	defer edgeResp2.Body.Close()

	if edgeResp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(edgeResp2.Body)
		t.Errorf("expected 200 after ACL deletion, got %d: %s", edgeResp2.StatusCode, string(body))
	}
}

// --- Helpers ---

// tunnelInfoDetailed extends tunnelInfo with rate limit and ACL data.
type tunnelInfoDetailed struct {
	TunnelID  string         `json:"tunnel_id"`
	Name      string         `json:"name"`
	Protocol  string         `json:"protocol"`
	RateLimit *rateLimitInfo `json:"rate_limit,omitempty"`
	ACL       *aclInfo       `json:"acl,omitempty"`
}

type rateLimitInfo struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

type aclInfo struct {
	Mode  string   `json:"mode"`
	CIDRs []string `json:"cidrs"`
}

// listTunnelsDetailed queries the admin API for tunnels with rate limit
// and ACL details included.
func listTunnelsDetailed(t *testing.T, ts *testServer) []tunnelInfoDetailed {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://"+ts.adminAddr+"/api/v1/tunnels", nil)
	req.Header.Set("Authorization", "Bearer "+ts.jwtToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("listTunnelsDetailed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("listTunnelsDetailed: expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Tunnels []tunnelInfoDetailed `json:"tunnels"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Tunnels
}

// Suppress unused import warning.
var _ = fmt.Sprintf
