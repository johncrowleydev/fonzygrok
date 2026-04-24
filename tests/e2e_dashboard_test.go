//go:build e2e

// Package tests — e2e_dashboard_test.go contains dashboard E2E tests.
//
// These tests verify that the dashboard is served on the edge router's
// apex/base domain fallback, that static assets work, that auth redirects
// happen, and that the theme toggle element exists.
//
// REF: SPR-013 T-039
package tests

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestE2E_Dashboard_ServedOnEdge verifies that GET to the base domain
// (apex fallback) returns HTML containing "Fonzygrok".
//
// When no subdomain is present, the edge router delegates to the
// dashboard handler, which redirects / → /login. The login page
// should contain "Fonzygrok" in the HTML title.
func TestE2E_Dashboard_ServedOnEdge(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	// Request the base domain (no subdomain). The edge router's
	// handleRequest sees tunnelID=="" and delegates to baseDomainHandler.
	// The dashboard's handleRoot redirects to /login.
	client := &http.Client{
		Timeout: httpClient().Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow redirects but rewrite the host so the edge still
			// routes to the dashboard (not to a random subdomain).
			req.Host = ts.domain
			return nil
		},
	}

	req, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/", nil)
	req.Host = ts.domain

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// The layout template title includes "Fonzygrok".
	if !strings.Contains(html, "Fonzygrok") {
		t.Errorf("response body does not contain 'Fonzygrok'; got %d bytes of HTML", len(html))
	}

	// Should be served as HTML.
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}

	t.Logf("Dashboard served on edge: %d bytes, Content-Type: %s", len(html), ct)
}

// TestE2E_Dashboard_ThemeToggleExists verifies that the dashboard HTML
// contains the theme-toggle-btn element.
func TestE2E_Dashboard_ThemeToggleExists(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	// We need to be authenticated to see the theme toggle (it's in the nav
	// of authenticated pages). Log in via the dashboard.
	// First, create an admin session cookie.
	loginBody := "username=" + ts.adminUsername + "&password=e2etestpassword1"
	loginReq, _ := http.NewRequest("POST", "http://"+ts.edgeAddr+"/login",
		strings.NewReader(loginBody))
	loginReq.Host = ts.domain
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Don't follow redirects — we want the Set-Cookie from POST /login.
	noRedirectClient := &http.Client{
		Timeout: httpClient().Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginResp, err := noRedirectClient.Do(loginReq)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", loginResp.StatusCode)
	}

	// Extract session cookie.
	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie set after login")
	}

	// Fetch the dashboard page with the session cookie.
	dashReq, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/dashboard", nil)
	dashReq.Host = ts.domain
	dashReq.AddCookie(sessionCookie)

	dashResp, err := httpClient().Do(dashReq)
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer dashResp.Body.Close()

	if dashResp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", dashResp.StatusCode)
	}

	body, _ := io.ReadAll(dashResp.Body)
	html := string(body)

	if !strings.Contains(html, "theme-toggle-btn") {
		t.Error("dashboard HTML does not contain 'theme-toggle-btn' element")
	}

	t.Log("Theme toggle button found in dashboard HTML")
}

// TestE2E_Dashboard_LoginRequired verifies that accessing an authenticated
// dashboard route without a session redirects to /login.
func TestE2E_Dashboard_LoginRequired(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	// Don't follow redirects — we want to verify the 303 redirect.
	noRedirect := &http.Client{
		Timeout: httpClient().Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Try to access /dashboard without session cookie.
	req, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/dashboard", nil)
	req.Host = ts.domain

	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/login") {
		t.Errorf("expected redirect to /login, got Location: %s", location)
	}

	t.Logf("Unauthenticated /dashboard correctly redirects to: %s", location)
}

// TestE2E_Dashboard_StaticAssets verifies that the CSS static asset
// is served with the correct Content-Type.
func TestE2E_Dashboard_StaticAssets(t *testing.T) {
	ts := startTestServer(t, defaultServerOpts())

	req, _ := http.NewRequest("GET", "http://"+ts.edgeAddr+"/static/style.css", nil)
	req.Host = ts.domain

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("GET /static/style.css: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected Content-Type text/css, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) < 100 {
		t.Errorf("style.css is suspiciously small: %d bytes", len(body))
	}

	t.Logf("Static CSS served: %d bytes, Content-Type: %s", len(body), ct)
}
