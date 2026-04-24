package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fonzygrok/fonzygrok/internal/auth"
	"github.com/fonzygrok/fonzygrok/internal/store"
)

// testDashboard creates a Dashboard with in-memory dependencies for testing.
func testDashboard(t *testing.T) (*Dashboard, *auth.JWTManager, *store.Store) {
	t.Helper()

	st := newTestStore(t)
	t.Cleanup(func() { st.Close() })

	jwtMgr, err := auth.NewJWTManager("", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewJWTManager: %v", err)
	}

	tunnels := NewTunnelManager("test.com", st, slog.Default())

	d := NewDashboard(st, jwtMgr, tunnels, slog.Default())
	return d, jwtMgr, st
}

// setupDashboardMux registers dashboard routes on a mux for testing.
func setupDashboardMux(t *testing.T) (*http.ServeMux, *Dashboard, *auth.JWTManager, *store.Store) {
	t.Helper()
	d, jwt, st := testDashboard(t)
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)
	return mux, d, jwt, st
}

// createTestUser creates a user in the store and returns a valid JWT cookie.
func createTestUser(t *testing.T, st *store.Store, jwtMgr *auth.JWTManager, username, role string) *http.Cookie {
	t.Helper()
	hash, err := auth.HashPassword("testpassword")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user, err := st.CreateUser(username, username+"@test.com", hash, role)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := jwtMgr.CreateToken(auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

// ── Template Rendering Tests ─────────────────────────────────────────

func TestDashboardLoginPageRenders(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /login status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Sign in") {
		t.Error("login page should contain 'Sign in'")
	}
	if !strings.Contains(string(body), "fonzygrok") {
		t.Error("login page should contain 'fonzygrok'")
	}
}

func TestDashboardRegisterPageRenders(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/register", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /register status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Create your account") {
		t.Error("register page should contain 'Create your account'")
	}
	if !strings.Contains(string(body), "invite_code") {
		t.Error("register page should contain invite code field")
	}
}

func TestDashboardMainPageRendersAuth(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "dashuser", "user")

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "My Tokens") {
		t.Error("dashboard should contain 'My Tokens'")
	}
	if !strings.Contains(string(body), "Active Tunnels") {
		t.Error("dashboard should contain 'Active Tunnels'")
	}
}

// ── Auth Redirect Tests ──────────────────────────────────────────────

func TestDashboardRedirectsUnauthToDashboard(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /dashboard unauth status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("redirect location = %q, want /login", loc)
	}
}

func TestDashboardRootRedirect(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET / status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/login" {
		t.Errorf("redirect location = %q, want /login", loc)
	}
}

func TestDashboardRootRedirectAuth(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "rootuser", "user")

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET / auth status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("redirect location = %q, want /dashboard", loc)
	}
}

// ── Admin Role Tests ─────────────────────────────────────────────────

func TestAdminUsersAccessDeniedForNonAdmin(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "normaluser", "user")

	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /admin/users non-admin status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminUsersAccessAllowedForAdmin(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "adminuser", "admin")

	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/users admin status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Users") {
		t.Error("admin users page should contain 'Users'")
	}
}

func TestAdminInvitesAccessDeniedForNonAdmin(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "reguser", "user")

	req := httptest.NewRequest("GET", "/admin/invite-codes", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /admin/invite-codes non-admin status = %d, want 403", resp.StatusCode)
	}
}

// ── Login Flow Tests ─────────────────────────────────────────────────

func TestLoginSuccess(t *testing.T) {
	mux, _, _, st := setupDashboardMux(t)

	// Create user directly.
	hash, _ := auth.HashPassword("mypassword1")
	st.CreateUser("logintest", "logintest@test.com", hash, "user")

	form := url.Values{
		"username": {"logintest"},
		"password": {"mypassword1"},
	}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("redirect = %q, want /dashboard", loc)
	}
	// Check session cookie was set.
	var foundSession bool
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			foundSession = true
		}
	}
	if !foundSession {
		t.Error("session cookie should be set on login")
	}
}

func TestLoginBadPassword(t *testing.T) {
	mux, _, _, st := setupDashboardMux(t)

	hash, _ := auth.HashPassword("goodpass1")
	st.CreateUser("badpwuser", "badpw@test.com", hash, "user")

	form := url.Values{
		"username": {"badpwuser"},
		"password": {"wrongpass"},
	}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /login bad pw status = %d, want 200 (re-render)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid credentials") {
		t.Error("should show 'Invalid credentials'")
	}
}

func TestLoginByEmail(t *testing.T) {
	mux, _, _, st := setupDashboardMux(t)

	hash, _ := auth.HashPassword("emailpass1")
	st.CreateUser("emailuser", "emaillogin@test.com", hash, "user")

	form := url.Values{
		"username": {"emaillogin@test.com"},
		"password": {"emailpass1"},
	}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login by email status = %d, want 303", resp.StatusCode)
	}
}

// ── Registration Flow Tests ──────────────────────────────────────────

