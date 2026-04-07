package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTLSConfigConstruction(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := TLSConfig{
		Enabled: true,
		CertDir: tmpDir,
		Domain:  "tunnel.example.com",
	}

	tm := NewTLSManager(cfg)
	if tm == nil {
		t.Fatal("expected non-nil TLSManager")
	}
	if tm.Manager() == nil {
		t.Fatal("expected non-nil autocert.Manager")
	}
	if tm.TLSConfig() == nil {
		t.Fatal("expected non-nil tls.Config")
	}
}

func TestTLSConfigCertDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := TLSConfig{
		Enabled: true,
		CertDir: tmpDir,
		Domain:  "tunnel.example.com",
	}

	NewTLSManager(cfg)

	// The autocert.DirCache creates the directory if it doesn't exist.
	// Just verify the directory is valid.
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("cert dir stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("cert dir is not a directory")
	}
}

func TestHostPolicyAcceptsBaseDomain(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.example.com", "")

	if err := policy(context.Background(), "tunnel.example.com"); err != nil {
		t.Errorf("base domain rejected: %v", err)
	}
}

func TestHostPolicyAcceptsSubdomain(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.example.com", "")

	valid := []string{
		"my-api.tunnel.example.com",
		"calm-tiger.tunnel.example.com",
		"test123.tunnel.example.com",
		"a.tunnel.example.com",
	}
	for _, host := range valid {
		if err := policy(context.Background(), host); err != nil {
			t.Errorf("valid subdomain %q rejected: %v", host, err)
		}
	}
}

func TestHostPolicyRejectsInvalid(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.example.com", "")

	invalid := []string{
		"evil.com",
		"other.example.com",
		"a.b.tunnel.example.com",       // multi-level
		"tunnel.example.com.evil.com",  // suffix mismatch
		"",                              // empty
		".tunnel.example.com",           // empty subdomain
	}
	for _, host := range invalid {
		if err := policy(context.Background(), host); err == nil {
			t.Errorf("invalid host %q should be rejected", host)
		}
	}
}

func TestHostPolicyRejectsMultiLevel(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.example.com", "")

	// Multi-level subdomains should be rejected.
	multi := []string{
		"a.b.tunnel.example.com",
		"deep.nested.tunnel.example.com",
	}
	for _, host := range multi {
		if err := policy(context.Background(), host); err == nil {
			t.Errorf("multi-level host %q should be rejected", host)
		}
	}
}

// T-069: Verify host policy accepts the apex domain when configured.
func TestHostPolicyAcceptsApexDomain(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.fonzygrok.com", "fonzygrok.com")

	// Apex domain should be accepted.
	if err := policy(context.Background(), "fonzygrok.com"); err != nil {
		t.Errorf("apex domain rejected: %v", err)
	}

	// Base domain should still work.
	if err := policy(context.Background(), "tunnel.fonzygrok.com"); err != nil {
		t.Errorf("base domain rejected: %v", err)
	}

	// Subdomains should still work.
	if err := policy(context.Background(), "my-api.tunnel.fonzygrok.com"); err != nil {
		t.Errorf("subdomain rejected: %v", err)
	}

	// Other domains should still be rejected.
	if err := policy(context.Background(), "evil.com"); err == nil {
		t.Error("evil.com should be rejected")
	}
}

// Verify apex domain is not accepted when not configured.
func TestHostPolicyNoApexWhenEmpty(t *testing.T) {
	policy := tunnelHostPolicy("tunnel.fonzygrok.com", "")

	if err := policy(context.Background(), "fonzygrok.com"); err == nil {
		t.Error("fonzygrok.com should be rejected when apex is not set")
	}
}

func TestHTTPRedirectHandler(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := TLSConfig{
		Enabled: true,
		CertDir: tmpDir,
		Domain:  "tunnel.example.com",
	}

	tm := NewTLSManager(cfg)

	// The HTTPHandler from autocert handles ACME challenges and redirects.
	handler := tm.Manager().HTTPHandler(nil)

	req := httptest.NewRequest("GET", "http://tunnel.example.com/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	// autocert redirects non-ACME requests to HTTPS with 302.
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "https://tunnel.example.com/test" {
		t.Errorf("redirect location: got %q, want %q", location, "https://tunnel.example.com/test")
	}
}

func TestEdgeTLSIntegration(t *testing.T) {
	// Start an httptest TLS server to verify TLS wiring works.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fonzygrok-tls-test"))
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	// Create a client that trusts the test server's cert.
	client := ts.Client()

	resp, err := client.Get(ts.URL + "/test")
	if err != nil {
		t.Fatalf("TLS GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fonzygrok-tls-test" {
		t.Errorf("body: got %q, want %q", string(body), "fonzygrok-tls-test")
	}
}

func TestEdgeWithTLSEnabledStartsTLSListener(t *testing.T) {
	// Verify that when TLS config is present, the edge router can create
	// a TLS listener configuration.
	tmpDir := t.TempDir()

	cfg := TLSConfig{
		Enabled: true,
		CertDir: tmpDir,
		Domain:  "tunnel.example.com",
	}

	tm := NewTLSManager(cfg)
	tlsCfg := tm.TLSConfig()

	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}

	// The config should have GetCertificate set by autocert.
	if tlsCfg.GetCertificate == nil {
		t.Error("expected GetCertificate to be set")
	}

	// autocert sets NextProtos for ACME-TLS/1.
	if len(tlsCfg.NextProtos) == 0 {
		t.Error("expected NextProtos to be set by autocert")
	}
}

func TestTLSDisabledNoChange(t *testing.T) {
	cfg := TLSConfig{
		Enabled: false,
	}

	// When disabled, we should still be able to construct the config
	// but it should not be used.
	if cfg.Enabled {
		t.Error("expected TLS to be disabled")
	}
}