func TestRegistrationSuccess(t *testing.T) {
	mux, _, _, st := setupDashboardMux(t)

	// Create admin and invite code.
	hash, _ := auth.HashPassword("adminpass1")
	admin, _ := st.CreateUser("regadmin", "regadmin@test.com", hash, "admin")
	_, code, _ := st.CreateInviteCode(admin.ID)

	form := url.Values{
		"username":    {"newuser"},
		"email":       {"newuser@test.com"},
		"password":    {"securepass"},
		"invite_code": {code},
	}
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /register status = %d, want 303. body: %s", resp.StatusCode, string(body))
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("redirect = %q, want /dashboard", loc)
	}
}

func TestRegistrationBadInviteCode(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	form := url.Values{
		"username":    {"badcode"},
		"email":       {"badcode@test.com"},
		"password":    {"securepass"},
		"invite_code": {"INVALID1"},
	}
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid or already used invite code") {
		t.Error("should show invite code error")
	}
}

func TestRegistrationShortPassword(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	form := url.Values{
		"username":    {"shortpw"},
		"email":       {"shortpw@test.com"},
		"password":    {"short"},
		"invite_code": {"AAAAAAAA"},
	}
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "at least 8 characters") {
		t.Error("should show password strength error")
	}
}

// ── Logout Test ──────────────────────────────────────────────────────

func TestLogoutClearsSession(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "logoutuser", "user")

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /logout status = %d, want 303", resp.StatusCode)
	}
	// Check session cookie was cleared.
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.MaxAge < 0 {
			return // success
		}
	}
	t.Error("session cookie should be cleared on logout")
}

// ── HTMX Token Create/Revoke Tests ──────────────────────────────────

func TestHTMXTokenCreate(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "htmxuser", "user")

	form := url.Values{"name": {"my-test-token"}}
	req := httptest.NewRequest("POST", "/dashboard/tokens", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /dashboard/tokens status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "fgk_") {
		t.Error("response should contain raw token (fgk_...)")
	}
	if !strings.Contains(string(body), "my-test-token") {
		t.Error("response should contain token name")
	}
}

func TestHTMXTokenRevoke(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "revokeuser", "user")
	user, err := st.GetUserByUsername("revokeuser")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	// Create a token owned by the logged-in user.
	tok, _, _ := st.CreateToken("to-revoke", user.ID)

	req := httptest.NewRequest("DELETE", "/dashboard/tokens/"+tok.ID, nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /dashboard/tokens/:id status = %d, want 200", resp.StatusCode)
	}
}

func TestHTMXTokenRevokeRejectsNonOwner(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	_ = createTestUser(t, st, jwt, "ownerone", "user")
	otherCookie := createTestUser(t, st, jwt, "ownertwo", "user")
	owner, err := st.GetUserByUsername("ownerone")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	tok, _, _ := st.CreateToken("not-yours", owner.ID)

	req := httptest.NewRequest("DELETE", "/dashboard/tokens/"+tok.ID, nil)
	req.AddCookie(otherCookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-owner DELETE /dashboard/tokens/:id status = %d, want 404", resp.StatusCode)
	}
	if _, err := st.ValidateTokenByID(tok.ID); err != nil {
		t.Fatalf("non-owner delete should leave token active: %v", err)
	}
}

// ── HTMX Tunnel Polling Test ─────────────────────────────────────────

func TestHTMXTunnelListEmpty(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "tunneluser", "user")

	req := httptest.NewRequest("GET", "/dashboard/tunnels", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard/tunnels status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "No active tunnels") {
		t.Error("empty tunnel list should show 'No active tunnels'")
	}
}

// ── Static File Test ─────────────────────────────────────────────────

func TestStaticCSS(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/style.css status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "--bg-primary") {
		t.Error("CSS should contain custom properties")
	}
}

func TestStaticHTMX(t *testing.T) {
	mux, _, _, _ := setupDashboardMux(t)

	req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/htmx.min.js status = %d, want 200", resp.StatusCode)
	}
}

// ── Admin Invite Code Create Test ────────────────────────────────────

func TestHTMXInviteCodeCreate(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "inviteadmin", "admin")

	req := httptest.NewRequest("POST", "/admin/invite-codes/new", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /admin/invite-codes/new status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(string(body)) < 8 {
		t.Error("response should contain invite code")
	}
}

// ── T-070: Theme Toggle Tests ────────────────────────────────────────

func TestLayoutIncludesThemeToggle(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "themeuser", "user")

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "theme-toggle") {
		t.Error("layout should contain theme toggle button (class='theme-toggle')")
	}
	if !strings.Contains(html, "toggleTheme") {
		t.Error("layout should contain toggleTheme function")
	}
	if !strings.Contains(html, "fonzygrok-theme") {
		t.Error("layout should reference localStorage key 'fonzygrok-theme'")
	}
}

func TestLayoutFOUCPreventionScript(t *testing.T) {
	mux, _, jwt, st := setupDashboardMux(t)
	cookie := createTestUser(t, st, jwt, "foucuser", "user")

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// The FOUC-preventing script must appear in <head> BEFORE the CSS link.
	cssIdx := strings.Index(html, "style.css")
	themeIdx := strings.Index(html, "fonzygrok-theme")
	if cssIdx < 0 || themeIdx < 0 {
		t.Fatal("layout should contain both style.css and fonzygrok-theme")
	}
	if themeIdx > cssIdx {
		t.Error("FOUC script (fonzygrok-theme) should appear BEFORE style.css link in <head>")
	}
}
